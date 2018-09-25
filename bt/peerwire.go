package bt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"

	"github.com/neoql/btlet/tools"
)

const (
	// Protocol is "BitTorrent protocol"
	Protocol = "BitTorrent protocol"
)

type omitWriter struct{}

func (w omitWriter) Write(b []byte) (int, error) { return len(b), nil }

// MessageSender can send message
type MessageSender struct {
	w io.Writer
}

// SendBytes can send a short message
func (ms *MessageSender) SendBytes(id byte, payload []byte) error {
	var length uint32

	length = uint32(len(payload) + 1)
	err := binary.Write(ms.w, binary.BigEndian, length)
	if err != nil {
		return nil
	}

	_, err = ms.w.Write([]byte{id})
	if err != nil {
		return err
	}

	_, err = io.Copy(ms.w, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	return nil
}

// HSOption is handshake option
type HSOption struct {
	Reserved uint64
	InfoHash string
	PeerID   string
}

// MessageHandleFunc used for handle message
type MessageHandleFunc func(id byte, r io.Reader, sender *MessageSender) error

// Session is peer wire session
type Session struct {
	w      io.Writer
	r      io.Reader
	sender *MessageSender
}

// NewSession returns a new Session
func NewSession(conn net.Conn) *Session {
	return &Session{
		w:      tools.NewStreamWriter(conn, time.Second*10),
		r:      tools.NewStreamReader(conn, time.Second*10),
		sender: &MessageSender{w: conn},
	}
}

// MessageSender always return same MessageSender
func (s *Session) MessageSender() *MessageSender {
	return s.sender
}

// Handshake send handshake and recieve handshake.
func (s *Session) Handshake(opt *HSOption) (*HSOption, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 68))

	buf.WriteByte(19)
	buf.WriteString(Protocol)
	binary.Write(buf, binary.BigEndian, opt.Reserved)
	buf.WriteString(opt.InfoHash)
	buf.WriteString(opt.PeerID)

	_, err := io.CopyN(s.w, buf, 68)
	if err != nil {
		return nil, err
	}

	buf.Reset()
	_, err = io.CopyN(buf, s.r, 68)
	if err != nil {
		return nil, err
	}

	b := buf.Bytes()

	if b[0] != 19 || string(b[1:20]) != Protocol {
		return nil, errors.New("invalid handshake response")
	}

	opt = &HSOption{}
	opt.Reserved = binary.BigEndian.Uint64(b[20:28])
	opt.InfoHash = string(b[28:48])
	opt.PeerID = string(b[48:])

	return opt, nil
}

// NextMessage read next message
func (s *Session) NextMessage() ([]byte, error) {
	var length uint32
	err := binary.Read(s.r, binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(make([]byte, 0, length))

	_, err = io.CopyN(buf, s.r, int64(length))
	return buf.Bytes(), err
}
