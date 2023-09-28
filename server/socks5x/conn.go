package socks5x

import "net"

type TCPConn struct {
	net.Conn
}

func (c *TCPConn) ID() string {
	return "tcp"
}
