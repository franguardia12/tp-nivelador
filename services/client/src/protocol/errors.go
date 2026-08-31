package protocol

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"
)

const errorPayloadMinimumSize = 5

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

// isKnownErrorCode reports whether a code is part of the protocol specification.
func isKnownErrorCode(code ErrorCode) bool {
	return code >= ErrorCodeMalformedMessage && code <= ErrorCodeInternal
}

// isKnownFailedType accepts known message types and zero for unidentified frames.
func isKnownFailedType(messageType MessageType) bool {
	return messageType == 0 || isKnownMessageType(messageType)
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
