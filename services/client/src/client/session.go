package client

import (
	"fmt"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/model"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/protocol"
)

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

// finishSendingBets notifies the server that the client will send no more BETS.
func (client *Client) finishSendingBets() error {
	if err := protocol.SendMessage(client.conn, protocol.MessageTypeEndBets, nil); err != nil {
		return fmt.Errorf("send end of bets: %w", err)
	}
	return nil
}
