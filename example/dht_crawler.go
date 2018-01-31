package main

import (
	"fmt"
	"time"

	"github.com/neoql/btlet/bt"
	"github.com/neoql/btlet/dht"

	"github.com/deckarep/golang-set"
)

func main() {
	c := dht.NewCrawler()
	s := mapset.NewSet()

	c.Run()
	fmt.Println("Start crawl ...")
	ch := c.ResultChan()

	for i := 0; i < 64; i++ {
		go func() {
			for r := range ch {
				if s.Contains(r.InfoHash) {
					continue
				}
				address := fmt.Sprintf("%s:%d", r.PeerIP, r.PeerPort)
				meta, err := bt.FetchMetadata(r.InfoHash, address)
				if err != nil {
					continue
				} else {
					s.Add(r.InfoHash)
					fmt.Println(meta["name"])
				}
			}
		}()
	}

	total := 0
	for range time.Tick(time.Minute) {
		num := s.Cardinality()
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf("Crawled %d magnet last minute\n", num-total)
		fmt.Println("----------------------------------------------------------------")
		total = num
	}
}
