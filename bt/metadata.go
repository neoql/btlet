package bt

import (
	"strings"

	"github.com/neoql/btlet/bencode"
)

// RawMeta is raw metadata
type RawMeta bencode.RawMessage

// Meta is meta
type Meta struct {
	Name        string  `bencode:"name"`
	Length      *uint64 `bencode:"length,omitempty"`
	PieceLength uint    `bencode:"piece length"`
	Pieces      []byte  `bencode:"pieces"`

	Files []struct {
		Path   []string `bencode:"path"`
		Length uint64   `bencode:"length"`
	} `bencode:"files,omitempty"`
}

// MetaOutline is outline of metadata
type MetaOutline interface {
	SetName(string)
	AddFile(string, uint64)
}

// FillOutline can fill MetaOutline
func (rm RawMeta) FillOutline(mo MetaOutline) error {
	var meta Meta

	err := bencode.Unmarshal(rm, &meta)
	if err != nil {
		return err
	}

	mo.SetName(meta.Name)
	if meta.Files == nil {
		mo.AddFile(meta.Name, *meta.Length)
	}

	for _, f := range meta.Files {
		mo.AddFile(strings.Join(f.Path, "/"), f.Length)
	}

	return nil
}
