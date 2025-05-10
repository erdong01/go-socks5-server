package main

import (
	"github.com/gcslaoli/go-socks5-server/httpproxy"
	_ "go.uber.org/automaxprocs"
)

func main() {
	// start http proxy
	httpproxy.StartProxy()
}
