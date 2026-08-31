package client

import (
	"net"
	"time"
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/safe_socket"
)

const CONNECTION_ATTEMPTS_MAX = 3
const CONNECTION_ATTEMPS_DELAY_MS = 200

const ECHO_CLIENT_BUFFER_SIZE = 512
const ECHO_CLIENT_MESSAGE_AMOUNT = 3
const ECHO_CLIENT_MESSAGE_DELAY_MS = 1000

type ClientConfig struct {
	ServerHost string
	ServerPort string
	AgencyId   string

	InputFile  string
	OutputFile string
}

type Client struct {
	conn   net.Conn
	config ClientConfig
}

func NewClient(config ClientConfig) (*Client, error) {
	conn, err := connectToServer(config.ServerHost, config.ServerPort)
	if err != nil {
		logger.Warn("connect-to-server", logger.Fail)
		return nil, err
	}

	client := &Client{conn: conn, config: config}
	return client, nil
}

func connectToServer(host, port string) (net.Conn, error) {
	const action = "connect-to-server"
	var err error
	var conn net.Conn

	logger.Info(action, logger.InProgress)
	for i := range CONNECTION_ATTEMPTS_MAX {
		conn, err = net.Dial("tcp", host+":"+port)
		if err != nil {
			logger.Warn(action, logger.Fail, "attempt", i)
			time.Sleep(CONNECTION_ATTEMPS_DELAY_MS * time.Millisecond)
			continue
		}

		logger.Info(action, logger.Success)
		break
	}

	return conn, err
}

func (client *Client) Run() error {
	const mainAction = "process-bet-file"
	defer client.conn.Close()

	inputFile, err := os.Open(client.config.InputFile)
	if err != nil {
		logger.Error("open-input-file", logger.Fail, "err", err)
		return err
	}
	defer inputFile.Close()

	outputDir := filepath.Dir(client.config.OutputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		logger.Error("create-output-dir", logger.Fail, "output-dir", outputDir, "err", err)
		return err
	}

	outputFile, err := os.OpenFile(client.config.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		logger.Error("open-output-file", logger.Fail, "err", err)
		return err
	}
	defer outputFile.Close()

	scanner := bufio.NewScanner(inputFile)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Loggear la apuesta que se está procesando
		messageArgs := []any{"agency-id", client.config.AgencyId, "bet", line}
		logger.Info(mainAction, logger.InProgress, messageArgs...)

		msgBytes := []byte(line)

		// Enviar al servidor la apuesta leída del archivo de entrada
		if err := safe_socket.SendAll(client.conn, msgBytes); err != nil {
			logger.Error("send-bet", logger.Fail, messageArgs...)
			return err
		}

		// Recibir del servidor la misma cantidad de bytes que se enviaron
		responseBuffer, err := safe_socket.RecvAll(client.conn, len(msgBytes))
		if err != nil {
			logger.Error("recv-response", logger.Fail, messageArgs...)
			return err
		}

		// Loggear la respuesta recibida del servidor
		response := strings.TrimSpace(string(responseBuffer))
		if response == "" {
			logger.Error("empty-response", logger.Fail, messageArgs...)
			continue
		}

		// Escribir la respuesta en el archivo de salida
		if _, err := outputFile.WriteString(response + "\n"); err != nil {
			logger.Error("write-output-file", logger.Fail, append(messageArgs, "err", err)...)
			return err
		}

		logger.Info(mainAction, logger.Success, messageArgs...)
	}

	if err := scanner.Err(); err != nil {
		logger.Error("read-input-file", logger.Fail, "err", err)
		return err
	}

	// Me aseguro que los datos se escriban en disco antes de cerrar el archivo
	_ = outputFile.Sync()

	logger.Info(mainAction, logger.Success, "agency-id", client.config.AgencyId)
	return nil
}
