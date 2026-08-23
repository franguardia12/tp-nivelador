package protocol

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/model"
)

const maxStringSize = 1<<16 - 1
const minimumEncodedBetSize = 18

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
	if len(payload) > MaxPayloadSize {
		return model.Bet{}, fmt.Errorf(
			"payload length %d exceeds maximum %d",
			len(payload),
			MaxPayloadSize,
		)
	}

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

// EncodeBets serializes a non-empty list as a uint32 count followed by each bet.
func EncodeBets(bets []model.Bet) ([]byte, error) {
	if len(bets) == 0 {
		return nil, fmt.Errorf("bets payload cannot be empty")
	}
	if uint64(len(bets)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("bet count %d exceeds uint32", len(bets))
	}

	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(len(bets)))
	for index, bet := range bets {
		encodedBet, err := EncodeBet(bet)
		if err != nil {
			return nil, fmt.Errorf("encode bet %d: %w", index, err)
		}
		if len(payload)+len(encodedBet) > MaxPayloadSize {
			return nil, fmt.Errorf(
				"payload length exceeds maximum %d while encoding bet %d",
				MaxPayloadSize,
				index,
			)
		}
		payload = append(payload, encodedBet...)
	}
	return payload, nil
}

// DecodeBets deserializes exactly the declared number of bets and rejects any
// truncated records or bytes left after the final record.
func DecodeBets(payload []byte) ([]model.Bet, error) {
	if len(payload) > MaxPayloadSize {
		return nil, fmt.Errorf(
			"payload length %d exceeds maximum %d",
			len(payload),
			MaxPayloadSize,
		)
	}
	if len(payload) < 4 {
		return nil, fmt.Errorf("incomplete bet count")
	}

	count := binary.BigEndian.Uint32(payload[:4])
	if count == 0 {
		return nil, fmt.Errorf("bets payload cannot be empty")
	}
	remainingSize := len(payload) - 4
	if uint64(count)*minimumEncodedBetSize > uint64(remainingSize) {
		return nil, fmt.Errorf(
			"bet count %d does not fit in payload of %d bytes",
			count,
			remainingSize,
		)
	}

	decoder := betDecoder{data: payload, offset: 4}
	bets := make([]model.Bet, 0, int(count))
	for index := uint32(0); index < count; index++ {
		bet, err := decodeBet(&decoder)
		if err != nil {
			return nil, fmt.Errorf("decode bet %d: %w", index, err)
		}
		bets = append(bets, bet)
	}
	if decoder.offset != len(payload) {
		return nil, fmt.Errorf(
			"bets payload has %d trailing bytes",
			len(payload)-decoder.offset,
		)
	}
	return bets, nil
}
