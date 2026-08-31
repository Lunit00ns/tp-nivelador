import socket

_MAX_CONSECUTIVE_NO_PROGRESS = 100


def recv_all(socket: socket.socket, size):
    """Lee del socket exactamente `size` bytes antes de retornar.

    Resuelve el problema de SHORT READ (cuando socket.recv retorna menos bytes
    de los que esperábamos recibir).
    """
    if size < 0:
        raise ValueError("size must be non-negative")

    chunks = bytearray()
    while len(chunks) < size:
        # Pide solamente los bytes que faltan para completar el mensaje
        bytes_needed = size - len(chunks)
        chunk = socket.recv(bytes_needed)

        # Si socket.recv retorna bytes vacíos (b""), significa que el otro extremo cerró la conexión
        if not chunk:
            raise ConnectionError("connection closed before receiving all data")

        chunks.extend(chunk)

    return bytes(chunks)


def send_all(socket: socket.socket, bytes):
    """Escribe en el socket la totalidad de los bytes especificados.

    Resuelve el problema de SHORT WRITE (cuando socket.send envía solo una parte
    de los bytes debido a que el buffer del Kernel está lleno).
    """
    sent = 0
    no_progress = 0

    while sent < len(bytes):
        # Reintenta desde el primer byte que no se envio
        written = socket.send(bytes[sent:])

        if written == 0:
            # Si el socket no acepta bytes de forma repetida, corto para evitar un bucle infinito
            no_progress += 1
            if no_progress == _MAX_CONSECUTIVE_NO_PROGRESS:
                raise ConnectionError("connection closed before sending all data")
            continue

        sent += written
        no_progress = 0
