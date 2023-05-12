package tun

import (
	"net"
)

type TCPConn struct {
	net.Conn
}

func (c *TCPConn) ID() int64 {
	return int64(1)
}

func dialTCP(addr string) (Stream, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &TCPConn{Conn: conn}, nil
}
