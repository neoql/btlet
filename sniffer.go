package btlet

import (
	"context"
	"fmt"
	"net"

	"github.com/neoql/btlet/bt"
	"github.com/neoql/btlet/dht"
)

// Meta is metadata of the torrent
type Meta struct {
	Hash  string
	Name  string
	Size  int
	Files []struct {
		Path string
		Size int
	}
}

// Pipeline is used for handle meta
type Pipeline interface {
	DisposeMeta(Meta)
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
		Port: 6881,
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
	return sniffer.crawler.Crawl(ctx, func(infoHash string, ip net.IP, port int) {
		address := fmt.Sprintf("%s:%d", ip, port)
		meta, _ := bt.FetchMetadata(infoHash, address)
		if sniffer.pipeline != nil {
			sniffer.pipeline.DisposeMeta(loadMeta(meta, infoHash))
		}
	})
}

func loadMeta(info map[string]interface{}, hash string) Meta {
	name := info["name"].(string)
	if l, ok := info["length"]; ok {
		size := l.(int)
		files := make([]struct {
			Path string
			Size int
		}, 1)
		files[0] = struct {
			Path string
			Size int
		}{name, size}
		return Meta{
			Hash:  hash,
			Name:  name,
			Size:  size,
			Files: files,
		}
	}

	fs := info["files"].([]interface{})
	files := make([]struct {
		Path string
		Size int
	}, len(fs))
	size := 0
	for i, f := range fs {
		file := f.(map[string]interface{})
		path := joinPath(file["path"].([]interface{}), "/")
		length := file["length"].(int)
		files[i] = struct {
			Path string
			Size int
		}{path, length}
		size += length
	}

	return Meta{
		Hash:  hash,
		Name:  name,
		Size:  size,
		Files: files,
	}
}

func joinPath(a []interface{}, sep string) string {
	switch len(a) {
	case 0:
		return ""
	case 1:
		return a[0].(string)
	case 2:
		// Special case for common small values.
		// Remove if golang.org/issue/6714 is fixed
		return a[0].(string) + sep + a[1].(string)
	case 3:
		// Special case for common small values.
		// Remove if golang.org/issue/6714 is fixed
		return a[0].(string) + sep + a[1].(string) + sep + a[2].(string)
	}
	n := len(sep) * (len(a) - 1)
	for i := 0; i < len(a); i++ {
		n += len(a[i].(string))
	}

	b := make([]byte, n)
	bp := copy(b, a[0].(string))
	for _, s := range a[1:] {
		bp += copy(b[bp:], sep)
		bp += copy(b[bp:], s.(string))
	}
	return string(b)
}

// SimplePipeline is a simple pipeline
type SimplePipeline struct {
	ch chan Meta
}

// NewSimplePipeline will returns a simple pipline
func NewSimplePipeline() *SimplePipeline {
	return &SimplePipeline{
		ch: make(chan Meta),
	}
}

// NewSimplePipelineWithBuf will returns a simple pipline with buffer
func NewSimplePipelineWithBuf(bufSize int) *SimplePipeline {
	return &SimplePipeline{
		ch: make(chan Meta, bufSize),
	}
}

// DisposeMeta will handle meta
func (p *SimplePipeline) DisposeMeta(meta Meta) {
	p.ch <- meta
}

// MetaChan returns a Meta channel
func (p *SimplePipeline) MetaChan() <-chan Meta {
	return p.ch
}
