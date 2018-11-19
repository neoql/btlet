package bt

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"

	"github.com/neoql/btlet/bencode"
	"github.com/neoql/btlet/tools"
)

// FetchMetadata fetch metadata from host.
func FetchMetadata(ctx context.Context, infoHash string, host string) (RawMeta, error) {
	// connect to peer
	var reserved uint64
	SetExtReserved(&reserved)
	stream, err := DialUseTCP(host, infoHash, tools.RandomString(20), reserved)
	if err != nil {
		return nil, err
	}

	defer stream.Close()

	// peer send different info_hash
	if stream.InfoHash() != infoHash {
		return nil, errors.New("handshake failed: different info_hash")
	}

	if !CheckExtReserved(stream.Reserved()) {
		// not support extensions
		return nil, errors.New("not support extensions")
	}

	proto := NewExtProtocol(stream)

	fmExt := NewFetchMetaExt(infoHash)
	proto.RegistExt(fmExt)

	err = proto.WriteHandshake()
	if err != nil {
		return nil, err
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		message, err := ReadMessageWithLimit(stream, 99999)
		if err != nil {
			return nil, err
		}

		if len(message) == 0 || message[0] != ExtID {
			continue
		}

		err = proto.HandlePayload(message[1:])
		if err != nil {
			return nil, err
		}

		if !fmExt.IsSupoort() {
			return nil, errors.New("not support Extension for Peers to Send Metadata Files")
		}

		if fmExt.CheckDone() {
			meta, err := fmExt.FetchRawMeta()
			if err == nil {
				return meta, nil
			}
		}
	}
}

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
func NewFetchMetaExt(infoHash string) *FetchMetaExt {
	return &FetchMetaExt{
		infoHash: infoHash,
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

	if size <= 0 {
		return errors.New("wrong size")
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
func (fm *FetchMetaExt) HandleMessage(content []byte, sender *ExtMsgSender) error {
	var msg map[string]int

	dec := bencode.NewDecoder(bytes.NewReader(content))
	err := dec.Decode(&msg)
	if err != nil {
		return err
	}

	switch msg["msg_type"] {
	default:
	case reject:
		return errors.New("peer reject out request")
	case data:
		no := msg["piece"]

		fm.pieces[no] = content[dec.BytesParsed():]
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
