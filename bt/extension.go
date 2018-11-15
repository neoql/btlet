package bt

import (
	"bytes"
	"errors"

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
	HandleMessage(content []byte, sender *ExtMsgSender) error
}

// ExtProtocol is extension center
type ExtProtocol struct {
	stream Stream
	exts   []Extension
	m      map[string]byte
	sender *ExtMsgSender
}

// NewExtProtocol returns a new extension center
func NewExtProtocol(stream Stream, exts ...Extension) *ExtProtocol {
	return &ExtProtocol{
		stream: stream,
		exts:   exts,
		m:      make(map[string]byte),
	}
}

// RegistExt regists extension into center
func (proto *ExtProtocol) RegistExt(ext Extension) {
	proto.exts = append(proto.exts, ext)
}

func (proto *ExtProtocol) getMsgSender(id byte) *ExtMsgSender {
	if proto.sender == nil {
		proto.sender = &ExtMsgSender{stream: proto.stream}
	}
	proto.sender.id = id
	return proto.sender
}

// WriteHandshake sends handshake
func (proto *ExtProtocol) WriteHandshake() error {
	hs := make(map[string]interface{})
	m := make(map[string]int)
	putter := ExtHSPutter{hs}
	for i, ext := range proto.exts {
		ext.BeforeHandshake(putter)
		m[ext.MapKey()] = i + 1
	}
	hs["m"] = m
	b, err := bencode.Marshal(hs)
	if err != nil {
		return err
	}
	return proto.getMsgSender(0).SendBytes(b)
}

// HandlePayload handle extention message
func (proto *ExtProtocol) HandlePayload(payload []byte) error {
	if payload[0] == 0 {
		// handshake
		var hs map[string]bencode.RawMessage

		err := bencode.Unmarshal(payload[1:], &hs)
		if err != nil {
			return err
		}

		rawm, ok := hs["m"]
		if !ok {
			return errors.New("invalid handshake")
		}

		err = bencode.Unmarshal(rawm, &proto.m)
		if err != nil {
			return err
		}

		getter := ExtHSGetter{hs}
		for i, ext := range proto.exts {
			id, ok := proto.m[ext.MapKey()]
			if !ok || id == 0 {
				ext.Unsupport()
				proto.exts[i] = nil
				continue
			}

			err = ext.AfterHandshake(getter, proto.getMsgSender(id))
			if err != nil {
				return err
			}
		}
	} else {
		if len(proto.m) == 0 {
			return errors.New("have not handshake")
		}
		id := payload[0] - 1
		if int(id) >= len(proto.exts) || proto.exts[id] == nil {
			return errors.New("unknown this extension id")
		}

		ext := proto.exts[id]

		return ext.HandleMessage(payload[1:], proto.getMsgSender(proto.m[ext.MapKey()]))
	}

	return nil
}

// ExtMsgSender used for send extension message
type ExtMsgSender struct {
	id     byte
	stream Stream
}

// SendBytes will send content
func (sender *ExtMsgSender) SendBytes(content []byte) error {
	return WriteMessage(sender.stream, ExtID, bytes.Join([][]byte{[]byte{sender.id}, content}, nil))
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
