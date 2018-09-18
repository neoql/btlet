package dht

import (
	"errors"
	"net"
	"strings"

	"github.com/neoql/btlet/bencode"
	"github.com/neoql/btlet/tools"
)

// NodePtrSlice is []*Node
type NodePtrSlice []*Node

// MarshalBencode implements bencode.Marshaler
func (nds NodePtrSlice) MarshalBencode() ([]byte, error) {
	raw, err := PackNodes(nds)
	if err != nil {
		return nil, err
	}

	return bencode.Marshal(raw)
}

// UnmarshalBencode implemts bencode.Unmarshaler
func (nds *NodePtrSlice) UnmarshalBencode(data []byte) error {
	var raw string

	err := bencode.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	nodes, err := unpackNodes(raw)
	if err != nil {
		return err
	}

	*nds = nodes
	return nil
}

func unpackNodes(data string) (nodes []*Node, err error) {
	if len(data)%26 != 0 {
		err = errors.New("the length of data should be an integer multiple of 26")
		return
	}

	nodes = make([]*Node, len(data)/26)
	for i := 0; i < len(data)/26; i++ {
		buf := data[i*26 : i*26+26]
		nodeID := buf[:20]
		ip, port, _ := tools.DecodeCompactIPPortInfo(buf[20:])
		nodes[i] = &Node{ID: nodeID, Addr: &net.UDPAddr{IP: ip, Port: port}}
	}

	return
}

// PackNodes packs nodes
func PackNodes(nodes []*Node) (string, error) {
	if len(nodes) == 0 {
		return "", nil
	}
	
	sb := strings.Builder{}
	for _, node := range nodes {
		s, err := tools.EncodeCompactIPPortInfo(node.Addr.IP, node.Addr.Port)
		if err != nil {
			return "", err
		}
		_, err = sb.WriteString(s)
		if err != nil {
			return "", err
		}
	}
	return sb.String(), nil
}