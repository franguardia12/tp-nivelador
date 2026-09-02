"""Creation and lifecycle management for per-client worker processes."""

import multiprocessing
import socket
from multiprocessing.connection import Connection

import logger
from lottery import Lottery

from .lottery_lock import LotteryLock
from .session.client_session import ClientSession
from .shutdown import ShutdownRequested, install_sigterm_handler, restore_sigterm_handler


class ClientWorker:
    """Parent-owned process and control connection for one client worker."""

    def __init__(self, process: multiprocessing.Process, connection: Connection | None, arrived: bool = False
                 ) -> None:
        self.process = process
        self.connection = connection
        self.arrived = arrived

    def close_connection(self) -> None:
        """Close the parent Pipe endpoint once."""

        if self.connection is not None:
            self.connection.close()
            self.connection = None

    def close(self) -> None:
        """Release every parent-side object after the process has stopped."""

        self.close_connection()
        self.process.close()


def _serve_client_process(client_socket: socket.socket, lottery: Lottery, lottery_lock: LotteryLock, 
                          coordinator_connection: Connection) -> None:
    """Own one client socket and its coordinator connection in a child process."""

    previous_sigterm_handler = install_sigterm_handler()
    try:
        with coordinator_connection, client_socket:
            ClientSession(lottery, lottery_lock, coordinator_connection).run(client_socket)
    except ShutdownRequested:
        logger.info("client-process-shutdown", logger.LogResult.success)
    finally:
        restore_sigterm_handler(previous_sigterm_handler)


class ClientWorkerRegistry:
    """Own all worker processes and their parent-side operating-system resources."""

    def __init__(self, lottery: Lottery, lottery_lock: LotteryLock) -> None:
        self._lottery = lottery
        self._lottery_lock = lottery_lock
        # Spawn transfers only explicitly supplied resources and avoids
        # inheriting the listener or connections belonging to earlier clients.
        self._process_context = multiprocessing.get_context("spawn")
        self._workers: dict[int, ClientWorker] = {}

    def start(self, client_socket: socket.socket) -> None:
        """Transfer an accepted socket and one Pipe endpoint to a new worker."""

        parent_connection, child_connection = self._process_context.Pipe(duplex=True)
        process = self._process_context.Process(target=_serve_client_process, args=(client_socket, 
                                                                                    self._lottery,
                                                                                    self._lottery_lock,
                                                                                    child_connection), 
                                                name=f"client-{client_socket.fileno()}", daemon=False)
        try:
            process.start()
        except Exception:
            parent_connection.close()
            child_connection.close()
            client_socket.close()
            raise

        client_socket.close()
        child_connection.close()
        self._workers[process.pid] = ClientWorker(process, parent_connection)

    def snapshot(self) -> list[tuple[int, ClientWorker]]:
        """Return a stable view that remains iterable while workers are reaped."""

        return list(self._workers.items())

    def get(self, process_id: int) -> ClientWorker | None:
        """Return one managed worker while it remains registered."""

        return self._workers.get(process_id)

    def wait_objects(self, server_socket: socket.socket) -> list[object]:
        """Collect the listener, unread control connections and sentinels."""

        wait_objects: list[object] = [server_socket]
        for worker in self._workers.values():
            if worker.connection is not None and not worker.arrived:
                wait_objects.append(worker.connection)
            wait_objects.append(worker.process.sentinel)
        return wait_objects

    def reap(self, process_id: int) -> None:
        """Join one completed process and close its parent-owned resources."""

        worker = self._workers.pop(process_id)
        worker.process.join()
        worker.close()

    def shutdown(self) -> None:
        """Request SIGTERM cleanup and join every managed worker."""

        if not self._workers:
            return

        action = "shutdown-client-processes"
        logger.info(action, logger.LogResult.in_progress, "processes-amount", len(self._workers))
        workers = list(self._workers.values())
        for worker in workers:
            if worker.process.is_alive():
                worker.process.terminate()

        for worker in workers:
            worker.process.join()
            worker.close()

        self._workers.clear()
        logger.info(action, logger.LogResult.success)
