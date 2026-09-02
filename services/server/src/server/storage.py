"""Lifecycle of the server's file-backed Lottery storage."""

import os

_LOTTERY_STORAGE_FILE = "bets.csv"


def _remove_file_if_present(path: str) -> None:
    """Remove a file, treating an already absent file as clean state."""

    try:
        os.remove(path)
    except FileNotFoundError:
        return


def _remove_directory_if_present(path: str) -> None:
    """Remove an empty directory, accepting a previous removal."""

    try:
        os.rmdir(path)
    except FileNotFoundError:
        return


class LotteryStorage:
    """Prepare clean storage files and remove them after server shutdown."""

    def __init__(self, directory: str) -> None:
        if not directory:
            raise ValueError("storage directory must not be empty")

        self._directory = directory
        self._created_directory = False
        self._prepared = False
        self.storage_path = os.path.join(directory, _LOTTERY_STORAGE_FILE)

    def prepare(self) -> None:
        """Create or truncate the storage file before the server starts."""

        if not os.path.exists(self._directory):
            os.makedirs(self._directory)
            self._created_directory = True
        elif not os.path.isdir(self._directory):
            raise ValueError(f"storage path is not a directory: {self._directory}")

        self._prepared = True
        with open(self.storage_path, "w", encoding="utf-8") as storage_file:
            storage_file.truncate(0)

    def cleanup(self) -> None:
        """Remove only resources successfully created by this instance."""

        if self._prepared:
            _remove_file_if_present(self.storage_path)
            self._prepared = False

        if self._created_directory:
            _remove_directory_if_present(self._directory)
            self._created_directory = False
