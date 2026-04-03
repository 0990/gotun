package tun

import (
	"github.com/0990/gotun/core"
	kcpx "github.com/0990/kcpx-go"
	"github.com/sirupsen/logrus"
	"github.com/xtaci/smux"
	"net"
	"sync/atomic"
	"time"
)

type inputKCPXMux struct {
	inputBase

	addr string
	cfg  KCPXConfig

	packetConn *net.UDPConn
	listener   *kcpx.Listener
	close      int32
}

func NewInputKCPXMux(addr string, extra string) (*inputKCPXMux, error) {
	cfg, err := parseKCPXConfig(extra)
	if err != nil {
		return nil, err
	}

	return &inputKCPXMux{
		addr: addr,
		cfg:  cfg,
	}, nil
}

func (p *inputKCPXMux) Run() error {
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

func (p *inputKCPXMux) Close() error {
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

func (p *inputKCPXMux) serve() {
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

func (p *inputKCPXMux) handleConn(conn net.Conn) {
	smuxConfig, err := newSmuxConfig(p.cfg.StreamBuf)
	if err != nil {
		return
	}
	mux, err := smux.Server(conn, smuxConfig)
	if err != nil {
		return
	}

	defer mux.Close()

	for {
		stream, err := mux.AcceptStream()
		if err != nil {
			return
		}

		if atomic.LoadInt32(&p.close) == 1 {
			return
		}

		s := &KCPXsmuxStream{Stream: stream}
		go func(stream core.IStream) {
			p.inputBase.OnNewStream(stream)
		}(s)
	}
}
