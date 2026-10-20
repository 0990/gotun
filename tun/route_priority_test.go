package tun

import (
	"context"
	"strings"
	"testing"
	"time"
)

func priorityMonitorForTest(t *testing.T) (*routeQualityMonitor, time.Time) {
	t.Helper()
	q, now := qualityMonitorForTest(t)
	q.cfg.Policy = "priority"
	return q, now
}

func TestRoutePriorityFullPathSwitchRules(t *testing.T) {
	t.Run("慢请求和窗口差异不转移", func(t *testing.T) {
		q, now := priorityMonitorForTest(t)
		for i := 0; i < 25; i++ {
			qualityRound(q, routeProbeSlow, routeProbeNormal, now.Add(time.Duration(i)*30*time.Second))
		}
		if q.current != "a" || q.snapshot()["a"].BadSamples != 20 {
			t.Fatal("优先级策略应采集同样的指标，但不能因慢请求择优切换")
		}
	})
	t.Run("三次故障转移且绕过冷却", func(t *testing.T) {
		q, now := priorityMonitorForTest(t)
		q.lastSwitch = now
		for i := 0; i < 2; i++ {
			qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(time.Duration(i)*time.Second))
			if q.current != "a" {
				t.Fatal("不足三次失败不能故障转移")
			}
		}
		qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(2*time.Second))
		if q.current != "b" || !strings.Contains(q.log.list()[0].Reason, "故障转移") {
			t.Fatal("三次故障且备用连续正常应转移，不必等窗口满或冷却")
		}
	})
	t.Run("备用慢和一次正常不能转移", func(t *testing.T) {
		q, now := priorityMonitorForTest(t)
		for i := 0; i < 3; i++ {
			qualityRound(q, routeProbeFailed, routeProbeSlow, now.Add(time.Duration(i)*time.Second))
		}
		qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(3*time.Second))
		if q.current != "a" {
			t.Fatal("备用必须连续正常两次")
		}
		qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(4*time.Second))
		if q.current != "b" {
			t.Fatal("备用确认恢复后应转移")
		}
	})
	t.Run("恢复回切需连续正常且冷却结束", func(t *testing.T) {
		q, now := priorityMonitorForTest(t)
		q.current, q.lastSwitch = "b", now
		qualityRound(q, routeProbeNormal, routeProbeNormal, now.Add(10*time.Second))
		qualityRound(q, routeProbeNormal, routeProbeNormal, now.Add(59*time.Second))
		if q.current != "b" {
			t.Fatal("冷却结束前不能回切")
		}
		qualityRound(q, routeProbeSlow, routeProbeNormal, now.Add(60*time.Second))
		qualityRound(q, routeProbeNormal, routeProbeNormal, now.Add(61*time.Second))
		if q.current != "b" || q.snapshot()["a"].ConsecutiveNormal != 1 {
			t.Fatal("慢请求应重置连续正常计数")
		}
		qualityRound(q, routeProbeNormal, routeProbeNormal, now.Add(62*time.Second))
		if q.current != "a" || !strings.Contains(q.log.list()[0].Reason, "优先级恢复回切") {
			t.Fatal("高优先级恢复稳定后应回切，无需窗口满")
		}
	})
	t.Run("多个备用按优先级选择", func(t *testing.T) {
		q, now := priorityMonitorForTest(t)
		q.cfg.Members = []string{"a", "b", "c"}
		q.states["c"] = &routeMemberQualityState{}
		for i := 0; i < 3; i++ {
			q.applyRound(map[string]routeProbeResult{
				"a": {status: routeProbeFailed}, "b": {status: routeProbeNormal}, "c": {status: routeProbeNormal},
			}, q.cfg.Members, now.Add(time.Duration(i)*time.Second))
		}
		if q.current != "b" {
			t.Fatal("多个可靠备用应选择优先级最高的")
		}
	})
	t.Run("共同失败保持且辅助成功清除故障", func(t *testing.T) {
		q, now := priorityMonitorForTest(t)
		qualityRound(q, routeProbeFailed, routeProbeNormal, now)
		for i := 0; i < 4; i++ {
			q.applyRound(map[string]routeProbeResult{
				"a": {status: routeProbeFailed, connectivity: "ok"}, "b": {status: routeProbeFailed, connectivity: "ok"},
			}, q.cfg.Members, now.Add(time.Duration(i+1)*time.Second))
		}
		if s := q.snapshot()["a"]; q.current != "a" || s.ConsecutiveFailures != 0 || s.Samples != 1 || s.Status != routeProbeCommon {
			t.Fatalf("共同失败不应触发切换或污染窗口：%+v", s)
		}
	})
	t.Run("全部故障后备用恢复", func(t *testing.T) {
		q, now := priorityMonitorForTest(t)
		for i := 0; i < 4; i++ {
			qualityRound(q, routeProbeFailed, routeProbeFailed, now.Add(time.Duration(i)*time.Second))
		}
		if q.current != "a" {
			t.Fatal("全部故障应保留当前选择")
		}
		qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(4*time.Second))
		qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(5*time.Second))
		if q.current != "b" {
			t.Fatal("共同故障期间辅助也失败应保留故障计数，备用恢复后转移")
		}
	})
}

