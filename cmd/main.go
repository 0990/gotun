package main

import (
	"flag"
	"github.com/0990/gotun"
	"github.com/sirupsen/logrus"
)

var cfg = flag.String("config", "app.yaml", "config file")
var tunDir = flag.String("tun_dir", "tunnel", "tun dir")

func main() {
	flag.Parse()
	err := gotun.Run(*cfg, *tunDir)
	if err != nil {
		logrus.WithError(err).Error("gotun run")
	}
}
