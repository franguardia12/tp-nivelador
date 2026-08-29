package client

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/model"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/protocol"
)

const connectionAttemptsMax = 10
const connectionAttemptDelay = 500 * time.Millisecond

type ClientConfig struct {
	ServerHost string
	ServerPort string
	AgencyID   uint32
	BatchSize  uint32
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
	for i := range connectionAttemptsMax {
		conn, err = net.Dial("tcp", host+":"+port)
		if err != nil {
			logger.Warn(action, logger.Fail, "attempt", i)
			if i+1 < connectionAttemptsMax {
				time.Sleep(connectionAttemptDelay)
			}
			continue
		}

		logger.Info(action, logger.Success)
		break
	}

	return conn, err
}

// closeResource closes a resource and preserves an earlier execution error. A
// close failure becomes the returned error only when no previous error exists.
func closeResource(name string, resource io.Closer, runErr *error) {
	if err := resource.Close(); err != nil && *runErr == nil {
		*runErr = fmt.Errorf("close %s: %w", name, err)
	}
}

// betFromCSV validates and converts one five-field CSV record into a domain Bet.
func betFromCSV(record []string) (model.Bet, error) {
	if len(record) != 5 {
		return model.Bet{}, fmt.Errorf("expected 5 fields, got %d", len(record))
	}

	document, err := strconv.ParseUint(record[2], 10, 64)
	if err != nil {
		return model.Bet{}, fmt.Errorf("invalid document %q: %w", record[2], err)
	}
	number, err := strconv.ParseUint(record[4], 10, 32)
	if err != nil {
		return model.Bet{}, fmt.Errorf("invalid number %q: %w", record[4], err)
	}

	return model.Bet{
		FirstName: record[0],
		LastName:  record[1],
		Document:  document,
		Birthdate: record[3],
		Number:    uint32(number),
	}, nil
}

// csvFromBet converts a domain Bet into the field order used by output files.
func csvFromBet(bet model.Bet) []string {
	return []string{
		bet.FirstName,
		bet.LastName,
		strconv.FormatUint(bet.Document, 10),
		bet.Birthdate,
		strconv.FormatUint(uint64(bet.Number), 10),
	}
}

// serverError converts a protocol ERROR response into a local descriptive error.
func serverError(message protocol.Message) error {
	protocolError, err := protocol.DecodeError(message.Payload)
	if err != nil {
		return fmt.Errorf("decode server error: %w", err)
	}
	return fmt.Errorf(
		"server rejected message 0x%02x with code %d: %s",
		uint8(protocolError.FailedType),
		protocolError.Code,
		protocolError.Detail,
	)
}

// expectAck receives and validates the acknowledgement for one client request.
func (client *Client) expectAck(expectedType protocol.MessageType, expectedCount uint32) error {
	message, err := protocol.ReceiveMessage(client.conn)
	if err != nil {
		return err
	}
	if message.Type == protocol.MessageTypeError {
		return serverError(message)
	}
	if message.Type != protocol.MessageTypeAck {
		return fmt.Errorf(
			"unexpected response type 0x%02x while waiting for ACK",
			uint8(message.Type),
		)
	}

	ack, err := protocol.DecodeAck(message.Payload)
	if err != nil {
		return fmt.Errorf("decode ACK: %w", err)
	}
	if ack.AcknowledgedType != expectedType {
		return fmt.Errorf(
			"ACK confirms message 0x%02x, expected 0x%02x",
			uint8(ack.AcknowledgedType),
			uint8(expectedType),
		)
	}
	if ack.ProcessedCount != expectedCount {
		return fmt.Errorf(
			"ACK reports %d processed records, expected %d",
			ack.ProcessedCount,
			expectedCount,
		)
	}
	return nil
}

// registerAgency associates the connection with the configured agency and waits
// until the server confirms the registration.
func (client *Client) registerAgency() error {
	payload := protocol.EncodeAgency(client.config.AgencyID)
	if err := protocol.SendMessage(client.conn, protocol.MessageTypeAgency, payload); err != nil {
		return fmt.Errorf("send agency: %w", err)
	}
	if err := client.expectAck(protocol.MessageTypeAgency, 0); err != nil {
		return fmt.Errorf("register agency: %w", err)
	}
	return nil
}

