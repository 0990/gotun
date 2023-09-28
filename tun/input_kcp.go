package tun

import (
	"encoding/json"
	"github.com/0990/gotun/core"
	"github.com/sirupsen/logrus"
	"github.com/xtaci/kcp-go/v5"
	"log"
	"net"
	"time"
)

type inputKCP struct {
	inputBase

	addr     string
	cfg      KCPConfig
	listener *kcp.Listener
	close    int32
}

func NewInputKCP(addr string, extra string) (*inputKCP, error) {
	var cfg KCPConfig

	if extra != "" {
		err := json.Unmarshal([]byte(extra), &cfg)
		if err != nil {
			return nil, err
		}
	} else {
		cfg = defaultKCPConfig
	}

	return &inputKCP{
		addr: addr,
		cfg:  cfg,
	}, nil
}

func (p *inputKCP) Run() error {
	lis, err := kcp.ListenWithOptions(p.addr, nil, p.cfg.DataShard, p.cfg.ParityShard)
	if err != nil {
		return err
	}

	config := p.cfg

	if err := lis.SetDSCP(config.DSCP); err != nil {
		log.Println("SetDSCP:", err)
	}
	if err := lis.SetReadBuffer(config.SockBuf); err != nil {
		log.Println("SetReadBuffer:", err)
	}
	if err := lis.SetWriteBuffer(config.SockBuf); err != nil {
		log.Println("SetWriteBuffer:", err)
	}

	p.listener = lis

	go p.serve()
	return nil
}

func (p *inputKCP) Close() error {
	return p.listener.Close()
}

func (p *inputKCP) serve() {
	config := p.cfg
	var tempDelay time.Duration
	for {
		conn, err := p.listener.AcceptKCP()
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
		conn.SetStreamMode(false)
		conn.SetWriteDelay(p.cfg.WriteDelay)
		conn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
		conn.SetMtu(config.MTU)
		conn.SetWindowSize(config.SndWnd, config.RcvWnd)
		conn.SetACKNoDelay(config.AckNodelay)

		go p.handleConn(conn)
	}
}

func (p *inputKCP) handleConn(session *kcp.UDPSession) {
	s := &KCPSession{UDPSession: session}
	go func(p1 core.IStream) {
		p.inputBase.OnNewStream(p1)
	}(s)
}
