package tun

import (
	"fmt"
	"github.com/0990/gotun/tun/msg"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"io"
	"sync/atomic"
	"time"
)

var FrpcHello = []byte("frpc")

const FRP_WORKER_COUNT = 10

type Frpc struct {
	cfg          Config
	worker       output
	output       output
	cryptoHelper *CryptoHelper

	idleWorkerCount  int64
	keepWorkerTicker *time.Ticker
}

func NewFrpc(cfg Config) (*Frpc, error) {
	worker, err := NewOutput(cfg.Input, cfg.InProtoCfg, cfg.InExtend)
	if err != nil {
		return nil, err
	}

	output, err := NewOutput(cfg.Output, cfg.OutProtoCfg, cfg.OutExtend)
	if err != nil {
		return nil, err
	}

	c, err := NewCryptoHelper(cfg)
	if err != nil {
		return nil, err
	}

	return &Frpc{
		cfg:          cfg,
		worker:       worker,
		output:       output,
		cryptoHelper: c,
	}, nil
}

func (s *Frpc) Run() error {

	err := s.worker.Run()
	if err != nil {
		return err
	}

	err = s.output.Run()
	if err != nil {
		return err
	}

	err = s.startWorker(FRP_WORKER_COUNT)
	if err != nil {
		return err
	}
	go s.keepWorkerPool()
	return nil
}

func (s *Frpc) Close() error {
	s.worker.Close()
	s.output.Close()
	if s.keepWorkerTicker != nil {
		s.keepWorkerTicker.Stop()
	}
	return nil
}
func (s *Frpc) Cfg() Config {
	return s.cfg
}

func (s *Frpc) handleWorkerStream(src Stream) {
	defer src.Close()

	err := s.prepareWork(src)
	if err != nil {
		logrus.WithError(err).Error("prepareWork")
		return
	}

	dst, err := s.output.GetStream()
	if err != nil {
		logrus.WithError(err).Error("openStream")
		return
	}
	defer dst.Close()

	logrus.Debug("stream opened", "in:", src.RemoteAddr(), "out:", fmt.Sprint(dst.RemoteAddr(), "(", dst.ID(), ")"))
	defer logrus.Debug("stream closed", "in:", src.RemoteAddr(), "out:", fmt.Sprint(dst.RemoteAddr(), "(", dst.ID(), ")"))

	err = s.cryptoHelper.Copy(dst, src)
	if err != nil {
		if err != io.EOF {
			logrus.WithError(err).Error("copy")
		}
	}
}

func (s *Frpc) prepareWork(src Stream) error {
	atomic.AddInt64(&s.idleWorkerCount, 1)
	defer atomic.AddInt64(&s.idleWorkerCount, -1)

	//udp模式下，没有发送数据，对端并不会创建stream，所以这里需要发送一个数据包
	_, err := src.Write(FrpcHello)
	if err != nil {
		return err
	}

	err = s.handleHead(src)
	if err != nil {
		return err
	}

	return nil
}

func (s *Frpc) startWorker(count int32) error {
	for i := 0; i < int(count); i++ {
		stream, err := s.worker.GetStream()
		if err != nil {
			return err
		}
		go s.handleWorkerStream(stream)
	}
	return nil
}

func (s *Frpc) handleHead(src Stream) error {
	head, err := s.readHead(src)
	if err != nil {
		return err
	}

	if head.AskWorkerCnt > 0 {
		go s.startWorker(head.AskWorkerCnt)
	}
	return nil
}

func (s *Frpc) readHead(src Stream) (*msg.FRPHead, error) {
	head := make([]byte, 2)
	src.SetReadDeadline(time.Now().Add(time.Minute * 2))
	_, err := io.ReadFull(src, head)
	if err != nil {
		return nil, err
	}

	size := (int(head[0])<<8 + int(head[1])) & 65535

	data := make([]byte, size)
	src.SetReadDeadline(time.Now().Add(time.Second * 10))
	_, err = io.ReadFull(src, data)
	if err != nil {
		return nil, err
	}

	var msg msg.FRPHead
	err = proto.Unmarshal(data, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (s *Frpc) GetIdleWorkerCount() int64 {
	return atomic.LoadInt64(&s.idleWorkerCount)
}

func (s *Frpc) keepWorkerPool() {
	ticker := time.NewTicker(time.Second * 10)
	for {
		select {
		case _, ok := <-ticker.C:
			if !ok {
				return
			}
			count := s.GetIdleWorkerCount()
			if count > FRP_WORKER_COUNT {
				return
			}
			go s.startWorker(FRP_WORKER_COUNT)
		}
	}
}
