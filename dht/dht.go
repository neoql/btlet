package dht

import (
	"context"
	"net"
	"time"

	"github.com/neoql/btlet/bencode"
	"github.com/neoql/workerpool"
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
	DisposeQuery(src *net.UDPAddr, transactionID string, q string, args bencode.RawMessage) error
	DisposeResponse(src *net.UDPAddr, transactionID string, resp bencode.RawMessage) error
	DisposeError(src *net.UDPAddr, transactionID string, code int, describe string) error
	DisposeUnknownMessage(src *net.UDPAddr, message bencode.RawMessage) error
}

// Host is the core of dht.
type Host struct {
	conn       *net.UDPConn
	maxWorkers int
}

// NewHost returns a new Core instance.
func NewHost(host string) (*Host, error) {
	addr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	return &Host{
		conn: conn,
	}, nil
}

// SetMaxWorkers set the max goroutine will be create to dispose dht message.
// If maxWorkers smaller than 0. it won't set upper limit.
func (core *Host) SetMaxWorkers(n int) {
	core.maxWorkers = n
}

// Addr returns the Addr of self.
func (core *Host) Addr() net.Addr {
	return core.conn.LocalAddr()
}

// Serv starts serving.
func (core *Host) Serv(ctx context.Context, disposer MessageDisposer) (err error) {
	defer core.conn.Close()

	proxy := newDisposerProxy(disposer, core.maxWorkers)
	buf := make([]byte, 8196)
	proxy.wp.Start()

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:
		}

		n, addr, err := core.conn.ReadFromUDP(buf)
		if err != nil {
			// TODO: handle error
			continue
		}

		core.disposeMessage(proxy, addr, buf[:n])
	}
	return nil
}

// SendMessage will send message to the node.
func (core *Host) SendMessage(dst *net.UDPAddr, msg interface{}) error {
	data, err := bencode.Marshal(msg)
	if err != nil {
		// TODO: handle error
		return err
	}
	core.conn.SetWriteDeadline(time.Now().Add(time.Second * 15))
	_, err = core.conn.WriteToUDP(data, dst)
	if err != nil {
		// TODO: handle error
		return err
	}

	return nil
}

func (core *Host) disposeMessage(disposer MessageDisposer, addr *net.UDPAddr, data []byte) (err error) {
	defer func() {
		if e := recover(); e != nil {
			// TODO: handle error
			err = e.(error)
			return
		}
	}()

	var msg Message

	err = bencode.Unmarshal(data, &msg)
	if err != nil {
		return err
	}

	switch msg.Y {
	default:
	case "q":
		return disposer.DisposeQuery(addr, msg.TransactionID, msg.Q, msg.Args)
	case "r":
		return disposer.DisposeResponse(addr, msg.TransactionID, msg.Resp)
	case "e":
	}

	return nil
}

// Handle returns a handle
func (core *Host) Handle(nodeID string) Handle {
	return &handle{
		core:   core,
		nodeID: nodeID,
	}
}

type handle struct {
	core   *Host
	nodeID string
}

func (h *handle) SendMessage(dst *net.UDPAddr, msg interface{}) error {
	return h.core.SendMessage(dst, msg)
}

func (h *handle) NodeID() string {
	return h.nodeID
}

type disposerProxy struct {
	disposer MessageDisposer
	wp       workerpool.WorkerPool
}

func newDisposerProxy(disposer MessageDisposer, maxWorkers int) *disposerProxy {
	proxy := &disposerProxy{
		disposer: disposer,
		wp:       workerpool.New(maxWorkers, time.Second*10),
	}

	return proxy
}

func (p *disposerProxy) DisposeQuery(src *net.UDPAddr, transactionID string, q string, args bencode.RawMessage) error {
	p.wp.WaitSpawn(p.fnDisposeQuery, &queryArgv{src: src, transactionID: transactionID, q: q, args: args})
	return nil
}

func (p *disposerProxy) DisposeResponse(src *net.UDPAddr, transactionID string, resp bencode.RawMessage) error {
	p.wp.WaitSpawn(p.fnDisposeResponse, &respArgv{src: src, transactionID: transactionID, resp: resp})
	return nil
}

func (p *disposerProxy) DisposeError(src *net.UDPAddr, transactionID string, code int, describe string) error {
	p.wp.WaitSpawn(p.fnDisposeError, &errorArgv{src: src, transactionID: transactionID, code: code, describe: describe})
	return nil
}

func (p *disposerProxy) DisposeUnknownMessage(src *net.UDPAddr, message bencode.RawMessage) error {
	p.wp.WaitSpawn(p.fnDisposeUnknownMessage, &unknownMessageArgv{src: src, message: message})
	return nil
}

func (p *disposerProxy) fnDisposeQuery(argv workerpool.Argv) {
	a := argv.(*queryArgv)
	p.disposer.DisposeQuery(a.src, a.transactionID, a.q, a.args)
}

func (p *disposerProxy) fnDisposeResponse(argv workerpool.Argv) {
	a := argv.(*respArgv)
	p.disposer.DisposeResponse(a.src, a.transactionID, a.resp)
}

func (p *disposerProxy) fnDisposeError(argv workerpool.Argv) {
	a := argv.(*errorArgv)
	p.disposer.DisposeError(a.src, a.transactionID, a.code, a.describe)
}

func (p *disposerProxy) fnDisposeUnknownMessage(argv workerpool.Argv) {
	a := argv.(*unknownMessageArgv)
	p.disposer.DisposeUnknownMessage(a.src, a.message)
}

type queryArgv struct {
	q             string
	src           *net.UDPAddr
	args          bencode.RawMessage
	transactionID string
}

type respArgv struct {
	src           *net.UDPAddr
	resp          bencode.RawMessage
	transactionID string
}

type errorArgv struct {
	src           *net.UDPAddr
	transactionID string
	code          int
	describe      string
}

type unknownMessageArgv struct {
	src     *net.UDPAddr
	message bencode.RawMessage
}
