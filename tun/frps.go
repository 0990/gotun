package tun

import (
	"fmt"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/msg"
	"github.com/sirupsen/logrus"
)

type Frps struct {
	cfg          Config
	input        input
	worker       input
	cryptoHelper *CryptoHelper

	ctlMgr *frpsControllerManager
	StatusX
}

func NewFrps(cfg Config) (*Frps, error) {
	input, err := newInput(cfg.Input, cfg.InProtoCfg)
	if err != nil {
		return nil, err
	}

	worker, err := newInput(cfg.Output, cfg.OutProtoCfg)
	if err != nil {
		return nil, err
	}

	c, err := NewCryptoHelperWithConfig(cfg)
	if err != nil {
		return nil, err
	}

	s := &Frps{
		cfg:          cfg,
		input:        input,
		worker:       worker,
		cryptoHelper: c,
		ctlMgr:       newFrpsControllerManager(),
	}
	s.SetStatus("init")
	return s, nil
}

func (s *Frps) Run() error {
	s.SetStatus("input run...")
	err := s.input.Run()
	if err != nil {
		s.SetStatus(fmt.Sprintf("input run:%s", err.Error()))
		return err
	}
	s.input.SetOnNewStream(s.handleInputStream)
	s.SetStatus("worker run...")
	err = s.worker.Run()
	if err != nil {
		s.SetStatus(fmt.Sprintf("worker run:%s", err.Error()))
		return err
	}
	s.worker.SetOnNewStream(s.HandlerWorkerStream)
	s.SetStatus("running")
	return nil
}

func (s *Frps) Cfg() Config {
	return s.cfg
}

func (s *Frps) HandlerWorkerStream(src core.IStream) {
	err := s.handleWorkerStream(src)
	if err != nil {
		logrus.WithError(err).Error("handleWorkerStream")
	}
}

func (s *Frps) handleWorkerStream(src core.IStream) error {
	rw, err := s.cryptoHelper.DstCrypto(src)
	if err != nil {
		return err
	}

	rawMsg, err := msg.ReadMsg(rw)
	if err != nil {
		return err
	}

	switch rawMsg.(type) {
	case *msg.Login:
		err := msg.WriteMsg(rw, &msg.LoginResp{})
		if err != nil {
			return err
		}

		ctl := NewFrpsController(rw)
		ctl.Run()
		s.ctlMgr.Set(ctl)
	case *msg.NewWorkConn:
		ctl, ok := s.ctlMgr.Get()
		if !ok {
			return fmt.Errorf("no controller")
		}
		ctl.RegisterWorker(src)

	default:

	}
	return nil
}

func (s *Frps) Close() error {
	s.input.Close()
	s.worker.Close()
	s.ctlMgr.Close()
	return nil
}

func (s *Frps) handleInputStream(src core.IStream) {
	defer src.Close()

	ctl, ok := s.ctlMgr.Get()
	if !ok {
		logrus.Error("no controller")
		return
	}

	dst, err := ctl.GetWorkConn()
	if err != nil {
		logrus.WithError(err).Error("openStream")
		return
	}
	defer dst.Close()

	//增加头标识，用于通知对端此连接开始工作了
	err = s.sayStart(dst)
	if err != nil {
		logrus.WithError(err).Error("writeHead")
		return
	}

	s.cryptoHelper.Pipe(dst, src)
}

func (s *Frps) sayStart(dst core.IStream) error {
	rw, err := s.cryptoHelper.DstReaderWriter(dst)
	if err != nil {
		return err
	}

	err = msg.WriteMsg(rw, &msg.StartWorkConn{})
	if err != nil {
		return err
	}
	return nil
}
