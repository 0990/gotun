package tun

import (
	"errors"
	"fmt"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/msg"
	"github.com/sirupsen/logrus"
)

const FrpWorkerCount = 10

type Frpc struct {
	cfg          Config
	worker       output
	output       output
	cryptoHelper *CryptoHelper

	ctl *frpcController
	StatusX
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

	s := &Frpc{
		cfg:          cfg,
		worker:       worker,
		output:       output,
		cryptoHelper: c,
	}

	ctl := newFrpcController(worker.GetStream, s.startWorker, c.SrcCrypto)
	s.ctl = ctl
	s.SetStatus("init")
	return s, nil
}

func (s *Frpc) Run() error {
	s.SetStatus("worker run...")
	err := s.worker.Run()
	if err != nil {
		s.SetStatus(fmt.Sprintf("worker run:%s", err.Error()))
		return err
	}

	s.SetStatus("output run...")
	err = s.output.Run()
	if err != nil {
		s.SetStatus(fmt.Sprintf("output run:%s", err.Error()))
		return err
	}

	s.SetStatus("ctx run...")
	s.ctl.Run(s.SetStatus)
	s.SetStatus("running")
	return nil
}

func (s *Frpc) Close() error {
	s.worker.Close()
	s.output.Close()
	s.ctl.Close()
	return nil
}
func (s *Frpc) Cfg() Config {
	return s.cfg
}

func (s *Frpc) handleWorkerStream(src core.IStream) {
	defer src.Close()

	err := s.sayHelloAndWait(src)
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

	s.cryptoHelper.Pipe(dst, src)
}

func (s *Frpc) sayHelloAndWait(src core.IStream) error {
	rw, err := s.cryptoHelper.SrcReaderWriter(src)
	if err != nil {
		return err
	}

	//udp模式下，没有发送数据，对端并不会创建stream，所以这里需要发送一个数据包
	err = msg.WriteMsg(rw, &msg.NewWorkConn{})
	if err != nil {
		return err
	}

	rawMsg, err := msg.ReadMsg(rw)
	if err != nil {
		return err
	}

	_, ok := rawMsg.(*msg.StartWorkConn)
	if !ok {
		return errors.New("not StartWorkConn")
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
