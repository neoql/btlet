package bt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"
)

// Stream is a BitProtocol stream
type Stream interface {
	io.Writer
	io.Reader
	io.Closer
	Protocol() string
	Reserved() uint64
	InfoHash() string
	PeerID() string
}

// DialUseTCP create a stream use tcp
func DialUseTCP(host string, infohash, peerID string, reserved uint64) (Stream, error) {
	conn, err := net.DialTimeout("tcp", host, time.Second*15)
	if err != nil {
		return nil, err
	}

	stream := &implStream{conn: conn}
	err = stream.handshake(infohash, peerID, reserved)
	if err != nil {
		conn.Close()
		return nil, err
	}

	if stream.Protocol() != Protocol {
		conn.Close()
		return nil, errors.New("unknown prorocol")
	}

	return stream, nil
}

type implStream struct {
	conn net.Conn
	protocol string
	reserved uint64
	infoHash string
	peerID   string
}

func (stream *implStream) handshake(infohash, peerID string, reserved uint64) error {
	buf := bytes.NewBuffer(make([]byte, 0, 68))

	buf.WriteByte(19)
	buf.WriteString(Protocol)
	binary.Write(buf, binary.BigEndian, reserved)
	buf.WriteString(infohash)
	buf.WriteString(peerID)

	_, err := io.CopyN(stream, buf, 68)
	if err != nil {
		return err
	}

	buf.Reset()
	_, err = io.CopyN(buf, stream, 68)
	if err != nil {
		return err
	}

	b := buf.Bytes()

	stream.protocol = string(b[1:20])
	stream.reserved = binary.BigEndian.Uint64(b[20:28])
	stream.infoHash = string(b[28:48])
	stream.peerID = string(b[48:])

	return nil
}

func (stream *implStream) Write(p []byte) (n int, err error) {
	err = stream.conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	if err != nil {
		return
	}

	return stream.conn.Write(p)
}

func (stream *implStream) Read(p []byte) (n int, err error) {
	err = stream.conn.SetReadDeadline(time.Now().Add(time.Second * 10))
	if err != nil {
		return
	}

	return stream.conn.Read(p)
}

func (stream *implStream) Close() error {
	return stream.conn.Close()
}

func (stream *implStream) Protocol() string {
	return stream.protocol
}

func (stream *implStream) Reserved() uint64 {
	return stream.reserved
}

func (stream *implStream) InfoHash() string {
	return stream.infoHash
}

func (stream *implStream) PeerID() string {
	return stream.peerID
}
