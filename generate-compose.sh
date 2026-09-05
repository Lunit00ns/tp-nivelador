#!/usr/bin/env bash
set -euo pipefail

CONFIG_FILE="${1:-compose.config}"

if [ ! -f "$CONFIG_FILE" ]; then
    echo "Error: no se encontro el archivo de configuracion '$CONFIG_FILE'"
    echo "Uso: $0 [archivo_config]"
    exit 1
fi

OUTPUT_FILE="docker-compose.yaml"
CLIENTS=""
BATCH_SIZE="100"
AGENCY_QUORUM_MIN=""

is_uint() { [[ "$1" =~ ^[0-9]+$ ]]; }

is_valid_quorum() {
    is_uint "$1" && (( $1 >= 1 && $1 <= $2 ))
}

while IFS= read -r line || [ -n "$line" ]; do
    # Eliminar (\r) y espacios
    line="${line%%$'\r'}"

    case "$line" in
        ''|\#*) continue ;;
    esac

    key="${line%%=*}"
    value="${line#*=}"

    # Limpieza de espacios
    key="${key//[[:space:]]/}"

    case "$key" in
        OUTPUT_FILE)       OUTPUT_FILE="$value" ;;
        CLIENTS)           CLIENTS="$value" ;;
        BATCH_SIZE)        BATCH_SIZE="$value" ;;
        AGENCY_QUORUM_MIN) AGENCY_QUORUM_MIN="$value" ;;
        *)
            echo "Error: clave desconocida en la configuracion: '$key'"
            exit 1
            ;;
    esac
done < "$CONFIG_FILE"

# --- Validaciones ---

if [ -z "$CLIENTS" ]; then
    echo "Error: la clave CLIENTS es necesaria en '$CONFIG_FILE'"
    exit 1
fi

if ! is_uint "$CLIENTS"; then
    echo "Error: CLIENTS debe ser un entero >= 0 (valor: '$CLIENTS')"
    exit 1
fi

if [ -z "$AGENCY_QUORUM_MIN" ]; then
    AGENCY_QUORUM_MIN="$CLIENTS"
fi

if ! is_uint "$BATCH_SIZE" || (( BATCH_SIZE < 1 )); then
    echo "Error: BATCH_SIZE debe ser un entero >= 1 (valor: '$BATCH_SIZE')"
    exit 1
fi

if ! is_valid_quorum "$AGENCY_QUORUM_MIN" "$CLIENTS"; then
    echo "Error: AGENCY_QUORUM_MIN debe ser un entero entre 1 y CLIENTS ($CLIENTS) (valor: '$AGENCY_QUORUM_MIN')."
    echo "       El servidor nunca alcanzaría el quórum y no realizaría el sorteo."
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
      - AGENCY_QUORUM_MIN=$AGENCY_QUORUM_MIN
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

echo "✔  '$OUTPUT_FILE' generado: $CLIENTS clientes, BATCH_SIZE=$BATCH_SIZE, AGENCY_QUORUM_MIN=$AGENCY_QUORUM_MIN"
