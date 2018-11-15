package bt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"
)

// BasicStream is a common stream.
type BasicStream interface {
	io.Writer
	io.Reader
	io.Closer
	Protocol() string
}

// Stream is a BitProtocol stream
type Stream interface {
	BasicStream
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

	basic, err := NewBasicStream(conn, Protocol)
	if err != nil {
		conn.Close()
		return nil, err
	}

	if basic.Protocol() != Protocol {
		conn.Close()
		return nil, errors.New("unknown protocol")
	}

	stream := &implStream{BasicStream: basic}
	err = stream.handshake(peerID, infohash, reserved)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return stream, nil
}

type basicStream struct {
	conn     net.Conn
	protocol string
}

// NewBasicStream returns a BasicStream
func NewBasicStream(conn net.Conn, protocol string) (BasicStream, error) {
	stream := &basicStream{conn: conn}

	buf := make([]byte, 1, len(protocol)+1)
	buf[0] = byte(len(protocol))
	buf = append(buf, []byte(protocol)...)

	_, err := io.Copy(stream, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}

	var length uint32

	err = binary.Read(stream, binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}

	buf = make([]byte, length)
	_, err = io.ReadFull(stream, buf)
	if err != nil {
		return nil, err
	}
	stream.protocol = string(buf)
	return stream, nil
}

func (stream *basicStream) Write(p []byte) (n int, err error) {
	err = stream.conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	if err != nil {
		return
	}

	return stream.conn.Write(p)
}

func (stream *basicStream) Read(p []byte) (n int, err error) {
	err = stream.conn.SetReadDeadline(time.Now().Add(time.Second * 10))
	if err != nil {
		return
	}

	return stream.conn.Read(p)
}

func (stream *basicStream) Close() error {
	return stream.conn.Close()
}

func (stream *basicStream) Protocol() string {
	return stream.protocol
}

type implStream struct {
	BasicStream
	reserved uint64
	infoHash string
	peerID   string
}

func (stream *implStream) handshake(peerID, infohash string, reserved uint64) error {
	err := binary.Write(stream, binary.BigEndian, reserved)
	if err != nil {
		return err
	}

	buf := make([]byte, 0, 40)
	buf = append(buf, []byte(infohash)...)
	buf = append(buf, []byte(peerID)...)

	_, err = io.Copy(stream, bytes.NewReader(buf))
	if err != nil {
		return err
	}

	err = binary.Read(stream, binary.BigEndian, &stream.reserved)
	if err != nil {
		return err
	}

	_, err = io.ReadFull(stream, buf)
	stream.infoHash = string(buf[0:20])
	stream.peerID = string(buf[20:40])

	return nil
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
