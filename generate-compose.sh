#!/usr/bin/env bash

CLIENTS=$2
OUTPUT_FILE=$1
BATCH_SIZE=${3:-100} # Valor por defecto de 100

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
    echo "Uso: $0 <archivo_salida> <cantidad_de_clientes> [batch_size]"
    exit 1
fi

if ! [[ "$CLIENTS" =~ ^[0-9]+$ ]]; then
  echo "Error: la cantidad de clientes debe ser un entero ≥ 0"
  exit 1
fi

if ! [[ "$BATCH_SIZE" =~ ^[0-9]+$ ]]; then
  echo "Error: batch_size debe ser un entero ≥ 0"
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
    ports:
      - "5678:5678"
    environment:
      - PYTHONUNBUFFERED=1
      - SERVER_HOST=server
      - SERVER_PORT=5678
EOF

for (( i=0; i<CLIENTS; i++ )); do
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
      - INPUT_FILE=/input/input-$i.csv
      - OUTPUT_FILE=/output/output-$i.csv
      - BATCH_SIZE=$BATCH_SIZE
    volumes:
      - ./input:/input:ro
      - ./output:/output
EOF
done

echo "✔  $OUTPUT_FILE generado exitosamente con $CLIENTS clientes y BATCH_SIZE=$BATCH_SIZE"