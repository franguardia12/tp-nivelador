"""Winner streaming for one completed client session."""

import socket

import protocol
from lottery import Lottery
from protocol.bets import encode_bet

from ..lottery_lock import LotteryLock

_MAX_WINNER_COUNT = 4294967295


class WinnerSender:
    """Read the shared lottery safely and stream one agency's winners."""

    def __init__(self, lottery: Lottery, lottery_lock: LotteryLock) -> None:
        self._lottery = lottery
        self._lottery_lock = lottery_lock

    def send(self, client_socket: socket.socket, agency_id: int) -> int:
        """Send every winner followed by its declared total count."""

        winner_count = 0
        # Lottery exposes a file-backed iterator. The lock covers the complete
        # iteration so that no other process modifies or reads a partial CSV.
        with self._lottery_lock.hold():
            for bet in self._lottery.load_bets():
                if bet.agency_id != agency_id or not self._lottery.has_won(bet):
                    continue
                if winner_count == _MAX_WINNER_COUNT:
                    raise OverflowError("winner count exceeds uint32")

                protocol.send_message(
                    client_socket, protocol.MessageType.WINNER, encode_bet(bet)
                )
                winner_count += 1

        protocol.send_message(
            client_socket,
            protocol.MessageType.WINNERS_END,
            protocol.encode_winners_end(winner_count),
        )
        return winner_count
