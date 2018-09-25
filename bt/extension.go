package bt

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/neoql/btlet/bencode"
)

const (
	// ExtID is extension message id.
	ExtID byte = 20
	// ExtReserved is reserved field in handshake when peer wire
	ExtReserved = 0x100000
)

// SetExtReserved set extesntion reserved on handshake
func SetExtReserved(reserved *uint64) {
	*reserved = *reserved | ExtReserved
}

// CheckExtReserved returns true if reserved support extension
func CheckExtReserved(reserved uint64) bool {
	return (reserved & ExtReserved) != 0
}

// Extension is extension
type Extension interface {
	MapKey() string
	BeforeHandshake(hs ExtHSPutter)
	// AfterHandshake will call after handsahke if peer support this extension
	AfterHandshake(hs ExtHSGetter, sender *ExtMsgSender) error
	// Unsupport will call after handshake if peer not support this extension.
	Unsupport()
	HandleMessage(r io.Reader, sender *ExtMsgSender) error
}

// ExtSession is extension session
type ExtSession struct {
	exts []Extension
	m    map[string]byte
}

// NewExtSession returns a new extension session
func NewExtSession(exts []Extension) *ExtSession {
	return &ExtSession{
		exts: exts,
		m:    make(map[string]byte),
	}
}

// SendHS sends handshake
func (es *ExtSession) SendHS(sender *MessageSender) error {
	hs := make(map[string]interface{})
	m := make(map[string]int)
	putter := ExtHSPutter{hs}
	for i, ext := range es.exts {
		ext.BeforeHandshake(putter)
		m[ext.MapKey()] = i + 1
	}
	hs["m"] = m
	b, err := bencode.Marshal(hs)
	if err != nil {
		return err
	}
	return (&ExtMsgSender{0, sender}).SendBytes(b)
}

// HandleMessage handle extention message
func (es *ExtSession) HandleMessage(r io.Reader, sender *MessageSender) error {
	var tmp [1]byte

	_, err := r.Read(tmp[:])
	if err != nil {
		return err
	}

	id := tmp[0]

	if id == 0 {
		var msg map[string]bencode.RawMessage

		dec := bencode.NewDecoder(r)
		err := dec.Decode(&msg)
		if err != nil {
			return err
		}

		rawm, ok := msg["m"]
		if !ok {
			return errors.New("invalid extension handshake")
		}

		err = bencode.Unmarshal(rawm, &es.m)
		if err != nil {
			return err
		}

		hs := ExtHSGetter{msg}
		for i, ext := range es.exts {
			id, ok := es.m[ext.MapKey()]
			if !ok || id == 0 {
				ext.Unsupport()
				es.exts[i] = nil
			}
			err = ext.AfterHandshake(hs, &ExtMsgSender{es.m[ext.MapKey()], sender})
			if err != nil {
				return err
			}
		}

		return nil
	}

	if len(es.m) == 0 {
		return errors.New("have not handshake")
	}

	if int(id) > len(es.exts) {
		buf := &bytes.Buffer{}
		_, err := io.Copy(buf, r)
		if err != nil {
			return err
		}
		var msg map[string]interface{}
		err = bencode.Unmarshal(buf.Bytes(), &msg)
		if err != nil {
			return err
		}
		fmt.Println(msg)

		return fmt.Errorf("unkown extension message id:%d", id)
	}

	ext := es.exts[id-1]
	if ext == nil {
		return errors.New("unsupport extension message id")
	}

	return ext.HandleMessage(r, &ExtMsgSender{es.m[ext.MapKey()], sender})
}

// ExtMsgSender used for send extension message
type ExtMsgSender struct {
	id byte
	s  *MessageSender
}

// Send will copy r to w, read n bytes from r
func (sender *ExtMsgSender) Send(n uint32, r io.Reader) error {
	msg := &Message{
		Len: n + 2, // ExtID 1 byte and sender.id 1 byte
		ID:  ExtID,
		R:   io.MultiReader(bytes.NewReader([]byte{sender.id}), r),
	}
	return sender.s.SendMessage(msg)
}

// SendBytes will send b
func (sender *ExtMsgSender) SendBytes(b []byte) error {
	return sender.Send(uint32(len(b)), bytes.NewReader(b))
}

// ExtHSPutter only can put entry into handshake
type ExtHSPutter struct {
	m map[string]interface{}
}

// Put entry into handshake
func (p *ExtHSPutter) Put(k string, v interface{}) {
	p.m[k] = v
}

// ExtHSGetter can get entry from handshake.
type ExtHSGetter struct {
	m map[string]bencode.RawMessage
}

// Get entry from handshake
func (g *ExtHSGetter) Get(k string, ptr interface{}) bool {
	b, ok := g.m[k]
	if ok == false {
		return ok
	}

	err := bencode.Unmarshal(b, ptr)
	if err != nil {
		return false
	}

	return true
}
