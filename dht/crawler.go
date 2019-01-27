package dht

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neoql/btlet/bencode"
	"github.com/neoql/btlet/tools"
	"github.com/willf/bloom"
)

// CrawCallback will call when Crawler craw a infohash
type CrawCallback func(infoHash string, peerIP net.IP, peerPort int)

// Crawler can crawl infohash from dht.
type Crawler interface {
	Crawl(ctx context.Context, callback CrawCallback) error
}

// Stimulater could be stimulate
type Stimulater interface {
	Stimulate()
}

// SybilCrawler can crawl info hash from DHT.
type SybilCrawler struct {
	host       string
	nodeID     string
	maxWorkers int

	transaction *sybilTransaction
}

// NewSybilCrawler returns a new Crawler instance.
func NewSybilCrawler(host string) *SybilCrawler {
	return &SybilCrawler{
		host:   host,
		nodeID: tools.RandomString(20),
	}
}

// SetMaxWorkers set the max goroutine will be create to dispose dht message.
// If maxWorkers smaller than 0. it won't set upper limit.
func (crawler *SybilCrawler) SetMaxWorkers(n int) {
	crawler.maxWorkers = n
}

// Crawl ovrride Crawler.Crawl
func (crawler *SybilCrawler) Crawl(ctx context.Context, callback CrawCallback) error {
	core, err := NewHost(crawler.host)
	if err != nil {
		return err
	}
	core.SetMaxWorkers(crawler.maxWorkers)

	handle := core.Handle(crawler.nodeID)
	dispatcher := NewTransactionDispatcher(handle)
	transaction := newSybilTransaction(tools.RandomString(2), callback)
	crawler.transaction = transaction

	err = dispatcher.Add(transaction)
	if err != nil {
		return err
	}
	defer dispatcher.Remove(transaction.ID())

	disposer := &sybilMessageDisposer{
		handle:      handle,
		dispatcher:  dispatcher,
		transaction: transaction,
	}

	return core.Serv(ctx, disposer)
}

// Stimulate implements Stimulater
func (crawler *SybilCrawler) Stimulate() {
	crawler.transaction.changeTarget()
	crawler.transaction.filter.Reset()
}

type sybilMessageDisposer struct {
	handle      Handle
	dispatcher  *TransactionDispatcher
	transaction *sybilTransaction
}

func (disposer *sybilMessageDisposer) DisposeQuery(src *net.UDPAddr, transactionID string, q string, args bencode.RawMessage) error {
	disposer.transaction.OnQuery(disposer.handle, src, transactionID, q, args)
	return nil
}

func (disposer *sybilMessageDisposer) DisposeResponse(src *net.UDPAddr, transactionID string, resp bencode.RawMessage) error {
	disposer.dispatcher.DisposeResponse(src, transactionID, resp)
	return nil
}

func (disposer *sybilMessageDisposer) DisposeError(src *net.UDPAddr, transactionID string, code int, describe string) error {
	return nil
}

func (disposer *sybilMessageDisposer) DisposeUnknownMessage(src *net.UDPAddr, message bencode.RawMessage) error {
	return nil
}

type sybilTransaction struct {
	id           string
	target       atomic.Value
	filter       *nodeFilter
	crawCallback CrawCallback
	finish       chan struct{}
}

func newSybilTransaction(id string, callback CrawCallback) *sybilTransaction {
	t := &sybilTransaction{
		id:           tools.RandomString(2),
		filter:       newNodeFilter(8 * 1024 * 1024),
		crawCallback: callback,
		finish:       make(chan struct{}),
	}
	t.target.Store(tools.RandomString(20))

	return t
}

func (transaction *sybilTransaction) ID() string {
	return transaction.id
}

func (transaction *sybilTransaction) ShelfLife() time.Duration {
	return time.Second * 30
}

func (transaction *sybilTransaction) Target() string {
	return transaction.target.Load().(string)
}

func (transaction *sybilTransaction) OnLaunch(handle Handle) {
	transaction.boot(handle, defaultBootstrap)
	go func() {
	LOOP:
		for {
			select {
			case <-time.After(time.Minute):
				if transaction.filter.IsFull() {
					transaction.changeTarget()
					transaction.filter.Reset()
				}
			case <-transaction.finish:
				break LOOP
			}
		}
	}()
}

func (transaction *sybilTransaction) boot(handle Handle, bootstrap []string) {
	nodes := make([]*Node, 0, len(bootstrap))
	for _, url := range bootstrap {
		addr, err := net.ResolveUDPAddr("udp", url)
		if err != nil {
			// TODO: handle error
			continue
		}
		nodes = append(nodes, &Node{addr, ""})
	}

	transaction.findTargetNode(handle, transaction.Target(), nodes...)
}

