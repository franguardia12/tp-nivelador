package client

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/betcsv"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/logger"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/model"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/protocol"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/shutdown"
)

// logSkippedBet records an invalid input row without interrupting later rows.
func logSkippedBet(recordIndex int, err error) {
	logger.Warn(
		"skip-invalid-bet",
		logger.Fail,
		"record", recordIndex,
		"err", err,
	)
}

// sendInput reads input incrementally, skips invalid records, and sends groups of
// at most BatchSize bets. The returned count includes only acknowledged records.
func (client *Client) sendInput(shutdownDone <-chan struct{}, input io.Reader) (int, error) {
	reader := csv.NewReader(input)
	reader.FieldsPerRecord = betcsv.FieldCount
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
		if shutdown.Requested(shutdownDone) {
			return processedRecords, shutdown.ErrRequested
		}
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

		bet, err := betcsv.Decode(record)
		if err != nil {
			logSkippedBet(currentRecordIndex, err)
			continue
		}

		if _, err := protocol.EncodeBet(bet); err != nil {
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
		return processedRecords, fmt.Errorf("process final batch: %w", err)
	}

	return processedRecords, nil
}
