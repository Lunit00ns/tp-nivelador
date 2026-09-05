package client

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/config"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/protocol"
)

const (
	connectionAttemptsMax    = 5
	connectionAttemptDelayMs = 500
)

type Client struct {
	conn   net.Conn
	config config.ClientConfig
}

func NewClient(cfg config.ClientConfig) (*Client, error) {
	conn, err := connectToServer(cfg.ServerHost, cfg.ServerPort)
	if err != nil {
		logger.Warn("connect-to-server", logger.Fail)
		return nil, err
	}

	return &Client{conn: conn, config: cfg}, nil
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

// Run ejecuta el flujo completo del cliente: anuncia la agencia, envía las
// apuestas por lotes y persiste los ganadores recibidos.
func (client *Client) Run() error {
	const mainAction = "process-bet-file"
	defer client.conn.Close()

	if err := protocol.SendHello(client.conn, client.config.AgencyId); err != nil {
		logger.Error("send-hello", logger.Fail, "agency-id", client.config.AgencyId, "err", err)
		return err
	}

	if err := client.sendBets(); err != nil {
		return err
	}

	outputFile, err := client.openOutputFile()
	if err != nil {
		return err
	}
	defer outputFile.Close()

	if err := client.receiveWinners(outputFile); err != nil {
		return err
	}

	logger.Info(mainAction, logger.Success, "agency-id", client.config.AgencyId)
	return nil
}

// sendBets lee el archivo de entrada línea a línea y envía las apuestas al
// servidor agrupadas en lotes de tamano BatchSize, seguido de un END.
func (client *Client) sendBets() error {
	inputFile, err := os.Open(client.config.InputFile)
	if err != nil {
		logger.Error("open-input-file", logger.Fail, "err", err)
		return err
	}
	defer inputFile.Close()

	// Acumulador de apuestas en memoria para enviar por lotes
	batch := make([][]string, 0, client.config.BatchSize)

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
			if err := client.flushBatch(batch); err != nil {
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
		if err := client.flushBatch(batch); err != nil {
			return err
		}
	}

	if err := protocol.SendEnd(client.conn); err != nil {
		logger.Error("send-end", logger.Fail, "agency-id", client.config.AgencyId, "err", err)
		return err
	}

	return nil
}

// flushBatch envía un lote de apuestas al servidor
func (client *Client) flushBatch(batch [][]string) error {
	if err := protocol.SendBet(client.conn, batch...); err != nil {
		logger.Error("send-bet-batch", logger.Fail, "agency-id", client.config.AgencyId, "err", err)
		return err
	}
	return nil
}

func (client *Client) openOutputFile() (*os.File, error) {
	outputDir := filepath.Dir(client.config.OutputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		logger.Error("create-output-dir", logger.Fail, "output-dir", outputDir, "err", err)
		return nil, err
	}

	outputFile, err := os.OpenFile(client.config.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		logger.Error("open-output-file", logger.Fail, "err", err)
		return nil, err
	}
	return outputFile, nil
}

func (client *Client) receiveWinners(outputFile *os.File) error {
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
			return nil

		default:
			return fmt.Errorf("unexpected response message %q", message.Type)
		}
	}
}
