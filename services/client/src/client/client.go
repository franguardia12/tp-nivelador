package client

import (
	"net"
	"sync"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/config"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
)

// ClientConfig keeps the construction API stable while configuration loading
// lives in its own package.
type ClientConfig = config.Config

// Client owns one agency's TCP session and its shutdown coordination state.
type Client struct {
	conn      net.Conn
	config    ClientConfig
	closeOnce sync.Once
	closeErr  error
}

// NewClient establishes the configured server connection.
func NewClient(shutdownDone <-chan struct{}, config ClientConfig) (*Client, error) {
	conn, err := connectToServer(shutdownDone, config.ServerHost, config.ServerPort)
	if err != nil {
		logger.Warn("connect-to-server", logger.Fail)
		return nil, err
	}

	client := &Client{conn: conn, config: config}
	return client, nil
}

// Close releases the server connection once, even when normal cleanup races
// with cancellation triggered by SIGTERM.
func (client *Client) Close() error {
	client.closeOnce.Do(func() {
		client.closeErr = client.conn.Close()
	})
	return client.closeErr
}
