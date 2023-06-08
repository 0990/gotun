package tun

import (
	"fmt"
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
	input, err := newInput(cfg.Input, cfg.InProtoCfg)
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
	s.SetStatus("input run...")
	err := s.input.Run()
	if err != nil {
		s.SetStatus(fmt.Sprintf("input run:%s", err.Error()))
		return err
	}
	s.input.SetOnNewStream(s.handleInputStream)

	s.SetStatus("output run...")
	err = s.output.Run()
	if err != nil {
		s.SetStatus(fmt.Sprintf("output run:%s", err.Error()))
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

func (s *Server) handleInputStream(src Stream) {
	defer src.Close()

	dst, err := s.output.GetStream()
	if err != nil {
		logrus.WithError(err).Error("openStream")
		return
	}
	defer dst.Close()

	srcLocalAddr := src.LocalAddr()
	srcRemoteAddr := src.RemoteAddr()
	dstLocalAddr := dst.LocalAddr()
	dstRemoteAddr := dst.RemoteAddr()

	log := logrus.WithFields(logrus.Fields{
		"inLocal":   srcLocalAddr,
		"inRemote":  srcRemoteAddr,
		"inID":      src.ID(),
		"outLocal":  dstLocalAddr,
		"outRemote": dstRemoteAddr,
		"outID":     dst.ID(),
	})

	log.Debug("stream opened")
	defer log.Debug("stream closed")

	s.cryptoHelper.Copy(dst, src)
}
