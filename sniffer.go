package btlet

import (
	"fmt"

	"github.com/neoql/btlet/bt"
	"github.com/neoql/btlet/dht"
	"github.com/neoql/container/queue"
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

// Sniffer can crawl Meta from dht.
type Sniffer struct {
	metaBuffer *queue.Queue

	IP                string
	Port              int16
	CrawlWorkersTotal int
	FetchWorkersTotal int
}

// NewSniffer returns a new Sniffer instance.
func NewSniffer() *Sniffer {
	return &Sniffer{
		metaBuffer: queue.New(),

		IP:                "0.0.0.0",
		Port:              6881,
		CrawlWorkersTotal: 32,
		FetchWorkersTotal: 64,
	}
}

// Run will launch the sniffer
func (sniffer *Sniffer) Run() error {
	crawler := dht.NewCrawler()
	crawler.SetWorkdersTotal(sniffer.CrawlWorkersTotal)
	crawler.SetAddress(sniffer.IP, sniffer.Port)

	for i := 0; i < sniffer.FetchWorkersTotal; i++ {
		go func() {
			for r := range crawler.ResultChan() {
				address := fmt.Sprintf("%s:%d", r.PeerIP, r.PeerPort)
				meta, err := bt.FetchMetadata(r.InfoHash, address)
				if err != nil {
					continue
				}
				sniffer.metaBuffer.Put(loadMeta(meta, r.InfoHash))
			}
		}()
	}

	return crawler.Run()
}

// MetaChan returns a Meta channel
func (sniffer *Sniffer) MetaChan() chan Meta {
	ch := make(chan Meta)
	go func() {
		for {
			meta, flag := sniffer.metaBuffer.Pop()
			if !flag {
				close(ch)
				break
			}
			ch <- meta.(Meta)
		}
	}()
	return ch
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
