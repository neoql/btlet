# btlet

通过实现BT协议族中的部分协议来完成一些toolkit

* [DHT-Spider](./README.md#dht-spider)

## Install

```
$ go get -v github.com/neoql/btlet
```

## Dependences

* [willf/bloom](https://github.com/willf/bloom)

## Modules

### DHT-Spider

运行截图

![](./screenshot/btsniffer.png)

> Example

下面是一个简单的爬虫例子，[这里](./example/btsniffer)是完整的Demo

```go
package main

import (
    "fmt"
    "github.com/neoql/btlet"
)

func main() {
    builder := btlet.NewSnifferBuilder()
	p := btlet.NewSimplePipeline()
	s := builder.NewSniffer(p)
	go s.Sniff(context.TODO())
    
    for meta := range p.MetaChan() {
        fmt.Println(meta)
    }
}
```

## License

MIT, read more [here](./LICENSE).
