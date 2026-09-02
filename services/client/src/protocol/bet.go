package protocol

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/model"
)

const maxStringSize = 65535

type betDecoder struct {
	data   []byte
	offset int
}

// appendString appends one UTF-8 string prefixed by its uint16 byte length.
func appendString(payload []byte, fieldName string, value string) ([]byte, error) {
	if !utf8.ValidString(value) {
		return nil, fmt.Errorf("%s is not valid UTF-8", fieldName)
	}
	if len(value) > maxStringSize {
		return nil, fmt.Errorf(
			"%s length %d exceeds maximum %d",
			fieldName,
			len(value),
			maxStringSize,
		)
	}

	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(value)))
	payload = append(payload, length...)
	return append(payload, value...), nil
}

// appendBet appends one bet to an existing payload without an agency identifier.
// It can therefore be reused while assembling a payload containing several bets.
func appendBet(payload []byte, bet model.Bet) ([]byte, error) {
	var err error
	payload, err = appendString(payload, "first name", bet.FirstName)
	if err != nil {
		return nil, err
	}
	payload, err = appendString(payload, "last name", bet.LastName)
	if err != nil {
		return nil, err
	}

	numericFields := make([]byte, 8)
	binary.BigEndian.PutUint64(numericFields, bet.Document)
	payload = append(payload, numericFields...)

	payload, err = appendString(payload, "birthdate", bet.Birthdate)
	if err != nil {
		return nil, err
	}

	numericFields = make([]byte, 4)
	binary.BigEndian.PutUint32(numericFields, bet.Number)
	return append(payload, numericFields...), nil
}

// read consumes size bytes from the decoder's current position.
func (decoder *betDecoder) read(size int, fieldName string) ([]byte, error) {
	if size < 0 || size > len(decoder.data)-decoder.offset {
		return nil, fmt.Errorf("incomplete %s", fieldName)
	}

	value := decoder.data[decoder.offset : decoder.offset+size]
	decoder.offset += size
	return value, nil
}

// readString consumes a uint16 length followed by that many UTF-8 bytes.
func (decoder *betDecoder) readString(fieldName string) (string, error) {
	encodedLength, err := decoder.read(2, fieldName+" length")
	if err != nil {
		return "", err
	}

	length := int(binary.BigEndian.Uint16(encodedLength))
	encodedValue, err := decoder.read(length, fieldName)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(encodedValue) {
		return "", fmt.Errorf("%s is not valid UTF-8", fieldName)
	}
	return string(encodedValue), nil
}

// decodeBet consumes the next bet from a shared decoder. It deliberately does
// not require reaching the end because another bet may follow in the payload.
func decodeBet(decoder *betDecoder) (model.Bet, error) {
	firstName, err := decoder.readString("first name")
	if err != nil {
		return model.Bet{}, err
	}
	lastName, err := decoder.readString("last name")
	if err != nil {
		return model.Bet{}, err
	}

	encodedDocument, err := decoder.read(8, "document")
	if err != nil {
		return model.Bet{}, err
	}

	birthdate, err := decoder.readString("birthdate")
	if err != nil {
		return model.Bet{}, err
	}

	encodedNumber, err := decoder.read(4, "number")
	if err != nil {
		return model.Bet{}, err
	}

	return model.Bet{
		FirstName: firstName,
		LastName:  lastName,
		Document:  binary.BigEndian.Uint64(encodedDocument),
		Birthdate: birthdate,
		Number:    binary.BigEndian.Uint32(encodedNumber),
	}, nil
}

// EncodeBet serializes one bet payload. The agency is omitted because it belongs
// to the connection established by the AGENCY message.
func EncodeBet(bet model.Bet) ([]byte, error) {
	return appendBet(nil, bet)
}

// DecodeBet deserializes a payload that must contain exactly one complete bet.
func DecodeBet(payload []byte) (model.Bet, error) {
	decoder := betDecoder{data: payload}
	bet, err := decodeBet(&decoder)
	if err != nil {
		return model.Bet{}, err
	}
	if decoder.offset != len(payload) {
		return model.Bet{}, fmt.Errorf(
			"bet payload has %d trailing bytes",
			len(payload)-decoder.offset,
		)
	}
	return bet, nil
}
