"""Parent-side lifecycle and quorum coordination for client processes."""

import multiprocessing
import socket
import time
from dataclasses import dataclass
from multiprocessing.connection import Connection, wait

import logger
from lottery import Lottery

from .client_session import ClientSession
from .lottery_file_lock import LotteryFileLock
from .quorum import AgencyQuorum
from .shutdown import (
    ShutdownRequested,
    install_sigterm_handler,
    restore_sigterm_handler,
)

_AGENCY_NOTIFICATION_SIZE = 4
_QUORUM_RELEASE = b"\x01"
_WORKER_SHUTDOWN_TIMEOUT_SECONDS = 3.0


@dataclass
class _ClientProcess:
    """Parent-owned process and control connection for one client worker."""

    process: multiprocessing.Process
    connection: Connection | None
    arrived: bool = False


def _serve_client_process(
    client_socket: socket.socket,
    lottery: Lottery,
    lottery_file_lock: LotteryFileLock,
    coordinator_connection: Connection,
) -> None:
    """Own one client socket and its coordinator connection in a child process."""

    previous_sigterm_handler = install_sigterm_handler()
    try:
        with coordinator_connection, client_socket:
            ClientSession(
                lottery,
                lottery_file_lock,
                coordinator_connection,
            ).run(client_socket)
    except ShutdownRequested:
        logger.info("client-process-shutdown", logger.LogResult.success)
    finally:
        restore_sigterm_handler(previous_sigterm_handler)


class ProcessCoordinator:
    """Create client workers and synchronize them from the parent process."""

    def __init__(
        self,
        lottery: Lottery,
        lottery_file_lock: LotteryFileLock,
        agency_quorum_min: int,
    ) -> None:
        """Configure shared storage, quorum state, and the spawn context."""

        self._lottery = lottery
        self._lottery_file_lock = lottery_file_lock
        self._agency_quorum = AgencyQuorum(agency_quorum_min)
        # Spawn transfers only the resources supplied to the worker and avoids
        # inheriting the listener and control connections of previous clients.
        self._process_context = multiprocessing.get_context("spawn")
        self._client_processes: dict[int, _ClientProcess] = {}

    def start_client(self, client_socket: socket.socket) -> None:
        """Transfer an accepted socket and one duplex Pipe end to a new worker."""

        parent_connection, child_connection = self._process_context.Pipe(duplex=True)
        client_process = self._process_context.Process(
            target=_serve_client_process,
            args=(
                client_socket,
                self._lottery,
                self._lottery_file_lock,
                child_connection,
            ),
            name=f"client-{client_socket.fileno()}",
            daemon=False,
        )
        try:
            client_process.start()
        except Exception:
            parent_connection.close()
            child_connection.close()
            client_socket.close()
            raise

        client_socket.close()
        child_connection.close()
        self._client_processes[client_process.pid] = _ClientProcess(
            process=client_process,
            connection=parent_connection,
        )

    @staticmethod
    def _release_worker(worker: _ClientProcess) -> None:
        """Wake one arrived worker exactly once through its control connection."""

        if worker.connection is None:
            return
        try:
            worker.connection.send_bytes(_QUORUM_RELEASE)
        except (BrokenPipeError, OSError) as error:
            logger.error("release-client", logger.LogResult.fail, "err", error)
        finally:
            worker.connection.close()
            worker.connection = None

    def _release_waiting_workers(self) -> None:
        """Release every arrived worker after the one-way quorum latch opens."""

        if not self._agency_quorum.is_open:
            return
        for worker in self._client_processes.values():
            if worker.arrived:
                self._release_worker(worker)

    def _read_worker_arrival(self, worker: _ClientProcess) -> None:
        """Receive and validate one complete agency notification message."""

        if worker.connection is None:
            return
        try:
            notification = worker.connection.recv_bytes(
                maxlength=_AGENCY_NOTIFICATION_SIZE
            )
        except (EOFError, OSError) as error:
            logger.error(
                "agency-quorum-arrival",
                logger.LogResult.fail,
                "err",
                error,
            )
            worker.connection.close()
            worker.connection = None
            return

        if len(notification) != _AGENCY_NOTIFICATION_SIZE:
            logger.error(
                "agency-quorum-arrival",
                logger.LogResult.fail,
                "err",
                f"invalid notification size {len(notification)}",
            )
            worker.connection.close()
            worker.connection = None
            return

        agency_id = int.from_bytes(notification, "big")
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

    def _reap_worker(self, process_id: int) -> None:
        """Join one completed process and close its parent-owned connection."""

        worker = self._client_processes.pop(process_id)
        worker.process.join()
        self._close_worker(worker)

    def _wait_objects(self, server_socket: socket.socket) -> list[object]:
        """Collect the listener, unread control connections, and sentinels."""

        wait_objects: list[object] = [server_socket]
        for worker in self._client_processes.values():
            if worker.connection is not None and not worker.arrived:
                wait_objects.append(worker.connection)
            wait_objects.append(worker.process.sentinel)
        return wait_objects

    def wait(self, server_socket: socket.socket) -> list[object]:
        """Block until the listener or one of the managed worker objects is ready."""

        return wait(self._wait_objects(server_socket))

    def process_ready(self, ready_objects: list[object]) -> None:
        """Handle worker notifications and collect every completed process."""

        for process_id, worker in list(self._client_processes.items()):
            if worker.connection in ready_objects and not worker.arrived:
                self._read_worker_arrival(worker)
            if worker.process.sentinel in ready_objects:
                self._reap_worker(process_id)

    @staticmethod
    def _close_worker(worker: _ClientProcess) -> None:
        """Release the parent-side objects of a worker that is no longer alive."""

        if worker.connection is not None:
            worker.connection.close()
            worker.connection = None
        worker.process.close()

    def shutdown(self) -> None:
        """Request orderly worker termination, then enforce a bounded deadline."""

        if not self._client_processes:
            return

        action = "shutdown-client-processes"
        logger.info(
            action,
            logger.LogResult.in_progress,
            "processes-amount",
            len(self._client_processes),
        )
        workers = list(self._client_processes.values())
        for worker in workers:
            if worker.process.is_alive():
                worker.process.terminate()

        deadline = time.monotonic() + _WORKER_SHUTDOWN_TIMEOUT_SECONDS
        for worker in workers:
            remaining = max(0.0, deadline - time.monotonic())
            worker.process.join(remaining)

        forced_count = 0
        for worker in workers:
            if worker.process.is_alive():
                forced_count += 1
                worker.process.kill()
                worker.process.join()
            self._close_worker(worker)

        self._client_processes.clear()
        result = (
            logger.LogResult.success if forced_count == 0 else logger.LogResult.fail
        )
        logger.info(
            action,
            result,
            "forced-processes-amount",
            forced_count,
        )
