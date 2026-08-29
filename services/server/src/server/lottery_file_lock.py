"""Inter-process locking for the file-backed Lottery implementation."""

import fcntl
from collections.abc import Generator
from contextlib import contextmanager


class LotteryFileLock:
    """Coordinate Lottery readers and writers through a kernel file lock."""

    def __init__(self, lock_path: str) -> None:
        self._lock_path = lock_path

    @contextmanager
    def read(self) -> Generator[None]:
        """Acquire a shared lock, allowing other readers but excluding writers."""

        with self._acquire(fcntl.LOCK_SH):
            yield

    @contextmanager
    def write(self) -> Generator[None]:
        """Acquire an exclusive lock for a Lottery storage mutation."""

        with self._acquire(fcntl.LOCK_EX):
            yield

    @contextmanager
    def _acquire(self, operation: int) -> Generator[None]:
        """Hold the requested advisory lock and always release its descriptor."""

        with open(self._lock_path, "r", encoding="utf-8") as lock_file:
            fcntl.flock(lock_file.fileno(), operation)
            try:
                yield
            finally:
                fcntl.flock(lock_file.fileno(), fcntl.LOCK_UN)
