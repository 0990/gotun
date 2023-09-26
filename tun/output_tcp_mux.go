package tun

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/yamux"
	"github.com/sirupsen/logrus"
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

func (c *TCPYamuxStream) ID() string {
	return fmt.Sprintf("yamuxstream-%d", c.StreamID())
}

func (c *TCPYamuxStream) SetReadDeadline(t time.Time) error {
	return c.Stream.SetReadDeadline(t)
}

func dialTCPYamuxBuilder(ctx context.Context, addr string, config string) (StreamMaker, error) {
	var cfg OutProtoTCPMux
	if config != "" {
		err := json.Unmarshal([]byte(config), &cfg)
		if err != nil {
			return nil, err
		}
	}

	conn, err := dialWithContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	err = tcpHeadAppend(conn, cfg.Head)
	if err != nil {
		return nil, err
	}
	session, err := yamux.Client(conn, nil)
	if err != nil {
		return nil, err
	}
	logrus.WithFields(logrus.Fields{
		"target":     addr,
		"localAddr":  conn.LocalAddr(),
		"remoteAddr": conn.RemoteAddr(),
	}).Debug("dialTCPYamuxBuilder")

	return &TCPYamuxSession{session: session}, nil
}

func dialWithContext(ctx context.Context, network string, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}