func (transaction *sybilTransaction) OnError(handle Handle, src *net.UDPAddr, code int, describe string) bool {
	return true
}

func (transaction *sybilTransaction) OnResponse(handle Handle, src *net.UDPAddr, resp bencode.RawMessage) bool {

	var r Response

	err := bencode.Unmarshal(resp, &r)
	if err != nil || r.Nodes == nil {
		return true
	}

	transaction.filter.AddNode(&Node{src, r.NodeID})

	target := transaction.Target()
	for _, nd := range *r.Nodes {
		if transaction.filter.Check(nd) {
			transaction.findTargetNode(handle, target, nd)
		}
	}

	return true
}

func (transaction *sybilTransaction) OnQuery(handle Handle, src *net.UDPAddr, transactionID string, q string, args bencode.RawMessage) {

	nd := &Node{Addr: src}
	switch q {
	default:
	case "ping":
		var a PingArgs
		err := bencode.Unmarshal(args, &a)
		if err != nil {
			return
		}

		r, err := MakeResponse(transactionID, &Response{
			NodeID: makeID(a.NodeID, handle.NodeID()),
		})
		if err != nil {
			return
		}

		nd.ID = a.NodeID
		handle.SendMessage(src, r)
	case "find_node":
	case "get_peers":
		var a GetPeersArgs
		err := bencode.Unmarshal(args, &a)
		if err != nil {
			return
		}

		nodes := NodePtrSlice(nil)
		r, err := MakeResponse(transactionID, &Response{
			NodeID: makeID(a.NodeID, handle.NodeID()),
			Token:  tools.RandomString(20),
			Nodes:  &nodes,
		})
		if err != nil {
			return
		}

		nd.ID = a.NodeID
		handle.SendMessage(src, r)
	case "announce_peer":
		var a AnnouncePeerArgs
		err := bencode.Unmarshal(args, &a)
		if err != nil {
			return
		}

		if transaction.crawCallback != nil {
			defer transaction.crawCallback(a.InfoHash, src.IP, a.Port)
		}

		r, err := MakeResponse(transactionID, &Response{
			NodeID: makeID(a.NodeID, handle.NodeID()),
		})
		if err != nil {
			return
		}

		nd.ID = a.NodeID
		handle.SendMessage(src, r)
	}

	if transaction.filter.Check(nd) {
		transaction.findTargetNode(handle, transaction.Target(), nd)
	}
}

func (transaction *sybilTransaction) OnTimeout(handle Handle) bool {
	defer transaction.boot(handle, defaultBootstrap)
	transaction.changeTarget()
	transaction.filter.Reset()
	return true
}

func (transaction *sybilTransaction) changeTarget() {
	transaction.target.Store(tools.RandomString(20))
}

func (transaction *sybilTransaction) OnFinish(handle Handle) {
	close(transaction.finish)
}

func (transaction *sybilTransaction) findTargetNode(handle Handle, target string, nodes ...*Node) {
	for _, nd := range nodes {
		msg, err := MakeQuery(transaction.id, &FindNodeArgs{
			Target: target,
			NodeID: makeID(nd.ID, handle.NodeID()),
		})
		if err != nil {
			// TODO: handle error
			continue
		}

		handle.SendMessage(nd.Addr, msg)
	}
}

func makeID(dst string, id string) string {
	if len(dst) == 0 {
		return id
	}
	return dst[:15] + id[15:]
}

type nodeFilter struct {
	lock   sync.RWMutex
	core   *bloom.BloomFilter
	amount uint
	cap    uint
}

func newNodeFilter(cap uint) *nodeFilter {
	return &nodeFilter{
		core: bloom.NewWithEstimates(cap, 0.001),
		cap:  cap,
	}
}

func (filter *nodeFilter) AddNode(nd *Node) bool {
	filter.lock.Lock()
	filter.lock.Unlock()

	key := fmt.Sprintf("%s:%d", nd.Addr.IP, nd.Addr.Port)
	filter.core.AddString(key)
	if !filter.core.TestAndAddString(key) {
		filter.amount++
		return false
	}
	return true
}

func (filter *nodeFilter) Check(nd *Node) bool {
	filter.lock.RLock()
	defer filter.lock.RUnlock()

	key := fmt.Sprintf("%s:%d", nd.Addr.IP, nd.Addr.Port)
	return !filter.core.TestString(key)
}

func (filter *nodeFilter) Reset() {
	filter.lock.Lock()
	defer filter.lock.Unlock()
	filter.core.ClearAll()
	filter.amount = 0
}

func (filter *nodeFilter) IsFull() bool {
	filter.lock.RLock()
	defer filter.lock.RUnlock()
	return filter.amount >= filter.cap
}