func TestRoutePriorityUsesFullPathInsteadOfFrameHealth(t *testing.T) {
	a, b := aliveSvc("a"), aliveSvc("b")
	a.cfg.Input, b.cfg.Input = "socks5x@127.0.0.1:1081", "socks5x@127.0.0.1:1082"
	a.quality.Status, b.quality.Status = QualityStatusUp, QualityStatusDown
	m := newMgrWithServices(a, b)
	cfg := RouteConfig{Name: "入口", Mode: RouteModeInput, Listen: "127.0.0.1:0", Members: []string{"a", "b"}, Policy: "priority"}
	svc, err := newRouteService(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	q := svc.selector.quality
	if q == nil {
		t.Fatal("优先级入口必须创建完整链路探测器")
	}
	q.probe = func(_ context.Context, cfg Config, _ RouteQualityConfig) routeProbeResult {
		if cfg.Name == "a" {
			return routeProbeResult{status: routeProbeFailed, err: "第一跳正常，但后续链路故障"}
		}
		return routeProbeResult{status: routeProbeNormal}
	}
	for i := 0; i < 3; i++ {
		q.round()
	}
	if svc.PreferredMember() != "b" || svc.selector.candidates()[0] != b || svc.MemberQuality()["a"].ConsecutiveFailures != 3 {
		t.Fatal("应按完整链路转移到 b，不受单跳 up/down 干扰")
	}
	// 当前成员被移除，且没有确认正常的备用时，按顺序尝试运行中的成员。
	c := aliveSvc("c")
	m.services["c"] = c
	svc.selector.members = []string{"c", "a", "b"}
	m.lock.Lock()
	delete(m.services, "b")
	m.lock.Unlock()
	if svc.PreferredMember() != "c" {
		t.Fatal("无可靠备用时应按顺序选择运行中的成员")
	}
}

func TestRoutePrioritySkipsUnavailableImmediately(t *testing.T) {
	a, b, c := aliveSvc("a"), aliveSvc("b"), aliveSvc("c")
	m := newMgrWithServices(a, b, c)
	q, _ := priorityMonitorForTest(t)
	q.states["c"] = &routeMemberQualityState{}
	q.states["c"].ConsecutiveNormal = 2
	sel := newRouteSelector(m, []string{"a", "b", "c"})
	sel.quality = q
	a.cfg.Disabled = true
	if sel.preferred() != "c" {
		t.Fatal("当前禁用应立即选择已确认正常的 c，跳过未知的 b")
	}
	c.status = "stopped"
	if sel.preferred() != "b" {
		t.Fatal("无可靠线路时应回退到运行中的 b")
	}
	delete(m.services, "b")
	if sel.preferred() != "" {
		t.Fatal("全部不可用时应返回空选择")
	}
}

func TestRoutePriorityLegacyMigration(t *testing.T) {
	for _, tc := range []struct {
		name, input, encrypt, extend, wantMode string
	}{
		{"兼容旧入口", "socks5x@127.0.0.1:1080", "", "", RouteProbeFullPath},
		{"TCP旧入口", "tcp@127.0.0.1:1080", "", "", RouteProbeLegacyFrame},
		{"加密旧入口", "socks5x@127.0.0.1:1080", "aes", "", RouteProbeLegacyFrame},
		{"帧头旧入口", "socks5x@127.0.0.1:1080", "", `{"frame_header_enable":true}`, RouteProbeLegacyFrame},
		{"重复成员旧入口", "socks5x@127.0.0.1:1080", "", "", RouteProbeLegacyFrame},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := aliveSvc("a")
			a.cfg.Input, a.cfg.InDecryptMode, a.cfg.InExtend = tc.input, tc.encrypt, tc.extend
			m := newMgrWithServices(a)
			m.tunDir = t.TempDir()
			cfg := RouteConfig{Name: "旧入口", Mode: RouteModeInput, Listen: "127.0.0.1:0", Members: []string{"a"}, Disabled: true}
			if tc.name == "重复成员旧入口" {
				cfg.Members = []string{"a", "a"}
			}
			if err := createRouteFile(m.tunDir, cfg); err != nil {
				t.Fatal(err)
			}
			files, err := loadAllRouteFile(m.tunDir)
			if err != nil || len(files) != 1 {
				t.Fatalf("旧配置加载失败：%v", err)
			}
			if err := m.AddRoute(files[0], false); err != nil {
				t.Fatal(err)
			}
			defer m.RemoveRoute(cfg.Name)
			rs, _ := m.GetRoute(cfg.Name)
			got := rs.RouteCfg()
			if got.ProbeMode != tc.wantMode || (rs.MemberQuality() != nil) != (tc.wantMode == RouteProbeFullPath) {
				t.Fatalf("升级结果错误：%+v", got)
			}
			if tc.wantMode == RouteProbeFullPath {
				if got.Quality.IntervalSec != 30 {
					t.Fatal("应补齐完整链路探测默认值")
				}
				return
			}
			if len(got.ProbeWarnings) != 1 || !strings.Contains(got.ProbeWarnings[0], "a：") {
				t.Fatal("兼容入口必须列出具体不支持的成员和原因")
			}
			if err := m.SetRouteDisabled(cfg.Name, false); err != nil {
				t.Fatalf("旧入口仍应允许启停：%v", err)
			}
			if err := m.SetRouteDisabled(cfg.Name, true); err != nil {
				t.Fatal(err)
			}
			files, _ = loadAllRouteFile(m.tunDir)
			if files[0].ProbeMode != RouteProbeLegacyFrame {
				t.Fatal("旧入口启停后必须保留兼容标记")
			}
			if err := m.ReplaceRouteByUUID(got); err == nil {
				t.Fatal("编辑不兼容旧入口应报错，不能静默保留单跳探测")
			}
			fixed := aliveSvc("a")
			fixed.cfg.Input = "socks5x@127.0.0.1:1080"
			m.lock.Lock()
			m.services["a"] = fixed
			m.lock.Unlock()
			got.Members = []string{"a"}
			if err := m.ReplaceRouteByUUID(got); err != nil {
				t.Fatalf("修复成员后应允许升级：%v", err)
			}
			rs, _ = m.GetRoute(cfg.Name)
			if rs.RouteCfg().ProbeMode != RouteProbeFullPath || len(rs.RouteCfg().ProbeWarnings) != 0 {
				t.Fatal("升级后应清除兼容模式和提示")
			}
		})
	}
}

