#!/usr/bin/env bash

CLIENTS=$1

if [ -z "$CLIENTS" ]; then
    echo "Uso: ./generate-compose.sh <cantidad_de_clientes>"
    exit 1
fi

cat <<EOF > docker-compose.yaml
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
cat <<EOF >> docker-compose.yaml

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

echo "✔ docker-compose.yaml generado exitosamente con $CLIENTS clientes."