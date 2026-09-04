package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/safe_socket"
)

// ESTRUCTURA EN EL SOCKET:
// [4 bytes BE: largo del payload][----------------- payload -----------------]
//
//	[1 byte: tipo][campo 1][campo 2]....[campo N]
//
// ESTRUCTURA DE CADA CAMPO:
// [4 bytes BE: largo del texto][texto codificado en UTF-8]
//
// El mensaje HELLO se envía una única vez al inicio de la conexión con el
// agency_id. A partir de ahí la agencia queda asociada a la conexión, por lo
// que los mensajes BET y END ya no transportan el agency_id.
const (
	messageHello byte = iota + 1
	messageBet
	messageEnd
	messageWinner
	messageDone

	frameHeaderSize = 4 // Bytes para guardar el tamaño total del payload
	fieldHeaderSize = 4 // Bytes para guardar el largo de cada campo
	maxFrameSize    = 1024 * 1024

	// Cantidad de campos que debe traer cada tipo de mensaje.
	betFieldCount    = 5
	winnerFieldCount = 5
	doneFieldCount   = 0
)

type MessageType byte

const (
	Winner = MessageType(messageWinner)
	Done   = MessageType(messageDone)
)

type Message struct {
	Type   MessageType
	Fields []string
}

// Anuncia el agency_id de la conexión. Se envía una única vez,
// antes de cualquier apuesta.
func SendHello(writer io.Writer, agencyID string) error {
	return sendMessage(writer, messageHello, []string{agencyID})
}

func SendBet(writer io.Writer, bets ...[]string) error {
	if len(bets) == 0 {
		return errors.New("cannot send an empty bet batch")
	}

	// Se aplanan todas las apuestas en un solo slice de campos:
	// [f1_1, f1_2..., f1_5, f2_1, f2_2..., f2_5, ...]
	// El agency_id ya no viaja en cada BET: se anunció una sola vez con HELLO.
	fields := make([]string, 0, len(bets)*betFieldCount)

	for _, bet := range bets {
		if len(bet) != betFieldCount {
			return fmt.Errorf("each bet in batch must contain %d fields, got %d", betFieldCount, len(bet))
		}
		fields = append(fields, bet...)
	}

	return sendMessage(writer, messageBet, fields)
}

func SendEnd(writer io.Writer) error {
	return sendMessage(writer, messageEnd, []string{})
}

func ReceiveMessage(reader io.Reader) (Message, error) {
	messageType, fields, err := receiveMessage(reader)
	if err != nil {
		return Message{}, err
	}

	switch messageType {
	case messageWinner:
		if len(fields) != winnerFieldCount {
			return Message{}, fmt.Errorf("winner message must contain %d fields, got %d", winnerFieldCount, len(fields))
		}
		return Message{Type: Winner, Fields: fields}, nil

	case messageDone:
		if len(fields) != doneFieldCount {
			return Message{}, fmt.Errorf("done message must contain no fields, got %d", len(fields))
		}
		return Message{Type: Done}, nil

	default:
		return Message{}, fmt.Errorf("unexpected message type: %d", messageType)
	}
}

func sendMessage(writer io.Writer, messageType byte, fields []string) error {
	// Arma el payload en memoria: tipo + cada campo precedido de su largo.
	payload := []byte{messageType}

	for _, field := range fields {
		fieldBytes := []byte(field)
		if len(fieldBytes) > maxFrameSize-len(payload)-fieldHeaderSize {
			return fmt.Errorf("protocol frame exceeds %d bytes", maxFrameSize)
		}

		// Se convierte el largo del campo a 4 bytes Big-Endian (el byte más significativo se envía primero)
		fieldLength := make([]byte, fieldHeaderSize)
		binary.BigEndian.PutUint32(fieldLength, uint32(len(fieldBytes)))

		payload = append(payload, fieldLength...)
		payload = append(payload, fieldBytes...)
	}

	return sendFrame(writer, payload)
}

func receiveMessage(reader io.Reader) (byte, []string, error) {
	payload, err := receiveFrame(reader)
	if err != nil {
		return 0, nil, err
	}
	if len(payload) == 0 {
		return 0, nil, errors.New("empty protocol message")
	}

	messageType := payload[0]
	fields := make([]string, 0)
	offset := 1 // Empieza después del byte de tipo

	for offset < len(payload) {
		if len(payload)-offset < fieldHeaderSize {
			return 0, nil, errors.New("truncated field length")
		}

		fieldLength := int(binary.BigEndian.Uint32(payload[offset : offset+fieldHeaderSize]))
		offset += fieldHeaderSize

		if fieldLength > len(payload)-offset {
			return 0, nil, errors.New("truncated field value")
		}

		fieldValue := string(payload[offset : offset+fieldLength])
		fields = append(fields, fieldValue)
		offset += fieldLength
	}

	return messageType, fields, nil
}

func sendFrame(writer io.Writer, payload []byte) error {
	if len(payload) > maxFrameSize {
		return fmt.Errorf("protocol frame exceeds %d bytes", maxFrameSize)
	}

	// Antepongo el largo del payload para que el receptor sepa cuanto leer
	header := make([]byte, frameHeaderSize)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))

	if err := safe_socket.SendAll(writer, header); err != nil {
		return err
	}

	return safe_socket.SendAll(writer, payload)
}

func receiveFrame(reader io.Reader) ([]byte, error) {
	// Primero lee el header de largo fijo, luego exactamente esa cantidad de bytes
	header, err := safe_socket.RecvAll(reader, frameHeaderSize)
	if err != nil {
		return nil, err
	}

	payloadSize := binary.BigEndian.Uint32(header)
	if payloadSize > maxFrameSize {
		return nil, fmt.Errorf("protocol frame exceeds %d bytes", maxFrameSize)
	}

	return safe_socket.RecvAll(reader, int(payloadSize))
}
