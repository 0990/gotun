package tun

import "net"

type UDPConn struct {
	*net.UDPConn
}

func (c *UDPConn) ID() int64 {
	return 0
}

func dialUDP(addr string) (Stream, error) {
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}

	return &UDPConn{conn}, nil
}
