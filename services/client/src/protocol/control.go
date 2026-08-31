package protocol

import (
	"encoding/binary"
	"fmt"
)

const agencyPayloadSize = 4
const ackPayloadSize = 5
const winnersEndPayloadSize = 4

// Ack describes which request was acknowledged and how many bets were processed.
type Ack struct {
	AcknowledgedType MessageType
	ProcessedCount   uint32
}

// EncodeAgency serializes the connection-scoped agency identifier.
func EncodeAgency(agencyID uint32) []byte {
	payload := make([]byte, agencyPayloadSize)
	binary.BigEndian.PutUint32(payload, agencyID)
	return payload
}

// DecodeAck deserializes a payload containing exactly one acknowledgement.
func DecodeAck(payload []byte) (Ack, error) {
	if len(payload) != ackPayloadSize {
		return Ack{}, fmt.Errorf(
			"ack payload length is %d, expected %d",
			len(payload),
			ackPayloadSize,
		)
	}

	ack := Ack{
		AcknowledgedType: MessageType(payload[0]),
		ProcessedCount:   binary.BigEndian.Uint32(payload[1:]),
	}
	if ack.AcknowledgedType != MessageTypeAgency && ack.AcknowledgedType != MessageTypeBets {
		return Ack{}, fmt.Errorf(
			"message type 0x%02x cannot be acknowledged",
			payload[0],
		)
	}
	return ack, nil
}

// DecodeWinnersEnd deserializes the declared number of winners.
func DecodeWinnersEnd(payload []byte) (uint32, error) {
	if len(payload) != winnersEndPayloadSize {
		return 0, fmt.Errorf(
			"winners-end payload length is %d, expected %d",
			len(payload),
			winnersEndPayloadSize,
		)
	}
	return binary.BigEndian.Uint32(payload), nil
}
