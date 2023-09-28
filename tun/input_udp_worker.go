package tun

import (
	"bytes"
	"errors"
	"github.com/0990/gotun/pkg/syncx"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"time"
)

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
	bReaders     chan io.Reader
	reader       io.Reader
	onClear      func()
	readDeadline time.Time
}

func (w *UDPWorker) ID() string {
	return "udpworker"
}

func (w *UDPWorker) RemoteAddr() net.Addr {
	return w.srcAddr
}

func (w *UDPWorker) LocalAddr() net.Addr {
	return w.relayer.LocalAddr()
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
	if len(w.bReaders) > 90 {
		logrus.Warn("UDPWorker writeData reach limit")
	}
	bReader := bytes.NewBuffer(data)
	w.bReaders <- bReader
}

func (w *UDPWorker) Close() error {
	w.onClear()
	return nil
}

func (w *UDPWorker) Read(p []byte) (n int, err error) {
	var timeout time.Duration = w.timeout
	if !w.readDeadline.IsZero() {
		timeout = time.Until(w.readDeadline)
	}

	if timeout <= 0 {
		return 0, errors.New("timeout")
	}

READ:
	if w.reader == nil {
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case reader, ok := <-w.bReaders:
			if !ok {
				return 0, io.EOF
			}
			w.reader = reader
		case <-timer.C:
			logrus.WithField("timeout", int32(timeout/time.Second)).Debug("UDPWorker Read timeout")
			return 0, ErrTimeout
		}
	}

	n, err = w.reader.Read(p)
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
	w.reader = nil
	goto READ
}

func (w *UDPWorker) Write(p []byte) (n int, err error) {
	n, err = w.relayer.WriteTo(p, w.srcAddr)
	return n, err
}
