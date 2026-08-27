import socket

import logger
import protocol
from lottery import Lottery
from protocol.bets import decode_bets, encode_bet


class ClientSessionError(Exception):
    """A protocol error that can be reported to the connected client."""

    def __init__(
        self,
        failed_type: int,
        code: protocol.ErrorCode,
        detail: str,
    ) -> None:
        super().__init__(detail)
        self.failed_type = failed_type
        self.code = code
        self.detail = detail


class Server:
    def __init__(
        self,
        server_host: str,
        server_port: int,
        lottery: Lottery,
    ) -> None:
        self.server_host = server_host
        self.server_port = server_port
        self.lottery = lottery

    @staticmethod
    def _unexpected_message(
        message: protocol.Message,
        expected: str,
    ) -> ClientSessionError:
        return ClientSessionError(
            failed_type=message.type,
            code=protocol.ErrorCode.UNEXPECTED_MESSAGE,
            detail=f"expected {expected}, received {message.type.name}",
        )

    @staticmethod
    def _receive_message(client_socket: socket.socket) -> protocol.Message:
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
        payload = protocol.encode_ack(
            protocol.Ack(
                acknowledged_type=acknowledged_type,
                processed_count=processed_count,
            )
        )
        protocol.send_message(client_socket, protocol.MessageType.ACK, payload)

    def _receive_agency(self, client_socket: socket.socket) -> int:
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

            self.lottery.store_bets(bets)
            self._send_ack(client_socket, protocol.MessageType.BETS, len(bets))
            stored_count += len(bets)

    def _send_winners(self, client_socket: socket.socket, agency_id: int) -> int:
        winner_count = 0
        for bet in self.lottery.load_bets():
            if bet.agency_id != agency_id or not self.lottery.has_won(bet):
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

    def _handle_client(self, client_socket: socket.socket) -> None:
        action = "handle-client"
        logger.info(action, logger.LogResult.in_progress)
        try:
            agency_id = self._receive_agency(client_socket)
            stored_count = self._receive_bets(client_socket, agency_id)
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

    def run(self) -> None:
        action = "accept-connection"
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as server_socket:
            server_socket.bind((self.server_host, self.server_port))
            server_socket.listen()
            while True:
                try:
                    logger.info(action, logger.LogResult.in_progress)
                    client_socket, _ = server_socket.accept()
                except Exception as error:
                    logger.error(action, logger.LogResult.fail, "err", error)
                    raise
                logger.info(action, logger.LogResult.success)

                with client_socket:
                    self._handle_client(client_socket)
