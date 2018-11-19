package bt

import (
	"fmt"
	"bytes"
	"encoding/binary"
	"io"
)

const (
	// Protocol is "BitTorrent protocol"
	Protocol = "BitTorrent protocol"
)

// ReadMessage read a message from stream
func ReadMessage(stream Stream) ([]byte, error) {
	var length uint32

	err := binary.Read(stream, binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, length)
	_, err = io.ReadFull(stream, buf)

	return buf, err
}

// ReadMessageWithLimit read a message from stream
func ReadMessageWithLimit(stream Stream, limit uint32) ([]byte, error) {
	var length uint32

	err := binary.Read(stream, binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}

	if length > limit {
		return nil, fmt.Errorf("message length is[%d] too large", length)
	}

	buf := make([]byte, length)
	_, err = io.ReadFull(stream, buf)

	return buf, err
}

// ReadMessageInto will reset the buffer and read message into it
func ReadMessageInto(stream Stream, buf *bytes.Buffer) error {
	buf.Reset()

	var length uint32

	err := binary.Read(stream, binary.BigEndian, &length)
	if err != nil {
		return err
	}

	buf.Grow(int(length))
	_, err = io.CopyN(buf, stream, int64(length))

	return err
}

// WriteMessage will write a message into stream
func WriteMessage(stream Stream, id byte, payload []byte) error {
	var length uint32

	length = uint32(len(payload) + 1)
	err := binary.Write(stream, binary.BigEndian, length)
	if err != nil {
		return nil
	}

	_, err = stream.Write([]byte{id})
	if err != nil {
		return err
	}

	_, err = io.Copy(stream, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	return nil
}
