import socket

import logger
from lottery import Lottery

from .lottery_file_lock import LotteryFileLock
from .process_coordinator import ProcessCoordinator


class Server:
    def __init__(
        self,
        server_host: str,
        server_port: int,
        lottery: Lottery,
        lottery_file_lock: LotteryFileLock,
        agency_quorum_min: int,
    ) -> None:
        self._server_host = server_host
        self._server_port = server_port
        self._process_coordinator = ProcessCoordinator(
            lottery,
            lottery_file_lock,
            agency_quorum_min,
        )

    def run(self) -> None:
        action = "accept-connection"
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as server_socket:
            server_socket.bind((self._server_host, self._server_port))
            server_socket.listen()

            try:
                while True:
                    ready_objects = self._process_coordinator.wait(server_socket)
                    if server_socket in ready_objects:
                        logger.info(action, logger.LogResult.in_progress)
                        client_socket, _ = server_socket.accept()
                        logger.info(action, logger.LogResult.success)
                        self._process_coordinator.start_client(client_socket)

                    self._process_coordinator.process_ready(ready_objects)
            finally:
                self._process_coordinator.shutdown()
