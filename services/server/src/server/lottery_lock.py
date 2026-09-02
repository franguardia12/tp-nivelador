"""Inter-process synchronization for the file-backed Lottery."""

import multiprocessing


class LotteryLock:
    """Serialize Lottery accesses with a lock shared by spawned workers."""

    def __init__(self) -> None:
        # The lock must use a spawn context because workers use that same
        # start method. Locks created in a fork context cannot be shared with
        # processes started through spawn.
        self._lock = multiprocessing.get_context("spawn").Lock()

    def hold(self):
        """Return the shared Lock so callers can protect a block with `with`."""

        return self._lock
