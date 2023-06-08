package tun

import (
	"encoding/json"
	"errors"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"time"
)

type inputTCP struct {
	inputBase

	addr     string
	cfg      InProtoTCP
	listener net.Listener
}

func NewInputTCP(addr string, extra string) (*inputTCP, error) {
	var cfg InProtoTCP

	if extra != "" {
		err := json.Unmarshal([]byte(extra), &cfg)
		if err != nil {
			return nil, err
		}
	}

	return &inputTCP{
		addr: addr,
		cfg:  cfg,
	}, nil
}

func (p *inputTCP) Run() error {
	lis, err := net.Listen("tcp", p.addr)
	if err != nil {
		return err
	}
	p.listener = lis
	go p.serve()
	return nil
}

func (p *inputTCP) serve() {
	var tempDelay time.Duration
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			logrus.WithError(err).Error("HandleListener Accept")
			if ne, ok := err.(*net.OpError); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				logrus.Errorf("http: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return
		}
		go p.handleConn(conn)
	}
}

func (p *inputTCP) handleConn(conn net.Conn) {
	err := p.OnNewConn(conn)
	if err != nil {
		logrus.WithError(err).Error("OnNewConn")
		conn.Close()
		return
	}

	c := &TCPConn{Conn: conn}
	p.inputBase.OnNewStream(c)
}

func (p *inputTCP) Close() error {
	return p.listener.Close()
}

func (p *inputTCP) OnNewConn(conn net.Conn) error {
	err := tcpTrimHead(conn, p.cfg.HeadTrim)
	if err != nil {
		return err
	}
	return nil
}

func tcpTrimHead(conn net.Conn, str string) error {

	trim := []byte(str)
	if len(trim) == 0 {
		return nil
	}

	conn.SetReadDeadline(time.Now().Add(time.Second * 5))

	data := make([]byte, len(trim))
	n, err := io.ReadFull(conn, data)
	if err != nil {
		return err
	}

	conn.SetReadDeadline(time.Time{})

	if n != len(trim) {
		return errors.New("read head trim failed")
	}

	if string(data) != string(trim) {
		return errors.New("head trim not match")
	}

	return nil
}
