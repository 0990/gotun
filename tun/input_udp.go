package tun

import (
	"encoding/json"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/pool"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"time"
)

type inputUDP struct {
	inputBase

	addr string
	cfg  UDPConfig
	conn net.PacketConn
}

func NewInputUDP(addr string, extra string) (*inputUDP, error) {
	var cfg UDPConfig

	if extra != "" {
		err := json.Unmarshal([]byte(extra), &cfg)
		if err != nil {
			return nil, err
		}
	}

	return &inputUDP{
		addr: addr,
		cfg:  cfg,
	}, nil
}

func (p *inputUDP) Run() error {
	conn, err := net.ListenPacket("udp", p.addr)
	if err != nil {
		return err
	}

	p.conn = conn
	go p.serve()
	return nil
}

func (p *inputUDP) Close() error {
	if p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

// send: client->relayer->sender->remote
// receive: client<-relayer<-sender<-remote
func (p *inputUDP) serve() {
	relayer := p.conn
	timeout := time.Duration(p.cfg.Timeout) * time.Second
	var tempDelay time.Duration
	var workers WorkerMap
	for {
		buf := pool.GetBuf(core.MaxSegmentSize)
		n, srcAddr, err := relayer.ReadFrom(buf)
		if err != nil {
			logrus.WithError(err).Error("relayer.ReadFrom")
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

		id := srcAddr.String()

		data := buf[0:n]

		worker := &UDPWorker{
			timeout:  timeout,
			srcAddr:  srcAddr,
			relayer:  relayer,
			bReaders: make(chan io.Reader, 100),
			onClear: func() {
				workers.Del(id)
			},
		}

		w, load := workers.LoadOrStore(id, worker)
		if !load {
			go func() {
				p.inputBase.OnNewStream(w)
			}()
		}

		w.insert(data)

		w.Logger().WithField("len", len(data)).Debug("client udp")
	}
}
