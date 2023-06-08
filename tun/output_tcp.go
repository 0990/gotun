package tun

import (
	"encoding/json"
	"io"
	"net"
)

type TCPConn struct {
	net.Conn
}

func (c *TCPConn) ID() int64 {
	return int64(-1)
}

func dialTCP(addr string, config string) (Stream, error) {
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

	err = tcpHeadAppend(conn, cfg.HeadAppend)
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
