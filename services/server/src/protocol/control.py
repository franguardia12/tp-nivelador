"""Binary codecs for protocol control-message payloads."""

from dataclasses import dataclass
from enum import IntEnum

from .framing import MessageType
from .primitives import MAX_STRING_SIZE, MAX_UINT32, encode_uint

AGENCY_PAYLOAD_SIZE = 4


@dataclass(frozen=True)
class Ack:
    """A request acknowledgement and its processed-record count."""

    acknowledged_type: MessageType
    processed_count: int


class ErrorCode(IntEnum):
    """Protocol error categories exchanged with clients."""

    MALFORMED_MESSAGE = 1
    UNEXPECTED_MESSAGE = 2
    INVALID_DATA = 3
    INTERNAL = 4


@dataclass(frozen=True)
class ErrorPayload:
    """The request, category, and human-readable detail of a protocol error."""

    failed_type: int
    code: ErrorCode
    detail: str


def decode_agency(payload: bytes) -> int:
    """Deserialize a payload containing exactly one agency identifier."""

    if len(payload) != AGENCY_PAYLOAD_SIZE:
        raise ValueError(
            f"agency payload length is {len(payload)}, "
            f"expected {AGENCY_PAYLOAD_SIZE}"
        )
    return int.from_bytes(payload, "big")


def _as_acknowledged_type(value: MessageType | int) -> MessageType:
    """Normalize a message type that is valid inside an ACK payload."""

    try:
        message_type = MessageType(value)
    except ValueError as error:
        raise ValueError(f"unknown acknowledged message type 0x{value:02x}") from error
    if message_type not in (MessageType.AGENCY, MessageType.BETS):
        raise ValueError(f"message type 0x{value:02x} cannot be acknowledged")
    return message_type


def encode_ack(ack: Ack) -> bytes:
    """Serialize the request type and the number of processed bets."""

    message_type = _as_acknowledged_type(ack.acknowledged_type)
    count = encode_uint("processed count", ack.processed_count, 4, MAX_UINT32)
    return bytes([message_type]) + count


def encode_winners_end(winner_count: int) -> bytes:
    """Serialize the total number of WINNER messages sent."""

    return encode_uint("winner count", winner_count, 4, MAX_UINT32)


def _as_failed_type(value: MessageType | int) -> int:
    """Normalize an ERROR request type, allowing zero when it is unknown."""

    if value == 0:
        return 0
    try:
        return int(MessageType(value))
    except ValueError as error:
        raise ValueError(f"unknown failed message type 0x{value:02x}") from error


def _as_error_code(value: ErrorCode | int) -> ErrorCode:
    """Normalize and validate an ERROR category."""

    try:
        return ErrorCode(value)
    except ValueError as error:
        raise ValueError(f"unknown error code {value}") from error


def encode_error(protocol_error: ErrorPayload) -> bytes:
    """Serialize an error code, failed message type and UTF-8 detail."""

    failed_type = _as_failed_type(protocol_error.failed_type)
    code = _as_error_code(protocol_error.code)
    if not isinstance(protocol_error.detail, str):
        raise ValueError("error detail must be a string")
    try:
        detail = protocol_error.detail.encode("utf-8")
    except UnicodeEncodeError as error:
        raise ValueError("error detail is not valid UTF-8") from error
    if len(detail) > MAX_STRING_SIZE:
        raise ValueError(
            f"error detail length {len(detail)} exceeds maximum {MAX_STRING_SIZE}"
        )

    return b"".join(
        [
            bytes([failed_type]),
            int(code).to_bytes(2, "big"),
            len(detail).to_bytes(2, "big"),
            detail,
        ]
    )
