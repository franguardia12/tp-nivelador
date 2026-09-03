import socket


def recv_all(sock: socket.socket, size: int) -> bytes:
    """Receive exactly size bytes, retrying successful partial operations.

    Socket exceptions are propagated. An empty result before the requested size
    is complete indicates that the peer closed the connection prematurely.
    """

    if size < 0:
        raise ValueError(f"invalid read size {size}")

    data = bytearray()

    while len(data) < size:
        recv_size = size - len(data)
        # Socket errors are deliberately propagated. Only successful partial
        # receives are retried; retrying an arbitrary error could loop forever.
        chunk = sock.recv(recv_size)
        if chunk == b"":
            raise EOFError(f"unexpected EOF after {len(data)} of {size} bytes received")

        data.extend(chunk)

    return bytes(data)


def send_all(sock: socket.socket, data: bytes) -> None:
    """Send every byte in data, retrying successful partial operations.

    Socket exceptions are propagated, while a zero-byte result without an
    exception is retried until the requested transfer is complete.
    """

    view = memoryview(data)
    total_sent = 0

    while total_sent < len(view):
        # Socket errors are deliberately propagated. Only successful partial
        # sends are retried; retrying an arbitrary error could loop forever.
        sent = sock.send(view[total_sent:])

        total_sent += sent

    return None
