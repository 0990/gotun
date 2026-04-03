package tun

import (
	"fmt"
	"time"

	"github.com/0990/gotun/core"
	"github.com/sirupsen/logrus"
)

type Server struct {
	cfg          Config
	input        input
	output       output
	cryptoHelper *CryptoHelper
	probeRunner  *FrameProbeRunner
	frameStreams *FrameStreamRegistry

	StatusX
}

func NewServer(cfg Config) (*Server, error) {
	input, err := newInput(cfg.Input, cfg.InProtoCfg, NewUplinkCounter(cfg.Name, cfg.Input), NewDownlinkCounter(cfg.Name, cfg.Input))
	if err != nil {
		return nil, err
	}

	output, err := NewOutput(cfg.Name, cfg.Output, cfg.OutProtoCfg, cfg.OutExtend, NewDownlinkCounter(cfg.Name, cfg.Output), NewUplinkCounter(cfg.Name, cfg.Output))
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
		frameStreams: &FrameStreamRegistry{},
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
	s.startProbe()
	return nil
}

func (s *Server) Close() error {
	s.input.Close()
	s.output.Close()
	if s.probeRunner != nil {
		s.probeRunner.Close()
	}
	return nil
}

func (s *Server) Cfg() Config {
	return s.cfg
}

func (s *Server) handleInputStream(src core.IStream) {
	defer src.Close()

	srcStream, err := s.cryptoHelper.WrapSrc(src)
	if err != nil {
		logrus.WithError(err).Error("wrap src")
		return
	}

	dst, err := s.output.GetStream()
	if err != nil {
		logrus.WithError(err).Error("openStream")
		return
	}
	defer dst.Close()

	dstStream, err := s.cryptoHelper.WrapDst(dst)
	if err != nil {
		logrus.WithError(err).Error("wrap dst")
		return
	}
	if frameStream, ok := dstStream.(*FrameStream); ok {
		s.frameStreams.Add(frameStream)
		defer s.frameStreams.Remove(frameStream)
	}

	s.cryptoHelper.PipePrepared(dstStream, srcStream)
}

func (s *Server) QualitySummary() QualitySummary {
	return s.output.QualitySummary()
}

func (s *Server) QualityDetails() map[string]QualitySnapshot {
	return map[string]QualitySnapshot{
		"output": s.output.QualitySnapshot(),
	}
}

func (s *Server) startProbe() {
	if !s.output.FrameHeaderEnabled() {
		return
	}
	s.probeRunner = NewFrameProbeRunner(s.frameStreams, outputTracker(s.output), s.output.ProbeConfig())
	s.probeRunner.Run()
}

func (s *Server) QuickProbe() bool {
	if s.probeRunner == nil {
		return false
	}
	return s.probeRunner.TriggerQuickProbe(time.Minute)
}
