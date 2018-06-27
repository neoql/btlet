package main

import (
	"os"
	"fmt"
	"time"
	"context"

	"github.com/neoql/btlet"
)

func main() {
	builder := btlet.NewSnifferBuilder()
	p := btlet.NewSimplePipelineWithBuf(512)
	s := builder.NewSniffer(p)
	go s.Sniff(context.TODO())

	total := 0
	go statistic(&total)
	fmt.Println("Start crawl ...")
	for range p.MetaChan() {
		total++
		os.Stdout.WriteString(fmt.Sprintf("\rHave already sniff %d torrents.", total))
	}
}

func statistic(total *int) {
	last := 0
	for range time.Tick(time.Minute) {
		t := *total
		sub := t - last
		last = t
		fmt.Printf("\rSniffed %d torrents last minute.\n", sub)
	}
}
