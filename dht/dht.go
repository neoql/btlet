package dht

import (
	"net"
	"strconv"
	"time"

	"github.com/neoql/btlet/bencode"
	"github.com/neoql/btlet/tools"
)

type node struct {
	addr *net.UDPAddr
	id   string
}

type dhtCore struct {
	conn               *net.UDPConn
	transactionManager *transactionManager
	RequestHandler     func(dht *dhtCore, nd *node, transactionID string,
		q string, args map[string]interface{})
	ErrorHandler func(transactionID, string, code int, msg string)

	IP     string
	Port   int16
	NodeID string
}

func newDHTCore() *dhtCore {
	core := &dhtCore{
		IP:     "0.0.0.0",
		Port:   6881,
		NodeID: tools.RandomString(20),
	}
	core.transactionManager = newTransactionManager(core)

	return core
}

func (dht *dhtCore) Run() (err error) {
	if err = dht.prepare(); err != nil {
		return
	}

	dht.launch()
	dht.loop()
	return
}

func (dht *dhtCore) loop() {
	for {
		buf := make([]byte, 8196)
		n, addr, err := dht.conn.ReadFromUDP(buf)
		if err != nil {
			// TODO: handle error
			continue
		}

		go dht.handleMsg(addr, buf[:n])
	}
}

func (dht *dhtCore) AddTransaction(t Transaction) error {
	return dht.transactionManager.Add(t)
}

func (dht *dhtCore) SendMsg(nd *node, msg map[string]interface{}) {
	data, err := bencode.Encode(msg)
	if err != nil {
		// TODO: handle error
		return
	}
	dht.conn.SetWriteDeadline(time.Now().Add(time.Second * 15))
	_, err = dht.conn.WriteToUDP(data, nd.addr)
	if err != nil {
		// TODO: handle error
		return
	}
}

func (dht *dhtCore) prepare() (err error) {
	addr, err := net.ResolveUDPAddr("udp", dht.IP+":"+strconv.Itoa(int(dht.Port)))
	if err != nil {
		return
	}
	dht.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return
	}
	return
}

func (dht *dhtCore) launch() {
	dht.transactionManager.launchAll()
}

func (dht *dhtCore) handleMsg(addr *net.UDPAddr, data []byte) {
	defer func() {
		if err := recover(); err != nil {
			// TODO: handle error
		}
	}()

	tmp, err := bencode.Decode(data)
	if err != nil {
		// TODO: handle error
		return
	}

	msg := tmp.(map[string]interface{})

	switch msg["y"] {
	case "q":
		transactionID := msg["t"].(string)
		q := msg["q"].(string)
		args := msg["a"].(map[string]interface{})
		nodeID := args["id"].(string)
		dht.RequestHandler(dht, &node{addr, nodeID}, transactionID, q, args)
	case "r":
		transactionID := msg["t"].(string)
		resp := msg["r"].(map[string]interface{})
		nodeID := resp["id"].(string)
		dht.transactionManager.HandleResponse(transactionID, &node{addr, nodeID}, resp)
	case "e":
	default:
		// TODO: unknown "y"
	}
}
