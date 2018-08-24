package dht

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/neoql/btlet/bencode"
)

var defaultBootstrap = []string{
	"router.bittorrent.com:6881",
	"router.utorrent.com:6881",
	"dht.transmissionbt.com:6881",
}

// Node is dht node.
type Node struct {
	Addr *net.UDPAddr
	ID   string
}

// MessageDisposer is used for dispose message.
type MessageDisposer interface {
	DisposeQuery(nd *Node, transactionID string, q string, args map[string]interface{}) error
	DisposeResponse(nd *Node, transactionID string, resp map[string]interface{}) error
	DisposeError(transactionID string, code int, describe string) error
	DisposeUnknownMessage(y string, message map[string]interface{}) error
}

// Core is the core of dht.
type Core struct {
	conn       *net.UDPConn
	maxWorkers int
}

// NewCore returns a new Core instance.
func NewCore(ip string, port int) (*Core, error) {
	addr, err := net.ResolveUDPAddr("udp", ip+":"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	return &Core{
		conn: conn,
	}, nil
}

// SetMaxWorkers set the max goroutine will be create to dispose dht message.
// If maxWorkers smaller than 0. it won't set upper limit.
func (core *Core) SetMaxWorkers(n int) {
	core.maxWorkers = n
}

// Addr returns the Addr of self.
func (core *Core) Addr() net.Addr {
	return core.conn.LocalAddr()
}

// Serv starts serving.
func (core *Core) Serv(ctx context.Context, disposer MessageDisposer) (err error) {
	defer core.conn.Close()
	
	var dispMsg func (disposer MessageDisposer, addr *net.UDPAddr, data []byte)
	if core.maxWorkers <= 0 {
		dispMsg = func (disposer MessageDisposer, addr *net.UDPAddr, data []byte)  {
			go core.disposeMessage(disposer, addr, data)
		}
	} else {
		lmt := make(chan struct{}, core.maxWorkers)
		dispMsg = func (disposer MessageDisposer, addr *net.UDPAddr, data []byte)  {
			lmt <- struct{}{}
			go func() {
				core.disposeMessage(disposer, addr, data)
				<-lmt
			}()
		}
	}

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:
		}

		buf := make([]byte, 8196)
		n, addr, err := core.conn.ReadFromUDP(buf)
		if err != nil {
			// TODO: handle error
			continue
		}

		dispMsg(disposer, addr, buf[:n])
	}
	return nil
}

// SendMessage will send message to the node.
func (core *Core) SendMessage(nd *Node, msg map[string]interface{}) error {
	data, err := bencode.Marshal(msg)
	if err != nil {
		// TODO: handle error
		return err
	}
	core.conn.SetWriteDeadline(time.Now().Add(time.Second * 15))
	_, err = core.conn.WriteToUDP(data, nd.Addr)
	if err != nil {
		// TODO: handle error
		return err
	}

	return nil
}

func (core *Core) disposeMessage(disposer MessageDisposer, addr *net.UDPAddr, data []byte) (err error) {
	defer func() {
		if e := recover(); e != nil {
			// TODO: handle error
			err = e.(error)
			return
		}
	}()

	var msg map[string]interface{}
	err = bencode.Unmarshal(data, &msg)
	if err != nil {
		// TODO: handle error
		return err
	}

	switch msg["y"] {
	case "q":
		transactionID := msg["t"].(string)
		q := msg["q"].(string)
		args := msg["a"].(map[string]interface{})
		nodeID := args["id"].(string)
		return disposer.DisposeQuery(&Node{addr, nodeID}, transactionID, q, args)
	case "r":
		transactionID := msg["t"].(string)
		resp := msg["r"].(map[string]interface{})
		nodeID := resp["id"].(string)
		return disposer.DisposeResponse(&Node{addr, nodeID}, transactionID, resp)
	case "e":
		transactionID := msg["t"].(string)
		e := msg["e"].([]interface{})
		return disposer.DisposeError(transactionID, e[0].(int), e[1].(string))
	default:
		return disposer.DisposeUnknownMessage(msg["y"].(string), msg)
	}
}

// Handle returns a handle
func (core *Core) Handle(nodeID string) Handle {
	return &handle{
		core:   core,
		nodeID: nodeID,
	}
}

type handle struct {
	core   *Core
	nodeID string
}

func (h *handle) SendMessage(nd *Node, msg map[string]interface{}) error {
	return h.core.SendMessage(nd, msg)
}

func (h *handle) NodeID() string {
	return h.nodeID
}
