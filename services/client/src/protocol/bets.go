package protocol

import (
	"encoding/binary"
	"fmt"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/model"
)

// BetsCountSize is the byte size of the record count prefix in a BETS payload.
const BetsCountSize = 4

// EncodeBets serializes a non-empty list as a uint32 count followed by each bet.
func EncodeBets(bets []model.Bet) ([]byte, error) {
	if len(bets) == 0 {
		return nil, fmt.Errorf("bets payload cannot be empty")
	}
	if uint64(len(bets)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("bet count %d exceeds uint32", len(bets))
	}

	payload := make([]byte, BetsCountSize)
	binary.BigEndian.PutUint32(payload, uint32(len(bets)))
	for index, bet := range bets {
		encodedBet, err := EncodeBet(bet)
		if err != nil {
			return nil, fmt.Errorf("encode bet %d: %w", index, err)
		}
		payload = append(payload, encodedBet...)
	}
	return payload, nil
}
