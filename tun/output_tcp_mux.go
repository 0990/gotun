package tun

import (
	"encoding/json"
	"github.com/hashicorp/yamux"
	"net"
	"time"
)

type TCPYamuxSession struct {
	session *yamux.Session
}

func (p *TCPYamuxSession) OpenStream() (Stream, error) {
	steam, err := p.session.OpenStream()
	return &TCPYamuxStream{Stream: steam}, err
}

func (p *TCPYamuxSession) IsClosed() bool {
	return p.session.IsClosed()
}

func (p *TCPYamuxSession) Close() error {
	return p.session.Close()
}

type TCPYamuxStream struct {
	*yamux.Stream
}

func (c *TCPYamuxStream) ID() int64 {
	return int64(c.StreamID())
}

func (c *TCPYamuxStream) SetReadDeadline(t time.Time) error {
	return c.Stream.SetReadDeadline(t)
}

func dialTCPYamuxBuilder(addr string, config string) (StreamMaker, error) {
	var cfg OutProtoTCPMux
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
	session, err := yamux.Client(conn, nil)
	if err != nil {
		return nil, err
	}
	return &TCPYamuxSession{session: session}, nil
}
