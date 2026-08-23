package protocol

import (
	"bytes"
	"strings"
	"testing"
)

var referenceFrame = []byte{
	byte(MessageTypeBets),
	0x00, 0x00, 0x00, 0x03,
	0x61, 0x62, 0x63,
}

func TestSendMessageUsesExpectedWireFormat(t *testing.T) {
	var output bytes.Buffer

	if err := SendMessage(&output, MessageTypeBets, []byte("abc")); err != nil {
		t.Fatalf("SendMessage returned an error: %v", err)
	}
	if !bytes.Equal(output.Bytes(), referenceFrame) {
		t.Fatalf("unexpected frame: got %x, want %x", output.Bytes(), referenceFrame)
	}
}

func TestReceiveMessageUsesExpectedWireFormat(t *testing.T) {
	message, err := ReceiveMessage(bytes.NewReader(referenceFrame))
	if err != nil {
		t.Fatalf("ReceiveMessage returned an error: %v", err)
	}
	if message.Type != MessageTypeBets {
		t.Fatalf("unexpected type: got %x, want %x", message.Type, MessageTypeBets)
	}
	if !bytes.Equal(message.Payload, []byte("abc")) {
		t.Fatalf("unexpected payload: got %x, want %x", message.Payload, []byte("abc"))
	}
}

func TestReceiveMessageRejectsUnknownType(t *testing.T) {
	frame := []byte{0x7F, 0x00, 0x00, 0x00, 0x00}

	_, err := ReceiveMessage(bytes.NewReader(frame))
	if err == nil || !strings.Contains(err.Error(), "unknown message type") {
		t.Fatalf("expected an unknown message type error, got %v", err)
	}
}

func TestReceiveMessageRejectsOversizedPayload(t *testing.T) {
	// 0x01000001 is one byte larger than the 16 MiB limit.
	header := []byte{byte(MessageTypeBets), 0x01, 0x00, 0x00, 0x01}

	_, err := ReceiveMessage(bytes.NewReader(header))
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("expected an oversized payload error, got %v", err)
	}
}
