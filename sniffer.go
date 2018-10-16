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

// Spider can crawl Meta from dht.
type Spider struct {
	crawler   dht.Crawler
	pipeline  Pipeline
	enableTex bool
}

// NewSpider returns a Sniffer
func NewSpider() *Spider {
	return &Spider{}
}

// Use set which crawler will use.
func (spi *Spider) Use(crawler dht.Crawler) {
	spi.crawler = crawler
}

// UseSybilCrawler will use dht.SybilCrawler
func (spi *Spider) UseSybilCrawler() {
	spi.Use(dht.NewSybilCrawler("0.0.0.0", 7878))
}

// LimitSybilCrawler will use a dht.SybilCrawler
func (spi *Spider) LimitSybilCrawler(limit int) {
	c := dht.NewSybilCrawler("0.0.0.0", 7878)
	c.SetMaxWorkers(limit)
	spi.Use(c)
}

// EnableFetchTracker makes spider fetch tracker
func (spi *Spider) EnableFetchTracker() {
	spi.enableTex = true
}

// Run starts sniff meta
func (spi *Spider) Run(ctx context.Context, pipeline Pipeline) error {
	if spi.crawler == nil {
		spi.UseSybilCrawler()
	}

	spi.pipeline = pipeline
	return spi.crawler.Crawl(ctx, spi.afterCrawl)
}

func (spi *Spider) afterCrawl(infoHash string, ip net.IP, port int) {
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

	if !spi.enableTex {
		if spi.pipeline.Has(infoHash) {
			return
		}

		fmExt = bt.NewFetchMetaExt(infoHash)
		center.RegistExt(fmExt)
	} else {
		trlist, ok := spi.pipeline.PullTrackerList(infoHash)
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

		if !spi.enableTex {
			if !fmExt.IsSupoort() {
				return
			}

			if fmExt.CheckDone() {
				meta, err := fmExt.FetchRawMeta()
				if err == nil && spi.pipeline != nil {
					spi.pipeline.DisposeMeta(infoHash, meta)
				}
				break
			}
		} else {
			if fmExt != nil && fmExt.IsSupoort() {
				if !txExt.IsSupoort() {
					if fmExt.CheckDone() {
						meta, err := fmExt.FetchRawMeta()
						if err == nil && spi.pipeline != nil {
							spi.pipeline.DisposeMeta(infoHash, meta)
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
								spi.pipeline.DisposeMeta(infoHash, rawmeta)
							}
						} else {
							if rawmeta != nil {
								spi.pipeline.DisposeMetaAndTracker(infoHash, rawmeta, trlist)
							} else {
								spi.pipeline.AppendTracker(infoHash, trlist)
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
						spi.pipeline.AppendTracker(infoHash, trlist)
					}
					break
				}
			}
		}
	}
}
