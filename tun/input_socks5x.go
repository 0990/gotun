package tun

import (
	"encoding/json"
	"github.com/0990/gotun/pkg/stats"
	"github.com/sirupsen/logrus"
	"net"
	"time"
)

type inputSocks5X struct {
	inputBase

	addr     string
	cfg      InProtoSocks5X
	listener net.Listener

	readCounter  stats.Counter
	writeCounter stats.Counter
}

func NewInputSocks5X(addr string, extra string, readCounter, writeCounter stats.Counter) (*inputSocks5X, error) {
	var cfg InProtoSocks5X

	if extra != "" {
		err := json.Unmarshal([]byte(extra), &cfg)
		if err != nil {
			return nil, err
		}
	} else {
		cfg = defaultInSocks5XConfig
	}

	return &inputSocks5X{
		addr:         addr,
		cfg:          cfg,
		readCounter:  readCounter,
		writeCounter: writeCounter,
	}, nil
}

func (p *inputSocks5X) Run() error {
	lis, err := net.Listen("tcp", p.addr)
	if err != nil {
		return err
	}
	p.listener = lis
	go p.serve()
	return nil
}

func (p *inputSocks5X) serve() {
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

func (p *inputSocks5X) handleConn(tcpConn net.Conn) {
	conn := &StatsConn{
		Conn:         tcpConn,
		readBytes:    0,
		writeBytes:   0,
		readCounter:  p.readCounter,
		writeCounter: p.writeCounter,
	}
	c := &Socks5XConn{Conn: conn, cfg: p.cfg}
	p.inputBase.OnNewStream(c)
}

func (p *inputSocks5X) Close() error {
	return p.listener.Close()
}
