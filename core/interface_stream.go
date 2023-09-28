package core

import (
	"io"
	"net"
	"time"
)

type IStreamMaker interface {
	OpenStream() (IStream, error)
	IsClosed() bool
	Close() error
}

type IStream interface {
	ID() string
	RemoteAddr() net.Addr
	LocalAddr() net.Addr
	io.ReadWriteCloser
	SetReadDeadline(t time.Time) error
}
