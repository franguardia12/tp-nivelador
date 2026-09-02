"""Agency registration and incremental bet reception for one session."""

import socket

import protocol
from lottery import Lottery
from protocol.bets import decode_bets

from ..lottery_lock import LotteryLock
from .errors import ClientSessionError, receive_message, unexpected_message


class BetReceiver:
    """Receive, validate and persist an agency's complete bet stream."""

    def __init__(self, lottery: Lottery, lottery_lock: LotteryLock) -> None:
        self._lottery = lottery
        self._lottery_lock = lottery_lock

    @staticmethod
    def _send_ack(client_socket: socket.socket, acknowledged_type: protocol.MessageType, 
                  processed_count: int) -> None:
        """Confirm a request only after its processing has completed."""

        payload = protocol.encode_ack(
            protocol.Ack(
                acknowledged_type=acknowledged_type,
                processed_count=processed_count,
            )
        )
        protocol.send_message(client_socket, protocol.MessageType.ACK, payload)

    def _receive_agency(self, client_socket: socket.socket) -> int:
        """Receive, validate and acknowledge the session agency identifier."""

        message = receive_message(client_socket)
        if message.type != protocol.MessageType.AGENCY:
            raise unexpected_message(message, protocol.MessageType.AGENCY.name)

        try:
            agency_id = protocol.decode_agency(message.payload)
        except ValueError as error:
            raise ClientSessionError(failed_type=message.type, code=protocol.ErrorCode.INVALID_DATA, 
                                     detail=str(error)) from error

        self._send_ack(client_socket, protocol.MessageType.AGENCY, 0)
        return agency_id

    def _receive_bets(self, client_socket: socket.socket, agency_id: int) -> int:
        """Receive and persist complete BETS messages until END_BETS arrives."""

        stored_count = 0
        while True:
            message = receive_message(client_socket)
            if message.type == protocol.MessageType.END_BETS:
                if message.payload:
                    raise ClientSessionError(failed_type=message.type, 
                                             code=protocol.ErrorCode.MALFORMED_MESSAGE, 
                                             detail="END_BETS must have an empty payload")
                return stored_count

            if message.type != protocol.MessageType.BETS:
                raise unexpected_message(message, f"{protocol.MessageType.BETS.name} or " 
                                         f"{protocol.MessageType.END_BETS.name}")

            try:
                bets = decode_bets(message.payload, agency_id)
            except ValueError as error:
                raise ClientSessionError(failed_type=message.type, code=protocol.ErrorCode.INVALID_DATA, 
                                         detail=str(error)) from error

            with self._lottery_lock.hold():
                self._lottery.store_bets(bets)
            self._send_ack(client_socket, protocol.MessageType.BETS, len(bets))
            stored_count += len(bets)

    def receive(self, client_socket: socket.socket) -> tuple[int, int]:
        """Receive the full input phase and return its agency and bet count."""

        agency_id = self._receive_agency(client_socket)
        stored_count = self._receive_bets(client_socket, agency_id)
        return agency_id, stored_count
