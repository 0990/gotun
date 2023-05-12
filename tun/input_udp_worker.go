package tun

import (
	"errors"
	"github.com/0990/gotun/syncx"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"time"
)

const socketBufSize = 64 * 1024

type WorkerMap struct {
	m syncx.Map[string, *UDPWorker]
}

func (p *WorkerMap) Del(key string) *UDPWorker {
	if conn, exist := p.m.Load(key); exist {
		p.m.Delete(key)
		return conn
	}

	return nil
}

func (p *WorkerMap) LoadOrStore(key string, worker *UDPWorker) (w *UDPWorker, load bool) {
	return p.m.LoadOrStore(key, worker)
}

type UDPWorker struct {
	srcAddr      net.Addr
	timeout      time.Duration
	relayer      net.PacketConn
	writeData    chan []byte
	onClear      func()
	readDeadline time.Time
}

func (w *UDPWorker) ID() int64 {
	return 0
}

func (w *UDPWorker) RemoteAddr() net.Addr {
	return w.srcAddr
}

func (w *UDPWorker) SetReadDeadline(t time.Time) error {
	w.readDeadline = t
	return nil
}

func (w *UDPWorker) Logger() *logrus.Entry {
	log := logrus.WithFields(logrus.Fields{
		"src": w.srcAddr,
	})
	return log
}

func (w *UDPWorker) insert(data []byte) {
	if len(w.writeData) > 90 {
		logrus.Warn("UDPWorker writeData reach limit")
	}
	w.writeData <- data
}

func (w *UDPWorker) Close() error {
	w.onClear()
	return nil
}

func (w *UDPWorker) Read(p []byte) (n int, err error) {
	timeout := time.Until(w.readDeadline)

	if timeout <= 0 {
		data, ok := <-w.writeData
		if !ok {
			return 0, io.EOF
		}
		return copy(p, data), nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case data, ok := <-w.writeData:
		if !ok {
			return 0, io.EOF
		}
		return copy(p, data), nil
	case <-timer.C:
		return 0, errors.New("timeout")
	}
}

func (w *UDPWorker) Write(p []byte) (n int, err error) {
	return w.relayer.WriteTo(p, w.srcAddr)
}
