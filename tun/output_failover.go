package tun

import (
	"errors"

	"github.com/0990/gotun/core"
)

// streamDialer 抽象「能拨出流」的能力，failoverOutput 只依赖这一小接口。
// 具体由 member 的 output（*Output）实现。
type streamDialer interface {
	GetStream() (core.IStream, error)
	GetProbeStream() (core.IStream, error)
	GetBandwidthStream() (core.IStream, error)
}

// dialerOf 从 member Service 取出其拨号器（仅 *Server 具备 output；frpc/frps 等不具备）
func dialerOf(svc Service) (streamDialer, bool) {
	if srv, ok := svc.(*Server); ok {
		return srv.output, true
	}
	return nil, false
}

// failoverOutput 出口模式的组合输出：实现现有 output 接口，
// 自身不持有 socket，把每次取流委托给按优先级+健康选中的 member 的 output。
type failoverOutput struct {
	name     string
	selector *routeSelector
}

func newFailoverOutput(name string, selector *routeSelector) *failoverOutput {
	return &failoverOutput{name: name, selector: selector}
}

// pickDialer 选出当前优先级最高且具备拨号能力的 member 的拨号器
func (f *failoverOutput) pickDialer() (streamDialer, error) {
	var lastErr error
	for _, svc := range f.selector.candidates() {
		d, ok := dialerOf(svc)
		if ok {
			return d, nil
		}
		lastErr = errors.New("member 非 Server 类型，无 output 可拨号: " + svc.Cfg().Name)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("没有可用 member（全部禁用/未运行/健康为 down/无 output）")
}

// getStream 取流并对失败做顺延：首选 member 拨号失败则尝试下一个
func (f *failoverOutput) getStream(pick func(d streamDialer) (core.IStream, error)) (core.IStream, error) {
	var lastErr error
	cands := f.selector.candidates()
	if len(cands) == 0 {
		return nil, errors.New("没有可用的 member（全部禁用/未运行/健康为 down）")
	}
	for _, svc := range cands {
		d, ok := dialerOf(svc)
		if !ok {
			lastErr = errors.New("member 无 output 可拨号: " + svc.Cfg().Name)
			continue
		}
		stream, err := pick(d)
		if err == nil {
			return stream, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (f *failoverOutput) Run() error {
	// member 由 manager 独立运行，无需在此预建连接
	return nil
}

func (f *failoverOutput) Close() error {
	return nil
}

func (f *failoverOutput) GetStream() (core.IStream, error) {
	return f.getStream(func(d streamDialer) (core.IStream, error) {
		return d.GetStream()
	})
}

func (f *failoverOutput) GetProbeStream() (core.IStream, error) {
	return f.getStream(func(d streamDialer) (core.IStream, error) {
		return d.GetProbeStream()
	})
}

func (f *failoverOutput) GetBandwidthStream() (core.IStream, error) {
	return f.getStream(func(d streamDialer) (core.IStream, error) {
		return d.GetBandwidthStream()
	})
}

// QualitySummary 报告当前首选 member 的健康；无可用 member 时报 down
func (f *failoverOutput) QualitySummary() QualitySummary {
	for _, svc := range f.selector.candidates() {
		return svc.QualitySummary()
	}
	return QualitySummary{Status: QualityStatusDown, LastError: "no available member"}
}

// QualitySnapshot 报告当前首选 member 的快照；无可用 member 时报 down
func (f *failoverOutput) QualitySnapshot() QualitySnapshot {
	for _, svc := range f.selector.candidates() {
		if srv, ok := svc.(*Server); ok {
			return srv.output.QualitySnapshot()
		}
	}
	return QualitySnapshot{Status: QualityStatusDown}
}

// FrameHeaderEnabled 当前首选 member 是否开启帧头（决定探测/带宽测试是否可用）
func (f *failoverOutput) FrameHeaderEnabled() bool {
	for _, svc := range f.selector.candidates() {
		if srv, ok := svc.(*Server); ok {
			return srv.output.FrameHeaderEnabled()
		}
	}
	return false
}

// ProbeConfig 返回当前首选 member 的探测配置
func (f *failoverOutput) ProbeConfig() Extend {
	for _, svc := range f.selector.candidates() {
		if srv, ok := svc.(*Server); ok {
			return srv.output.ProbeConfig()
		}
	}
	return Extend{}
}
