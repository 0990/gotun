package tun

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/0990/gotun/core"
	"github.com/sirupsen/logrus"
)

// routeService 智能路由入口 service，同时承载出口(output)/入口(input)两种模式。
// 与 Server/Frpc/Frps 并列，是一个受 manager 管理的 Service。
type routeService struct {
	cfg      RouteConfig
	mgr      *Manager
	selector *routeSelector

	// 出口模式专有
	input        input
	output       output
	cryptoHelper *CryptoHelper

	// 入口模式（透明转发）专有
	listener net.Listener

	bandwidth *BandwidthTracker
	closeOnce sync.Once

	StatusX
}

// newRouteService 按模式构造智能路由 service（需要 manager 以解析 member）
func newRouteService(cfg RouteConfig, mgr *Manager) (*routeService, error) {
	if err := cfg.validateBasic(); err != nil {
		return nil, err
	}

	s := &routeService{
		cfg:       cfg,
		mgr:       mgr,
		selector:  newRouteSelector(mgr, cfg.Members),
		bandwidth: NewBandwidthTracker(false), // 智能路由自身不做带宽测试
	}
	s.SetStatus("init")

	if cfg.Mode == RouteModeOutput {
		if err := s.buildOutputMode(); err != nil {
			return nil, err
		}
	}
	// 入口模式的 listener 在 Run() 中建立
	return s, nil
}

// buildOutputMode 构造出口模式：本地 input + 故障转移组合 output
func (s *routeService) buildOutputMode() error {
	proto, addr, err := parseProtocol(s.cfg.Listen)
	if err != nil {
		return err
	}
	// 复用普通 tunnel 的 input 配置形态（proto@addr），InProtoCfg 为空
	_ = proto
	_ = addr

	input, err := newInput(s.cfg.Listen, "", NewUplinkCounter(s.cfg.Name, s.cfg.Listen), NewDownlinkCounter(s.cfg.Name, s.cfg.Listen))
	if err != nil {
		return err
	}

	// 入口自身不加解密（member 的 output 已含各自的加密），用空 crypto 配置
	ch, err := NewCryptoHelper("", "", "", "", Extend{}, Extend{})
	if err != nil {
		return err
	}

	s.input = input
	s.output = newFailoverOutput(s.cfg.Name, s.selector)
	s.cryptoHelper = ch
	return nil
}

func (s *routeService) Run() error {
	if s.cfg.Disabled {
		// 禁用入口不监听
		s.SetStatus("disabled")
		return nil
	}
	if s.cfg.Mode == RouteModeInput {
		return s.runInputMode()
	}
	return s.runOutputMode()
}

// runOutputMode 出口模式：与普通 Server 相同的 input->output 数据路径
func (s *routeService) runOutputMode() error {
	s.SetStatus("output run...")
	if err := s.output.Run(); err != nil {
		s.SetStatus(fmt.Sprintf("output run:%s", err.Error()))
		return err
	}

	s.SetStatus("input run...")
	s.input.SetOnNewStream(s.handleOutputStream)
	if err := s.input.Run(); err != nil {
		s.SetStatus(fmt.Sprintf("input run:%s", err.Error()))
		return err
	}

	s.SetStatus("running")
	return nil
}

// handleOutputStream 出口模式数据面：与 Server.handleInputStream 同构，
// 只是 output 换成了 failoverOutput（内部已做选址+顺延）
func (s *routeService) handleOutputStream(src core.IStream) {
	defer src.Close()

	srcStream, err := s.cryptoHelper.WrapSrc(src)
	if err != nil {
		logrus.WithError(err).Error("wrap src")
		return
	}

	dst, err := s.output.GetStream()
	if err != nil {
		logrus.WithError(err).Error("route output openStream")
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

// runInputMode 入口模式：裸 TCP 透明转发
func (s *routeService) runInputMode() error {
	s.SetStatus("input run...")
	lis, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		s.SetStatus(fmt.Sprintf("input run:%s", err.Error()))
		return err
	}
	s.listener = lis
	s.SetStatus("running")
	go s.serveInput()
	return nil
}

// serveInput 入口模式 accept 循环
func (s *routeService) serveInput() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			logrus.WithError(err).Error("route input accept")
			return
		}
		go s.handleInputConn(conn)
	}
}

