"""Protocol processing for one agency connection."""

import socket
from multiprocessing.connection import Connection

import logger
import protocol
from lottery import Lottery
from protocol.bets import decode_bets, encode_bet

from .lottery_file_lock import LotteryFileLock

_AGENCY_NOTIFICATION_SIZE = 4
_QUORUM_RELEASE = b"\x01"


class ClientSessionError(Exception):
    """A protocol error that can be reported to the connected client."""

    def __init__(
        self,
        failed_type: int,
        code: protocol.ErrorCode,
        detail: str,
    ) -> None:
        """Store the wire-level context needed to report the session failure."""

        super().__init__(detail)
        self.failed_type = failed_type
        self.code = code
        self.detail = detail


class ClientSession:
    """Execute the complete protocol state machine for one connected agency."""

    def __init__(
        self,
        lottery: Lottery,
        lottery_file_lock: LotteryFileLock,
        coordinator_connection: Connection,
    ) -> None:
        """Keep the storage and IPC resources owned by this worker session."""

        self._lottery = lottery
        self._lottery_file_lock = lottery_file_lock
        self._coordinator_connection = coordinator_connection

    @staticmethod
    def _unexpected_message(
        message: protocol.Message,
        expected: str,
    ) -> ClientSessionError:
        """Build a reportable error for a message received out of order."""

        return ClientSessionError(
            failed_type=message.type,
            code=protocol.ErrorCode.UNEXPECTED_MESSAGE,
            detail=f"expected {expected}, received {message.type.name}",
        )

    @staticmethod
    def _receive_message(client_socket: socket.socket) -> protocol.Message:
        """Receive a frame and translate codec failures into session errors."""

        try:
            return protocol.receive_message(client_socket)
        except ValueError as error:
            raise ClientSessionError(
                failed_type=0,
                code=protocol.ErrorCode.MALFORMED_MESSAGE,
                detail=str(error),
            ) from error

    @staticmethod
    def _send_ack(
        client_socket: socket.socket,
        acknowledged_type: protocol.MessageType,
        processed_count: int,
    ) -> None:
        """Confirm a request only after its processing has completed."""

        payload = protocol.encode_ack(
            protocol.Ack(
                acknowledged_type=acknowledged_type,
                processed_count=processed_count,
            )
        )
        protocol.send_message(client_socket, protocol.MessageType.ACK, payload)

    def _receive_agency(self, client_socket: socket.socket) -> int:
        """Receive, validate, and acknowledge the session's agency identifier."""

        message = self._receive_message(client_socket)
        if message.type != protocol.MessageType.AGENCY:
            raise self._unexpected_message(message, protocol.MessageType.AGENCY.name)

        try:
            agency_id = protocol.decode_agency(message.payload)
        except ValueError as error:
            raise ClientSessionError(
                failed_type=message.type,
                code=protocol.ErrorCode.INVALID_DATA,
                detail=str(error),
            ) from error

        self._send_ack(client_socket, protocol.MessageType.AGENCY, 0)
        return agency_id

    def _receive_bets(self, client_socket: socket.socket, agency_id: int) -> int:
        """Receive and persist complete BETS messages until END_BETS arrives."""

        stored_count = 0
        while True:
            message = self._receive_message(client_socket)
            if message.type == protocol.MessageType.END_BETS:
                if message.payload:
                    raise ClientSessionError(
                        failed_type=message.type,
                        code=protocol.ErrorCode.MALFORMED_MESSAGE,
                        detail="END_BETS must have an empty payload",
                    )
                return stored_count

            if message.type != protocol.MessageType.BETS:
                raise self._unexpected_message(
                    message,
                    f"{protocol.MessageType.BETS.name} or "
                    f"{protocol.MessageType.END_BETS.name}",
                )

            try:
                bets = decode_bets(message.payload, agency_id)
            except ValueError as error:
                raise ClientSessionError(
                    failed_type=message.type,
                    code=protocol.ErrorCode.INVALID_DATA,
                    detail=str(error),
                ) from error

            with self._lottery_file_lock.write():
                self._lottery.store_bets(bets)
            self._send_ack(client_socket, protocol.MessageType.BETS, len(bets))
            stored_count += len(bets)

    def _wait_for_quorum(self, agency_id: int) -> None:
        """Notify the parent through a pipe and block for its release token."""

        action = "wait-agency-quorum"
        logger.info(
            action,
            logger.LogResult.in_progress,
            "agency-id",
            agency_id,
        )
        self._coordinator_connection.send_bytes(
            agency_id.to_bytes(_AGENCY_NOTIFICATION_SIZE, "big")
        )
        release = self._coordinator_connection.recv_bytes(
            maxlength=len(_QUORUM_RELEASE)
        )
        if release != _QUORUM_RELEASE:
            raise ValueError("invalid quorum release token")
        logger.info(
            action,
            logger.LogResult.success,
            "agency-id",
            agency_id,
        )

    def _send_winners(self, client_socket: socket.socket, agency_id: int) -> int:
        """Stream winners for one agency and finish with their declared count."""

        winner_count = 0
        # Lottery exposes a file-backed iterator. Keep the shared read lock for
        # the complete iteration so a process cannot append a partial CSV view.
        # Other winner readers may proceed concurrently.
        with self._lottery_file_lock.read():
            for bet in self._lottery.load_bets():
                if bet.agency_id != agency_id or not self._lottery.has_won(bet):
                    continue
                if winner_count == (1 << 32) - 1:
                    raise OverflowError("winner count exceeds uint32")

                protocol.send_message(
                    client_socket,
                    protocol.MessageType.WINNER,
                    encode_bet(bet),
                )
                winner_count += 1

        protocol.send_message(
            client_socket,
            protocol.MessageType.WINNERS_END,
            protocol.encode_winners_end(winner_count),
        )
        return winner_count

    @staticmethod
    def _try_send_error(
        client_socket: socket.socket,
        session_error: ClientSessionError,
    ) -> None:
        """Best-effort reporting of a session failure on a usable connection."""

        try:
            payload = protocol.encode_error(
                protocol.ErrorPayload(
                    failed_type=session_error.failed_type,
                    code=session_error.code,
                    detail=session_error.detail,
                )
            )
            protocol.send_message(client_socket, protocol.MessageType.ERROR, payload)
        except Exception as error:
            logger.error(
                "send-client-error",
                logger.LogResult.fail,
                "err",
                error,
            )

    def run(self, client_socket: socket.socket) -> None:
        """Run one client session and report protocol or processing failures."""

        action = "handle-client"
        logger.info(action, logger.LogResult.in_progress)
        try:
            agency_id = self._receive_agency(client_socket)
            stored_count = self._receive_bets(client_socket, agency_id)
            self._wait_for_quorum(agency_id)
            winner_count = self._send_winners(client_socket, agency_id)
        except ClientSessionError as error:
            self._try_send_error(client_socket, error)
            logger.error(action, logger.LogResult.fail, "err", error)
            return
        except Exception as error:
            self._try_send_error(
                client_socket,
                ClientSessionError(
                    failed_type=0,
                    code=protocol.ErrorCode.INTERNAL,
                    detail="internal server error",
                ),
            )
            logger.error(action, logger.LogResult.fail, "err", error)
            return

        logger.info(
            action,
            logger.LogResult.success,
            "agency-id",
            agency_id,
            "bets-amount",
            stored_count,
            "winners-amount",
            winner_count,
        )
