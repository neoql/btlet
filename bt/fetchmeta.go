package bt

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"io"

	"github.com/neoql/btlet/bencode"
)

const (
	request = iota
	data
	reject
)

const (
	maxPieceSize = 16 * 1024
)

// FetchMetaExt is extension can fetch metadata
type FetchMetaExt struct {
	infoHash string
	pieces   [][]byte
	support  bool
}

// NewFetchMetaExt returns a new FetchMetaExt
func NewFetchMetaExt(opt HSOption) *FetchMetaExt {
	return &FetchMetaExt{
		infoHash: opt.InfoHash,
	}
}

// MapKey implements Extension.MapKey
func (fm *FetchMetaExt) MapKey() string {
	return "ut_metadata"
}

// IsSupoort returns true if peer support this extension other false.
// Should use it after extesion handshake
func (fm *FetchMetaExt) IsSupoort() bool {
	return fm.support
}

// BeforeHandshake implements Extension.BeforeHandshake
func (fm *FetchMetaExt) BeforeHandshake(hs ExtHSPutter) {}

// AfterHandshake implements Extension.AfterHandshake
func (fm *FetchMetaExt) AfterHandshake(hs ExtHSGetter, sender *ExtMsgSender) error {
	fm.support = true
	var size int64
	ok := hs.Get("metadata_size", &size)
	if !ok {
		return errors.New("don't known metadata size")
	}

	piecesNum := getPiecesNum(size)
	fm.pieces = make([][]byte, piecesNum)

	go func() {
		for i := 0; i < piecesNum; i++ {
			m := map[string]int{
				"msg_type": request,
				"piece":    i,
			}
	
			b, err := bencode.Marshal(m)
			if err != nil {
				return
			}
	
			err = sender.SendBytes(b)
			if err != nil {
				return
			}
		}
	}()

	return nil
}

// Unsupport implements Extension.Unsupport
func (fm *FetchMetaExt) Unsupport() {
	fm.support = false
}

// HandleMessage implements Extension.HandleMessage
func (fm *FetchMetaExt) HandleMessage(r io.Reader, sender *ExtMsgSender) error {
	var msg map[string]int

	buf := &bytes.Buffer{}

	_, err := io.Copy(buf, r)
	if err != nil {
		return err
	}

	raw := buf.Bytes()
	dec := bencode.NewDecoder(bytes.NewReader(raw))
	err = dec.Decode(&msg)
	if err != nil {
		return err
	}

	switch msg["msg_type"] {
	default:
	case reject:
		return errors.New("peer reject out request")
	case data:
		no := msg["piece"]

		fm.pieces[no] = raw[dec.BytesParsed():]
	}

	return nil
}

// CheckDone if download all pieces returns true else false
func (fm *FetchMetaExt) CheckDone() bool {
	for _, piece := range fm.pieces {
		if len(piece) == 0 {
			return false
		}
	}
	return true
}

// FetchRawMeta get the raw metadata
func (fm *FetchMetaExt) FetchRawMeta() (RawMeta, error) {
	metadata := bytes.Join(fm.pieces, nil)
	hash := sha1.Sum(metadata)
	if bytes.Equal(hash[:], []byte(fm.infoHash)) {
		return metadata, nil
	}

	return nil, errors.New("metadata's sha1 hash is different from info_hash")
}

func getPiecesNum(size int64) int {
	piecesNum := size / maxPieceSize
	if size%maxPieceSize != 0 {
		piecesNum++
	}

	return int(piecesNum)
}
