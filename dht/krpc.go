package dht

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Transaction is KRPC transaction.
type Transaction interface {
	ID() string
	ShelfLife() time.Duration
	OnTimeout(dht *dhtCore) bool
	OnLaunch(dht *dhtCore)
	OnResponse(dht *dhtCore, nd *node, resp map[string]interface{})
	OnFinish(dht *dhtCore)
}

type transactionContext struct {
	transaction Transaction
	alive       chan struct{}
}

func newTransactionContext(t Transaction) *transactionContext {
	return &transactionContext{
		transaction: t,
		alive:       make(chan struct{}),
	}
}

func (context *transactionContext) Fresh() {
	context.alive <- struct{}{}
}

func (context *transactionContext) loop(dht *dhtCore, rmself func()) {
	context.transaction.OnLaunch(dht)

	for {
		timeout := time.NewTimer(context.transaction.ShelfLife())
		select {
		case <-context.alive:
			timeout.Stop()
		case <-timeout.C:
			if context.transaction.OnTimeout(dht) {
				context.transaction.OnFinish(dht)
				break
			}
		}
	}
}

type transactionManager struct {
	lock         sync.RWMutex
	dht          *dhtCore
	transactions map[string]*transactionContext

	CleanPeriod time.Duration
}

func newTransactionManager(dht *dhtCore) *transactionManager {
	return &transactionManager{
		dht:          dht,
		transactions: make(map[string]*transactionContext),

		CleanPeriod: time.Second * 10,
	}
}

func (manager *transactionManager) Add(t Transaction) error {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	if _, ok := manager.transactions[t.ID()]; ok {
		return errors.New("transation id is already exist")
	}

	context := newTransactionContext(t)
	manager.transactions[t.ID()] = context
	if manager.dht.conn != nil {
		go context.loop(manager.dht, manager.mkRemoveCallback(t))
	}

	return nil
}

func (manager *transactionManager) remove(transactionID string) {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	delete(manager.transactions, transactionID)
}

func (manager *transactionManager) mkRemoveCallback(t Transaction) func() {
	return func() {
		manager.remove(t.ID())
	}
}

func (manager *transactionManager) launchAll() {
	manager.lock.RLock()
	defer manager.lock.RUnlock()

	for _, context := range manager.transactions {
		go context.loop(manager.dht, manager.mkRemoveCallback(context.transaction))
	}
}

func (manager *transactionManager) HandleResponse(transactionID string,
	nd *node, resp map[string]interface{}) {

	manager.lock.RLock()
	defer manager.lock.RUnlock()

	if context, ok := manager.transactions[transactionID]; ok {
		context.Fresh()
		context.transaction.OnResponse(manager.dht, nd, resp)
	}
	return
}

func makeQuery(q string, transactionID string,
	args map[string]interface{}) (map[string]interface{}, error) {

	if q != "ping" && q != "find_node" &&
		q != "get_peers" && q != "announce_peer" {
		return nil, fmt.Errorf("Wrong query type '%s'", q)
	}

	ret := map[string]interface{}{
		"t": transactionID,
		"y": "q",
		"q": q,
		"a": args,
	}

	return ret, nil
}

func makeResponse(transactionID string,
	resp map[string]interface{}) map[string]interface{} {

	ret := map[string]interface{}{
		"t": transactionID,
		"y": "r",
		"r": resp,
	}
	return ret
}

func makeError(transactionID string, code int, msg string) map[string]interface{} {
	ret := map[string]interface{}{
		"t": transactionID,
		"y": "e",
		"e": []interface{}{code, msg},
	}
	return ret
}
