package btlet

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"net"
	"time"
	"os"

	"github.com/neoql/btlet/bencode"
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
	extHs := map[string]interface{}{
		"m": map[string]int{"ut_metadata": 1},
	}

	raw, _ := bencode.Marshal(extHs)
	msg := bytes.Join([][]byte{[]byte{0}, raw}, nil)
	err = sender.SendShortMessage(bt.ExtID, msg)
	if err != nil {
		return
	}

	var pieces [][]byte

loop:
	for {
		payload, err := session.NextPayload()
		if err != nil {
			return
		}

		if len(payload) == 0 || payload[0] != bt.ExtID {
			continue
		}

		if payload[1] == 0 {
			var msg map[string]bencode.RawMessage

			err := bencode.Unmarshal(payload[2:], &msg)
			if err != nil {
				return
			}

			var m map[string]byte
			err = bencode.Unmarshal(msg["m"], &m)
			if err != nil {
				return
			}

			var size int64
			_, ok := msg["metadata_size"]
			if !ok {
				return
			}
			err = bencode.Unmarshal(msg["metadata_size"], &size)
			if err != nil {
				return
			}

			id := m["ut_metadata"]

			piecesNum := size / (16 * 1024)
			if size%(16*1024) != 0 {
				piecesNum++
			}

			go func() {
				for i := 0; i < int(piecesNum); i++ {
					content, _ := bencode.Marshal(map[string]int{
						"msg_type": 0,
						"piece":    i,
					})
					err := sender.SendShortMessage(bt.ExtID, bytes.Join([][]byte{[]byte{id}, content}, nil))
					if err != nil {
						return
					}
				}
			}()
			os.Stderr.WriteString(fmt.Sprintln(piecesNum))
			if piecesNum < 0 {
				return
			}
			pieces = make([][]byte, piecesNum)
		} else {
			if pieces == nil {
				return
			}

			var msg map[string]int
			content := payload[2:]
			dec := bencode.NewDecoder(bytes.NewReader(content))
			err := dec.Decode(&msg)
			if err != nil {
				return
			}

			if msg["msg_type"] != 1 {
				continue
			}

			excess := content[dec.BytesParsed():]
			pieces[msg["piece"]] = excess

			if checkPiecesDone(pieces) {
				metadata := bytes.Join(pieces, nil)
				if checkMetadata(metadata, infoHash) && sniffer.pipeline != nil {
					defer sniffer.pipeline.DisposeMeta(infoHash, metadata)
					break loop
				}
			}
		}
	}
}

func checkMetadata(metadata []byte, infoHash string) bool {
	hash := sha1.Sum(metadata)
	return bytes.Equal(hash[:], []byte(infoHash))
}

func checkPiecesDone(pieces [][]byte) bool {
	for _, piece := range pieces {
		if len(piece) == 0 {
			return false
		}
	}
	return true
}
