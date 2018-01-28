package main

import (
	"fmt"
	"time"

	"github.com/deckarep/golang-set"
	"github.com/neoql/btlet/dht"
)

var s mapset.Set

func main() {
	s = mapset.NewSet()

	c := dht.NewCrawler()
	c.Run()

	fmt.Println("Start crawl ...")

	go statistic()
	for r := range c.ResultChan() {
		fmt.Printf("%x\n", r.InfoHash)
		s.Add(r.InfoHash)
	}
}

func statistic() {
	total := 0
	for range time.Tick(time.Minute) {
		num := s.Cardinality()
		fmt.Printf("There are %d new hashes crawled last minute, total is % d\n", num-total, num)
		total = num
	}
}
