package tun

import (
	"errors"
	"github.com/0990/gotun/core"
	"net"
)

type input interface {
	Run() error
	Close() error
	SetOnNewStream(func(stream core.IStream))
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
	newStream func(stream core.IStream)
	newConn   func(conn net.Conn)
}

func (i *inputBase) SetOnNewConn(f func(conn net.Conn)) {
	i.newConn = f
}
func (i *inputBase) SetOnNewStream(f func(stream core.IStream)) {
	i.newStream = f
}

func (i *inputBase) Close() error {
	return nil
}

func (i *inputBase) Run() error {
	return nil
}

func (i *inputBase) OnNewStream(stream core.IStream) {
	if i.newStream != nil {
		i.newStream(stream)
	}
}
