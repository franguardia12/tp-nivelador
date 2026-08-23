import socket


def recv_all(sock: socket.socket, size: int) -> bytes:
    if size < 0:
        raise ValueError(f"invalid read size {size}")

    data = bytearray()

    while len(data) < size:
        recv_size = size - len(data)
        # Socket errors are deliberately propagated. Only successful partial
        # receives are retried; retrying an arbitrary error could loop forever.
        chunk = sock.recv(recv_size)

        data.extend(chunk)

    return bytes(data)


def send_all(sock: socket.socket, data: bytes) -> None:
    view = memoryview(data)
    total_sent = 0

    while total_sent < len(view):
        # Socket errors are deliberately propagated. Only successful partial
        # sends are retried; retrying an arbitrary error could loop forever.
        sent = sock.send(view[total_sent:])            

        total_sent += sent

    return None
