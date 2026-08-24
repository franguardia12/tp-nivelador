package client

import (
	"bytes"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/model"
	"github.com/7574-sistemas-distribuidos/tp-nivelador/src/protocol"
)

type scriptedConnection struct {
	incoming *bytes.Reader
	outgoing bytes.Buffer
}

func (connection *scriptedConnection) Read(payload []byte) (int, error) {
	return connection.incoming.Read(payload)
}

func (connection *scriptedConnection) Write(payload []byte) (int, error) {
	return connection.outgoing.Write(payload)
}

func (connection *scriptedConnection) Close() error {
	return nil
}

func (connection *scriptedConnection) LocalAddr() net.Addr {
	return nil
}

func (connection *scriptedConnection) RemoteAddr() net.Addr {
	return nil
}

func (connection *scriptedConnection) SetDeadline(time.Time) error {
	return nil
}

func (connection *scriptedConnection) SetReadDeadline(time.Time) error {
	return nil
}

func (connection *scriptedConnection) SetWriteDeadline(time.Time) error {
	return nil
}

func appendServerMessage(
	t *testing.T,
	responses *bytes.Buffer,
	messageType protocol.MessageType,
	payload []byte,
) {
	t.Helper()
	if err := protocol.SendMessage(responses, messageType, payload); err != nil {
		t.Fatalf("append server message: %v", err)
	}
}

func TestClientProtocolFlow(t *testing.T) {
	const agencyID uint32 = 7
	winner := model.Bet{
		FirstName: "Ana",
		LastName:  "López",
		Document:  30904465,
		Birthdate: "1999-03-17",
		Number:    7574,
	}

	var responses bytes.Buffer
	agencyAck, err := protocol.EncodeAck(protocol.Ack{
		AcknowledgedType: protocol.MessageTypeAgency,
		ProcessedCount:   0,
	})
	if err != nil {
		t.Fatalf("encode agency ACK: %v", err)
	}
	appendServerMessage(t, &responses, protocol.MessageTypeAck, agencyAck)

	betAck, err := protocol.EncodeAck(protocol.Ack{
		AcknowledgedType: protocol.MessageTypeBets,
		ProcessedCount:   1,
	})
	if err != nil {
		t.Fatalf("encode bet ACK: %v", err)
	}
	appendServerMessage(t, &responses, protocol.MessageTypeAck, betAck)

	winnerPayload, err := protocol.EncodeBet(winner)
	if err != nil {
		t.Fatalf("encode winner: %v", err)
	}
	appendServerMessage(t, &responses, protocol.MessageTypeWinner, winnerPayload)
	appendServerMessage(
		t,
		&responses,
		protocol.MessageTypeWinnersEnd,
		protocol.EncodeWinnersEnd(1),
	)

	connection := &scriptedConnection{incoming: bytes.NewReader(responses.Bytes())}
	client := Client{
		conn: connection,
		config: ClientConfig{
			AgencyID: agencyID,
		},
	}

	if err := client.registerAgency(); err != nil {
		t.Fatalf("registerAgency returned an error: %v", err)
	}
	input := strings.NewReader("Ana,López,30904465,1999-03-17,7574\n")
	processed, err := client.sendInput(input)
	if err != nil {
		t.Fatalf("sendInput returned an error: %v", err)
	}
	if processed != 1 {
		t.Fatalf("unexpected processed count: got %d, want 1", processed)
	}
	if err := client.finishSendingBets(); err != nil {
		t.Fatalf("finishSendingBets returned an error: %v", err)
	}

	var output bytes.Buffer
	winnerCount, err := client.receiveWinners(&output)
	if err != nil {
		t.Fatalf("receiveWinners returned an error: %v", err)
	}
	if winnerCount != 1 {
		t.Fatalf("unexpected winner count: got %d, want 1", winnerCount)
	}
	if output.String() != "Ana,López,30904465,1999-03-17,7574\n" {
		t.Fatalf("unexpected output: %q", output.String())
	}

	requests := bytes.NewReader(connection.outgoing.Bytes())
	agencyMessage, err := protocol.ReceiveMessage(requests)
	if err != nil {
		t.Fatalf("receive agency message: %v", err)
	}
	gotAgencyID, err := protocol.DecodeAgency(agencyMessage.Payload)
	if err != nil {
		t.Fatalf("decode agency message: %v", err)
	}
	if agencyMessage.Type != protocol.MessageTypeAgency || gotAgencyID != agencyID {
		t.Fatalf("unexpected agency message: %+v, agency ID %d", agencyMessage, gotAgencyID)
	}

	betsMessage, err := protocol.ReceiveMessage(requests)
	if err != nil {
		t.Fatalf("receive bets message: %v", err)
	}
	bets, err := protocol.DecodeBets(betsMessage.Payload)
	if err != nil {
		t.Fatalf("decode bets message: %v", err)
	}
	if betsMessage.Type != protocol.MessageTypeBets || !reflect.DeepEqual(bets, []model.Bet{winner}) {
		t.Fatalf("unexpected bets message: %+v, bets %+v", betsMessage, bets)
	}

	endMessage, err := protocol.ReceiveMessage(requests)
	if err != nil {
		t.Fatalf("receive end message: %v", err)
	}
	if endMessage.Type != protocol.MessageTypeEndBets || len(endMessage.Payload) != 0 {
		t.Fatalf("unexpected end message: %+v", endMessage)
	}
	if requests.Len() != 0 {
		t.Fatalf("client sent %d unexpected trailing bytes", requests.Len())
	}
}
