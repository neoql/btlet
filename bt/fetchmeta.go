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
	Ch       chan RawMeta
}

// NewFetchMetaExt returns a new FetchMetaExt
func NewFetchMetaExt(opt HSOption) *FetchMetaExt {
	return &FetchMetaExt{
		infoHash: opt.InfoHash,
	}
}

// GenFetchMetaExt Create a FetchMetaExt
func GenFetchMetaExt(opt HSOption) Extension {
	return NewFetchMetaExt(opt)
}

// MapKey implements Extension.MapKey
func (fm *FetchMetaExt) MapKey() string {
	return "ut_metadata"
}

// BeforeHandshake implements Extension.BeforeHandshake
func (fm *FetchMetaExt) BeforeHandshake(hs ExtHSPutter) {}

// AfterHandshake implements Extension.AfterHandshake
func (fm *FetchMetaExt) AfterHandshake(hs ExtHSGetter, sender *ExtMsgSender) error {
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
	fm.Ch <- nil
}

// HandleMessage implements Extension.HandleMessage
func (fm *FetchMetaExt) HandleMessage(r io.Reader, sender *ExtMsgSender) error {
	var msg map[string]int

	buf := bytes.NewBuffer(make([]byte, 0, r.(*io.LimitedReader).N))

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

		if checkPiecesDone(fm.pieces) {
			metadata := bytes.Join(fm.pieces, nil)
			if checkMetadata(metadata, fm.infoHash) {
				fm.Ch <- metadata
			} else {
				fm.Ch <- nil
			}
		}
	}

	return nil
}

func getPiecesNum(size int64) int {
	piecesNum := size / maxPieceSize
	if size%maxPieceSize != 0 {
		piecesNum++
	}

	return int(piecesNum)
}

func checkPiecesDone(pieces [][]byte) bool {
	for _, piece := range pieces {
		if len(piece) == 0 {
			return false
		}
	}
	return true
}

func checkMetadata(metadata []byte, infoHash string) bool {
	hash := sha1.Sum(metadata)
	return bytes.Equal(hash[:], []byte(infoHash))
}
