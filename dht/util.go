package dht

import (
	"net"
	"errors"

	"github.com/neoql/btlet/tools"
)

func unpackNodes(data string) (nodes []*node, err error) {
	if len(data)%26 != 0 {
		err = errors.New("the length of data should be an integer multiple of 26")
		return
	}

	for i := 0; i < len(data)/26; i++ {
		buf := data[i*26 : i*26+26]
		nodeID := buf[:20]
		ip, port, _ := tools.DecodeCompactIPPortInfo(buf[20:])
		nodes = append(nodes, &node{id: nodeID, addr: &net.UDPAddr{IP: ip, Port: port}})
	}

	return
}

func packNodes(nodes []*node) string {
	return ""
}
