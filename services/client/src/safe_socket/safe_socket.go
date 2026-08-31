package safe_socket

import (
	"fmt"
	"io"
)

const maxConsecutiveNoProgress = 100

// SendAll garantiza el envío completo del buffer de bytes.
// Evita el problema de SHORT WRITE reintentando enviar los bytes pendientes.
func SendAll(socket io.Writer, bytes []byte) error {
	for sent, noProgress := 0, 0; sent < len(bytes); {
		// Reintenta solo los bytes que todavia no fueron enviados
		n, err := socket.Write(bytes[sent:])

		if n < 0 || n > len(bytes)-sent {
			return fmt.Errorf("invalid write count: %d", n)
		}
		sent += n

		if err != nil {
			return err
		}

		if n == 0 {
			// Si no hubo avance en la escritura, incrementa el contador para no caer en un loop
			noProgress++
			if noProgress == maxConsecutiveNoProgress {
				return io.ErrNoProgress
			}
			continue
		}

		noProgress = 0
	}
	return nil
}

// RecvAll garantiza la lectura de exactamente `size` bytes.
// Evita el problema de SHORT READ acumulando datos hasta llenar el buffer.
func RecvAll(socket io.Reader, size int) ([]byte, error) {
	if size < 0 {
		return nil, fmt.Errorf("invalid read size: %d", size)
	}

	buff := make([]byte, size)
	for received, noProgress := 0, 0; received < size; {
		// Conserva lo ya leido y completa el resto del buffer
		n, err := socket.Read(buff[received:])

		if n < 0 || n > size-received {
			return nil, fmt.Errorf("invalid read count: %d", n)
		}
		received += n

		if received == size {
			return buff, nil
		}

		if err != nil {
			return nil, err
		}

		if n == 0 {
			noProgress++
			if noProgress == maxConsecutiveNoProgress {
				return nil, io.ErrNoProgress
			}
			continue
		}

		noProgress = 0
	}
	return buff, nil
}
