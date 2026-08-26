#!/usr/bin/env bash
set -e

# Colores
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

MSG="Hello World"
SERVER_PORT=5678

# Nombre de la red del proyecto
NETWORK_NAME=$(docker network ls --filter "name=tp-nivelador" --format "{{.Name}}" | head -n 1)
NETWORK_NAME=${NETWORK_NAME:-"tp-nivelador_default"}

# Ejecutar netcat limitando el tiempo de espera a 2 segundos (-w 2)
RESPONSE=$(docker run --rm --network "$NETWORK_NAME" busybox sh -c "echo '$MSG' | nc -w 1 server $SERVER_PORT" 2>/dev/null | tr -d '\r\n')

if [ "$RESPONSE" = "$MSG" ]; then
  echo -e "action: test_echo_server | result: ${GREEN}success${NC}"
else
  echo -e "action: test_echo_server | result: ${RED}fail${NC}"
  echo -e "  Expected: '$MSG'"
  echo -e "  Got:      '${RESPONSE:-<empty>}'"
fi