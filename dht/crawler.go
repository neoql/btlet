package dht

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/neoql/btlet/tools"
	"github.com/willf/bloom"
)

// Crawler can crawl info hash from DHT.
type Crawler struct {
	*dhtCore
}

// NewCrawler returns a new Crawler instance.
func NewCrawler(ip string, port int16, 
	callback func(string, net.IP, int)) *Crawler {

	transaction := newCrawlTransaction(tools.RandomString(2), callback)
	dht := newDHTCore()
	
	dht.IP	 = ip
	dht.Port = port

	dht.AddTransaction(transaction)
	dht.RequestHandler = transaction.OnRequest
	
	return &Crawler{dht}
}

type crawlTransaction struct {
	id         string
	LaunchUrls []string

	lock         sync.RWMutex
	target       string
	filter       *nodeFilter
	crawCallback func(infoHash string, peerIP net.IP, peerPort int)
}

func newCrawlTransaction(id string, 
	callback func(string, net.IP, int)) *crawlTransaction {
	
	return &crawlTransaction{
		id: tools.RandomString(2),
		LaunchUrls: []string{
			"router.bittorrent.com:6881",
			"router.utorrent.com:6881",
			"dht.transmissionbt.com:6881",
		},

		target:       tools.RandomString(20),
		filter:       newNodeFilter(),
		crawCallback: callback,
	}
}

func (transaction *crawlTransaction) ID() string {
	return transaction.id
}

func (transaction *crawlTransaction) ShelfLife() time.Duration {
	return time.Second * 30
}

func (transaction *crawlTransaction) Target() string {
	transaction.lock.RLock()
	defer transaction.lock.RUnlock()
	return transaction.target
}

func (transaction *crawlTransaction) OnLaunch(dht *dhtCore) {
	nodes := make([]*node, len(transaction.LaunchUrls))
	for i, url := range transaction.LaunchUrls {
		addr, err := net.ResolveUDPAddr("udp", url)
		if err != nil {
			// TODO: handle error
			continue
		}
		nodes[i] = &node{addr, ""}
	}

	transaction.findTargetNode(dht, transaction.Target(), nodes...)
}

func (transaction *crawlTransaction) OnFinish(dht *dhtCore) {}

func (transaction *crawlTransaction) OnResponse(dht *dhtCore,
	nd *node, resp map[string]interface{}) {

	transaction.filter.AddNode(nd)

	if nds, ok := resp["nodes"]; ok {
		nodes, err := unpackNodes(nds.(string))
		if err != nil {
			// TODO: handle error
		}

		target := transaction.Target()
		for _, nd := range nodes {
			if transaction.filter.Check(nd) {
				transaction.findTargetNode(dht, target, nd)
			}
		}
	}
}

func (transaction *crawlTransaction) OnRequest(dht *dhtCore,
	nd *node, transactionID string, q string, args map[string]interface{}) {

	switch q {
	case "ping":
		dht.SendMsg(nd, makeResponse(transactionID, map[string]interface{}{
			"id": makeID(nd.id, dht.NodeID),
		}))
	case "find_node":
	case "get_peers":
		dht.SendMsg(nd, makeResponse(transactionID, map[string]interface{}{
			"id":    makeID(nd.id, dht.NodeID),
			"token": tools.RandomString(20),
			"nodes": "",
		}))
	case "announce_peer":
		infoHash := args["info_hash"].(string)
		port := args["port"].(int)
		if transaction.crawCallback != nil {
			defer transaction.crawCallback(infoHash, nd.addr.IP, port)
		}
		dht.SendMsg(nd, makeResponse(transactionID, map[string]interface{}{
			"id": makeID(nd.id, dht.NodeID),
		}))
	default:
	}

	if transaction.filter.Check(nd) {
		transaction.findTargetNode(dht, transaction.Target(), nd)
	}
}

func (transaction *crawlTransaction) OnTimeout(dht *dhtCore) bool {
	defer transaction.OnLaunch(dht)

	transaction.lock.Lock()
	defer transaction.lock.Unlock()

	len := rand.Int() % 20
	transaction.target = transaction.target[:len] + tools.RandomString(uint(20-len))
	transaction.filter.Reset()

	return false
}

func (transaction *crawlTransaction) findTargetNode(dht *dhtCore, target string, nodes ...*node) {
	for _, nd := range nodes {
		msg, err := makeQuery("find_node", transaction.id, map[string]interface{}{
			"target": target,
			"id":     makeID(nd.id, dht.NodeID),
		})
		if err != nil {
			// TODO: handle error
			continue
		}

		dht.SendMsg(nd, msg)
	}
}

func makeID(dst string, id string) string {
	if len(dst) == 0 {
		return id
	}
	return dst[:15] + id[15:]
}

type nodeFilter struct {
	lock sync.RWMutex
	core *bloom.BloomFilter
}

func newNodeFilter() *nodeFilter {
	return &nodeFilter{
		core: bloom.NewWithEstimates(8*1024, 0.001),
	}
}

func (filter *nodeFilter) AddNode(nd *node) {
	filter.lock.Lock()
	filter.lock.Unlock()

	key := fmt.Sprintf("%s:%d", nd.addr.IP, nd.addr.Port)
	filter.core.AddString(key)
}

func (filter *nodeFilter) Check(nd *node) bool {
	filter.lock.RLock()
	defer filter.lock.RUnlock()

	key := fmt.Sprintf("%s:%d", nd.addr.IP, nd.addr.Port)
	return !filter.core.TestString(key)
}

func (filter *nodeFilter) Reset() {
	filter.lock.Lock()
	filter.lock.Unlock()
	filter.core.ClearAll()
}
