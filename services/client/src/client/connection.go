package client

import (
	"net"
	"time"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/shutdown"
)

const connectionAttemptsMax = 10
const connectionAttemptDelay = 500 * time.Millisecond
const connectionAttemptTimeout = time.Second

// connectToServer retries transient startup failures and makes the delay
// interruptible so SIGTERM does not have to wait for the next attempt.
func connectToServer(shutdownDone <-chan struct{}, host, port string) (net.Conn, error) {
	const action = "connect-to-server"
	address := net.JoinHostPort(host, port)
	var err error
	var conn net.Conn

	logger.Info(action, logger.InProgress)
	for attempt := range connectionAttemptsMax {
		if shutdown.Requested(shutdownDone) {
			return nil, shutdown.ErrRequested
		}

		conn, err = net.DialTimeout("tcp", address, connectionAttemptTimeout)
		if shutdown.Requested(shutdownDone) {
			if conn != nil {
				if closeErr := conn.Close(); closeErr != nil {
					logger.Error(
						"close-resource",
						logger.Fail,
						"resource", "server connection",
						"err", closeErr,
					)
				}
			}
			return nil, shutdown.ErrRequested
		}
		if err == nil {
			logger.Info(action, logger.Success)
			return conn, nil
		}

		logger.Warn(action, logger.Fail, "attempt", attempt)
		if attempt+1 < connectionAttemptsMax {
			if err := waitForRetry(shutdownDone); err != nil {
				return nil, err
			}
		}
	}

	return nil, err
}

// waitForRetry blocks without busy waiting and remains cancelable.
func waitForRetry(shutdownDone <-chan struct{}) error {
	retryTimer := time.NewTimer(connectionAttemptDelay)
	defer retryTimer.Stop()

	select {
	case <-shutdownDone:
		return shutdown.ErrRequested
	case <-retryTimer.C:
		return nil
	}
}
