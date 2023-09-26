package tun

import (
	"bytes"
	"io"
	"net"
)

type UDPConn struct {
	*net.UDPConn
	reader io.Reader
}

func (c *UDPConn) ID() string {
	return "udpconn"
}

func (c *UDPConn) Read(b []byte) (int, error) {
	if c.reader == nil {
		buf := make([]byte, 65535)
		n, err := c.UDPConn.Read(buf)
		if err != nil {
			return n, err
		}
		c.reader = bytes.NewBuffer(buf[0:n])
	}

	n, err := c.reader.Read(b)
	if err == nil {
		return n, nil
	}
	if err != io.EOF {
		return n, err
	}

	//以下是err==io.EOF情况
	if n > 0 {
		return n, nil
	}

	//以下是err==io.EOF的情况
	c.reader = nil
	return c.Read(b)
}

func dialUDP(addr string, config string) (Stream, error) {
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}

	return &UDPConn{UDPConn: conn}, nil
}
