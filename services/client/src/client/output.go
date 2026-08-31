package client

import (
	"encoding/csv"
	"fmt"
	"io"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/betcsv"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/protocol"
)

// receiveWinners consumes the streamed winner sequence, writes each record to
// output, and verifies the count declared by WINNERS_END.
func (client *Client) receiveWinners(output io.Writer) (receivedCount uint32, err error) {
	writer := csv.NewWriter(output)
	defer func() {
		writer.Flush()
		if flushErr := writer.Error(); flushErr != nil && err == nil {
			err = fmt.Errorf("flush winners: %w", flushErr)
		}
	}()

	for {
		message, receiveErr := protocol.ReceiveMessage(client.conn)
		if receiveErr != nil {
			return receivedCount, receiveErr
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
			if err := writer.Write(betcsv.Encode(bet)); err != nil {
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
