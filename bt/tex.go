package bt

import (
	"crypto/sha1"
	"fmt"
	"strings"

	"github.com/neoql/btlet/bencode"
)

type texMessage struct {
	Added []string `bencode:"added"`
}

// TexExtension is tracker exchange extension
type TexExtension struct {
	trlist  []string
	support bool
	more    bool
}

// NewTexExtension returns a new TexExtension
func NewTexExtension(trlist []string) *TexExtension {
	return &TexExtension{
		trlist: trlist,
	}
}

// MapKey implements Extension.MapKey
func (tex *TexExtension) MapKey() string {
	return "lt_tex"
}

// BeforeHandshake implements Extension.BeforeHandshake
func (tex *TexExtension) BeforeHandshake(hs ExtHSPutter) {
	hash := sha1.Sum([]byte(strings.Join(tex.trlist, "")))
	hs.Put("tr", hash[:])
}

// AfterHandshake will call after handsahke if peer support this extension
func (tex *TexExtension) AfterHandshake(hs ExtHSGetter, sender *ExtMsgSender) error {
	tex.support = true

	var tr []byte
	ok := hs.Get("tr", &tr)
	if !ok || isEmpty(tr) {
		tex.more = false
	} else {
		tex.more = true
	}
	return nil
}

// Unsupport will call after handshake if peer not support this extension.
func (tex *TexExtension) Unsupport() {
	tex.support = false
	tex.more = false
}

// IsSupoort returns true if peer support this extension other false.
// Should use it after extesion handshake
func (tex *TexExtension) IsSupoort() bool {
	return tex.support
}

// HandleMessage implements Extension.HandleMessage
func (tex *TexExtension) HandleMessage(content []byte, sender *ExtMsgSender) error {
	var msg texMessage

	err := bencode.Unmarshal(content, &msg)
	if err != nil {
		return err
	}

	tex.more = false
	tex.trlist = msg.Added

	return nil
}

// More returns peer have more tracker or not
func (tex *TexExtension) More() bool {
	return tex.more
}

// TrackerList returns tracker list
func (tex *TexExtension) TrackerList() []string {
	return tex.trlist
}

func trhash(trlist []string) []byte {
	hash := sha1.Sum([]byte(strings.Join(trlist, "")))
	return hash[:]
}

func isEmpty(tr []byte) bool {
	return fmt.Sprintf("%x", tr) == "da39a3ee5e6b4b0d3255bfef95601890afd80709"
}
