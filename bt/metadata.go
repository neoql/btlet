package bt

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/neoql/btlet/bencode"
	"github.com/neoql/btlet/tools"
)

const (
	// utMetadata is th ui_metadata id
	utMetadata = 2
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

// FetchMetadata from assign address
func FetchMetadata(infoHash string, address string) (meta RawMeta, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()

	conn, err := net.DialTimeout("tcp", address, time.Second*15)
	if err != nil {
		return
	}

	defer conn.Close()
	r := tools.NewStreamReader(conn, time.Second*10)

	var req, resp []byte

	req = makeHandshake(extReserved, infoHash, tools.RandomString(20))
	err = send(conn, req)
	if err != nil {
		return
	}

	resp, err = r.ReadBytes(68)
	if err != nil {
		return
	}
	reserved, hash, _, err := parseHandshake(resp)
	if err != nil || (reserved&extReserved) == 0 || hash != infoHash {
		err = errors.New("invalid handshake response")
		return
	}

	req = mkExtHandshakeMsg(map[string]interface{}{
		"m": map[string]interface{}{"ut_metadata": utMetadata},
	})
	err = sendMsg(conn, req)
	if err != nil {
		return
	}

	var pieces [][]byte

	for {
		resp, err = readMsg(r)
		if err != nil {
			return
		}

		if len(resp) == 0 {
			continue
		}

		switch resp[0] {
		case extID:
			if resp[1] == 0 {
				// handle handshake
				var msg map[string]interface{}
				err = bencode.Unmarshal(resp[2:], &msg)
				if err != nil {
					// TODO: handle error
					return nil, err
				}
				m := msg["m"].(map[string]interface{})
				size := int(msg["metadata_size"].(int64))
				ut := byte(m["ut_metadata"].(int64))
				num := reqMetadataPieces(conn, ut, size)
				pieces = make([][]byte, num)
			} else {
				if pieces == nil {
					return nil, errors.New("have not handshake")
				}
				content := resp[2:]
				var msg map[string]interface{}
				dec := bencode.NewDecoder(bytes.NewReader(content))
				err = dec.Decode(&msg)
				if err != nil {
					return nil, err
				}
				excess := content[dec.BytesParsed():]
				if msg["msg_type"] != int64(data) {
					continue
				}

				index := int(msg["piece"].(int64))
				pieces[index] = excess

				if checkPiecesDone(pieces) {
					metadata := bytes.Join(pieces, nil)
					if checkMetadata(metadata, infoHash) {
						return metadata, nil
					}
					return nil, errors.New("wrong hash")
				}
			}
		default:
		}
	}
}

func reqMetadataPieces(conn net.Conn, ut byte, size int) int {
	piecesNum := size / (16 * 1024)
	if size%(16*1024) != 0 {
		piecesNum++
	}

	go func() {
		for i := 0; i < piecesNum; i++ {
			content, _ := bencode.Marshal(map[string]interface{}{
				"msg_type": request,
				"piece":    i,
			})
			err := sendMsg(conn, mkExtMsg(ut, content))
			if err != nil {
				return
			}
		}
	}()

	return piecesNum
}
