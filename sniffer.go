package btlet

import (
	"context"
	"fmt"
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
	crawler  dht.Crawler
	pipeline Pipeline
	ctx      context.Context
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
	return sniffer.crawler.Crawl(ctx, sniffer.afterCrawl)
}

func (sniffer *Sniffer) afterCrawl(infoHash string, ip net.IP, port int) {
	defer recover()
	// connect to peer
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), time.Second*15)
	if err != nil {
		return
	}
	defer conn.Close()

	session := bt.NewSession(conn)

	// handshake
	var reserved uint64
	bt.SetExtReserved(&reserved)
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

	if !bt.CheckExtReserved(opt.Reserved) {
		// not support extensions
		return
	}

	sender := session.MessageSender()

	fmExt := bt.NewFetchMetaExt(infoHash)
	center := bt.NewExtCenter()
	center.RegistExt(fmExt)

	err = center.SendHS(sender)
	if err != nil {
		return
	}

	for {
		message, err := session.NextMessage()
		if err != nil {
			// fmt.Println(err)
			return
		}

		if len(message) == 0 || message[0] != bt.ExtID {
			continue
		}

		err = center.HandlePayload(message[1:], sender)
		if err != nil {
			// fmt.Println(err)
			return
		}

		if fmExt.CheckDone() {
			meta, err := fmExt.FetchRawMeta()
			if err == nil && sniffer.pipeline != nil {
				sniffer.pipeline.DisposeMeta(infoHash, meta)
			}
			break
		}
	}
}
