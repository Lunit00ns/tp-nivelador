package config

import (
	"fmt"
	"os"
	"strconv"
)

type ClientConfig struct {
	ServerHost string
	ServerPort string
	AgencyId   string
	BatchSize  int

	InputFile  string
	OutputFile string
}

func LoadConfig() (ClientConfig, error) {
	agencyId, err := requireEnv("AGENCY_ID")
	if err != nil {
		return ClientConfig{}, err
	}

	serverHost, err := requireEnv("SERVER_HOST")
	if err != nil {
		return ClientConfig{}, err
	}

	serverPort, err := requireEnv("SERVER_PORT")
	if err != nil {
		return ClientConfig{}, err
	}

	inputFile, err := requireEnv("INPUT_FILE")
	if err != nil {
		return ClientConfig{}, err
	}

	outputFile, err := requireEnv("OUTPUT_FILE")
	if err != nil {
		return ClientConfig{}, err
	}

	batchSize, err := requirePositiveInt("BATCH_SIZE")
	if err != nil {
		return ClientConfig{}, err
	}

	return ClientConfig{
		ServerHost: serverHost,
		ServerPort: serverPort,
		AgencyId:   agencyId,
		BatchSize:  batchSize,
		InputFile:  inputFile,
		OutputFile: outputFile,
	}, nil
}

func requireEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s environment variable is required", name)
	}
	return value, nil
}

func requirePositiveInt(name string) (int, error) {
	raw, err := requireEnv(name)
	if err != nil {
		return 0, err
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s environment variable must be a positive integer", name)
	}
	return value, nil
}
