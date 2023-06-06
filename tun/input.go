package tun

import (
	"errors"
	"net"
)

type input interface {
	Run() error
	Close() error
	SetOnNewStream(func(stream Stream))
}

func newInput(input string, config string) (input, error) {
	proto, addr, err := parseProtocol(input)
	if err != nil {
		return nil, err
	}
	switch proto {
	case TCP:
		return NewInputTCP(addr, config)
	case TcpMux:
		return NewInputTcpMux(addr, config)
	case UDP:
		return NewInputUDP(addr, config)
	case QUIC:
		return NewInputQUIC(addr, config)
	case KCP:
		return NewInputKCP(addr, config)
	case KcpMux:
		return NewInputKCPMux(addr, config)
	case Socks5X:
		return NewInputSocks5X(addr, config)
	default:
		return nil, errors.New("unknown protocol")
	}
}

type inputBase struct {
	newStream func(stream Stream)
	newConn   func(conn net.Conn)
}

func (i *inputBase) SetOnNewConn(f func(conn net.Conn)) {
	i.newConn = f
}
func (i *inputBase) SetOnNewStream(f func(stream Stream)) {
	i.newStream = f
}

func (i *inputBase) Close() error {
	return nil
}

func (i *inputBase) Run() error {
	return nil
}

func (i *inputBase) OnNewStream(stream Stream) {
	if i.newStream != nil {
		i.newStream(stream)
	}
}
