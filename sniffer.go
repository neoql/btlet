package btlet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/neoql/btlet/bt"
	"github.com/neoql/btlet/dht"
	"github.com/neoql/btlet/tools"
)

// Pipeline is used for handle meta
type Pipeline interface {
	Has(string) bool
	DisposeMeta(string, bt.RawMeta)
}

// PipelineX used for spider.RunAndFetchTracker
type PipelineX interface {
	Pipeline
	DisposeMetaAndTracker(string, bt.RawMeta, []string)
	PullTrackerList(string) ([]string, bool)
	AppendTracker(string, []string)
}

// Spider can crawl Meta from dht.
type Spider struct {
	crawler   dht.Crawler
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
	spi.Use(dht.NewSybilCrawler("0.0.0.0:6881"))
}

// LimitSybilCrawler will use a dht.SybilCrawler
func (spi *Spider) LimitSybilCrawler(limit int) {
	c := dht.NewSybilCrawler("0.0.0.0:6881")
	c.SetMaxWorkers(limit)
	spi.Use(c)
}

// Run starts sniff meta
func (spi *Spider) Run(ctx context.Context, pipeline Pipeline) error {
	if spi.crawler == nil {
		spi.UseSybilCrawler()
	}

	return spi.crawler.Crawl(ctx, func(infoHash string, ip net.IP, port int) {
		if pipeline.Has(infoHash) {
			return
		}

		meta, err := bt.FetchMetadata(infoHash, fmt.Sprintf("%s:%d", ip, port))
		if err != nil {
			return
		}

		pipeline.DisposeMeta(infoHash, meta)
	})
}

// RunAndFetchTracker run and fetch tracker use tex protocal
func (spi *Spider) RunAndFetchTracker(ctx context.Context, pipeline PipelineX) error {
	if spi.crawler == nil {
		spi.UseSybilCrawler()
	}

	return spi.crawler.Crawl(ctx, func(infoHash string, ip net.IP, port int) {
		// connect to peer
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), time.Second*15)
		if err != nil {
			return
		}
		defer conn.Close()

		session := bt.NewSession(conn)

		mtf := &metaTrackFetcher{
			infoHash: infoHash,
			sess:     session,
			pipe:     pipeline,
		}

		err = mtf.Handshake()
		if err != nil {
			return
		}

		mtf.RegistExts()
		err = mtf.MainLoop()
		if err != nil {
			return
		}
	})
}

type metaTrackFetcher struct {
	infoHash string
	sess     *bt.Session
	pipe     PipelineX

	extCenter *bt.ExtCenter
	fmExt     *bt.FetchMetaExt
	txExt     *bt.TexExtension
	rawmeta   []byte
	fmdone    bool
}

func (mtf *metaTrackFetcher) Handshake() error {
	var reserved uint64
	bt.SetExtReserved(&reserved)
	opt, err := mtf.sess.Handshake(&bt.HSOption{
		Reserved: reserved,
		InfoHash: mtf.infoHash,
		PeerID:   tools.RandomString(20),
	})

	if err != nil {
		return err
	}

	// peer send different info_hash
	if opt.InfoHash != mtf.infoHash {
		return errors.New("handshake failed: different info_hash")
	}

	if !bt.CheckExtReserved(opt.Reserved) {
		// not support extensions
		return errors.New("not support extensions")
	}

	return nil
}

func (mtf *metaTrackFetcher) RegistExts() {
	mtf.extCenter = bt.NewExtCenter()
	trlist, ok := mtf.pipe.PullTrackerList(mtf.infoHash)
	mtf.txExt = bt.NewTexExtension(trlist)
	mtf.extCenter.RegistExt(mtf.txExt)
	if !ok {
		mtf.fmExt = bt.NewFetchMetaExt(mtf.infoHash)
		mtf.extCenter.RegistExt(mtf.fmExt)
	}
}

func (mtf *metaTrackFetcher) MainLoop() error {
	sender := mtf.sess.MessageSender()
	err := mtf.extCenter.SendHS(sender)
	if err != nil {
		return err
	}

	for {
		message, err := mtf.sess.NextMessage()
		if err != nil {
			return err
		}

		if len(message) == 0 || message[0] != bt.ExtID {
			continue
		}

		err = mtf.extCenter.HandlePayload(message[1:], sender)
		if err != nil {
			return err
		}

		if mtf.fmExt != nil && mtf.fmExt.IsSupoort() {
			if !mtf.txExt.IsSupoort() {
				if !mtf.onlyMeta() {
					break
				}
			} else {
				if !mtf.metaAndTracker() {
					break
				}
			}
		} else {
			if !mtf.txExt.IsSupoort() {
				return errors.New("not support tex")
			}
			if !mtf.onlyTracker() {
				break
			}
		}
	}

	return nil
}

func (mtf *metaTrackFetcher) onlyMeta() bool {
	if mtf.fmExt.CheckDone() {
		meta, err := mtf.fmExt.FetchRawMeta()
		if err == nil && mtf.pipe != nil {
			mtf.pipe.DisposeMeta(mtf.infoHash, meta)
		}
		return false
	}
	return true
}

func (mtf *metaTrackFetcher) onlyTracker() bool {
	if !mtf.txExt.More() {
		trlist := mtf.txExt.TrackerList()
		if len(trlist) != 0 {
			mtf.pipe.AppendTracker(mtf.infoHash, trlist)
		}
		return false
	}
	return true
}

func (mtf *metaTrackFetcher) metaAndTracker() bool {
	if !mtf.fmdone && mtf.fmExt.CheckDone() {
		mtf.fmdone = true
		meta, err := mtf.fmExt.FetchRawMeta()
		if err != nil {
			mtf.rawmeta = meta
		}
	}

	if mtf.fmdone && !mtf.txExt.More() {
		trlist := mtf.txExt.TrackerList()
		if len(trlist) == 0 {
			if mtf.rawmeta != nil {
				mtf.pipe.DisposeMeta(mtf.infoHash, mtf.rawmeta)
			}
		} else {
			if mtf.rawmeta != nil {
				mtf.pipe.DisposeMetaAndTracker(mtf.infoHash, mtf.rawmeta, trlist)
			} else {
				mtf.pipe.AppendTracker(mtf.infoHash, trlist)
			}
		}
		return false
	}
	return true
}
