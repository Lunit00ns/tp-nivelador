package main

import (
	"os"

	client "github.com/7574-sistemas-distribuidos/tp-nivelador/src/client"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/config"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
)

func run() int {
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("load-config", logger.Fail, "err", err)
		return 1
	}

	c, err := client.NewClient(cfg)
	if err != nil {
		logger.Error("client-new", logger.Fail, "err", err)
		return 1
	}

	if err := c.Run(); err != nil {
		logger.Error("client-run", logger.Fail, "err", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run())
}
