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
	OnError(code int, describe string)
}

type transactionBox struct {
	transaction Transaction
	alive       chan struct{}
	end         chan struct{}
}

func newTransactionBox(t Transaction) *transactionBox {
	return &transactionBox{
		transaction: t,
		alive:       make(chan struct{}),
		end:         make(chan struct{}),
	}
}

func (box *transactionBox) keepAlive() {
	select {
	case box.alive <- struct{}{}:
	default:
	}
}

func (box *transactionBox) done() {
	box.end <- struct{}{}
}

func (box *transactionBox) loop(handle Handle, rmself func()) {
	box.transaction.OnLaunch(handle)

LOOP:
	for {
		timeout := time.NewTimer(box.transaction.ShelfLife())
		select {
		case <-box.alive:
			timeout.Stop()
		case <-timeout.C:
			if box.transaction.OnTimeout(handle) {
				rmself()
				break LOOP
			}
		case <-box.end:
			rmself()
			break LOOP
		}
	}
}

// TransactionDispatcher can dispatch transactions
type TransactionDispatcher struct {
	handle       Handle
	transactions *sync.Map
}

// NewTransactionDispatcher creates a new TransactionDispatcher
func NewTransactionDispatcher(handle Handle) *TransactionDispatcher {
	return &TransactionDispatcher{
		handle:       handle,
		transactions: new(sync.Map),
	}
}

// Add the transaction into dispathcher and launch it
func (dispatcher *TransactionDispatcher) Add(t Transaction) error {
	box := newTransactionBox(t)
	_, ok := dispatcher.transactions.LoadOrStore(t.ID(), box)

	if ok {
		return errors.New("transation id is already exist")
	}

	go box.loop(dispatcher.handle, dispatcher.mkRemoveCallback(t))

	return nil
}

func (dispatcher *TransactionDispatcher) remove(transactionID string) {
	dispatcher.transactions.Delete(transactionID)
}

// Remove the transaction from the dispatcher.
func (dispatcher *TransactionDispatcher) Remove(transactionID string) {
	box, ok := dispatcher.transactions.Load(transactionID)
	if ok {
		box.(*transactionBox).done()
	}
}

func (dispatcher *TransactionDispatcher) mkRemoveCallback(t Transaction) func() {
	return func() {
		dispatcher.remove(t.ID())
	}
}

// DisposeResponse dispose response.
func (dispatcher *TransactionDispatcher) DisposeResponse(transactionID string, nd *Node, resp map[string]interface{}) {
	v, ok := dispatcher.transactions.Load(transactionID)
	if ok {
		box := v.(*transactionBox)
		box.keepAlive()
		box.transaction.OnResponse(dispatcher.handle, nd, resp)
	}
	return
}

// DisposeError dispose error
func (dispatcher *TransactionDispatcher) DisposeError(transactionID string, code int, describe string) {
	v, ok := dispatcher.transactions.Load(transactionID)
	if ok {
		box := v.(*transactionBox)
		box.keepAlive()
		box.transaction.OnError(code, describe)
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
