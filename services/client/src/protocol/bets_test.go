package protocol

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/model"
)

var referenceBet = model.Bet{
	FirstName: "Ana",
	LastName:  "López",
	Document:  30904465,
	Birthdate: "1999-03-17",
	Number:    7574,
}

var referenceBetPayload = []byte{0x00, 0x03, 0x41, 0x6E, 0x61, 0x00, 0x06, 0x4C, 0xC3, 0xB3, 0x70, 0x65, 0x7A,
	0x00, 0x00, 0x00, 0x00, 0x01, 0xD7, 0x90, 0x91, 0x00, 0x0A, 0x31, 0x39, 0x39, 0x39, 0x2D, 0x30, 0x33, 0x2D,
	0x31, 0x37, 0x00, 0x00, 0x1D, 0x96}

func TestEncodeBetUsesExpectedWireFormat(t *testing.T) {
	payload, err := EncodeBet(referenceBet)
	if err != nil {
		t.Fatalf("EncodeBet returned an error: %v", err)
	}
	if !bytes.Equal(payload, referenceBetPayload) {
		t.Fatalf("unexpected payload: got %x, want %x", payload, referenceBetPayload)
	}
}

func TestDecodeBetUsesExpectedWireFormat(t *testing.T) {
	bet, err := DecodeBet(referenceBetPayload)
	if err != nil {
		t.Fatalf("DecodeBet returned an error: %v", err)
	}
	if !reflect.DeepEqual(bet, referenceBet) {
		t.Fatalf("unexpected bet: got %+v, want %+v", bet, referenceBet)
	}
}

func TestBetsRoundTrip(t *testing.T) {
	want := []model.Bet{referenceBet, {
		FirstName: "Camila",
		LastName:  "Varela",
		Document:  37130775,
		Birthdate: "1995-05-09",
		Number:    1024,
	}}

	payload, err := EncodeBets(want)
	if err != nil {
		t.Fatalf("EncodeBets returned an error: %v", err)
	}
	got, err := DecodeBets(payload)
	if err != nil {
		t.Fatalf("DecodeBets returned an error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected bets: got %+v, want %+v", got, want)
	}
}

func TestDecodeBetRejectsTrailingBytes(t *testing.T) {
	payload := append(append([]byte{}, referenceBetPayload...), 0x00)

	_, err := DecodeBet(payload)
	if err == nil || !strings.Contains(err.Error(), "trailing bytes") {
		t.Fatalf("expected a trailing bytes error, got %v", err)
	}
}

func TestDecodeBetRejectsInvalidUTF8(t *testing.T) {
	payload := []byte{0x00, 0x01, 0xFF}

	_, err := DecodeBet(payload)
	if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("expected an invalid UTF-8 error, got %v", err)
	}
}

func TestEncodeBetsRejectsEmptyList(t *testing.T) {
	_, err := EncodeBets(nil)
	if err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("expected an empty bets error, got %v", err)
	}
}

func TestDecodeBetsRejectsCountThatDoesNotFitPayload(t *testing.T) {
	payload := []byte{0x00, 0x00, 0x00, 0x03}
	payload = append(payload, referenceBetPayload...)

	_, err := DecodeBets(payload)
	if err == nil || !strings.Contains(err.Error(), "does not fit") {
		t.Fatalf("expected an invalid count error, got %v", err)
	}
}
