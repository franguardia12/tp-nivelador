"""State machine for the complete protocol session of one agency."""

import socket
from multiprocessing.connection import Connection

import logger
import protocol
from lottery import Lottery

from ..lottery_file_lock import LotteryFileLock
from ..quorum_messages import notify_arrival_and_wait
from ..shutdown import ShutdownRequested
from .bet_receiver import BetReceiver
from .errors import ClientSessionError, try_send_error
from .winner_sender import WinnerSender


class ClientSession:
    """Coordinate the input, quorum and output phases of one connection."""

    def __init__(
        self,
        lottery: Lottery,
        lottery_file_lock: LotteryFileLock,
        coordinator_connection: Connection,
    ) -> None:
        self._bet_receiver = BetReceiver(lottery, lottery_file_lock)
        self._winner_sender = WinnerSender(lottery, lottery_file_lock)
        self._coordinator_connection = coordinator_connection

    def _wait_for_quorum(self, agency_id: int) -> None:
        """Notify the parent and block until assigned to a complete round."""

        action = "wait-agency-quorum"
        logger.info(
            action,
            logger.LogResult.in_progress,
            "agency-id",
            agency_id,
        )
        notify_arrival_and_wait(self._coordinator_connection, agency_id)
        logger.info(
            action,
            logger.LogResult.success,
            "agency-id",
            agency_id,
        )

    def run(self, client_socket: socket.socket) -> None:
        """Run one client session and report protocol or processing failures."""

        action = "handle-client"
        logger.info(action, logger.LogResult.in_progress)
        try:
            agency_id, stored_count = self._bet_receiver.receive(client_socket)
            self._wait_for_quorum(agency_id)
            winner_count = self._winner_sender.send(client_socket, agency_id)
        except ShutdownRequested:
            raise
        except ClientSessionError as error:
            try_send_error(client_socket, error)
            logger.error(action, logger.LogResult.fail, "err", error)
            return
        except Exception as error:
            try_send_error(
                client_socket,
                ClientSessionError(
                    failed_type=0,
                    code=protocol.ErrorCode.INTERNAL,
                    detail="internal server error",
                ),
            )
            logger.error(action, logger.LogResult.fail, "err", error)
            return

        logger.info(
            action,
            logger.LogResult.success,
            "agency-id",
            agency_id,
            "bets-amount",
            stored_count,
            "winners-amount",
            winner_count,
        )
