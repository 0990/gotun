package tun

import (
	"encoding/json"
	"errors"
)

type input interface {
	Run() error
	Close() error
	SetStreamHandler(func(stream Stream))
}

func newInput(input string, extra string) (input, error) {
	proto, addr, err := parseProtocol(input)
	if err != nil {
		return nil, err
	}
	switch proto {
	case TCP:
		var tcpCfg TCPConfig
		err := json.Unmarshal([]byte(extra), &tcpCfg)
		if err != nil {
			return nil, err
		}
		return NewInputTCP(addr, tcpCfg)
	default:
		return nil, errors.New("unknown protocol")
	}
}
