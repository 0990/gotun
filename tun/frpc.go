package tun

import (
	"errors"
	"fmt"
	"io"
	"sync"

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

	ctl          *frpcController
	outputProbe  *FrameProbeRunner
	probeChannel *ProbeChannel
	bandwidth    *BandwidthTracker
	bandwidthMu  sync.Mutex
	StatusX
}

func NewFrpc(cfg Config) (*Frpc, error) {
	worker, err := NewOutput(cfg.Name, cfg.Input, cfg.InProtoCfg, cfg.InExtend, NewCommonCounter(cfg.UUID, cfg.Input), NewCommonCounter(cfg.UUID, cfg.Input))
	if err != nil {
		return nil, err
	}

	output, err := NewOutput(cfg.Name, cfg.Output, cfg.OutProtoCfg, cfg.OutExtend, NewCommonCounter(cfg.UUID, cfg.Output), NewCommonCounter(cfg.UUID, cfg.Output))
	if err != nil {
		return nil, err
	}

	c, err := NewCryptoHelperWithConfig(cfg)
	if err != nil {
		return nil, err
	}

	s := &Frpc{
		cfg:          cfg,
		worker:       worker,
		output:       output,
		cryptoHelper: c,
		bandwidth:    NewBandwidthTracker(output.FrameHeaderEnabled()),
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
	s.startProbe()
	return nil
}

func (s *Frpc) Close() error {
	s.worker.Close()
	if s.outputProbe != nil {
		s.outputProbe.Close()
	}
	if s.probeChannel != nil {
		s.probeChannel.Close()
	}
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

	srcStream, role, err := s.cryptoHelper.WrapSrcWithRole(src)
	if err != nil {
		logrus.WithError(err).Error("wrap src")
		return
	}
	if role == streamRoleProbe {
		if frameStream, ok := srcStream.(*FrameStream); ok {
			if err := frameStream.ServeControlLoop(); err != nil && !errors.Is(err, io.EOF) {
				logrus.WithError(err).Debug("serve probe stream")
			}
			return
		}
		logrus.Error("probe role requires frame stream")
		return
	}
	if role == streamRoleBandwidth {
		if frameStream, ok := srcStream.(*FrameStream); ok {
			if err := ServeBandwidthLoop(frameStream); err != nil {
				logrus.WithError(err).Debug("serve bandwidth stream")
			}
			return
		}
		logrus.Error("bandwidth role requires frame stream")
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

	s.cryptoHelper.PipePrepared(dstStream, srcStream)
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

func (s *Frpc) QualitySummary() QualitySummary {
	return s.output.QualitySummary()
}

func (s *Frpc) QualityDetails() map[string]QualitySnapshot {
	return map[string]QualitySnapshot{
		"output": s.output.QualitySnapshot(),
	}
}

func (s *Frpc) BandwidthSummary() BandwidthSummary {
	return s.bandwidth.Summary()
}

func (s *Frpc) BandwidthTest() (BandwidthSummary, error) {
	s.bandwidthMu.Lock()
	defer s.bandwidthMu.Unlock()

	if !s.output.FrameHeaderEnabled() {
		return s.bandwidth.DisabledSummary("frame_header_enable disabled"), nil
	}

	summary, err := RunBandwidthTest(s.output, s.cryptoHelper)
	if err != nil {
		return s.bandwidth.ErrorSummary(err), err
	}
	s.bandwidth.Store(summary)
	return summary, nil
}

func (s *Frpc) startProbe() {
	if s.output.FrameHeaderEnabled() {
		s.probeChannel = NewProbeChannel(s.output, s.cryptoHelper)
		s.probeChannel.Run()
		s.outputProbe = NewFrameProbeRunner(s.probeChannel, outputTracker(s.output), s.output.ProbeConfig())
		s.outputProbe.Run()
	}
}

func (s *Frpc) Probe() bool {
	if s.outputProbe == nil {
		return false
	}
	return s.outputProbe.TriggerProbe()
}
