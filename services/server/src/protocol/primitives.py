"""Primitive values shared by protocol payload codecs."""

MAX_STRING_SIZE = (1 << 16) - 1
MAX_UINT32 = (1 << 32) - 1
MAX_UINT64 = (1 << 64) - 1


def encode_uint(field_name: str, value: int, size: int, maximum: int) -> bytes:
    """Validate and encode an unsigned integer using big-endian order."""

    if isinstance(value, bool) or not isinstance(value, int):
        raise ValueError(f"{field_name} must be an integer")
    if value < 0 or value > maximum:
        raise ValueError(f"{field_name} is outside the uint{size * 8} range")
    return value.to_bytes(size, "big")
