package client

import (
	"context"
	"net"
	"time"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
)

const connectionAttemptsMax = 10
const connectionAttemptDelay = 500 * time.Millisecond

// connectToServer retries transient startup failures and makes the delay
// interruptible so SIGTERM does not have to wait for the next attempt.
func connectToServer(ctx context.Context, host, port string) (net.Conn, error) {
	const action = "connect-to-server"
	address := net.JoinHostPort(host, port)
	var err error
	var conn net.Conn

	logger.Info(action, logger.InProgress)
	for attempt := range connectionAttemptsMax {
		var dialer net.Dialer
		conn, err = dialer.DialContext(ctx, "tcp", address)
		if err == nil {
			logger.Info(action, logger.Success)
			return conn, nil
		}

		logger.Warn(action, logger.Fail, "attempt", attempt)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt+1 < connectionAttemptsMax {
			if err := waitForRetry(ctx); err != nil {
				return nil, err
			}
		}
	}

	return nil, err
}

// waitForRetry blocks without busy waiting and remains cancelable.
func waitForRetry(ctx context.Context) error {
	retryTimer := time.NewTimer(connectionAttemptDelay)
	defer retryTimer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-retryTimer.C:
		return nil
	}
}
