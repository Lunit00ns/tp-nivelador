#!/usr/bin/env bash

CLIENTS=$2
OUTPUT_FILE=$1

if [ "$#" -ne 2 ]; then
    echo "Uso: $0 <archivo_salida> <cantidad_de_clientes>"
    exit 1
fi

if ! [[ "$CLIENTS" =~ ^[0-9]+$ ]]; then
  echo "Error: la cantidad de clientes debe ser un entero ≥ 0"
  exit 1
fi

cat <<EOF > "$OUTPUT_FILE"
name: tp-nivelador
services:
  server:
    build:
      context: ./services/server
      dockerfile: Dockerfile
    container_name: server
    environment:
      - PYTHONUNBUFFERED=1
      - SERVER_HOST=server
      - SERVER_PORT=5678
EOF

for i in $(seq 1 $CLIENTS); do
  cat <<EOF >> "$OUTPUT_FILE"

    client_$i:
      build:
        context: ./services/client
        dockerfile: Dockerfile
      container_name: client_$i
      depends_on:
        - server
      environment:
        - AGENCY_ID=$i
        - SERVER_HOST=server
        - SERVER_PORT=5678
EOF
done

echo "✔  docker-compose.yaml generado exitosamente con $CLIENTS clientes"