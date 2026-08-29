package safe_socket

import (
	"fmt"
	"io"
)

// SendAll keeps writing the remaining suffix until every byte is transferred.
// Writer errors are returned immediately; successful zero-byte writes are retried.
func SendAll(socket io.Writer, data []byte) error {
	totalSent := 0

	for totalSent < len(data) {
		sent, err := socket.Write(data[totalSent:])
		totalSent += sent
		// Retry only successful short writes. An arbitrary error may be
		// permanent, so retrying it could hide the failure or loop forever.
		if err != nil {
			return err
		}

	}

	return nil
}

// RecvAll reads exactly size bytes, preserving bytes returned together with an
// error and reporting an early EOF as io.ErrUnexpectedEOF.
func RecvAll(socket io.Reader, size int) ([]byte, error) {
	if size < 0 {
		return nil, fmt.Errorf("invalid read size %d", size)
	}

	buff := make([]byte, size)
	totalReceived := 0

	for totalReceived < size {
		received, err := socket.Read(buff[totalReceived:])
		totalReceived += received
		if totalReceived == size {
			return buff, nil
		}
		// Keep bytes returned together with an error, but propagate that error
		// if the requested message is still incomplete. Its cause may be permanent.
		if err != nil {
			if err == io.EOF {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}

	}

	return buff, nil
}
