package bt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// Protocol is "BitTorrent protocol"
	Protocol = "BitTorrent protocol"
)

// ReadMessage read a message from stream
func ReadMessage(reader io.Reader) ([]byte, error) {
	var length uint32

	err := binary.Read(reader, binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, length)
	_, err = io.ReadFull(reader, buf)

	return buf, err
}

// ReadMessageWithLimit read a message from stream
func ReadMessageWithLimit(reader io.Reader, limit uint32) ([]byte, error) {
	var length uint32

	err := binary.Read(reader, binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}

	if length > limit {
		return nil, fmt.Errorf("message length is[%d] too large", length)
	}

	buf := make([]byte, length)
	_, err = io.ReadFull(reader, buf)

	return buf, err
}

// ReadMessageInto will reset the buffer and read message into it
func ReadMessageInto(reader io.Reader, buf *bytes.Buffer) error {
	buf.Reset()

	var length uint32

	err := binary.Read(reader, binary.BigEndian, &length)
	if err != nil {
		return err
	}

	buf.Grow(int(length))
	_, err = io.CopyN(buf, reader, int64(length))

	return err
}

// WriteMessage will write a message into stream
func WriteMessage(writer io.Writer, id byte, payload []byte) error {
	var length uint32

	length = uint32(len(payload) + 1)
	err := binary.Write(writer, binary.BigEndian, length)
	if err != nil {
		return nil
	}

	if w, ok := writer.(io.ByteWriter); ok {
		err = w.WriteByte(id)
		if err != nil {
			return err
		}
	} else {
		_, err = writer.Write([]byte{id})
		if err != nil {
			return err
		}
	}

	_, err = writer.Write(payload)
	if err != nil {
		return err
	}

	return nil
}
