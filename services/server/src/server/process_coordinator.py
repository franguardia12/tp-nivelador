"""Parent-side quorum coordination for per-client worker processes."""

import socket
from multiprocessing.connection import wait

import logger
from lottery import Lottery

from .client_workers import ClientWorker, ClientWorkerRegistry
from .lottery_lock import LotteryLock
from .quorum import AgencyRound, AgencyRounds
from .quorum_messages import receive_arrival, send_release


class ProcessCoordinator:
    """Synchronize completed agencies while delegating worker lifecycle."""

    def __init__(
        self, lottery: Lottery, lottery_lock: LotteryLock, agency_quorum_min: int
    ) -> None:
        self._agency_rounds = AgencyRounds(agency_quorum_min)
        self._workers = ClientWorkerRegistry(lottery, lottery_lock)

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

    def _release_round(self, round_to_start: AgencyRound) -> None:
        """Release exactly the workers selected for one complete round."""

        logger.info(
            "agency-round-start",
            logger.LogResult.success,
            "round-id",
            round_to_start.number,
            "agencies-amount",
            len(round_to_start.agency_ids),
        )
        for process_id in round_to_start.process_ids:
            worker = self._workers.get(process_id)
            if worker is not None:
                self._release_worker(worker)

    def _start_ready_rounds(self) -> None:
        """Open every complete round while leaving an incomplete remainder queued."""

        for round_to_start in self._agency_rounds.start_ready():
            self._release_round(round_to_start)

    def _read_worker_arrival(self, worker: ClientWorker) -> None:
        """Receive and register one complete agency notification."""

        if worker.connection is None:
            return
        try:
            agency_id = receive_arrival(worker.connection)
        except (EOFError, OSError, ValueError) as error:
            logger.error("agency-quorum-arrival", logger.LogResult.fail, "err", error)
            worker.close_connection()
            return

        worker.arrived = True
        waiting_count = self._agency_rounds.register(worker.process.pid, agency_id)
        logger.info(
            "agency-quorum-arrival",
            logger.LogResult.success,
            "agency-id",
            agency_id,
            "waiting-agencies-amount",
            waiting_count,
        )
        self._start_ready_rounds()

    def _finish_worker(self, process_id: int) -> None:
        """Reap one worker and report when its own round has finished."""

        completed_round = self._agency_rounds.remove_process(process_id)
        self._workers.reap(process_id)
        if completed_round is not None:
            logger.info(
                "agency-round-finish",
                logger.LogResult.success,
                "round-id",
                completed_round,
            )

    def wait(self, server_socket: socket.socket) -> list[object]:
        """Block until the listener or one managed worker object is ready."""

        return wait(self._workers.wait_objects(server_socket))

    def process_ready(self, ready_objects: list[object]) -> None:
        """Handle worker notifications and collect every completed process."""

        for process_id, worker in self._workers.snapshot():
            if worker.connection in ready_objects and not worker.arrived:
                self._read_worker_arrival(worker)
            if worker.process.sentinel in ready_objects:
                self._finish_worker(process_id)

    def shutdown(self) -> None:
        """Propagate termination and release every managed worker resource."""

        self._workers.shutdown()
