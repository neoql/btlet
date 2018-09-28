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
	DisposeMetaAndTracker(string, bt.RawMeta, []string)
	Has(string) bool
	PullTrackerList(string) ([]string, bool)
	AppendTracker(string, []string)
}

// SniffMode is the mode to sniff infohash
type SniffMode int

const (
	// SybilMode is sybil mode
	SybilMode = SniffMode(iota)
)

// SnifferBuilder can build Sniffer
type SnifferBuilder struct {
	IP           string
	Port         int
	Mode         SniffMode
	MaxWorkers   int
	FetchTracker bool
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
	return &Sniffer{
		pipeline:  p,
		crawler:   crawler,
		enableTex: builder.FetchTracker,
	}
}

// Sniffer can crawl Meta from dht.
type Sniffer struct {
	crawler   dht.Crawler
	pipeline  Pipeline
	enableTex bool
}

// NewSniffer returns a Sniffer
func NewSniffer(c dht.Crawler, p Pipeline) *Sniffer {
	return &Sniffer{
		crawler:  c,
		pipeline: p,
	}
}

// EnableFetchTracker makes sniffer fetch tracker
func (sniffer *Sniffer) EnableFetchTracker() {
	sniffer.enableTex = true
}

// Sniff starts sniff meta
func (sniffer *Sniffer) Sniff(ctx context.Context) error {
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

	center := bt.NewExtCenter()
	var fmExt *bt.FetchMetaExt
	var txExt *bt.TexExtension

	if !sniffer.enableTex {
		if sniffer.pipeline.Has(infoHash) {
			return
		}

		fmExt = bt.NewFetchMetaExt(infoHash)
		center.RegistExt(fmExt)
	} else {
		trlist, ok := sniffer.pipeline.PullTrackerList(infoHash)
		txExt = bt.NewTexExtension(trlist)
		center.RegistExt(txExt)
		if !ok {
			fmExt = bt.NewFetchMetaExt(infoHash)
			center.RegistExt(fmExt)
		}
	}

	err = center.SendHS(sender)
	if err != nil {
		return
	}

	var rawmeta []byte
	var fmdone bool
	for {
		message, err := session.NextMessage()
		if err != nil {
			return
		}

		if len(message) == 0 || message[0] != bt.ExtID {
			continue
		}

		err = center.HandlePayload(message[1:], sender)
		if err != nil {
			return
		}

		if !sniffer.enableTex {
			if !fmExt.IsSupoort() {
				return
			}

			if fmExt.CheckDone() {
				meta, err := fmExt.FetchRawMeta()
				if err == nil && sniffer.pipeline != nil {
					sniffer.pipeline.DisposeMeta(infoHash, meta)
				}
				break
			}
		} else {
			if fmExt != nil && fmExt.IsSupoort() {
				if !txExt.IsSupoort() {
					if fmExt.CheckDone() {
						meta, err := fmExt.FetchRawMeta()
						if err == nil && sniffer.pipeline != nil {
							sniffer.pipeline.DisposeMeta(infoHash, meta)
						}
						break
					}
				} else {
					if !fmdone && fmExt.CheckDone() {
						fmdone = true
						meta, err := fmExt.FetchRawMeta()
						if err != nil {
							rawmeta = meta
						}
					}

					if fmdone && !txExt.More() {
						trlist := txExt.TrackerList()
						if len(trlist) == 0 {
							if rawmeta != nil {
								sniffer.pipeline.DisposeMeta(infoHash, rawmeta)
							}
						} else {
							if rawmeta != nil {
								sniffer.pipeline.DisposeMetaAndTracker(infoHash, rawmeta, trlist)
							} else {
								sniffer.pipeline.AppendTracker(infoHash, trlist)
							}
						}
						break
					}
				}
			} else {
				if !txExt.IsSupoort() {
					return
				}
				if !txExt.More() {
					trlist := txExt.TrackerList()
					if len(trlist) != 0 {
						sniffer.pipeline.AppendTracker(infoHash, trlist)
					}
					break
				}
			}
		}
	}
}
