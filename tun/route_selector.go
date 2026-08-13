package tun

import (
	"errors"
	"fmt"
)

// routeSelector 智能路由的共享选址器
// 按 RouteConfig.Members 的顺序（索引 0 优先级最高），经 manager 惰性解析 member，
// 依据 member 的 Service.QualitySummary() 健康分类挑选可用 member。
// 两种模式（output/input）共用这一套选址逻辑。
type routeSelector struct {
	mgr     *Manager
	members []string
}

func newRouteSelector(mgr *Manager, members []string) *routeSelector {
	return &routeSelector{mgr: mgr, members: members}
}

// memberAlive 判断 member 是否可参与选址：存在、未禁用、未宕机
// 健康为 unknown（未开帧头探测，QualityStatusDisabled）的 member 仍可被选中
func memberAlive(s Service) bool {
	if s == nil {
		return false
	}
	if s.Cfg().Disabled {
		return false
	}
	// 生命周期状态非 running（如 output run:<err>）视为不可用
	if s.Status() != "running" {
		return false
	}
	// 健康被判定为 down 的跳过；up/degraded/disabled(unknown) 均可选
	if s.QualitySummary().Status == QualityStatusDown {
		return false
	}
	return true
}

// candidates 返回按优先级排序、当前可用的 member（惰性解析，member 重启/编辑后仍可命中）
func (r *routeSelector) candidates() []Service {
	var out []Service
	for _, name := range r.members {
		svc, ok := r.mgr.GetService(name)
		if !ok {
			continue
		}
		if memberAlive(svc) {
			out = append(out, svc)
		}
	}
	return out
}

// preferred 返回当前首选（优先级最高且可用）的 member 名称；无可用 member 时返回空串
func (r *routeSelector) preferred() string {
	for _, name := range r.members {
		svc, ok := r.mgr.GetService(name)
		if !ok {
			continue
		}
		if memberAlive(svc) {
			return name
		}
	}
	return ""
}

// tryForward 对候选 member 依次尝试某种「转发」操作（fn），失败顺延到下一个，
// 返回首个成功结果；全部失败则返回最后一次的错误。
// fn 返回 (结果, 是否成功, error)。
func (r *routeSelector) tryForward(fn func(svc Service) (bool, error)) error {
	cands := r.candidates()
	if len(cands) == 0 {
		return errors.New("没有可用的 member（全部禁用/未运行/健康为 down）")
	}

	var lastErr error
	for _, svc := range cands {
		ok, err := fn(svc)
		if ok {
			return nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("所有可用 member 转发均失败")
	}
	return fmt.Errorf("所有 member 均不可用: %w", lastErr)
}
