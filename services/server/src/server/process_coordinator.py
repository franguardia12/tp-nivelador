"""Parent-side quorum coordination for per-client worker processes."""

import socket
from multiprocessing.connection import wait

import logger
from lottery import Lottery

from .client_workers import ClientWorker, ClientWorkerRegistry
from .lottery_file_lock import LotteryFileLock
from .quorum import AgencyQuorum
from .quorum_messages import receive_arrival, send_release


class ProcessCoordinator:
    """Synchronize completed agencies while delegating worker lifecycle."""

    def __init__(
        self,
        lottery: Lottery,
        lottery_file_lock: LotteryFileLock,
        agency_quorum_min: int,
    ) -> None:
        self._agency_quorum = AgencyQuorum(agency_quorum_min)
        self._workers = ClientWorkerRegistry(lottery, lottery_file_lock)

    def start_client(self, client_socket: socket.socket) -> None:
        """Start a dedicated process for one accepted client connection."""

        self._workers.start(client_socket)

    @staticmethod
    def _release_worker(worker: ClientWorker) -> None:
        """Wake one arrived worker exactly once through its control connection."""

        if worker.connection is None:
            return
        try:
            send_release(worker.connection)
        except (BrokenPipeError, OSError) as error:
            logger.error("release-client", logger.LogResult.fail, "err", error)
        finally:
            worker.close_connection()

    def _release_waiting_workers(self) -> None:
        """Release every arrived worker after the one-way quorum latch opens."""

        if not self._agency_quorum.is_open:
            return
        for _, worker in self._workers.snapshot():
            if worker.arrived:
                self._release_worker(worker)

    def _read_worker_arrival(self, worker: ClientWorker) -> None:
        """Receive and register one complete agency notification."""

        if worker.connection is None:
            return
        try:
            agency_id = receive_arrival(worker.connection)
        except (EOFError, OSError, ValueError) as error:
            logger.error(
                "agency-quorum-arrival",
                logger.LogResult.fail,
                "err",
                error,
            )
            worker.close_connection()
            return

        worker.arrived = True
        completed_count = self._agency_quorum.register(agency_id)
        logger.info(
            "agency-quorum-arrival",
            logger.LogResult.success,
            "agency-id",
            agency_id,
            "agencies-amount",
            completed_count,
        )
        self._release_waiting_workers()

    def wait(self, server_socket: socket.socket) -> list[object]:
        """Block until the listener or one managed worker object is ready."""

        return wait(self._workers.wait_objects(server_socket))

    def process_ready(self, ready_objects: list[object]) -> None:
        """Handle worker notifications and collect every completed process."""

        for process_id, worker in self._workers.snapshot():
            if worker.connection in ready_objects and not worker.arrived:
                self._read_worker_arrival(worker)
            if worker.process.sentinel in ready_objects:
                self._workers.reap(process_id)

    def shutdown(self) -> None:
        """Release all worker resources within the configured grace period."""

        self._workers.shutdown()
