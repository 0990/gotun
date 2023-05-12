package tun

import (
	"github.com/hashicorp/yamux"
	"github.com/sirupsen/logrus"
	"net"
	"time"
)

type inputTCP struct {
	addr     string
	cfg      TCPConfig
	listener net.Listener

	streamHandler func(stream Stream)
}

func NewInputTCP(addr string, cfg TCPConfig) (*inputTCP, error) {
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

func (p *inputTCP) SetStreamHandler(f func(stream Stream)) {
	p.streamHandler = f
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
	if p.cfg.NoMux {
		p.handleConnNoMux(conn)
		return
	}

	p.handleConnYamux(conn)
}

func (p *inputTCP) handleConnNoMux(conn net.Conn) {
	c := &TCPConn{Conn: conn}
	p.streamHandler(c)
}

func (p *inputTCP) handleConnYamux(conn net.Conn) {
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
			p.streamHandler(p1)
		}(s)
	}
}

func (p *inputTCP) Close() error {
	return p.listener.Close()
}
