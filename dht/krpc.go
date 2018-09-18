package dht

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/neoql/btlet/bencode"
)

// Handle used for operate dht.
type Handle interface {
	NodeID() string
	SendMessage(dst *net.UDPAddr, msg interface{}) error
}

// Transaction is KRPC transaction.
type Transaction interface {
	ID() string
	ShelfLife() time.Duration

	OnLaunch(handle Handle)
	OnFinish(handle Handle)
	OnResponse(handle Handle, src *net.UDPAddr, resp bencode.RawMessage) bool
	OnError(handle Handle, src *net.UDPAddr, code int, describe string) bool
	OnTimeout(handle Handle) bool
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
			if !box.transaction.OnTimeout(handle) {
				rmself()
				break LOOP
			}
		case <-box.end:
			rmself()
			break LOOP
		}
	}
	box.transaction.OnFinish(handle)
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

	go box.loop(dispatcher.handle, dispatcher.mkRemoveSelf(t))

	return nil
}

// Remove the transaction from the dispatcher.
func (dispatcher *TransactionDispatcher) Remove(transactionID string) {
	box, ok := dispatcher.transactions.Load(transactionID)
	if ok {
		box.(*transactionBox).done()
	}
}

func (dispatcher *TransactionDispatcher) mkRemoveSelf(t Transaction) func() {
	return func() {
		dispatcher.transactions.Delete(t.ID())
	}
}

// DisposeResponse dispose response.
func (dispatcher *TransactionDispatcher) DisposeResponse(src *net.UDPAddr, transactionID string, resp bencode.RawMessage) {
	v, ok := dispatcher.transactions.Load(transactionID)
	if ok {
		box := v.(*transactionBox)
		box.keepAlive()
		if !box.transaction.OnResponse(dispatcher.handle, src, resp) {
			box.done()
		}
	}
	return
}

// DisposeError dispose error
func (dispatcher *TransactionDispatcher) DisposeError(src *net.UDPAddr, transactionID string, code int, describe string) {
	v, ok := dispatcher.transactions.Load(transactionID)
	if ok {
		box := v.(*transactionBox)
		box.keepAlive()
		if !box.transaction.OnError(dispatcher.handle, src, code, describe) {
			box.done()
		}
	}
	return
}

// Message is KRPC message
type Message struct {
	TransactionID string             `bencode:"t"`
	Y             string             `bencode:"y"`
	Q             string             `bencode:"q,omitempty"`
	Args          bencode.RawMessage `bencode:"a,omitempty"`
	Resp          bencode.RawMessage `bencode:"r,omitempty"`
	Err           *KRPCErr           `bencode:"e,omitempty"`
}

// PingArgs is ping query "a" field
type PingArgs struct {
	NodeID string `bencode:"id"`
}

// FindNodeArgs is find_node query "a" field
type FindNodeArgs struct {
	NodeID string `bencode:"id"`
	Target string `bencode:"target"`
}

// GetPeersArgs is get_peers query "a" field
type GetPeersArgs struct {
	NodeID   string `bencode:"id"`
	InfoHash string `bencode:"info_hash"`
}

// AnnouncePeerArgs is announce_peer query "a" field
type AnnouncePeerArgs struct {
	NodeID   string `bencode:"id"`
	InfoHash string `bencode:"info_hash"`
	Port     int    `bencode:"port"`
	Token    string `bencode:"token"`
}

// Response is reponse "r" field
type Response struct {
	NodeID string        `bencode:"id"`
	Nodes  *NodePtrSlice `bencode:"nodes,omitempty"`
	Token  string        `bencode:"token,omitempty"`
	Values []string      `bencode:"values,omitempty"`
}

// KRPCErr is krpc errors
type KRPCErr struct {
	code     int
	describe string
}

func (kerr *KRPCErr) Error() string {
	return fmt.Sprintf("KRPC error: (%d)%s", kerr.code, kerr.describe)
}

// Code returns code
func (kerr *KRPCErr) Code() int {
	return kerr.code
}

// Describe returns describe
func (kerr *KRPCErr) Describe() string {
	return kerr.describe
}

// UnmarshalBencode implements bencode.Unmarshaler
func (kerr *KRPCErr) UnmarshalBencode(b []byte) error {
	var e []interface{}
	err := bencode.Unmarshal(b, &e)
	if err != nil {
		return err
	}

	if len(e) < 2 {
		return &KRPCErr{203, "Irregular package"}
	}

	var ok bool
	var code int64

	code, ok = e[0].(int64)
	kerr.code = int(code)
	kerr.describe, ok = e[1].(string)

	if !ok {
		return &KRPCErr{203, "Irregular package"}
	}

	return nil
}

// MarshalBencode implements bencode.Marshaler
func (kerr KRPCErr) MarshalBencode() ([]byte, error) {
	e := []interface{}{kerr.code, kerr.describe}
	return bencode.Marshal(e)
}

// MakeQuery make a krpc query Message
func MakeQuery(transactionID string, args interface{}) (*Message, error) {
	var q string

	switch args.(type) {
	default:
		return nil, errors.New("Unknown args type")
	case *PingArgs, PingArgs:
		q = "ping"
	case *FindNodeArgs, FindNodeArgs:
		q = "find_node"
	case *GetPeersArgs, GetPeersArgs:
		q = "get_peers"
	case *AnnouncePeerArgs, AnnouncePeerArgs:
		q = "announce_peer"
	}

	return MakeQueryEx(q, transactionID, args)
}

// MakeQueryEx can make query optional q
func MakeQueryEx(q string, transactionID string, args interface{}) (*Message, error) {
	a, err := bencode.Marshal(args)
	if err != nil {
		return nil, err
	}

	return &Message{
		TransactionID: transactionID,
		Y:             "q",
		Q:             q,
		Args:          a,
	}, nil
}

// MakeResponse make a krpc resp Message
func MakeResponse(transactionID string, resp interface{}) (*Message, error) {
	r, err := bencode.Marshal(resp)
	if err != nil {
		return nil, err
	}

	return &Message{
		TransactionID: transactionID,
		Y:             "r",
		Resp:          r,
	}, nil
}

// MakeError make a krpc error Message
func MakeError(transactionID string, err *KRPCErr) (*Message, error) {
	return &Message{
		TransactionID: transactionID,
		Y:             "e",
		Err:           err,
	}, nil
}
