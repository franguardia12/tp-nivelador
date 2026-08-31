package client

import (
	"fmt"
	"io"
	"os"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/shutdown"
)

// closeResource closes a resource and preserves an earlier execution error. A
// close failure becomes the returned error only when no previous error exists.
func closeResource(name string, resource io.Closer, runErr *error) {
	if err := resource.Close(); err != nil {
		logger.Error("close-resource", logger.Fail, "resource", name, "err", err)
		if *runErr == nil {
			*runErr = fmt.Errorf("close %s: %w", name, err)
		}
	}
}

// watchShutdown closes the connection when SIGTERM closes the notification
// channel, waking any blocked socket operation. The returned function joins the
// watcher.
func (client *Client) watchShutdown(shutdownDone <-chan struct{}) func() {
	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		select {
		case <-shutdownDone:
			// The deferred closeResource call observes and reports this stored
			// result after the blocked socket operation has been released.
			_ = client.Close()
		case <-stop:
		}
	}()

	return func() {
		close(stop)
		<-done
	}
}

// Run performs one complete client session and owns all acquired resources.
func (client *Client) Run(shutdownDone <-chan struct{}) (err error) {
	const action = "process-input-file"
	// Cancellation caused by SIGTERM is a successful controlled termination.
	// Register this defer first so resource-close defers run before it.
	defer func() {
		if shutdown.Requested(shutdownDone) {
			err = nil
		}
	}()
	defer closeResource("server connection", client, &err)

	stopShutdownWatcher := client.watchShutdown(shutdownDone)
	defer stopShutdownWatcher()

	if shutdown.Requested(shutdownDone) {
		return shutdown.ErrRequested
	}

	inputFile, err := os.Open(client.config.InputFile)
	if err != nil {
		return fmt.Errorf("open input file: %w", err)
	}
	defer closeResource("input file", inputFile, &err)

	outputFile, err := os.Create(client.config.OutputFile)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer closeResource("output file", outputFile, &err)

	logger.Info(action, logger.InProgress, "agency-id", client.config.AgencyID)
	if err := client.registerAgency(); err != nil {
		return err
	}
	processedRecords, err := client.sendInput(shutdownDone, inputFile)
	if err != nil {
		return err
	}
	if err := client.finishSendingBets(); err != nil {
		return err
	}
	winnerCount, err := client.receiveWinners(outputFile)
	if err != nil {
		return err
	}
	logger.Info(
		action,
		logger.Success,
		"agency-id", client.config.AgencyID,
		"records-amount", processedRecords,
		"winners-amount", winnerCount,
	)

	return nil
}
