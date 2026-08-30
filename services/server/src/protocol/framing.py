import socket
from dataclasses import dataclass
from enum import IntEnum

import safe_socket

HEADER_SIZE = 5
_MAX_ENCODED_PAYLOAD_LENGTH = (1 << 32) - 1


class MessageType(IntEnum):
    AGENCY = 0x01
    BETS = 0x02
    END_BETS = 0x03
    ACK = 0x80
    WINNER = 0x81
    WINNERS_END = 0x82
    ERROR = 0xFF


@dataclass(frozen=True)
class Message:
    type: MessageType
    payload: bytes


def _as_message_type(value: MessageType | int) -> MessageType:
    try:
        return MessageType(value)
    except ValueError as error:
        raise ValueError(f"unknown message type 0x{value:02x}") from error


def _validate_payload_size(payload_size: int) -> None:
    if payload_size > _MAX_ENCODED_PAYLOAD_LENGTH:
        raise ValueError(
            f"payload length {payload_size} cannot be represented by uint32"
        )


def send_message(
    sock: socket.socket,
    message_type: MessageType,
    payload: bytes = b"",
) -> None:
    normalized_type = _as_message_type(message_type)
    _validate_payload_size(len(payload))

    header = bytes([normalized_type]) + len(payload).to_bytes(4, "big")
    safe_socket.send_all(sock, header + payload)


def receive_message(sock: socket.socket) -> Message:
    header = safe_socket.recv_all(sock, HEADER_SIZE)
    message_type = _as_message_type(header[0])
    payload_size = int.from_bytes(header[1:HEADER_SIZE], "big")

    payload = safe_socket.recv_all(sock, payload_size)
    return Message(message_type, payload)
