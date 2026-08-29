package protocol

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/safe_socket"
)

const HeaderSize = 5

// MessageType identifies the purpose and expected payload of a protocol frame.
type MessageType uint8

const (
	MessageTypeAgency     MessageType = 0x01
	MessageTypeBets       MessageType = 0x02
	MessageTypeEndBets    MessageType = 0x03
	MessageTypeAck        MessageType = 0x80
	MessageTypeWinner     MessageType = 0x81
	MessageTypeWinnersEnd MessageType = 0x82
	MessageTypeError      MessageType = 0xFF
)

// Message is one decoded frame composed of a known type and its raw payload.
type Message struct {
	Type    MessageType
	Payload []byte
}

// isKnownMessageType reports whether a value belongs to the protocol vocabulary.
func isKnownMessageType(messageType MessageType) bool {
	switch messageType {
	case MessageTypeAgency,
		MessageTypeBets,
		MessageTypeEndBets,
		MessageTypeAck,
		MessageTypeWinner,
		MessageTypeWinnersEnd,
		MessageTypeError:
		return true
	default:
		return false
	}
}

// SendMessage validates and frames one message, then transfers the complete frame.
// Payload length must fit in the uint32 field defined by the wire format.
func SendMessage(writer io.Writer, messageType MessageType, payload []byte) error {
	if !isKnownMessageType(messageType) {
		return fmt.Errorf("unknown message type 0x%02x", uint8(messageType))
	}
	if uint64(len(payload)) > uint64(^uint32(0)) {
		return fmt.Errorf(
			"payload length %d cannot be represented by uint32",
			len(payload),
		)
	}

	frame := make([]byte, HeaderSize+len(payload))
	frame[0] = byte(messageType)
	binary.BigEndian.PutUint32(frame[1:HeaderSize], uint32(len(payload)))
	copy(frame[HeaderSize:], payload)

	if err := safe_socket.SendAll(writer, frame); err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	return nil
}

// ReceiveMessage obtains a complete header and payload and rejects unknown types
// or lengths that cannot be represented on the current platform.
func ReceiveMessage(reader io.Reader) (Message, error) {
	header, err := safe_socket.RecvAll(reader, HeaderSize)
	if err != nil {
		return Message{}, fmt.Errorf("receive message header: %w", err)
	}

	messageType := MessageType(header[0])
	if !isKnownMessageType(messageType) {
		return Message{}, fmt.Errorf("unknown message type 0x%02x", header[0])
	}

	payloadSize := binary.BigEndian.Uint32(header[1:HeaderSize])
	maxInt := uint64(^uint(0) >> 1)
	if uint64(payloadSize) > maxInt {
		return Message{}, fmt.Errorf(
			"payload length %d exceeds platform capacity %d",
			payloadSize,
			maxInt,
		)
	}

	payload, err := safe_socket.RecvAll(reader, int(payloadSize))
	if err != nil {
		return Message{}, fmt.Errorf("receive message payload: %w", err)
	}

	return Message{Type: messageType, Payload: payload}, nil
}
