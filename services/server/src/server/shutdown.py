"""SIGTERM translation into the server's normal exception flow."""

import signal


class ShutdownRequested(Exception):
    """Request an orderly stop of the current server process."""


def _raise_shutdown_request(_signal_number: int, _frame) -> None:
    """Interrupt a blocking operation by raising the shutdown exception."""

    raise ShutdownRequested


def install_sigterm_handler():
    """Translate SIGTERM into ShutdownRequested and return the old handler."""

    return signal.signal(signal.SIGTERM, _raise_shutdown_request)


def restore_sigterm_handler(previous_handler) -> None:
    """Restore the handler that was active before server startup."""

    signal.signal(signal.SIGTERM, previous_handler)
