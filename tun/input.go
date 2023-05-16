package tun

import (
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
		return NewInputTCP(addr, extra)
	case TcpMux:
		return NewInputTcpMux(addr, extra)
	case UDP:
		return NewInputUDP(addr, extra)
	case QUIC:
		return NewInputQUIC(addr, extra)
	case KCP:
		return NewInputKCP(addr, extra)
	case KcpMux:
		return NewInputKCPMux(addr, extra)
	default:
		return nil, errors.New("unknown protocol")
	}
}
