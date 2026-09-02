"""Lifecycle of the server's file-backed Lottery storage."""

import os

_LOTTERY_STORAGE_FILE = "bets.csv"
_LOTTERY_LOCK_FILE = "bets.lock"


class LotteryStorage:
    """Prepare clean storage files and remove them after server shutdown."""

    def __init__(self, directory: str) -> None:
        if not directory:
            raise ValueError("storage directory must not be empty")

        self._directory = directory
        self._created_directory = False
        self._prepared = False
        self.storage_path = os.path.join(directory, _LOTTERY_STORAGE_FILE)
        self.lock_path = os.path.join(directory, _LOTTERY_LOCK_FILE)

    def prepare(self) -> None:
        """Create or truncate both files before the server starts."""

        if not os.path.exists(self._directory):
            os.makedirs(self._directory)
            self._created_directory = True
        elif not os.path.isdir(self._directory):
            raise ValueError(f"storage path is not a directory: {self._directory}")

        self._prepared = True
        for path in (self.storage_path, self.lock_path):
            with open(path, "w", encoding="utf-8"):
                pass

    def cleanup(self) -> None:
        """Remove only resources successfully created by this instance."""

        if self._prepared:
            for path in (self.lock_path, self.storage_path):
                try:
                    os.remove(path)
                except FileNotFoundError:
                    pass
            self._prepared = False

        if self._created_directory:
            try:
                os.rmdir(self._directory)
            except FileNotFoundError:
                pass
            self._created_directory = False
