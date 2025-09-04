package tun

import (
	"encoding/json"
	"github.com/0990/gotun/core"
	"github.com/hashicorp/yamux"
	"github.com/sirupsen/logrus"
	"net"
	"sync/atomic"
	"time"
)

type inputTcpMux struct {
	inputBase

	addr     string
	cfg      InProtoTCPMux
	listener net.Listener

	uuid string

	close int32
}

func NewInputTcpMux(addr string, config string) (*inputTcpMux, error) {
	var cfg InProtoTCPMux

	if config != "" {
		err := json.Unmarshal([]byte(config), &cfg)
		if err != nil {
			return nil, err
		}
	}

	return &inputTcpMux{
		addr: addr,
		cfg:  cfg,
		uuid: time.Now().String(),
	}, nil
}

func (p *inputTcpMux) Run() error {
	lis, err := net.Listen("tcp", p.addr)
	if err != nil {
		return err
	}
	p.listener = lis
	go p.serve()
	return nil
}

func (p *inputTcpMux) serve() {
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

func (p *inputTcpMux) handleConn(conn net.Conn) {
	err := p.OnNewConn(conn)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"remote": conn.RemoteAddr(),
			"local":  conn.LocalAddr(),
		}).WithError(err).Error("OnNewConn")
		conn.Close()
		return
	}
	p.handleConnYamux(conn)
}

func (p *inputTcpMux) OnNewConn(conn net.Conn) error {
	err := tcpTrimHead(conn, p.cfg.Head)
	if err != nil {
		return err
	}
	return nil
}

func (p *inputTcpMux) handleConnYamux(conn net.Conn) {
	defer conn.Close()

	muxCfg := yamux.DefaultConfig()
	muxCfg.KeepAliveInterval = 20 * time.Second
	muxCfg.MaxStreamWindowSize = 6 * 1024 * 1024

	session, err := yamux.Server(conn, muxCfg)
	if err != nil {
		return
	}
	defer session.Close()

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return
		}

		if atomic.LoadInt32(&p.close) == 1 {
			return
		}

		s := &TCPYamuxStream{stream}
		go func(p1 core.IStream) {
			p.inputBase.OnNewStream(p1)
		}(s)
	}
}

func (p *inputTcpMux) Close() error {
	atomic.StoreInt32(&p.close, 1)
	if p.listener == nil {
		return nil
	}
	return p.listener.Close()
}
