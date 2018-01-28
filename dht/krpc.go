package dht

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type transaction interface {
	ID() string
	Timeout(dht *dhtCore) bool
	OnLaunch(dht *dhtCore)
	OnResponse(dht *dhtCore, nd *node, resp map[string]interface{})
	OnFinish(dht *dhtCore)
}

type transactionContext struct {
	lock        sync.RWMutex
	transaction transaction
	time        time.Time
}

func (context *transactionContext) Fresh() {
	context.lock.Lock()
	defer context.lock.Unlock()
	context.time = time.Now()
}

func (context *transactionContext) Expire(expiration time.Duration) bool {
	context.lock.RLock()
	defer context.lock.RUnlock()
	return time.Now().Sub(context.time) > expiration
}

type transactionManager struct {
	lock         sync.RWMutex
	dht          *dhtCore
	transactions map[string]*transactionContext

	Expiration  time.Duration
	CleanPeriod time.Duration
}

func newTransactionManager(dht *dhtCore) *transactionManager {
	return &transactionManager{
		dht:          dht,
		transactions: make(map[string]*transactionContext),

		Expiration:  time.Second * 30,
		CleanPeriod: time.Second * 10,
	}
}

func (manager *transactionManager) Add(t transaction) error {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	if _, ok := manager.transactions[t.ID()]; ok {
		return errors.New("transation id is already exist")
	}

	manager.transactions[t.ID()] = &transactionContext{transaction: t, time: time.Now()}
	if manager.dht.conn != nil {
		t.OnLaunch(manager.dht)
	}

	return nil
}

func (manager *transactionManager) remove(transactionID string) {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	delete(manager.transactions, transactionID)
}

func (manager *transactionManager) PeriodicClean() {
	tick := time.Tick(manager.CleanPeriod)
	for range tick {
		manager.lock.RLock()
		for id, ctx := range manager.transactions {
			if ctx.Expire(manager.Expiration) {
				manager.lock.RUnlock()
				manager.remove(id)
				manager.lock.RLock()
			}
		}
		manager.lock.RUnlock()
	}
}

func (manager *transactionManager) launchAll() {
	manager.lock.RLock()
	defer manager.lock.RUnlock()

	for _, context := range manager.transactions {
		context.transaction.OnLaunch(manager.dht)
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
