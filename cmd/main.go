package main

import (
	"flag"
	"github.com/0990/gotun"
	"github.com/0990/gotun/server/socks5client"
	"github.com/sirupsen/logrus"
)

var cfg = flag.String("config", "app.yaml", "config file")
var tunDir = flag.String("tun_dir", "tunnel", "tun dir")
var checkSocks5 = flag.String("check_socks5", "", "socks5 check addr")

func main() {
	flag.Parse()

	if *checkSocks5 != "" {
		socks5client.CheckServer(*checkSocks5)
		return
	}

	err := gotun.Run(*cfg, *tunDir)
	if err != nil {
		logrus.WithError(err).Error("gotun run")
	}
}
