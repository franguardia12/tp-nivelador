// Package betcsv converts between CSV records and lottery bets.
package betcsv

import (
	"fmt"
	"strconv"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/model"
)

// FieldCount is the number of columns in one lottery bet CSV record.
const FieldCount = 5

// Decode validates and converts one CSV record into a domain Bet.
func Decode(record []string) (model.Bet, error) {
	if len(record) != FieldCount {
		return model.Bet{}, fmt.Errorf("expected %d fields, got %d", FieldCount, len(record))
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

// Encode converts a domain Bet into the field order used by output files.
func Encode(bet model.Bet) []string {
	return []string{
		bet.FirstName,
		bet.LastName,
		strconv.FormatUint(bet.Document, 10),
		bet.Birthdate,
		strconv.FormatUint(uint64(bet.Number), 10),
	}
}
