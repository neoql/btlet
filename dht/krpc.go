package dht

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Handle used for operate dht.
type Handle interface {
	NodeID() string
	SendMessage(nd *Node, msg map[string]interface{}) error
}

// Transaction is KRPC transaction.
type Transaction interface {
	ID() string
	ShelfLife() time.Duration

	OnTimeout(handle Handle) bool
	OnLaunch(handle Handle)
	OnResponse(handle Handle, nd *Node, resp map[string]interface{})
	OnFinish(handle Handle)
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

func (context *transactionContext) loop(handle Handle, rmself func()) {
	context.transaction.OnLaunch(handle)

	for {
		timeout := time.NewTimer(context.transaction.ShelfLife())
		select {
		case <-context.alive:
			timeout.Stop()
		case <-timeout.C:
			if context.transaction.OnTimeout(handle) {
				context.transaction.OnFinish(handle)
				break
			}
		}
	}
}

type transactionManager struct {
	handle       Handle
	transactions *sync.Map
}

func newTransactionManager(handle Handle) *transactionManager {
	return &transactionManager{
		handle:       handle,
		transactions: new(sync.Map),
	}
}

func (manager *transactionManager) Add(t Transaction) error {
	context := newTransactionContext(t)
	_, ok := manager.transactions.LoadOrStore(t.ID(), context)

	if ok {
		return errors.New("transation id is already exist")
	}

	go context.loop(manager.handle, manager.mkRemoveCallback(t))

	return nil
}

func (manager *transactionManager) remove(transactionID string) {
	manager.transactions.Delete(transactionID)
}

func (manager *transactionManager) mkRemoveCallback(t Transaction) func() {
	return func() {
		manager.remove(t.ID())
	}
}

func (manager *transactionManager) launchAll() {
	manager.transactions.Range(func(k, v interface{}) bool {
		manager.letContextAlive(v.(*transactionContext))
		return true
	})
}

func (manager *transactionManager) letContextAlive(ctx *transactionContext) {
	go ctx.loop(manager.handle, manager.mkRemoveCallback(ctx.transaction))
}

func (manager *transactionManager) HandleResponse(transactionID string,
	nd *Node, resp map[string]interface{}) {

	v, ok := manager.transactions.Load(transactionID)
	if ok {
		context := v.(*transactionContext)
		context.Fresh()
		context.transaction.OnResponse(manager.handle, nd, resp)
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
