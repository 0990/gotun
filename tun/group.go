package tun

import (
	"fmt"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/syncx"
	"github.com/sirupsen/logrus"
	"math"
	"sync"
	"time"
)

type Group struct {
	cfg          GroupConfig
	input        input
	pingRunners  syncx.Map[*PingRunner, IOConfig]
	cryptoHelper *CryptoHelper

	sync.RWMutex

	StatusX
	close chan struct{}

	outputAddr string
	output     output
}

func newGroup(cfg GroupConfig) (*Group, error) {
	input, err := newInput(cfg.Input.Addr, cfg.Input.ProtoCfg)
	if err != nil {
		return nil, err
	}

	m := syncx.Map[*PingRunner, IOConfig]{}
	for _, outputCfg := range cfg.Outputs {
		s, err := newPingRunner(outputCfg.Ping)
		if err != nil {
			return nil, err
		}
		s.Run()
		m.Store(s, outputCfg.Output)
	}
	s := &Group{cfg: cfg, pingRunners: m, close: make(chan struct{}, 1), input: input}
	s.SetStatus("init")
	return s, nil
}

func (g *Group) Run() error {
	g.SetStatus("input run...")
	g.input.SetOnNewStream(g.handleInputStream)
	err := g.input.Run()
	if err != nil {
		g.SetStatus(fmt.Sprintf("input run:%s", err.Error()))
		return err
	}

	g.SetStatus("running")

	go g.keepSelectBestOutput()
	return nil
}

func (g *Group) keepSelectBestOutput() {
	ticker := time.NewTicker(time.Second * time.Duration(2))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			g.selectBestOutput()
		case <-g.close:
			return
		}
	}
}

func (g *Group) Close() error {
	g.RLock()
	defer g.RUnlock()

	close(g.close)
	g.input.Close()
	if g.output != nil {
		g.output.Close()
	}
	return nil
}

func (g *Group) selectBestOutput() {
	cfg, ok := g.GetBestOutputCfg()
	if !ok {
		return
	}

	g.Lock()
	defer g.Unlock()

	if cfg.Addr == g.outputAddr {
		return
	}

	g.outputAddr = cfg.Addr
	g.createOutput(cfg)
}

func (g *Group) createOutput(config IOConfig) error {
	output, err := NewOutput(config.Addr, config.ProtoCfg, config.Extend)
	if err != nil {
		return err
	}
	//TODO 延迟关闭
	if g.output != nil {
		g.output.Close()
	}
	g.output = output
	return nil
}

func (g *Group) GetBestOutputCfg() (IOConfig, bool) {
	var min int = math.MaxInt
	var result IOConfig
	var find bool = false
	g.pingRunners.Range(func(s *PingRunner, value IOConfig) bool {
		if ping, err := s.GetPing(); err == nil && ping >= 0 && ping < min {
			ping = min
			result = value
			find = true
		}
		return true
	})

	return result, find
}

func (g *Group) handleInputStream(src core.IStream) {
	defer src.Close()

	output, ok := g.getOutput()
	if !ok {
		return
	}
	dst, err := output.GetStream()
	if err != nil {
		logrus.WithError(err).Error("openStream")
		return
	}
	defer dst.Close()

	g.cryptoHelper.Pipe(dst, src)
}

func (g *Group) getOutput() (output, bool) {
	g.RLock()
	defer g.RUnlock()
	if g.output == nil {
		return nil, false
	}

	return g.output, true
}

func (g *Group) Cfg() GroupConfig {
	return g.cfg
}
