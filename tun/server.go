package tun

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
)

type Server struct {
	cfg          Config
	input        input
	output       output
	cryptoHelper *CryptoHelper
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

	return &Server{
		cfg:          cfg,
		input:        input,
		output:       output,
		cryptoHelper: c,
	}, nil
}

func (s *Server) Run() error {
	err := s.input.Run()
	if err != nil {
		return err
	}
	s.input.SetOnNewStream(s.handleInputStream)
	return nil
}

func (s *Server) Close() error {
	s.input.Close()
	s.output.Close()
	return nil
}

func (s *Server) handleInputStream(src Stream) {
	defer src.Close()

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
