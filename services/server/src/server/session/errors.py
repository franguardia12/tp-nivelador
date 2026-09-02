"""Errors produced while executing one client protocol session."""

import socket

import logger
import protocol


class ClientSessionError(Exception):
    """A protocol error that can be reported to the connected client."""

    def __init__(self, failed_type: int, code: protocol.ErrorCode, detail: str) -> None:
        """Store the wire-level context needed to report the failure."""

        super().__init__(detail)
        self.failed_type = failed_type
        self.code = code
        self.detail = detail


def unexpected_message(
    message: protocol.Message,
    expected: str,
) -> ClientSessionError:
    """Build a reportable error for a message received out of order."""

    return ClientSessionError(failed_type=message.type, code=protocol.ErrorCode.UNEXPECTED_MESSAGE, 
                              detail=f"expected {expected}, received {message.type.name}")


def receive_message(client_socket: socket.socket) -> protocol.Message:
    """Receive a frame and translate codec failures into session errors."""

    try:
        return protocol.receive_message(client_socket)
    except ValueError as error:
        raise ClientSessionError(failed_type=0, code=protocol.ErrorCode.MALFORMED_MESSAGE, 
                                 detail=str(error)) from error


def try_send_error(client_socket: socket.socket, session_error: ClientSessionError) -> None:
    """Best-effort report a session failure when the socket remains usable."""

    try:
        payload = protocol.encode_error(protocol.ErrorPayload(failed_type=session_error.failed_type, 
                                                              code=session_error.code, 
                                                              detail=session_error.detail))
        protocol.send_message(client_socket, protocol.MessageType.ERROR, payload)
    except Exception as error:
        logger.error("send-client-error", logger.LogResult.fail, "err", error)
