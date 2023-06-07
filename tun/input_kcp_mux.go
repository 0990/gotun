package tun

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
	"log"
	"net"
	"sync/atomic"
	"time"
)

type inputKCPMux struct {
	inputBase

	addr string
	cfg  KCPConfig

	listener *kcp.Listener
	close    int32
}

func NewInputKCPMux(addr string, extra string) (*inputKCPMux, error) {
	var cfg KCPConfig

	if extra != "" {
		err := json.Unmarshal([]byte(extra), &cfg)
		if err != nil {
			return nil, err
		}
	} else {
		cfg = defaultKCPConfig
	}

	return &inputKCPMux{
		addr: addr,
		cfg:  cfg,
	}, nil
}

func (p *inputKCPMux) Run() error {
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

func (p *inputKCPMux) Close() error {
	atomic.StoreInt32(&p.close, 1)
	return p.listener.Close()
}

func (p *inputKCPMux) serve() {
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
		conn.SetStreamMode(true)
		conn.SetWriteDelay(p.cfg.WriteDelay)
		conn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
		conn.SetMtu(config.MTU)
		conn.SetWindowSize(config.SndWnd, config.RcvWnd)
		conn.SetACKNoDelay(config.AckNodelay)

		go p.handleConn(conn)
	}
}

func (p *inputKCPMux) handleConn(conn net.Conn) {
	smuxConfig := smux.DefaultConfig()
	smuxConfig.MaxReceiveBuffer = p.cfg.StreamBuf
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

		s := &KCPsmuxStream{Stream: stream}

		go func(p1 Stream) {
			p.inputBase.OnNewStream(p1)
		}(s)
	}
}
