package bt

import (
	"encoding/binary"
	"errors"
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
	HandleMessage(content []byte, sender *ExtMsgSender) error
}

// ExtProtocol is extension center
type ExtProtocol struct {
	exts   []Extension
	m      map[string]byte
	sender *ExtMsgSender
}

// NewExtProtocol returns a new extension center
func NewExtProtocol(exts ...Extension) *ExtProtocol {
	return &ExtProtocol{
		exts: exts,
		m:    make(map[string]byte),
	}
}

// RegistExt regists extension into center
func (proto *ExtProtocol) RegistExt(ext Extension) {
	proto.exts = append(proto.exts, ext)
}

// WriteHandshake sends handshake
func (proto *ExtProtocol) WriteHandshake(w io.Writer) error {
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
	return (&ExtMsgSender{0, w}).SendBytes(b)
}

// HandlePayload handle extention message
func (proto *ExtProtocol) HandlePayload(payload []byte, w io.Writer) error {
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

			err = ext.AfterHandshake(getter, &ExtMsgSender{id, w})
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

		return ext.HandleMessage(payload[1:], &ExtMsgSender{proto.m[ext.MapKey()], w})
	}

	return nil
}

// ExtMsgSender used for send extension message
type ExtMsgSender struct {
	id byte
	w  io.Writer
}

// SendBytes will send content
func (sender *ExtMsgSender) SendBytes(content []byte) error {
	return WriteExtensionMessage(sender.w, sender.id, content)
}

// WriteExtensionMessage write extension message to writer
func WriteExtensionMessage(writer io.Writer, id byte, content []byte) error {
	var length uint32

	length = uint32(len(content) + 2)
	err := binary.Write(writer, binary.BigEndian, length)
	if err != nil {
		return nil
	}

	if w, ok := writer.(io.ByteWriter); ok {
		err = w.WriteByte(ExtID)
		if err != nil {
			return err
		}
		err = w.WriteByte(id)
		if err != nil {
			return err
		}
	} else {
		_, err = writer.Write([]byte{ExtID, id})
		if err != nil {
			return err
		}
	}

	_, err = writer.Write(content)
	if err != nil {
		return err
	}

	return nil
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
