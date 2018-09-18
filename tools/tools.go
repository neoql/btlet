package tools

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
)

// RandomString generates a size-length string randomly.
func RandomString(size uint) string {
	if size == 0 {
		return ""
	}

	buf := make([]byte, size)
	rand.Read(buf)
	return string(buf)
}

// CommonPrefixLen returns the length of common prefix
func CommonPrefixLen(a, b string) int {
	var aa, bb byte
	var i, j int
	for i = 0; i < 20; i++ {
		if a[i] != b[i] {
			aa, bb = a[i], b[i]
			break
		}
	}

	v := aa ^ bb

	for j = 1; j <= 8; j++ {
		if (v >> uint(8-j)) > 0 {
			break
		}
	}

	return i*8 + j - 1
}

// DecodeCompactIPPortInfo decodes compactIP-address/port info in BitTorrent
// DHT Protocol. It returns the ip and port number.
func DecodeCompactIPPortInfo(info string) (ip net.IP, port int, err error) {
	if len(info) != 6 {
		err = errors.New("compact info should be 6-length long")
		return
	}

	ip = net.IPv4(info[0], info[1], info[2], info[3])
	port = int((uint16(info[4]) << 8) | uint16(info[5]))
	return
}

// EncodeCompactIPPortInfo encodes an ip and a port number to
// compactIP-address/port info.
func EncodeCompactIPPortInfo(ip net.IP, port int) (info string, err error) {
	if port > 65535 || port < 0 {
		err = errors.New("port should be no greater than 65535 and no less than 0")
		return
	}

	buf := make([]byte, 6)
	copy(buf, ip)
	binary.BigEndian.PutUint16(buf[4:], uint16(port))

	return string(buf), nil
}
