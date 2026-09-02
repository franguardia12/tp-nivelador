"""Inter-process locking for the file-backed Lottery implementation."""

import fcntl


class LotteryFileLock:
    """Coordinate Lottery readers and writers through a kernel file lock."""

    def __init__(self, lock_path: str) -> None:
        self._lock_path = lock_path

    def acquire_read(self):
        """Acquire and return a shared-lock file descriptor."""

        return self._acquire(fcntl.LOCK_SH)

    def acquire_write(self):
        """Acquire and return an exclusive-lock file descriptor."""

        return self._acquire(fcntl.LOCK_EX)

    def _acquire(self, operation: int):
        """Open the lock file and close it if acquisition is interrupted."""

        lock_file = open(self._lock_path, "r", encoding="utf-8")
        try:
            fcntl.flock(lock_file.fileno(), operation)
        except BaseException:
            lock_file.close()
            raise
        return lock_file

    @staticmethod
    def release(lock_file) -> None:
        """Release a previously acquired lock and always close its descriptor."""

        try:
            fcntl.flock(lock_file.fileno(), fcntl.LOCK_UN)
        finally:
            lock_file.close()
