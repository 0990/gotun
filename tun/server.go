package tun

import (
	"fmt"

	"github.com/0990/gotun/core"
	"github.com/sirupsen/logrus"
)

type Server struct {
	cfg          Config
	input        input
	output       output
	cryptoHelper *CryptoHelper

	StatusX
}

func NewServer(cfg Config) (*Server, error) {
	input, err := newInput(cfg.Input, cfg.InProtoCfg, NewUplinkCounter(cfg.Name, cfg.Input), NewDownlinkCounter(cfg.Name, cfg.Input))
	if err != nil {
		return nil, err
	}

	output, err := NewOutput(cfg.Output, cfg.OutProtoCfg, cfg.OutExtend, NewDownlinkCounter(cfg.Name, cfg.Output), NewUplinkCounter(cfg.Name, cfg.Output))
	if err != nil {
		return nil, err
	}

	c, err := NewCryptoHelperWithConfig(cfg)
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:          cfg,
		input:        input,
		output:       output,
		cryptoHelper: c,
	}
	s.SetStatus("init")
	return s, nil
}

func (s *Server) Run() error {
	s.SetStatus("output run...")
	err := s.output.Run()
	if err != nil {
		s.SetStatus(fmt.Sprintf("output run:%s", err.Error()))
		return err
	}

	s.SetStatus("input run...")
	s.input.SetOnNewStream(s.handleInputStream)
	err = s.input.Run()
	if err != nil {
		s.SetStatus(fmt.Sprintf("input run:%s", err.Error()))
		return err
	}

	s.SetStatus("running")
	return nil
}

func (s *Server) Close() error {
	s.input.Close()
	s.output.Close()
	return nil
}

func (s *Server) Cfg() Config {
	return s.cfg
}

func (s *Server) handleInputStream(src core.IStream) {
	defer src.Close()

	dst, err := s.output.GetStream()
	if err != nil {
		logrus.WithError(err).Error("openStream")
		return
	}
	defer dst.Close()

	s.cryptoHelper.Pipe(dst, src)
}
