import os
from dataclasses import dataclass

_MAX_PORT = 65535


class ConfigError(Exception):
    """Error de configuracion en las variables de entorno del servidor."""


@dataclass(frozen=True)
class ServerConfig:
    host: str
    port: int
    agency_quorum_min: int


def load_config() -> ServerConfig:
    host = os.environ.get("SERVER_HOST")
    if not host:
        raise ConfigError("SERVER_HOST environment variable is required")

    port = _parse_positive_int("SERVER_PORT")
    if port > _MAX_PORT:
        raise ConfigError(f"SERVER_PORT must be between 1 and {_MAX_PORT}")

    agency_quorum_min = _parse_positive_int("AGENCY_QUORUM_MIN")

    return ServerConfig(host=host, port=port, agency_quorum_min=agency_quorum_min)


def _parse_positive_int(name: str) -> int:
    """Lee una variable de entorno y la valida como entero positivo."""
    raw = os.environ.get(name)
    if not raw:
        raise ConfigError(f"{name} environment variable is required")

    try:
        value = int(raw)
    except ValueError:
        raise ConfigError(f"{name} must be an integer")

    if value <= 0:
        raise ConfigError(f"{name} must be a positive integer")

    return value
