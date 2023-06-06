package tun

import (
	"io"
	"net"
	"time"
)

type StreamMaker interface {
	OpenStream() (Stream, error)
	IsClosed() bool
	Close() error
}

type Stream interface {
	ID() int64
	RemoteAddr() net.Addr
	LocalAddr() net.Addr
	io.ReadWriteCloser
	SetReadDeadline(t time.Time) error
}
