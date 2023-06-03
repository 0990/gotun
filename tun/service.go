package tun

import "fmt"

type Service interface {
	Run() error
	Close() error
}

func NewService(cfg Config) (Service, error) {
	switch cfg.Mode {
	case "":
		return NewServer(cfg)
	case "frpc":
		return NewFrpc(cfg)
	case "frps":
		return NewFrps(cfg)
	default:
		return nil, fmt.Errorf("invalid mode: %s", cfg.Mode)
	}
}
