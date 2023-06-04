package main

import (
	"flag"
	"github.com/0990/gotun"
	"github.com/sirupsen/logrus"
)

var cfg = flag.String("config", "app.yaml", "config file")

func main() {
	flag.Parse()
	err := gotun.Run(*cfg)
	if err != nil {
		logrus.WithError(err).Error("gotun run")
	}
}
