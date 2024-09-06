package tun

import "errors"

type IPing interface {
	Ping() (int, error)
}

func NewPing(typ string, addr string) (IPing, error) {
	switch typ {
	case "ping":
		return &Ping{addr: addr}, nil
	case "tcp_ack":
		return &TcpAckPing{addr: addr}, nil
	case "socks5_ack":
		return &Socks5AckPing{addr: addr}, nil
	default:
		return nil, errors.New("unknown protocol")
	}
}

type Socks5AckPing struct {
	addr string
}

func (p *Socks5AckPing) Ping() (int, error) {
	return 0, nil
}

type TcpAckPing struct {
	addr string
}

func (p *TcpAckPing) Ping() (int, error) {
	return 0, nil
}

type Ping struct {
	addr string
}

func (p *Ping) Ping() (int, error) {
	return 0, nil
}
