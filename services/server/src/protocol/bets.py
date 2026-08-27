"""Binary codecs for individual bets and counted BETS payloads.

The wire representation omits the agency identifier because it is established
once per connection. Decoding adds that session value to the domain Bet.
"""

from lottery import Bet

from .primitives import MAX_STRING_SIZE, MAX_UINT32, MAX_UINT64, encode_uint

MINIMUM_ENCODED_BET_SIZE = 18
BETS_COUNT_SIZE = 4


class _BetDecoder:
    """Consume fields sequentially from a shared payload."""

    def __init__(self, payload: bytes, offset: int = 0) -> None:
        self.payload = payload
        self.offset = offset

    def read(self, size: int, field_name: str) -> bytes:
        """Consume exactly size bytes from the current offset."""

        if size < 0 or size > len(self.payload) - self.offset:
            raise ValueError(f"incomplete {field_name}")

        value = self.payload[self.offset : self.offset + size]
        self.offset += size
        return value

    def read_string(self, field_name: str) -> str:
        """Consume a uint16 byte length and its UTF-8 string."""

        encoded_length = self.read(2, f"{field_name} length")
        length = int.from_bytes(encoded_length, "big")
        encoded_value = self.read(length, field_name)
        try:
            return encoded_value.decode("utf-8")
        except UnicodeDecodeError as error:
            raise ValueError(f"{field_name} is not valid UTF-8") from error


def _encode_string(field_name: str, value: str) -> bytes:
    """Encode one UTF-8 string prefixed by its uint16 byte length."""

    if not isinstance(value, str):
        raise ValueError(f"{field_name} must be a string")
    try:
        encoded_value = value.encode("utf-8")
    except UnicodeEncodeError as error:
        raise ValueError(f"{field_name} is not valid UTF-8") from error
    if len(encoded_value) > MAX_STRING_SIZE:
        raise ValueError(
            f"{field_name} length {len(encoded_value)} "
            f"exceeds maximum {MAX_STRING_SIZE}"
        )
    return len(encoded_value).to_bytes(2, "big") + encoded_value


def _validate_agency_id(agency_id: int) -> None:
    """Validate the session agency before adding it to decoded domain bets."""

    encode_uint("agency id", agency_id, 4, MAX_UINT32)


def _decode_bet(decoder: _BetDecoder, agency_id: int) -> Bet:
    """Consume the next bet without requiring the payload to end afterward."""

    first_name = decoder.read_string("first name")
    last_name = decoder.read_string("last name")
    document = int.from_bytes(decoder.read(8, "document"), "big")
    birthdate = decoder.read_string("birthdate")
    number = int.from_bytes(decoder.read(4, "number"), "big")
    return Bet(agency_id, first_name, last_name, document, birthdate, number)


def encode_bet(bet: Bet) -> bytes:
    """Serialize one bet, omitting the connection-scoped agency identifier."""

    return b"".join(
        [
            _encode_string("first name", bet.first_name),
            _encode_string("last name", bet.last_name),
            encode_uint("document", bet.document, 8, MAX_UINT64),
            _encode_string("birthdate", bet.birthdate),
            encode_uint("number", bet.number, 4, MAX_UINT32),
        ]
    )


def decode_bet(payload: bytes, agency_id: int) -> Bet:
    """Deserialize a payload that must contain exactly one complete bet."""
    _validate_agency_id(agency_id)

    decoder = _BetDecoder(payload)
    bet = _decode_bet(decoder, agency_id)
    if decoder.offset != len(payload):
        raise ValueError(
            f"bet payload has {len(payload) - decoder.offset} trailing bytes"
        )
    return bet


def encode_bets(bets: list[Bet]) -> bytes:
    """Serialize a non-empty list as a uint32 count followed by each bet."""

    if not bets:
        raise ValueError("bets payload cannot be empty")
    if len(bets) > MAX_UINT32:
        raise ValueError(f"bet count {len(bets)} exceeds uint32")

    encoded_bets: list[bytes] = []
    for bet in bets:
        try:
            encoded_bet = encode_bet(bet)
        except ValueError as error:
            raise ValueError(f"encode bet {index}: {error}") from error
        encoded_bets.append(encoded_bet)

    return len(bets).to_bytes(BETS_COUNT_SIZE, "big") + b"".join(encoded_bets)


def decode_bets(payload: bytes, agency_id: int) -> list[Bet]:
    """Decode exactly the declared bets and reject truncation or trailing bytes."""
    if len(payload) < BETS_COUNT_SIZE:
        raise ValueError("incomplete bet count")
    _validate_agency_id(agency_id)

    count = int.from_bytes(payload[:BETS_COUNT_SIZE], "big")
    if count == 0:
        raise ValueError("bets payload cannot be empty")
    remaining_size = len(payload) - BETS_COUNT_SIZE
    if count * MINIMUM_ENCODED_BET_SIZE > remaining_size:
        raise ValueError(
            f"bet count {count} does not fit in payload of {remaining_size} bytes"
        )

    decoder = _BetDecoder(payload, offset=BETS_COUNT_SIZE)
    bets = []
    for index in range(count):
        try:
            bets.append(_decode_bet(decoder, agency_id))
        except ValueError as error:
            raise ValueError(f"decode bet {index}: {error}") from error
    if decoder.offset != len(payload):
        raise ValueError(
            f"bets payload has {len(payload) - decoder.offset} trailing bytes"
        )
    return bets
