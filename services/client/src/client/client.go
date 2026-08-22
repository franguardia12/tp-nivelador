package client

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/safe_socket"
)

const CONNECTION_ATTEMPTS_MAX = 3
const CONNECTION_ATTEMPS_DELAY_MS = 200

type ClientConfig struct {
	ServerHost string
	ServerPort string
	AgencyId   string
	InputFile  string
	OutputFile string
}

type Client struct {
	conn   net.Conn
	config ClientConfig
}

func NewClient(config ClientConfig) (*Client, error) {
	conn, err := connectToServer(config.ServerHost, config.ServerPort)
	if err != nil {
		logger.Warn("connect-to-server", logger.Fail)
		return nil, err
	}

	client := &Client{conn: conn, config: config}
	return client, nil
}

func connectToServer(host, port string) (net.Conn, error) {
	const action = "connect-to-server"
	var err error
	var conn net.Conn

	logger.Info(action, logger.InProgress)
	for i := range CONNECTION_ATTEMPTS_MAX {
		conn, err = net.Dial("tcp", host+":"+port)
		if err != nil {
			logger.Warn(action, logger.Fail, "attempt", i)
			time.Sleep(CONNECTION_ATTEMPS_DELAY_MS * time.Millisecond)
			continue
		}

		logger.Info(action, logger.Success)
		break
	}

	return conn, err
}

func closeResource(name string, resource io.Closer, runErr *error) {
	if err := resource.Close(); err != nil && *runErr == nil {
		*runErr = fmt.Errorf("close %s: %w", name, err)
	}
}

func (client *Client) processInput(input io.Reader, output io.Writer) (int, error) {
	scanner := bufio.NewScanner(input)
	writer := bufio.NewWriter(output)
	processedRecords := 0

	for scanner.Scan() {
		message := scanner.Bytes()
		if len(message) == 0 {
			continue
		}

		if err := safe_socket.SendAll(client.conn, message); err != nil {
			return processedRecords, fmt.Errorf("send record %d: %w", processedRecords, err)
		}

		response, err := safe_socket.RecvAll(client.conn, len(message))
		if err != nil {
			return processedRecords, fmt.Errorf("receive record %d: %w", processedRecords, err)
		}
		if !bytes.Equal(response, message) {
			return processedRecords, fmt.Errorf("unexpected echo for record %d", processedRecords)
		}

		written, err := writer.Write(response)
		if err != nil {
			return processedRecords, fmt.Errorf("write record %d: %w", processedRecords, err)
		}
		if written != len(response) {
			return processedRecords, fmt.Errorf("write record %d: %w", processedRecords, io.ErrShortWrite)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return processedRecords, fmt.Errorf("write record delimiter %d: %w", processedRecords, err)
		}

		processedRecords++
	}

	if err := scanner.Err(); err != nil {
		return processedRecords, fmt.Errorf("read input: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return processedRecords, fmt.Errorf("flush output: %w", err)
	}

	return processedRecords, nil
}

func (client *Client) Run() (err error) {
	const action = "process-input-file"
	defer closeResource("server connection", client.conn, &err)

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

	logger.Info(action, logger.InProgress, "agency-id", client.config.AgencyId)
	processedRecords, err := client.processInput(inputFile, outputFile)
	if err != nil {
		return err
	}
	logger.Info(
		action,
		logger.Success,
		"agency-id", client.config.AgencyId,
		"records-amount", processedRecords,
	)

	return nil
}