// sendBatch serializes one non-empty group of bets, sends it in a single BETS
// message, and verifies that the server processed every record.
func (client *Client) sendBatch(bets []model.Bet) error {
	payload, err := protocol.EncodeBets(bets)
	if err != nil {
		return fmt.Errorf("encode batch: %w", err)
	}
	if err := protocol.SendMessage(client.conn, protocol.MessageTypeBets, payload); err != nil {
		return fmt.Errorf("send batch: %w", err)
	}
	if err := client.expectAck(protocol.MessageTypeBets, uint32(len(bets))); err != nil {
		return fmt.Errorf("store batch: %w", err)
	}
	return nil
}

// logSkippedBet records an invalid input row without interrupting later rows.
func logSkippedBet(recordIndex int, err any) {
	logger.Warn(
		"skip-invalid-bet",
		logger.Fail,
		"record", recordIndex,
		"err", err,
	)
}

// sendInput reads input incrementally, skips invalid records, and sends groups of
// at most BatchSize bets. The returned count includes only acknowledged records.
func (client *Client) sendInput(input io.Reader) (int, error) {
	reader := csv.NewReader(input)
	reader.FieldsPerRecord = 5
	processedRecords := 0
	recordIndex := 0
	batch := make([]model.Bet, 0)

	flushBatch := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := client.sendBatch(batch); err != nil {
			return err
		}
		processedRecords += len(batch)
		batch = batch[:0]
		return nil
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		currentRecordIndex := recordIndex
		recordIndex++
		if err != nil {
			var parseError *csv.ParseError
			if errors.As(err, &parseError) {
				logSkippedBet(currentRecordIndex, err)
				continue
			}
			return processedRecords, fmt.Errorf(
				"read record %d: %w",
				currentRecordIndex,
				err,
			)
		}

		bet, err := betFromCSV(record)
		if err != nil {
			logSkippedBet(currentRecordIndex, err)
			continue
		}

		_, err = protocol.EncodeBet(bet)
		if err != nil {
			logSkippedBet(currentRecordIndex, err)
			continue
		}

		batch = append(batch, bet)

		if uint32(len(batch)) == client.config.BatchSize {
			if err := flushBatch(); err != nil {
				return processedRecords, fmt.Errorf("process count-limited batch: %w", err)
			}
		}
	}

	if err := flushBatch(); err != nil {
		return processedRecords, fmt.Errorf(
			"process final batch: %w",
			err,
		)
	}

	return processedRecords, nil
}

// finishSendingBets notifies the server that the client will send no more BETS.
func (client *Client) finishSendingBets() error {
	if err := protocol.SendMessage(client.conn, protocol.MessageTypeEndBets, nil); err != nil {
		return fmt.Errorf("send end of bets: %w", err)
	}
	return nil
}

// receiveWinners consumes the streamed winner sequence, writes each record to
// output, and verifies the count declared by WINNERS_END.
func (client *Client) receiveWinners(output io.Writer) (uint32, error) {
	writer := csv.NewWriter(output)
	var receivedCount uint32

	for {
		message, err := protocol.ReceiveMessage(client.conn)
		if err != nil {
			return receivedCount, err
		}

		switch message.Type {
		case protocol.MessageTypeWinner:
			bet, err := protocol.DecodeBet(message.Payload)
			if err != nil {
				return receivedCount, fmt.Errorf("decode winner %d: %w", receivedCount, err)
			}
			if receivedCount == ^uint32(0) {
				return receivedCount, fmt.Errorf("winner count exceeds uint32")
			}
			if err := writer.Write(csvFromBet(bet)); err != nil {
				return receivedCount, fmt.Errorf("write winner %d: %w", receivedCount, err)
			}
			receivedCount++

		case protocol.MessageTypeWinnersEnd:
			expectedCount, err := protocol.DecodeWinnersEnd(message.Payload)
			if err != nil {
				return receivedCount, fmt.Errorf("decode winners end: %w", err)
			}
			if expectedCount != receivedCount {
				return receivedCount, fmt.Errorf(
					"received %d winners, server reported %d",
					receivedCount,
					expectedCount,
				)
			}
			writer.Flush()
			if err := writer.Error(); err != nil {
				return receivedCount, fmt.Errorf("flush winners: %w", err)
			}
			return receivedCount, nil

		case protocol.MessageTypeError:
			return receivedCount, serverError(message)

		default:
			return receivedCount, fmt.Errorf(
				"unexpected response type 0x%02x while receiving winners",
				uint8(message.Type),
			)
		}
	}
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

	logger.Info(action, logger.InProgress, "agency-id", client.config.AgencyID)
	if err := client.registerAgency(); err != nil {
		return err
	}
	processedRecords, err := client.sendInput(inputFile)
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