func TestRoutePriorityNewConfigValidation(t *testing.T) {
	a := aliveSvc("a")
	a.cfg.Input = "tcp@127.0.0.1:1080"
	m := newMgrWithServices(a)
	m.tunDir = t.TempDir()
	cfg := RouteConfig{Name: "入口", Mode: RouteModeInput, Listen: "127.0.0.1:0", Members: []string{"a"}, Policy: "priority", Disabled: true}
	if err := m.AddRoute(cfg, true); err == nil {
		t.Fatal("新入口不应接受非 SOCKS5 成员")
	}
	cfg.ProbeMode = RouteProbeFullPath
	if err := m.AddRoute(cfg, false); err == nil {
		t.Fatal("显式完整链路配置即使重启加载也不能降级")
	}
	cfg.ProbeMode = RouteProbeLegacyFrame
	if err := m.AddRoute(cfg, true); err == nil {
		t.Fatal("新建不能指定兼容模式绕过校验")
	}
	cfg.ProbeMode = ""
	a.cfg.Input = "socks5x@127.0.0.1:1080"
	cfg.Quality.DownloadURL = "http://example.com"
	if err := m.AddRoute(cfg, true); err == nil {
		t.Fatal("优先级入口同样要求 HTTPS 探测目标")
	}
	cfg.Quality.DownloadURL = "https://priority.example.com/probe.bin"
	cfg.Quality.IntervalSec = 45
	if err := m.AddRoute(cfg, true); err != nil {
		t.Fatal(err)
	}
	defer m.RemoveRoute(cfg.Name)
	files, err := loadAllRouteFile(m.tunDir)
	if err != nil || len(files) != 1 || files[0].ProbeMode != RouteProbeFullPath || files[0].Quality.IntervalSec != 45 || files[0].Quality.DownloadURL != cfg.Quality.DownloadURL {
		t.Fatalf("优先级入口探测配置持久化失败：%+v %v", files, err)
	}
}
