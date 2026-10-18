package tun

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/0990/gotun/pkg/probehistory"
)

const routeQualityWindow = 20

// RouteMemberQuality 是完整业务路径的运行快照，与单跳帧探测分开展示。
type RouteMemberQuality struct {
	Status              string    `json:"status"`
	Samples             int       `json:"samples"`
	BadSamples          int       `json:"bad_samples"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LastDurationMS      int64     `json:"last_duration_ms"`
	LastError           string    `json:"last_error"`
	Connectivity        string    `json:"connectivity"`
	LastProbeAt         time.Time `json:"last_probe_at"`
}

type routeMemberQualityState struct {
	RouteMemberQuality
	window       []bool
	normalStreak int
	service      Service
}

type routeQualityMonitor struct {
	mu         sync.Mutex
	cfg        RouteConfig
	mgr        *Manager
	states     map[string]*routeMemberQualityState
	current    string
	lastSwitch time.Time
	log        *routeSwitchLog
	ctx        context.Context
	cancel     context.CancelFunc
	probe      func(context.Context, Config, RouteQualityConfig) routeProbeResult
}

func newRouteQualityMonitor(cfg RouteConfig, mgr *Manager, log *routeSwitchLog) *routeQualityMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	q := &routeQualityMonitor{cfg: cfg, mgr: mgr, log: log, states: make(map[string]*routeMemberQualityState), ctx: ctx, cancel: cancel, probe: probeRouteMember}
	for _, name := range cfg.Members {
		q.states[name] = &routeMemberQualityState{RouteMemberQuality: RouteMemberQuality{Status: "evaluating"}}
	}
	return q
}

func memberRunning(s Service) bool { return s != nil && !s.Cfg().Disabled && s.Status() == "running" }

func (q *routeQualityMonitor) chooseLocked(name, reason string, now time.Time) {
	if name == q.current {
		return
	}
	q.log.recordReason(q.current, name, reason)
	q.current, q.lastSwitch = name, now
}

func containsMember(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

func (q *routeQualityMonitor) ensureCurrentLocked(available []string, now time.Time) {
	if containsMember(available, q.current) {
		return
	}
	name, reason := "", "没有运行中的 member"
	if len(available) > 0 {
		name, reason = available[0], "按成员顺序选择运行中的线路"
	}
	q.chooseLocked(name, reason, now)
}

func (q *routeQualityMonitor) preferred(available []string) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ensureCurrentLocked(available, time.Now())
	return q.current
}

func (q *routeQualityMonitor) snapshot() map[string]RouteMemberQuality {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make(map[string]RouteMemberQuality, len(q.states))
	for name, s := range q.states {
		out[name] = s.RouteMemberQuality
	}
	return out
}

// applyRound 只在整轮完成后比较，避免先完成的成员提前占优。
func (q *routeQualityMonitor) applyRound(results map[string]routeProbeResult, available []string, now time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.ctx.Err() != nil {
		return
	}
	allFailed := len(available) > 1
	for _, name := range available {
		if results[name].status != routeProbeFailed {
			allFailed = false
		}
	}
	for name, r := range results {
		s := q.states[name]
		if s == nil {
			continue
		}
		s.Status, s.LastDurationMS, s.LastError, s.Connectivity, s.LastProbeAt = r.status, r.duration.Milliseconds(), r.err, r.connectivity, now
		if r.status == routeProbeFailed {
			s.normalStreak = 0
			if allFailed && r.connectivity == "ok" {
				s.ConsecutiveFailures = 0
			} else {
				s.ConsecutiveFailures++
			}
		} else {
			s.ConsecutiveFailures = 0
			if r.status == routeProbeNormal {
				s.normalStreak++
			} else {
				s.normalStreak = 0
			}
		}
		if allFailed {
			s.Status = routeProbeCommon
			q.recordHistory(s)
			continue
		}
		s.window = append(s.window, r.status != routeProbeNormal)
		if len(s.window) > routeQualityWindow {
			s.window = s.window[1:]
		}
		s.Samples, s.BadSamples = len(s.window), 0
		for _, bad := range s.window {
			if bad {
				s.BadSamples++
			}
		}
		q.recordHistory(s)
	}
	q.ensureCurrentLocked(available, now)
	cur := q.states[q.current]
	if cur == nil || allFailed {
		return
	}
	if cur.ConsecutiveFailures >= 3 {
		for _, name := range available {
			if name != q.current && q.states[name].normalStreak >= 2 {
				q.chooseLocked(name, fmt.Sprintf("故障切换：当前连续失败 %d 次，备用连续正常 %d 次", cur.ConsecutiveFailures, q.states[name].normalStreak), now)
				return
			}
		}
	}
	if cur.Samples < routeQualityWindow || now.Sub(q.lastSwitch) < 60*time.Second {
		return
	}
	best := q.current
	for _, name := range available {
		s := q.states[name]
		if s.Samples == routeQualityWindow && s.ConsecutiveFailures == 0 && s.BadSamples < q.states[best].BadSamples {
			best = name
		}
	}
	if best != q.current && cur.BadSamples-q.states[best].BadSamples >= 3 {
		q.chooseLocked(best, fmt.Sprintf("质量切换：最近 20 次不良由 %d 次降为 %d 次", cur.BadSamples, q.states[best].BadSamples), now)
	}
}

// 仅提交通过成员实例校验的完整轮次；共同失败也保留原始异常信息。
// round 持有 manager 读锁，此处不能再次取 manager 锁。
func (q *routeQualityMonitor) recordHistory(state *routeMemberQualityState) {
	if q.mgr == nil || q.mgr.history == nil || state.service == nil {
		return
	}
	q.mgr.recordProbeHistory(probehistory.Record{
		TunnelUUID: state.service.Cfg().UUID, Time: state.LastProbeAt, Kind: "route",
		RouteUUID: q.cfg.UUID, RouteName: q.cfg.Name, Status: state.Status,
		DurationMS: state.LastDurationMS, Connectivity: state.Connectivity, Error: state.LastError,
	})
}

func (q *routeQualityMonitor) run() {
	ticker := time.NewTicker(time.Duration(q.cfg.Quality.IntervalSec) * time.Second)
	defer ticker.Stop()
	for {
		if q.ctx.Err() != nil {
			return
		}
		q.round()
		select {
		case <-q.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (q *routeQualityMonitor) round() {
	type answer struct {
		name    string
		service Service
		result  routeProbeResult
	}
	ch := make(chan answer, len(q.cfg.Members))
	for _, name := range q.cfg.Members {
		svc, _ := q.mgr.GetService(name)
		go func(name string, svc Service) {
			r := routeProbeResult{status: routeProbeFailed, err: "member 未运行或已禁用", connectivity: "failed"}
			if memberRunning(svc) {
				r = q.probe(q.ctx, svc.Cfg(), q.cfg.Quality)
			}
			ch <- answer{name, svc, r}
		}(name, svc)
	}
	answers := make(map[string]answer, len(q.cfg.Members))
	for range q.cfg.Members {
		select {
		case <-q.ctx.Done():
			return
		case a := <-ch:
			answers[a.name] = a
		}
	}
	results := make(map[string]routeProbeResult, len(answers))
	var available []string
	// 提交期间禁止 member 替换，避免校验后又写入旧实例的结果。
	q.mgr.lock.RLock()
	defer q.mgr.lock.RUnlock()
	for _, name := range q.cfg.Members {
		a := answers[name]
		svc := q.mgr.services[name]
		// 成员重建后的旧探测不能提交；下一轮重新评估。
		if svc != a.service {
			return
		}
		q.mu.Lock()
		if q.states[name].service != svc {
			q.states[name] = &routeMemberQualityState{RouteMemberQuality: RouteMemberQuality{Status: "evaluating"}, service: svc}
		}
		q.mu.Unlock()
		results[name] = a.result
		if memberRunning(svc) {
			available = append(available, name)
		}
	}
	q.applyRound(results, available, time.Now())
}
