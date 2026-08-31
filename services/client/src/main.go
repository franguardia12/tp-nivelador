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

func run(ctx context.Context) int {
	clientConfig, err := config.Load()
	if err != nil {
		logger.Error("load-config", logger.Fail, "err", err)
		return 1
	}

	lotteryClient, err := client.NewClient(ctx, clientConfig)
	if err != nil {
		if ctx.Err() != nil {
			logger.Info("client-shutdown", logger.Success)
			return 0
		}
		logger.Error("client-new", logger.Fail, "err", err)
		return 1
	}

	if err := lotteryClient.Run(ctx); err != nil {
		logger.Error("client-run", logger.Fail, "err", err)
		return 1
	}
	if ctx.Err() != nil {
		logger.Info("client-shutdown", logger.Success)
	}
	return 0
}

func main() {
	ctx, stopSignalNotifications := signal.NotifyContext(
		context.Background(),
		syscall.SIGTERM,
	)
	exitCode := run(ctx)
	stopSignalNotifications()
	os.Exit(exitCode)
}
