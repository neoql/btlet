package bt

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"

	"github.com/neoql/btlet/bencode"
	"github.com/neoql/btlet/tools"
)

const (
	// Protocol is "BitTorrent protocol"
	Protocol = "BitTorrent protocol"
)

const (
	// extReserved is reserved field in handshake when peer wire
	extReserved = 0x100000
	// extID is extension message id.
	extID = 20
)

func makeHandshake(reserved uint64, infoHash, peerID string) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 68))

	buf.WriteByte(19)
	buf.WriteString(Protocol)
	binary.Write(buf, binary.BigEndian, reserved)
	buf.WriteString(infoHash)
	buf.WriteString(peerID)

	return buf.Bytes()
}

func mkExtMsg(id byte, data []byte) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, len(data)+2))
	buf.WriteByte(extID)
	buf.WriteByte(id)
	buf.Write(data)
	return buf.Bytes()
}

func mkExtHandshakeMsg(dict map[string]interface{}) []byte {
	data, _ := bencode.Marshal(dict)
	return mkExtMsg(0, data)
}

func parseHandshake(data []byte) (reserved uint64, infoHash, peerID string, err error) {
	if len(data) != 68 || data[0] != 19 || string(data[1:20]) != "BitTorrent protocol" {
		err = errors.New("invalid handshake response")
		return
	}

	reserved = binary.BigEndian.Uint64(data[20:28])
	infoHash = string(data[28:48])
	peerID = string(data[48:])
	return
}

func send(conn net.Conn, data []byte) error {
	conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	_, err := conn.Write(data)
	return err
}

func sendMsg(conn net.Conn, msg []byte) error {
	buf := bytes.NewBuffer(make([]byte, 0, len(msg)+4))
	binary.Write(buf, binary.BigEndian, uint32(len(msg)))
	buf.Write(msg)
	return send(conn, buf.Bytes())
}

func readMsg(r *tools.StreamReader) ([]byte, error) {
	len, err := r.ReadUInt32()
	if err != nil {
		return nil, err
	}

	if len == 0 {
		return make([]byte, 0), nil
	}

	return r.ReadBytes(int64(len))
}

// Message is message
type Message struct {
	R   io.Reader
	ID  byte
	Len uint32
}

type omitWriter struct{}

func (w omitWriter) Write(b []byte) (int, error) { return len(b), nil }

// MessageSender can send message
type MessageSender struct {
	w   io.Writer
	tmp [1]byte
}

// SendMessage sends Message
func (ms *MessageSender) SendMessage(msg *Message) error {
	err := binary.Write(ms.w, binary.BigEndian, msg.Len)
	if err != nil {
		return err
	}

	ms.tmp[0] = msg.ID
	_, err = ms.w.Write(ms.tmp[:])
	if err != nil {
		return err
	}

	_, err = io.CopyN(ms.w, msg.R, int64(msg.Len)-1)
	if err != nil {
		return err
	}

	return nil
}

// SendShortMessage can send a short message
func (ms *MessageSender) SendShortMessage(id byte, b []byte) error {
	return ms.SendMessage(&Message{R: bytes.NewReader(b), ID: id, Len: uint32(len(b) + 1)})
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

// Loop read message and prossess it with assign MessageHandler
func (s *Session) Loop(ctx context.Context, f MessageHandleFunc) error {
	var length uint32
	var tmp [1]byte

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:
		}

		err := binary.Read(s.r, binary.BigEndian, &length)
		if err != nil {
			return err
		}

		_, err = s.r.Read(tmp[:])
		if err != nil {
			return err
		}

		err = func() (err error) {
			defer func() {
				if e := recover(); e != nil {
					err = e.(error)
				}
			}()

			return f(tmp[0], io.LimitReader(s.r, int64(length-1)), s.sender)
		}()

		if err != nil {
			return err
		}
	}

	return nil
}
