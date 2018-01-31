package bt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"time"

	"github.com/neoql/btlet/bencode"
	"github.com/neoql/btlet/tools"
)

const (
	// extReserved is reserved field in handshake when peer wire
	extReserved = 0x100000
	// extID is extension message id.
	extID = 20
)

func makeHandshake(reserved uint64, infoHash, peerID string) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 68))

	buf.WriteByte(19)
	buf.WriteString("BitTorrent protocol")
	binary.Write(buf, binary.BigEndian, reserved)
	buf.WriteString(infoHash)
	buf.WriteString(peerID)

	return buf.Bytes()
}

func mkExtMsg(id byte, data []byte) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, len(data)+2))
	buf.WriteByte(extID)
	buf.WriteByte(id)
	buf.Write(data)
	return buf.Bytes()
}

func mkExtHandshakeMsg(dict map[string]interface{}) []byte {
	data, _ := bencode.Encode(dict)
	return mkExtMsg(0, data)
}

func parseHandshake(data []byte) (reserved uint64, infoHash, peerID string, err error) {
	if len(data) != 68 || data[0] != 19 || string(data[1:20]) != "BitTorrent protocol" {
		err = errors.New("invalid handshake response")
		return
	}

	reserved = binary.BigEndian.Uint64(data[20:28])
	infoHash = string(data[28:48])
	peerID = string(data[48:])
	return
}

func send(conn net.Conn, data []byte) error {
	conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	_, err := conn.Write(data)
	return err
}

func sendMsg(conn net.Conn, msg []byte) error {
	buf := bytes.NewBuffer(make([]byte, 0, len(msg)+4))
	binary.Write(buf, binary.BigEndian, uint32(len(msg)))
	buf.Write(msg)
	return send(conn, buf.Bytes())
}

func readMsg(r *tools.StreamReader) ([]byte, error) {
	len, err := r.ReadUInt32()
	if err != nil {
		return nil, err
	}

	if len == 0 {
		return make([]byte, 0), nil
	}

	return r.ReadBytes(int64(len))
}
