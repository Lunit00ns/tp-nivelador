package client

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/protocol"
)

const (
	connectionAttemptsMax    = 3
	connectionAttemptDelayMs = 200
)

type ClientConfig struct {
	ServerHost string
	ServerPort string
	AgencyId   string
	BatchSize  int

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

	return &Client{conn: conn, config: config}, nil
}

func connectToServer(host, port string) (net.Conn, error) {
	const action = "connect-to-server"
	logger.Info(action, logger.InProgress)

	for i := range connectionAttemptsMax {
		conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
		if err == nil {
			logger.Info(action, logger.Success)
			return conn, nil
		}

		logger.Warn(action, logger.Fail, "attempt", i+1, "err", err)
		time.Sleep(connectionAttemptDelayMs * time.Millisecond)
	}

	return nil, fmt.Errorf("failed to connect to server after %d attempts", connectionAttemptsMax)
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

	// Acumulador de apuestas en memoria para enviar por lotes
	batch := make([][]string, 0, client.config.BatchSize)

	// Leer archivo de apuestas y enviarlas al servidor
	scanner := bufio.NewScanner(inputFile)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Split(line, ",")
		batch = append(batch, fields)

		// Si se llega al límite del tamaño del lote, se envía
		if len(batch) >= client.config.BatchSize {
			if err := protocol.SendBet(client.conn, client.config.AgencyId, batch...); err != nil {
				logger.Error("send-bet-batch", logger.Fail, "agency-id", client.config.AgencyId, "err", err)
				return err
			}
			batch = batch[:0] // Limpia el slice manteniendo la capacidad asignada
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Error("read-input-file", logger.Fail, "err", err)
		return err
	}

	// Enviar las apuestas que hayan quedado
	if len(batch) > 0 {
		if err := protocol.SendBet(client.conn, client.config.AgencyId, batch...); err != nil {
			logger.Error("send-bet-batch", logger.Fail, "agency-id", client.config.AgencyId, "err", err)
			return err
		}
	}

	if err := protocol.SendEnd(client.conn, client.config.AgencyId); err != nil {
		logger.Error("send-end", logger.Fail, "agency-id", client.config.AgencyId, "err", err)
		return err
	}

	// Recibir el listado de ganadores hasta obtener DONE
	for {
		message, err := protocol.ReceiveMessage(client.conn)
		if err != nil {
			logger.Error("recv-winners", logger.Fail, "agency-id", client.config.AgencyId, "err", err)
			return err
		}

		switch message.Type {
		case protocol.Winner:
			record := strings.Join(message.Fields, ",") + "\n"
			if _, err := outputFile.WriteString(record); err != nil {
				logger.Error("write-output-file", logger.Fail, "agency-id", client.config.AgencyId, "err", err)
				return err
			}

		case protocol.Done:
			if err := outputFile.Sync(); err != nil {
				logger.Error("sync-output-file", logger.Fail, "agency-id", client.config.AgencyId, "err", err)
				return err
			}
			logger.Info(mainAction, logger.Success, "agency-id", client.config.AgencyId)
			return nil

		default:
			return fmt.Errorf("unexpected response message %q", message.Type)
		}
	}
}
