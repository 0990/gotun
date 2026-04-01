package tun

import (
	"github.com/0990/gotun/core"
	kcpx "github.com/0990/kcpx-go"
	"github.com/sirupsen/logrus"
	"net"
	"sync/atomic"
	"time"
)

type inputKCPX struct {
	inputBase

	addr string
	cfg  KCPXConfig

	packetConn *net.UDPConn
	listener   *kcpx.Listener
	close      int32
}

func NewInputKCPX(addr string, extra string) (*inputKCPX, error) {
	cfg, err := parseKCPXConfig(extra)
	if err != nil {
		return nil, err
	}

	return &inputKCPX{
		addr: addr,
		cfg:  cfg,
	}, nil
}

func (p *inputKCPX) Run() error {
	udpAddr, err := net.ResolveUDPAddr("udp", p.addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	configureKCPXListenSocket(conn, p.cfg)

	lis, err := kcpx.ServeConn(conn, p.cfg.ToOptions())
	if err != nil {
		_ = conn.Close()
		return err
	}

	p.packetConn = conn
	p.listener = lis

	go p.serve()
	return nil
}

func (p *inputKCPX) Close() error {
	atomic.StoreInt32(&p.close, 1)

	var retErr error
	if p.listener != nil {
		if err := p.listener.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}
	if p.packetConn != nil {
		if err := p.packetConn.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}
	return retErr
}

func (p *inputKCPX) serve() {
	var tempDelay time.Duration
	for {
		conn, err := p.listener.AcceptKCP()
		if err != nil {
			if atomic.LoadInt32(&p.close) == 1 {
				return
			}

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

		applyKCPXSessionConfig(conn.Session(), p.cfg)
		go p.handleConn(conn)
	}
}

func (p *inputKCPX) handleConn(conn *kcpx.Conn) {
	s := &KCPXConn{Conn: conn}
	go func(stream core.IStream) {
		p.inputBase.OnNewStream(stream)
	}(s)
}
