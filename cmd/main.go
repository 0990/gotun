package main

import (
	"flag"
	"fmt"
	"github.com/0990/gotun"
	"github.com/0990/gotun/server/httpproxy"
	"github.com/0990/gotun/server/socks5client"
	"github.com/sirupsen/logrus"
	"time"
)

var cfg = flag.String("config", "app.yaml", "config file")
var tunDir = flag.String("tun_dir", "tunnel", "tun dir")
var checkSocks5 = flag.String("check_socks5", "", "socks5 check addr")
var checkHttpProxy = flag.String("check_httpproxy", "", "httpproxy check addr")

func main() {
	flag.Parse()

	if *checkSocks5 != "" {
		socks5client.CheckServer(*checkSocks5)
		return
	}

	if *checkHttpProxy != "" {
		resp, err := httpproxy.Check(*checkHttpProxy, time.Second*2)
		if err != nil {
			fmt.Printf("failed:%s \n", err.Error())
		} else {
			fmt.Printf("passed,response(ipinfo.io):%s \n", resp)
		}
		return
	}

	err := gotun.Run(*cfg, *tunDir)
	if err != nil {
		logrus.WithError(err).Error("gotun run")
	}
}
