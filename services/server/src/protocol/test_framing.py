import unittest

from protocol import MessageType, receive_message, send_message

REFERENCE_FRAME = bytes(
    [
        MessageType.BETS,
        0x00,
        0x00,
        0x00,
        0x03,
        0x61,
        0x62,
        0x63,
    ]
)


class BufferSocket:
    def __init__(self, data: bytes = b"") -> None:
        self.data = bytearray(data)

    def send(self, data: bytes) -> int:
        self.data.extend(data)
        return len(data)

    def recv(self, size: int) -> bytes:
        chunk = self.data[:size]
        del self.data[:size]
        return bytes(chunk)


class FramingTest(unittest.TestCase):
    def test_send_message_uses_expected_wire_format(self) -> None:
        sock = BufferSocket()

        send_message(sock, MessageType.BETS, b"abc")

        self.assertEqual(bytes(sock.data), REFERENCE_FRAME)

    def test_receive_message_uses_expected_wire_format(self) -> None:
        message = receive_message(BufferSocket(REFERENCE_FRAME))

        self.assertEqual(message.type, MessageType.BETS)
        self.assertEqual(message.payload, b"abc")

    def test_receive_message_rejects_unknown_type(self) -> None:
        frame = bytes([0x7F, 0x00, 0x00, 0x00, 0x00])

        with self.assertRaisesRegex(ValueError, "unknown message type"):
            receive_message(BufferSocket(frame))

    def test_receive_message_rejects_oversized_payload(self) -> None:
        # 0x01000001 is one byte larger than the 16 MiB limit.
        header = bytes([MessageType.BETS, 0x01, 0x00, 0x00, 0x01])

        with self.assertRaisesRegex(ValueError, "exceeds maximum"):
            receive_message(BufferSocket(header))


if __name__ == "__main__":
    unittest.main()
