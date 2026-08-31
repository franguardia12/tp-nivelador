package main

import (
	"os"

	client "github.com/7574-sistemas-distribuidos/tp-nivelador/src/client"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/config"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/shutdown"
)

func run(shutdownDone <-chan struct{}) int {
	clientConfig, err := config.Load()
	if err != nil {
		logger.Error("load-config", logger.Fail, "err", err)
		return 1
	}

	lotteryClient, err := client.NewClient(shutdownDone, clientConfig)
	if err != nil {
		if shutdown.Requested(shutdownDone) {
			logger.Info("client-shutdown", logger.Success)
			return 0
		}
		logger.Error("client-new", logger.Fail, "err", err)
		return 1
	}

	if err := lotteryClient.Run(shutdownDone); err != nil {
		logger.Error("client-run", logger.Fail, "err", err)
		return 1
	}
	if shutdown.Requested(shutdownDone) {
		logger.Info("client-shutdown", logger.Success)
	}
	return 0
}

func main() {
	shutdownNotifier := shutdown.NewSIGTERMNotifier()
	exitCode := run(shutdownNotifier.Done())
	shutdownNotifier.Stop()
	os.Exit(exitCode)
}
