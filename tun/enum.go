package tun

import "errors"

type protocol int

const (
	TCP protocol = iota
	TcpMux
	QUIC
	KCP
	KcpMux
	UDP
	Socks5X = 6
)

func (p protocol) String() string {
	switch p {
	case TCP:
		return "tcp"
	case TcpMux:
		return "tcp_mux"
	case QUIC:
		return "quic"
	case KCP:
		return "kcp"
	case KcpMux:
		return "kcp_mux"
	case UDP:
		return "udp"
	case Socks5X:
		return "socks5x"
	default:
		return "unknown"
	}
}

func toProtocol(s string) (protocol, error) {
	switch s {
	case "tcp":
		return TCP, nil
	case "tcp_mux", "tcpmux":
		return TcpMux, nil
	case "quic":
		return QUIC, nil
	case "kcp":
		return KCP, nil
	case "kcp_mux", "kcpmux":
		return KcpMux, nil
	case "udp":
		return UDP, nil
	case "socks5x":
		return Socks5X, nil
	default:
		return -1, errors.New("unknown protocol")
	}
}
