from dataclasses import dataclass
from enum import Enum

import safe_socket

# ESTRUCTURA EN EL SOCKET:
# [4 bytes BE: largo del payload][----------------- payload -----------------]
#                                [1 byte: tipo][campo 1][campo 2]....[campo N]
#
# ESTRUCTURA DE CADA CAMPO:
# [4 bytes BE: largo del texto][texto codificado en UTF-8]
#
# El mensaje HELLO se recibe una única vez al inicio de la conexión con el
# agency_id. A partir de ahí la agencia queda asociada a la conexión, por lo
# que los mensajes BET y END ya no transportan el agency_id.
_MESSAGE_HELLO = 1
_MESSAGE_BET = 2
_MESSAGE_END = 3
_MESSAGE_WINNER = 4
_MESSAGE_DONE = 5

_FRAME_HEADER_SIZE = 4  # 4 bytes para guardar el largo del frame (uint32)
_FIELD_HEADER_SIZE = 4  # 4 bytes para guardar el largo de cada campo de texto
_MAX_FRAME_SIZE = 1024 * 1024  # 1 MB máximo para evitar consumo excesivo de memoria

_HELLO_FIELD_COUNT = 1
_END_FIELD_COUNT = 0
_FIELDS_PER_BET = 5  # Nombre, Apellido, DNI, Fecha de Nacimiento, Número


class MessageType(Enum):
    HELLO = _MESSAGE_HELLO
    BET = _MESSAGE_BET
    END = _MESSAGE_END


@dataclass(frozen=True)
class BetData:
    """Datos de una apuesta tal como viajan en el protocolo.

    El agency_id no viaja en el mensaje BET; lo asigna el servidor a partir del
    HELLO de la conexión. Queda como campo para reutilizar la estructura al
    responder los ganadores (WINNER).
    """

    first_name: str
    last_name: str
    document: int
    birthdate: str
    number: int


@dataclass(frozen=True)
class Message:
    type: MessageType
    agency_id: int | None = None
    bets: list[BetData] | None = None


def receive_message(socket):
    """Lee del socket un frame completo y lo convierte en un objeto Message."""
    message_type, fields = _receive_message(socket)

    if message_type == _MESSAGE_HELLO:
        if len(fields) != _HELLO_FIELD_COUNT:
            raise ValueError(
                f"hello message must contain {_HELLO_FIELD_COUNT} field, got {len(fields)}"
            )
        return Message(MessageType.HELLO, int(fields[0]))

    if message_type == _MESSAGE_BET:
        if len(fields) % _FIELDS_PER_BET != 0 or len(fields) == 0:
            raise ValueError(
                f"invalid number of bet fields in batch: expected non-zero multiple of {_FIELDS_PER_BET}, got {len(fields)}"
            )

        bets = []
        for i in range(0, len(fields), _FIELDS_PER_BET):
            bet = BetData(
                first_name=fields[i],
                last_name=fields[i + 1],
                document=int(fields[i + 2]),
                birthdate=fields[i + 3],
                number=int(fields[i + 4]),
            )
            bets.append(bet)

        return Message(MessageType.BET, bets=bets)

    if message_type == _MESSAGE_END:
        if len(fields) != _END_FIELD_COUNT:
            raise ValueError(
                f"end message must contain {_END_FIELD_COUNT} fields, got {len(fields)}"
            )
        return Message(MessageType.END)

    raise ValueError(f"unexpected message type: {message_type}")


def send_winner(socket, bet: BetData):
    """Manda al cliente los datos de una apuesta ganadora."""

    _send_message(
        socket,
        _MESSAGE_WINNER,
        [
            bet.first_name,
            bet.last_name,
            str(bet.document),
            bet.birthdate,
            str(bet.number),
        ],
    )


def send_done(socket):
    """Notifica al cliente que no hay más ganadores."""

    _send_message(socket, _MESSAGE_DONE, [])


def _send_message(socket, message_type, fields):
    # Payload = [1 byte de tipo] + [cada campo precedido por su largo]
    payload = bytearray([message_type])

    for field in fields:
        field_bytes = field.encode("utf-8")

        if len(field_bytes) > _MAX_FRAME_SIZE - len(payload) - _FIELD_HEADER_SIZE:
            raise ValueError(f"protocol frame exceeds {_MAX_FRAME_SIZE} bytes")

        payload.extend(len(field_bytes).to_bytes(_FIELD_HEADER_SIZE, "big"))
        payload.extend(field_bytes)

    _send_frame(socket, payload)


def _receive_message(socket):
    payload = _receive_frame(socket)
    if not payload:
        raise ValueError("empty protocol message")

    message_type = payload[0]
    fields = []
    offset = 1  # Salteo el primer byte que es el tipo de mensaje

    # Avanzo campo por campo leyendo primero su largo (4 bytes) y luego su valor
    while offset < len(payload):
        if len(payload) - offset < _FIELD_HEADER_SIZE:
            raise ValueError("truncated field length")

        field_length = int.from_bytes(
            payload[offset : offset + _FIELD_HEADER_SIZE], "big"
        )
        offset += _FIELD_HEADER_SIZE

        if field_length > len(payload) - offset:
            raise ValueError("truncated field value")

        fields.append(payload[offset : offset + field_length].decode("utf-8"))
        offset += field_length

    return message_type, fields


def _send_frame(socket, payload):
    if len(payload) > _MAX_FRAME_SIZE:
        raise ValueError(f"protocol frame exceeds {_MAX_FRAME_SIZE} bytes")

    # Antepongo 4 bytes con el tamaño total del payload para que el receptor sepa cuanto leer
    safe_socket.send_all(socket, len(payload).to_bytes(_FRAME_HEADER_SIZE, "big"))
    safe_socket.send_all(socket, payload)


def _receive_frame(socket):
    # Leo exactamente 4 bytes para saber el tamaño del payload que viene después
    header = safe_socket.recv_all(socket, _FRAME_HEADER_SIZE)
    payload_size = int.from_bytes(header, "big")

    if payload_size > _MAX_FRAME_SIZE:
        raise ValueError(f"protocol frame exceeds {_MAX_FRAME_SIZE} bytes")

    # Leo exactamente la cantidad de bytes recibidos
    return safe_socket.recv_all(socket, payload_size)
