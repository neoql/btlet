package btlet

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/neoql/btlet/bt"
	"github.com/neoql/btlet/dht"
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
	crawler dht.Crawler
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

		fctx, cancel := context.WithTimeout(ctx, time.Minute*5)
		defer cancel()
		meta, err := bt.FetchMetadata(fctx, infoHash, fmt.Sprintf("%s:%d", ip, port))
		if err != nil {
			return
		}

		pipeline.DisposeMeta(infoHash, meta)
	})
}

// Stimulate will Stimulate the spider when Crawler is Stimulater and returns true.
// else do nothing and returns false.
func (spi *Spider) Stimulate() bool {
	stimulater, ok := spi.crawler.(dht.Stimulater)
	if ok {
		stimulater.Stimulate()
	}
	return ok
}
