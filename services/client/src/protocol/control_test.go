package protocol

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestAgencyRoundTrip(t *testing.T) {
	const agencyID uint32 = 0x01020304

	got, err := DecodeAgency(EncodeAgency(agencyID))
	if err != nil {
		t.Fatalf("DecodeAgency returned an error: %v", err)
	}
	if got != agencyID {
		t.Fatalf("unexpected agency ID: got %d, want %d", got, agencyID)
	}
}

func TestAckUsesExpectedWireFormat(t *testing.T) {
	ack := Ack{AcknowledgedType: MessageTypeBets, ProcessedCount: 1}
	want := []byte{byte(MessageTypeBets), 0x00, 0x00, 0x00, 0x01}

	payload, err := EncodeAck(ack)
	if err != nil {
		t.Fatalf("EncodeAck returned an error: %v", err)
	}
	if !bytes.Equal(payload, want) {
		t.Fatalf("unexpected payload: got %x, want %x", payload, want)
	}
	got, err := DecodeAck(payload)
	if err != nil {
		t.Fatalf("DecodeAck returned an error: %v", err)
	}
	if got != ack {
		t.Fatalf("unexpected ack: got %+v, want %+v", got, ack)
	}
}

func TestWinnersEndRoundTrip(t *testing.T) {
	const winnerCount uint32 = 42

	got, err := DecodeWinnersEnd(EncodeWinnersEnd(winnerCount))
	if err != nil {
		t.Fatalf("DecodeWinnersEnd returned an error: %v", err)
	}
	if got != winnerCount {
		t.Fatalf("unexpected winner count: got %d, want %d", got, winnerCount)
	}
}

func TestErrorRoundTrip(t *testing.T) {
	want := ErrorPayload{
		FailedType: MessageTypeBets,
		Code:       ErrorCodeInvalidData,
		Detail:     "apuesta inválida",
	}

	payload, err := EncodeError(want)
	if err != nil {
		t.Fatalf("EncodeError returned an error: %v", err)
	}
	got, err := DecodeError(payload)
	if err != nil {
		t.Fatalf("DecodeError returned an error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected error: got %+v, want %+v", got, want)
	}
}

func TestDecodeAckRejectsUnexpectedAcknowledgedType(t *testing.T) {
	payload := []byte{byte(MessageTypeWinner), 0x00, 0x00, 0x00, 0x00}

	_, err := DecodeAck(payload)
	if err == nil || !strings.Contains(err.Error(), "cannot be acknowledged") {
		t.Fatalf("expected an invalid acknowledged type error, got %v", err)
	}
}

func TestDecodeErrorRejectsInvalidDetailLength(t *testing.T) {
	payload := []byte{
		byte(MessageTypeBets),
		0x00, byte(ErrorCodeInvalidData),
		0x00, 0x02,
		0x61,
	}

	_, err := DecodeError(payload)
	if err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("expected an invalid detail length error, got %v", err)
	}
}
