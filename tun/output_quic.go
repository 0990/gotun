package tun

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/0990/gotun/core"
	"github.com/quic-go/quic-go"
	"net"
	"time"
)

type QUICConn struct {
	quic.Connection
}

func (c *QUICConn) OpenStream() (core.IStream, error) {
	stream, err := c.Connection.OpenStream()
	return &QUICStream{
		Stream:     stream,
		localAddr:  c.LocalAddr(),
		remoteAddr: c.RemoteAddr(),
	}, err
}

func (c *QUICConn) IsClosed() bool {
	return c.Context().Err() == context.Canceled
}

func (c *QUICConn) Close() error {
	return c.Connection.CloseWithError(0, "")
}

type QUICStream struct {
	quic.Stream

	localAddr, remoteAddr net.Addr
}

func (p *QUICStream) ID() string {
	return fmt.Sprintf("quicstream-%d", p.Stream.StreamID())
}

func (p *QUICStream) RemoteAddr() net.Addr {
	return p.remoteAddr
}

func (p *QUICStream) LocalAddr() net.Addr {
	return p.localAddr
}

func (p *QUICStream) SetReadDeadline(t time.Time) error {
	return p.Stream.SetReadDeadline(t)
}

func dialQUICBuilder(ctx context.Context, addr string, config string) (core.IStreamMaker, error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-stunnel"},
	}
	session, err := quic.DialAddr(ctx, addr, tlsConf, nil)
	if err != nil {
		return nil, err
	}
	return &QUICConn{Connection: session}, nil
}
