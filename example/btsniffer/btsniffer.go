package main

import (
	"fmt"
	"time"

	"github.com/neoql/btlet"
)

func main() {
	p := btlet.NewSimplePipelineWithBuf(512)
	s := btlet.NewSniffer(p)
	s.Run()

	total := 0
	go statistic(&total)
	fmt.Println("Start crawl ...")
	for meta := range p.MetaChan() {
		fmt.Println("--------------------------------------------------------")
		fmt.Printf("magnet:?xt=urn:btih:%x\n", meta.Hash)
		fmt.Printf("name: %s\n", meta.Name)
		fmt.Printf("size: %s\n", getSize(meta.Size))
		for _, f := range meta.Files {
			fmt.Println(f.Path)
		}
		total++
	}
}

func statistic(total *int) {
	last := 0
	for range time.Tick(time.Minute) {
		sub := *total - last
		last = *total
		fmt.Println("=========================================")
		fmt.Printf("Crawled %d meta last minute, total is %d\n", sub, last)
		fmt.Println("=========================================")
	}
}

func getSize(size int) string {
	if size > 1024*1024*1024 {
		return fmt.Sprintf("%.2fGB", float64(size)/(1024*1024*1024))
	}

	if size > 1024*1024 {
		return fmt.Sprintf("%.2fMB", float64(size)/(1024*1024))
	}

	if size > 1024 {
		return fmt.Sprintf("%.2fKB", float64(size)/(1024))
	}

	return fmt.Sprintf("%dB", size)
}
