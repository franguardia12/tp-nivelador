import os
import sys
import tempfile

import logger
import server
from lottery import Lottery

SERVER_HOST = os.environ["SERVER_HOST"]
SERVER_PORT = int(os.environ["SERVER_PORT"])
_LOTTERY_STORAGE_FILE = "bets.csv"


def main():
    logger.init()
    try:
        with tempfile.TemporaryDirectory(prefix="lottery-") as storage_directory:
            storage_path = os.path.join(storage_directory, _LOTTERY_STORAGE_FILE)
            with open(storage_path, "x", encoding="utf-8"):
                pass

            lottery = Lottery(storage_path)
            server.Server(SERVER_HOST, SERVER_PORT, lottery).run()
    except Exception as e:
        logger.error("server-run", logger.LogResult.fail, "err", e)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
