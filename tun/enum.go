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
	default:
		return "unknown"
	}
}

func toProtocol(s string) (protocol, error) {
	switch s {
	case "tcp":
		return TCP, nil
	case "tcp_mux":
		return TcpMux, nil
	case "quic":
		return QUIC, nil
	case "kcp":
		return KCP, nil
	case "kcp_mux":
		return KcpMux, nil
	case "udp":
		return UDP, nil
	default:
		return -1, errors.New("unknown protocol")
	}
}
