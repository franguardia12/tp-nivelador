package protocol

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"
)

const agencyPayloadSize = 4
const ackPayloadSize = 5
const winnersEndPayloadSize = 4
const errorPayloadMinimumSize = 5

// Ack describes which request was acknowledged and how many bets were processed.
type Ack struct {
	AcknowledgedType MessageType
	ProcessedCount   uint32
}

// ErrorCode classifies protocol, validation, and processing failures.
type ErrorCode uint16

const (
	ErrorCodeMalformedMessage  ErrorCode = 1
	ErrorCodeUnexpectedMessage ErrorCode = 2
	ErrorCodeInvalidData       ErrorCode = 3
	ErrorCodeInternal          ErrorCode = 4
)

// ErrorPayload contains the request associated with an error and a UTF-8 detail.
type ErrorPayload struct {
	FailedType MessageType
	Code       ErrorCode
	Detail     string
}

// EncodeAgency serializes the connection-scoped agency identifier.
func EncodeAgency(agencyID uint32) []byte {
	payload := make([]byte, agencyPayloadSize)
	binary.BigEndian.PutUint32(payload, agencyID)
	return payload
}

// DecodeAgency deserializes a payload containing exactly one agency identifier.
func DecodeAgency(payload []byte) (uint32, error) {
	if len(payload) != agencyPayloadSize {
		return 0, fmt.Errorf(
			"agency payload length is %d, expected %d",
			len(payload),
			agencyPayloadSize,
		)
	}
	return binary.BigEndian.Uint32(payload), nil
}

// EncodeAck serializes the request type and the number of processed bets.
func EncodeAck(ack Ack) ([]byte, error) {
	if ack.AcknowledgedType != MessageTypeAgency && ack.AcknowledgedType != MessageTypeBets {
		return nil, fmt.Errorf(
			"message type 0x%02x cannot be acknowledged",
			uint8(ack.AcknowledgedType),
		)
	}

	payload := make([]byte, ackPayloadSize)
	payload[0] = byte(ack.AcknowledgedType)
	binary.BigEndian.PutUint32(payload[1:], ack.ProcessedCount)
	return payload, nil
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

// EncodeWinnersEnd serializes the total number of WINNER messages sent.
func EncodeWinnersEnd(winnerCount uint32) []byte {
	payload := make([]byte, winnersEndPayloadSize)
	binary.BigEndian.PutUint32(payload, winnerCount)
	return payload
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

// isKnownErrorCode reports whether a code is part of the protocol specification.
func isKnownErrorCode(code ErrorCode) bool {
	return code >= ErrorCodeMalformedMessage && code <= ErrorCodeInternal
}

// isKnownFailedType accepts known message types and zero for unidentified frames.
func isKnownFailedType(messageType MessageType) bool {
	return messageType == 0 || isKnownMessageType(messageType)
}

// EncodeError serializes an error code, its failed message type and a UTF-8 detail.
func EncodeError(protocolError ErrorPayload) ([]byte, error) {
	if !isKnownFailedType(protocolError.FailedType) {
		return nil, fmt.Errorf("unknown failed message type 0x%02x", protocolError.FailedType)
	}
	if !isKnownErrorCode(protocolError.Code) {
		return nil, fmt.Errorf("unknown error code %d", protocolError.Code)
	}
	if !utf8.ValidString(protocolError.Detail) {
		return nil, fmt.Errorf("error detail is not valid UTF-8")
	}
	if len(protocolError.Detail) > maxStringSize {
		return nil, fmt.Errorf(
			"error detail length %d exceeds maximum %d",
			len(protocolError.Detail),
			maxStringSize,
		)
	}

	payload := make([]byte, errorPayloadMinimumSize+len(protocolError.Detail))
	payload[0] = byte(protocolError.FailedType)
	binary.BigEndian.PutUint16(payload[1:3], uint16(protocolError.Code))
	binary.BigEndian.PutUint16(payload[3:5], uint16(len(protocolError.Detail)))
	copy(payload[errorPayloadMinimumSize:], protocolError.Detail)
	return payload, nil
}

// DecodeError deserializes an ERROR payload and rejects invalid codes or lengths.
func DecodeError(payload []byte) (ErrorPayload, error) {
	if len(payload) < errorPayloadMinimumSize {
		return ErrorPayload{}, fmt.Errorf("error payload is incomplete")
	}

	protocolError := ErrorPayload{
		FailedType: MessageType(payload[0]),
		Code:       ErrorCode(binary.BigEndian.Uint16(payload[1:3])),
	}
	if !isKnownFailedType(protocolError.FailedType) {
		return ErrorPayload{}, fmt.Errorf(
			"unknown failed message type 0x%02x",
			payload[0],
		)
	}
	if !isKnownErrorCode(protocolError.Code) {
		return ErrorPayload{}, fmt.Errorf("unknown error code %d", protocolError.Code)
	}

	detailSize := int(binary.BigEndian.Uint16(payload[3:5]))
	if len(payload) != errorPayloadMinimumSize+detailSize {
		return ErrorPayload{}, fmt.Errorf(
			"error payload length is %d, expected %d",
			len(payload),
			errorPayloadMinimumSize+detailSize,
		)
	}
	detail := payload[errorPayloadMinimumSize:]
	if !utf8.Valid(detail) {
		return ErrorPayload{}, fmt.Errorf("error detail is not valid UTF-8")
	}
	protocolError.Detail = string(detail)
	return protocolError, nil
}
