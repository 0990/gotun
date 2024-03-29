package tun

import (
	"encoding/json"
	"github.com/0990/gotun/core"
	"io"
	"net"
)

type TCPConn struct {
	net.Conn
}

func (c *TCPConn) ID() string {
	return "tcp"
}

func dialTCP(addr string, config string) (core.IStream, error) {
	var cfg OutProtoTCP
	if config != "" {
		err := json.Unmarshal([]byte(config), &cfg)
		if err != nil {
			return nil, err
		}
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	err = tcpHeadAppend(conn, cfg.Head)
	if err != nil {
		return nil, err
	}

	return &TCPConn{Conn: conn}, nil
}

func tcpHeadAppend(conn net.Conn, str string) error {
	head := []byte(str)
	if len(head) == 0 {
		return nil
	}
	n, err := conn.Write(head)
	if err != nil {
		return err
	}

	if n != len(head) {
		return io.ErrShortWrite
	}
	return nil
}
