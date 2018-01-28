package dht

import (
	"math/rand"
	"net"
	"sync"
	// "time"

	"github.com/neoql/btlet/tools"
	"github.com/neoql/container/queue"

	"github.com/willf/bloom"
)

// Result contains info hash and peer addr.
type Result struct {
	InfoHash string
	PeerIP   net.IP
	PeerPort int
}

// Crawler can crawl info hash from DHT.
type Crawler struct {
	dht          *dhtCore
	resultBuffer *queue.Queue
}

// NewCrawler returns a new Crawler instance.
func NewCrawler() *Crawler {
	return NewCrawlerWithBuffer(queue.New())
}

// NewCrawlerWithBuffer returns a new Crawler instance with assign buffer
func NewCrawlerWithBuffer(buf *queue.Queue) *Crawler {
	transaction := newCrawlTransaction(tools.RandomString(2), buf)
	dht := newDHTCore()

	// dht.transactionManager.CleanPeriod = time.Minute * 5
	// dht.transactionManager.Expiration = 0
	dht.AddTransaction(transaction)
	dht.RequestHandler = transaction.OnRequest

	return &Crawler{
		dht:          dht,
		resultBuffer: buf,
	}
}

// Run will launch the crawler
func (crawler *Crawler) Run() error {
	return crawler.dht.Run()
}

// ResultChan returns an info hash channel
func (crawler *Crawler) ResultChan() chan Result {
	ch := make(chan Result)
	go func() {
		for {
			result, flag := crawler.resultBuffer.Pop()
			if !flag {
				close(ch)
			}
			ch <- result.(Result)
		}
	}()
	return ch
}

type crawlTransaction struct {
	id         string
	LaunchUrls []string

	task         *crawlTask
	resultBuffer *queue.Queue
}

func newCrawlTransaction(id string, buf *queue.Queue) *crawlTransaction {
	return &crawlTransaction{
		id: tools.RandomString(2),
		LaunchUrls: []string{
			"router.bittorrent.com:6881",
			"router.utorrent.com:6881",
			"dht.transmissionbt.com:6881",
		},

		task:         newCrawTask(),
		resultBuffer: buf,
	}
}

func (transaction *crawlTransaction) ID() string {
	return transaction.id
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

	transaction.joinDHT(dht, nodes)
}

func (transaction *crawlTransaction) OnFinish(dht *dhtCore) {}

func (transaction *crawlTransaction) OnResponse(dht *dhtCore,
	nd *node, resp map[string]interface{}) {

	transaction.task.AddFiltNode(nd)

	if nds, ok := resp["nodes"]; ok {
		nodes, err := unpackNodes(nds.(string))
		if err != nil {
			// TODO: handle error
		}

		target := transaction.task.Target()
		for _, nd := range nodes {
			if transaction.task.Check(nd) {
				msg, err := makeQuery("find_node", transaction.id, map[string]interface{}{
					"target": target,
					"id":     makeID(nd.id, dht.NodeID),
				})

				if err != nil {
					// TODO: handle msg
					continue
				}

				dht.SendMsg(nd, msg)
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
		transaction.resultBuffer.Put(Result{infoHash, nd.addr.IP, port})
		dht.SendMsg(nd, makeResponse(transactionID, map[string]interface{}{
			"id": makeID(nd.id, dht.NodeID),
		}))
	default:
	}

	if transaction.task.Check(nd) {
		msg, err := makeQuery("find_node", transaction.id, map[string]interface{}{
			"target": transaction.task.Target(),
			"id":     makeID(nd.id, dht.NodeID),
		})

		if err != nil {
			// TODO: handle msg
			return
		}

		dht.SendMsg(nd, msg)
	}
}

func (transaction *crawlTransaction) Timeout(dht *dhtCore) bool {
	if transaction.task.RespTotal() == 0 {
		transaction.OnLaunch(dht)
	} else {
		nodes := transaction.task.Reset()
		transaction.joinDHT(dht, nodes)
	}

	return false
}

func (transaction *crawlTransaction) joinDHT(dht *dhtCore, nodes []*node) {
	target := transaction.task.Target()
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

type crawlTask struct {
	target  string
	tgtLock sync.RWMutex

	lock           sync.RWMutex
	filter         *bloom.BloomFilter
	buf            []*node
	bufSize        int
	tail           int
	total          int
	longPrefixLen  int
	shortPrefixLen int
	longerTotal    int
	shorterTotal   int
}

func newCrawTask() *crawlTask {
	return &crawlTask{
		filter:         bloom.NewWithEstimates(8*1024, 0.001),
		buf:            make([]*node, 16),
		bufSize:        0,
		target:         tools.RandomString(20),
		tail:           0,
		total:          0,
		longPrefixLen:  21,
		shortPrefixLen: 0,
		longerTotal:    0,
		shorterTotal:   0,
	}
}

func (task *crawlTask) AddFiltNode(nd *node) {
	task.lock.Lock()
	defer task.lock.Unlock()

	if task.filter.TestAndAddString(nd.id) {
		return
	}
	task.total++

	cpl := tools.CommonPrefixLen(nd.id, task.Target())

	if cpl <= task.shortPrefixLen {
		return
	}
	task.longerTotal++

	if cpl <= task.longPrefixLen {
		task.longPrefixLen = cpl
		task.shorterTotal++
	}

	if task.longerTotal >= 16 {
		task.shortPrefixLen = task.longPrefixLen
		task.longPrefixLen = 21
		task.longerTotal = task.longerTotal - task.shorterTotal
		task.shorterTotal = 0
	}

	if task.buf[task.tail] == nil {
		task.bufSize++
	}

	task.buf[task.tail] = nd
	task.tail = (task.tail + 1) % 16
}

func (task *crawlTask) Check(nd *node) bool {
	task.lock.RLock()
	defer task.lock.RUnlock()

	if task.filter.TestString(nd.id) {
		return false
	}

	if tools.CommonPrefixLen(nd.id, task.Target()) < task.shortPrefixLen {
		return false
	}

	return true
}

func (task *crawlTask) Reset() (nodes []*node) {
	task.lock.Lock()
	defer task.lock.Unlock()
	task.tgtLock.Lock()
	defer task.tgtLock.Unlock()

	task.filter.ClearAll()

	nodes = task.buf[:task.bufSize+1]
	task.buf = make([]*node, 16)
	task.bufSize = 0
	task.tail = 0

	len := rand.Int() % task.shortPrefixLen
	task.target = task.target[:len] + tools.RandomString(uint(20-len))
	task.total = 0

	task.longPrefixLen = 21
	task.shortPrefixLen = 0
	task.longerTotal = 0
	task.shorterTotal = 0

	return
}

func (task *crawlTask) RespTotal() int {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.total
}

func (task *crawlTask) Target() string {
	task.tgtLock.RLock()
	defer task.tgtLock.RUnlock()

	return task.target
}
