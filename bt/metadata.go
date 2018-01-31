package bt

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"net"
	"time"

	"github.com/neoql/btlet/bencode"
	"github.com/neoql/btlet/tools"
)

const (
	// utMetadata is th ui_metadata id
	utMetadata = 2
)

const (
	request = iota
	data
	reject
)

// FetchMetadata from assign address
func FetchMetadata(infoHash string, address string) (meta map[string]interface{}, err error) {
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
				d, err := bencode.Decode(resp[2:])
				if err != nil {
					return nil, err
				}
				msg := d.(map[string]interface{})
				m := msg["m"].(map[string]interface{})
				size := msg["metadata_size"].(int)
				ut := byte(m["ut_metadata"].(int))
				num := reqMetadataPieces(conn, ut, size)
				pieces = make([][]byte, num)
			} else {
				if pieces == nil {
					return nil, errors.New("have not handshake")
				}
				content := resp[2:]
				d, excess, err := bencode.DecodeWithExcess(content)
				if err != nil {
					return nil, err
				}
				msg := d.(map[string]interface{})
				if msg["msg_type"] != data {
					continue
				}

				index := msg["piece"].(int)
				pieces[index] = excess

				if checkPiecesDone(pieces) {
					metadata := bytes.Join(pieces, nil)
					if checkMetadata(metadata, infoHash) {
						m, err := bencode.Decode(metadata)
						if err != nil {
							return nil, err
						}
						return m.(map[string]interface{}), nil
					}
					return nil, errors.New("wrong hash")
				}
			}
		default:
		}
	}
}

func checkMetadata(metadata []byte, infoHash string) bool {
	hash := sha1.Sum(metadata)
	return bytes.Equal(hash[:], []byte(infoHash))
}

func checkPiecesDone(pieces [][]byte) bool {
	for _, piece := range pieces {
		if len(piece) == 0 {
			return false
		}
	}
	return true
}

func reqMetadataPieces(conn net.Conn, ut byte, size int) int {
	piecesNum := size / (16 * 1024)
	if size%(16*1024) != 0 {
		piecesNum++
	}

	go func() {
		for i := 0; i < piecesNum; i++ {
			content, _ := bencode.Encode(map[string]interface{}{
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
