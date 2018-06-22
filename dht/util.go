package dht

import (
	"errors"
	"net"

	"github.com/neoql/btlet/tools"
)

func unpackNodes(data string) (nodes []*Node, err error) {
	if len(data)%26 != 0 {
		err = errors.New("the length of data should be an integer multiple of 26")
		return
	}

	for i := 0; i < len(data)/26; i++ {
		buf := data[i*26 : i*26+26]
		nodeID := buf[:20]
		ip, port, _ := tools.DecodeCompactIPPortInfo(buf[20:])
		nodes = append(nodes, &Node{ID: nodeID, Addr: &net.UDPAddr{IP: ip, Port: port}})
	}

	return
}
