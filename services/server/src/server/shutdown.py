"""SIGTERM translation into the server's normal exception flow."""

import signal
from collections.abc import Callable
from types import FrameType


class ShutdownRequested(Exception):
    """Request an orderly stop of the current server process."""


SignalHandler = Callable[[int, FrameType | None], None] | int | None


def _raise_shutdown_request(
    _signal_number: int,
    _frame: FrameType | None,
) -> None:
    """Interrupt a blocking operation by raising the shutdown exception."""

    raise ShutdownRequested


def install_sigterm_handler() -> SignalHandler:
    """Translate SIGTERM into ShutdownRequested and return the old handler."""

    return signal.signal(signal.SIGTERM, _raise_shutdown_request)


def restore_sigterm_handler(previous_handler: SignalHandler) -> None:
    """Restore the handler that was active before server startup."""

    signal.signal(signal.SIGTERM, previous_handler)
