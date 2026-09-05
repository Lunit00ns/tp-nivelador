package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	client "github.com/7574-sistemas-distribuidos/tp-nivelador/src/client"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/config"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
)

func run() int {
	// El contexto se cancela ante SIGTERM/SIGINT, habilitando el cierre
	// ordenado del cliente en cualquier etapa de la comunicación
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("load-config", logger.Fail, "err", err)
		return 1
	}

	c, err := client.NewClient(ctx, cfg)
	if err != nil {
		// Si el error se debe a la señal de terminación, es un cierre esperado
		if ctx.Err() != nil {
			return 0
		}
		logger.Error("client-new", logger.Fail, "err", err)
		return 1
	}

	if err := c.Run(); err != nil {
		if ctx.Err() != nil {
			return 0
		}
		logger.Error("client-run", logger.Fail, "err", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run())
}
