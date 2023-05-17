package tun

import (
	"encoding/json"
	"github.com/hashicorp/yamux"
	"github.com/sirupsen/logrus"
	"net"
	"time"
)

type inputTcpMux struct {
	inputBase

	addr     string
	cfg      InProtoTCPMux
	listener net.Listener
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
		logrus.WithError(err).Error("OnNewConn")
		conn.Close()
		return
	}
	p.handleConnYamux(conn)
}

func (p *inputTcpMux) OnNewConn(conn net.Conn) error {
	err := tcpTrimHead(conn, p.cfg.HeadTrim)
	if err != nil {
		return err
	}
	return nil
}

func (p *inputTcpMux) handleConnYamux(conn net.Conn) {
	session, err := yamux.Server(conn, nil)
	if err != nil {
		return
	}
	defer session.Close()

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return
		}

		s := &TCPYamuxStream{stream}
		go func(p1 Stream) {
			p.inputBase.OnNewStream(p1)
		}(s)
	}
}

func (p *inputTcpMux) Close() error {
	return p.listener.Close()
}
