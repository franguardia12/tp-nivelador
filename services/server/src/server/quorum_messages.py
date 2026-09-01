"""Validated messages exchanged between workers and the quorum coordinator."""

from multiprocessing.connection import Connection

_AGENCY_NOTIFICATION_SIZE = 4
_QUORUM_RELEASE = b"\x01"


def notify_arrival_and_wait(connection: Connection, agency_id: int) -> None:
    """Notify one completed agency and wait for the quorum release token."""

    connection.send_bytes(agency_id.to_bytes(_AGENCY_NOTIFICATION_SIZE, "big"))
    release = connection.recv_bytes(maxlength=len(_QUORUM_RELEASE))
    if release != _QUORUM_RELEASE:
        raise ValueError("invalid quorum release token")


def receive_arrival(connection: Connection) -> int:
    """Receive and validate one complete agency notification."""

    notification = connection.recv_bytes(maxlength=_AGENCY_NOTIFICATION_SIZE)
    if len(notification) != _AGENCY_NOTIFICATION_SIZE:
        raise ValueError(f"invalid notification size {len(notification)}")
    return int.from_bytes(notification, "big")


def send_release(connection: Connection) -> None:
    """Release a worker after the quorum latch has opened."""

    connection.send_bytes(_QUORUM_RELEASE)
