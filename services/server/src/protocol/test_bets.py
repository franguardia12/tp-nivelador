import unittest

from lottery import Bet
from protocol.bets import decode_bet, decode_bets, encode_bet, encode_bets

REFERENCE_BET = Bet(
    agency_id=42,
    first_name="Ana",
    last_name="López",
    document=30904465,
    birthdate="1999-03-17",
    number=7574,
)

REFERENCE_BET_PAYLOAD = bytes([0x00, 0x03, 0x41, 0x6E, 0x61, 0x00, 0x06, 0x4C, 0xC3, 0xB3, 0x70, 0x65, 0x7A, 0x00, 
0x00, 0x00, 0x00, 0x01, 0xD7, 0x90, 0x91, 0x00, 0x0A, 0x31, 0x39, 0x39, 0x39, 0x2D, 0x30, 0x33, 0x2D, 0x31, 0x37,
0x00, 0x00, 0x1D, 0x96,])

class BetsTest(unittest.TestCase):
    def test_encode_bet_uses_expected_wire_format(self) -> None:
        self.assertEqual(encode_bet(REFERENCE_BET), REFERENCE_BET_PAYLOAD)

    def test_decode_bet_uses_expected_wire_format(self) -> None:
        self.assertEqual(decode_bet(REFERENCE_BET_PAYLOAD, 42), REFERENCE_BET)

    def test_bets_round_trip(self) -> None:
        bets = [
            REFERENCE_BET,
            Bet(42, "Camila", "Varela", 37130775, "1995-05-09", 1024),
        ]

        self.assertEqual(decode_bets(encode_bets(bets), 42), bets)

    def test_decode_bet_rejects_trailing_bytes(self) -> None:
        with self.assertRaisesRegex(ValueError, "trailing bytes"):
            decode_bet(REFERENCE_BET_PAYLOAD + b"\x00", 42)

    def test_decode_bet_rejects_invalid_utf8(self) -> None:
        with self.assertRaisesRegex(ValueError, "not valid UTF-8"):
            decode_bet(b"\x00\x01\xff", 42)

    def test_encode_bets_rejects_empty_list(self) -> None:
        with self.assertRaisesRegex(ValueError, "cannot be empty"):
            encode_bets([])

    def test_decode_bets_rejects_count_that_does_not_fit_payload(self) -> None:
        payload = b"\x00\x00\x00\x03" + REFERENCE_BET_PAYLOAD

        with self.assertRaisesRegex(ValueError, "does not fit"):
            decode_bets(payload, 42)


if __name__ == "__main__":
    unittest.main()
