package btlet

import (
	"fmt"
	"net"
	"context"

	"github.com/neoql/btlet/bt"
	"github.com/neoql/btlet/dht"
)

// File contains info of file
type File struct {
	Path string
	Size int
}

// Meta is metadata of the torrent
type Meta struct {
	Hash  string
	Name  string
	Size  int
	Files []File
}

// Pipeline is used for handle meta
type Pipeline interface {
	HandleMeta(Meta)
}

// Sniffer can crawl Meta from dht.
type Sniffer struct {
	IP       string
	Port     int16
	pipeline Pipeline
}

// NewSniffer returns a new Sniffer instance.
func NewSniffer(p Pipeline) *Sniffer {
	return &Sniffer{
		IP:       "0.0.0.0",
		Port:     6881,
		pipeline: p,
	}
}

// Run will launch the sniffer
func (sniffer *Sniffer) Run() error {
	crawler := dht.NewSybilCrawler(sniffer.IP, int(sniffer.Port))
	return crawler.Crawl(context.TODO(), sniffer.onCrawInfohash)
}

func (sniffer *Sniffer) onCrawInfohash(infoHash string, ip net.IP, port int) {
	address := fmt.Sprintf("%s:%d", ip, port)
	meta, _ := bt.FetchMetadata(infoHash, address)
	if sniffer.pipeline != nil {
		sniffer.pipeline.HandleMeta(loadMeta(meta, infoHash))
	}
}

func loadMeta(info map[string]interface{}, hash string) Meta {
	name := info["name"].(string)
	if l, ok := info["length"]; ok {
		size := l.(int)
		files := make([]File, 1)
		files[0] = File{name, size}
		return Meta{
			Hash:  hash,
			Name:  name,
			Size:  size,
			Files: files,
		}
	}

	fs := info["files"].([]interface{})
	files := make([]File, len(fs))
	size := 0
	for i, f := range fs {
		file := f.(map[string]interface{})
		path := joinPath(file["path"].([]interface{}), "/")
		length := file["length"].(int)
		files[i] = File{path, length}
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

// HandleMeta will handle meta
func (p *SimplePipeline) HandleMeta(meta Meta) {
	p.ch <- meta
}

// MetaChan returns a Meta channel
func (p *SimplePipeline) MetaChan() <-chan Meta {
	return p.ch
}
