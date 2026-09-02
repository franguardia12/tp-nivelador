import os
import sys

import logger
import server
from lottery import Lottery

SERVER_HOST = os.environ["SERVER_HOST"]
SERVER_PORT = int(os.environ["SERVER_PORT"])
_STORAGE_DIRECTORY = "/tmp/lottery"


def _load_agency_quorum_min() -> int:
    """Read and validate the minimum number of agencies required for the draw."""

    value = os.environ.get("AGENCY_QUORUM_MIN")
    if value is None:
        raise ValueError("AGENCY_QUORUM_MIN environment variable is required")
    try:
        minimum = int(value)
    except ValueError as error:
        raise ValueError("AGENCY_QUORUM_MIN must be a positive integer") from error
    if minimum <= 0:
        raise ValueError("AGENCY_QUORUM_MIN must be greater than zero")
    return minimum


def main():
    logger.init()
    previous_sigterm_handler = server.install_sigterm_handler()
    try:
        agency_quorum_min = _load_agency_quorum_min()
        storage = server.LotteryStorage(_STORAGE_DIRECTORY)
        try:
            storage.prepare()
            lottery = Lottery(storage.storage_path)
            server.Server(SERVER_HOST, SERVER_PORT, lottery, server.LotteryFileLock(storage.lock_path),
                          agency_quorum_min).run()
        finally:
            storage.cleanup()
    except server.ShutdownRequested:
        logger.info("server-shutdown", logger.LogResult.success)
        return 0
    except Exception as e:
        logger.error("server-run", logger.LogResult.fail, "err", e)
        return 1
    finally:
        server.restore_sigterm_handler(previous_sigterm_handler)
    return 0


if __name__ == "__main__":
    sys.exit(main())
