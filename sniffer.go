package btlet

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/neoql/btlet/bt"
	"github.com/neoql/btlet/dht"
	"github.com/neoql/btlet/tools"
)

// Pipeline is used for handle meta
type Pipeline interface {
	DisposeMeta(string, bt.RawMeta)
	PullTrackerList(string) ([]string, bool)
}

// SniffMode is the mode to sniff infohash
type SniffMode int

const (
	// SybilMode is sybil mode
	SybilMode = SniffMode(iota)
)

// SnifferBuilder can build Sniffer
type SnifferBuilder struct {
	IP         string
	Port       int
	Mode       SniffMode
	MaxWorkers int
}

// NewSnifferBuilder returns a new SnifferBuilder with default config
func NewSnifferBuilder() *SnifferBuilder {
	return &SnifferBuilder{
		IP:   "0.0.0.0",
		Port: 7878,
		Mode: SybilMode,
	}
}

// NewSniffer returns a Sniffer with the builder's config.
// If Mode is unknow will return nil
func (builder *SnifferBuilder) NewSniffer(p Pipeline) *Sniffer {
	var crawler dht.Crawler
	switch builder.Mode {
	case SybilMode:
		c := dht.NewSybilCrawler(builder.IP, builder.Port)
		c.SetMaxWorkers(builder.MaxWorkers)
		crawler = c
	default:
		return nil
	}
	return NewSniffer(crawler, p)
}

// Sniffer can crawl Meta from dht.
type Sniffer struct {
	crawler   dht.Crawler
	pipeline  Pipeline
	ctx       context.Context
	extCenter *bt.ExtCenter
}

// NewSniffer returns a Sniffer
func NewSniffer(c dht.Crawler, p Pipeline) *Sniffer {
	return &Sniffer{
		crawler:  c,
		pipeline: p,
	}
}

// Sniff starts sniff meta
func (sniffer *Sniffer) Sniff(ctx context.Context) error {
	sniffer.ctx = ctx

	// regist extesions
	sniffer.extCenter = bt.NewExtCenter()
	sniffer.extCenter.RegistExt(bt.GenFetchMetaExt)

	return sniffer.crawler.Crawl(ctx, sniffer.afterCrawl)
}

func (sniffer *Sniffer) afterCrawl(infoHash string, ip net.IP, port int) {
	// connect to peer
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), time.Second*15)
	if err != nil {
		return
	}
	defer conn.Close()

	session := bt.NewSession(conn)

	// handshake
	var reserved uint64
	sniffer.extCenter.SetReservedBit(&reserved)
	opt, err := session.Handshake(&bt.HSOption{
		Reserved: reserved,
		InfoHash: infoHash,
		PeerID:   tools.RandomString(20),
	})

	if err != nil {
		return
	}

	// peer send different info_hash
	if opt.InfoHash != infoHash {
		return 
	}

	esession := sniffer.extCenter.NewExtSession(opt)
	if esession == nil {
		// not support extensions
		return
	}

	mtCh := make(chan bt.RawMeta)
	exit := make(chan bool, 1)

	esession.RangeExts(func(ext bt.Extension) bool {
		switch e := ext.(type) {
		case *bt.FetchMetaExt:
			e.Ch = mtCh
		default:
		}
		return true
	})

	// send extension handshake.(only send, have not recieve)
	err = esession.SendHS(session.MessageSender())
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(sniffer.ctx)
	defer cancel()
	go func() {
		err := session.Loop(ctx, func(id byte, r io.Reader, sender *bt.MessageSender) error {
			if id != bt.ExtID {
				return nil
			}
			return esession.HandleMessage(r, sender)
		})

		exit <- (err == nil)
	}()

	select {
	case rawmeta := <-mtCh:
		if sniffer.pipeline != nil {
			sniffer.pipeline.DisposeMeta(infoHash, rawmeta)
		}
	case <-exit:
	}
}