// handleInputConn 入口模式数据面：按选址把字节透明转发到所选 member 的 input 监听地址
func (s *routeService) handleInputConn(src net.Conn) {
	defer src.Close()

	err := s.selector.tryForward(func(svc Service) (bool, error) {
		_, addr, err := parseProtocol(svc.Cfg().Input)
		if err != nil {
			return false, err
		}
		dst, err := net.Dial("tcp", addr)
		if err != nil {
			return false, err
		}
		// 命中：开始双向转发（阻塞直至连接结束）
		relayConn(src, dst)
		return true, nil
	})
	if err != nil {
		logrus.WithError(err).WithField("route", s.cfg.Name).Debug("route input no available member")
	}
}

// relayConn 双向拷贝，任一方向结束即关闭两端
func relayConn(a, b net.Conn) {
	defer a.Close()
	defer b.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(b, a)
		if tc, ok := b.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(a, b)
		if tc, ok := a.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	wg.Wait()
}

func (s *routeService) Close() error {
	s.closeOnce.Do(func() {
		if s.cfg.Mode == RouteModeOutput {
			if s.input != nil {
				s.input.Close()
			}
			if s.output != nil {
				s.output.Close()
			}
			return
		}
		if s.listener != nil {
			s.listener.Close()
		}
	})
	return nil
}

func (s *routeService) Cfg() Config {
	// Service 接口要求返回 tunnel Config；智能路由用 RouteConfig。
	// 仅填充共享字段（Name/UUID/Disabled/CreatedAt），供 manager 以名称为键管理。
	return Config{
		UUID:      s.cfg.UUID,
		Name:      s.cfg.Name,
		Disabled:  s.cfg.Disabled,
		CreatedAt: s.cfg.CreatedAt,
	}
}

// RouteCfg 返回智能路由自身的配置（供管理 API 使用）
func (s *routeService) RouteCfg() RouteConfig {
	return s.cfg
}

// PreferredMember 返回当前首选 member 名称（供管理列表显示实时状态）
func (s *routeService) PreferredMember() string {
	if s.selector == nil {
		return ""
	}
	return s.selector.preferred()
}

// MemberHealth 返回各 member 的健康状态（member 名称 -> up/degraded/down/disabled）
func (s *routeService) MemberHealth() map[string]string {
	out := map[string]string{}
	if s.mgr == nil {
		return out
	}
	for _, name := range s.cfg.Members {
		if svc, ok := s.mgr.GetService(name); ok {
			out[name] = svc.QualitySummary().Status
			continue
		}
		out[name] = QualityStatusDisabled
	}
	return out
}

// QualitySummary 入口健康 = 当前首选 member 的健康；无可用 member 报 down
func (s *routeService) QualitySummary() QualitySummary {
	for _, svc := range s.selector.candidates() {
		return svc.QualitySummary()
	}
	return QualitySummary{Status: QualityStatusDown, LastError: "no available member"}
}

// QualityDetails 报告各 member 的健康快照，key 为 member 名称
func (s *routeService) QualityDetails() map[string]QualitySnapshot {
	out := map[string]QualitySnapshot{}
	for _, name := range s.cfg.Members {
		if svc, ok := s.mgr.GetService(name); ok {
			if srv, isServer := svc.(*Server); isServer {
				out[name] = srv.output.QualitySnapshot()
				continue
			}
		}
		out[name] = QualitySnapshot{Status: QualityStatusDisabled}
	}
	return out
}

func (s *routeService) BandwidthSummary() BandwidthSummary {
	return s.bandwidth.Summary()
}

// BandwidthTest 智能路由自身不提供带宽测试
func (s *routeService) BandwidthTest() (BandwidthSummary, error) {
	return s.bandwidth.DisabledSummary("smart route: use member bandwidth test"), nil
}

// Probe 触发各 member 的探测
func (s *routeService) Probe() bool {
	triggered := false
	for _, name := range s.cfg.Members {
		if svc, ok := s.mgr.GetService(name); ok {
			if svc.Probe() {
				triggered = true
			}
		}
	}
	return triggered
}
