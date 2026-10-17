package tun

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func qualityMonitorForTest(t *testing.T) (*routeQualityMonitor, time.Time) {
	t.Helper()
	cfg := RouteConfig{Members: []string{"a", "b"}}
	cfg.Quality.normalize()
	q := newRouteQualityMonitor(cfg, nil, &routeSwitchLog{})
	t.Cleanup(q.cancel)
	now := time.Now()
	q.current, q.lastSwitch = "a", now.Add(-time.Hour)
	return q, now
}
func qualityRound(q *routeQualityMonitor, a, b string, now time.Time) {
	q.applyRound(map[string]routeProbeResult{"a": {status: a}, "b": {status: b}}, []string{"a", "b"}, now)
}

func TestRouteQualitySwitchRules(t *testing.T) {
	t.Run("窗口满才切且不抢回", func(t *testing.T) {
		q, now := qualityMonitorForTest(t)
		for i := 0; i < 19; i++ {
			qualityRound(q, routeProbeSlow, routeProbeNormal, now.Add(time.Duration(i)*time.Second))
		}
		if q.current != "a" {
			t.Fatal("窗口未满不应普通切换")
		}
		qualityRound(q, routeProbeNormal, routeProbeNormal, now.Add(20*time.Second))
		if q.current != "b" || !strings.Contains(q.log.list()[0].Reason, "质量切换") {
			t.Fatal("应质量切到 b")
		}
		for i := 0; i < 22; i++ {
			qualityRound(q, routeProbeNormal, routeProbeNormal, now.Add(time.Duration(21+i)*time.Second))
		}
		if q.current != "b" {
			t.Fatal("质量相同后 a 不应抢回")
		}
	})
	t.Run("微小差别保持", func(t *testing.T) {
		q, now := qualityMonitorForTest(t)
		for i := 0; i < 20; i++ {
			a := routeProbeNormal
			if i < 2 {
				a = routeProbeSlow
			}
			qualityRound(q, a, routeProbeNormal, now.Add(time.Duration(i)*time.Second))
		}
		if q.current != "a" {
			t.Fatal("只改善两次不应切换")
		}
	})
	t.Run("普通冷却故障绕过", func(t *testing.T) {
		q, now := qualityMonitorForTest(t)
		q.lastSwitch = now
		for i := 0; i < 20; i++ {
			qualityRound(q, routeProbeSlow, routeProbeNormal, now.Add(time.Duration(i)*time.Second))
		}
		if q.current != "a" {
			t.Fatal("冷却期间不应普通切换")
		}
		for i := 0; i < 3; i++ {
			qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(time.Duration(21+i)*time.Second))
		}
		if q.current != "b" || !strings.Contains(q.log.list()[0].Reason, "故障切换") {
			t.Fatal("故障切换应绕过冷却")
		}
	})
	t.Run("备用需两次正常", func(t *testing.T) {
		q, now := qualityMonitorForTest(t)
		qualityRound(q, routeProbeFailed, routeProbeSlow, now)
		qualityRound(q, routeProbeFailed, routeProbeSlow, now.Add(time.Second))
		qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(2*time.Second))
		if q.current != "a" {
			t.Fatal("备用只有一次正常不应故障切换")
		}
		qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(3*time.Second))
		if q.current != "b" {
			t.Fatal("备用两次正常后应故障切换")
		}
	})
	t.Run("共同失败不污染窗口", func(t *testing.T) {
		q, now := qualityMonitorForTest(t)
		qualityRound(q, routeProbeNormal, routeProbeNormal, now)
		for i := 0; i < 5; i++ {
			q.applyRound(map[string]routeProbeResult{"a": {status: routeProbeFailed, connectivity: "ok"}, "b": {status: routeProbeFailed, connectivity: "ok"}}, []string{"a", "b"}, now.Add(time.Duration(i+1)*time.Second))
		}
		s := q.snapshot()["a"]
		if s.Samples != 1 || s.BadSamples != 0 || s.Status != routeProbeCommon || s.ConsecutiveFailures != 0 || q.current != "a" {
			t.Fatalf("共同异常不应污染评分: %+v", s)
		}
	})
	t.Run("全体不可达后恢复备用", func(t *testing.T) {
		q, now := qualityMonitorForTest(t)
		for i := 0; i < 4; i++ {
			qualityRound(q, routeProbeFailed, routeProbeFailed, now.Add(time.Duration(i)*time.Second))
		}
		qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(5*time.Second))
		qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(6*time.Second))
		if q.current != "b" {
			t.Fatal("应切到已恢复备用")
		}
	})
}

func TestRouteQualitySelectorAndLifecycle(t *testing.T) {
	a, b := aliveSvc("a"), aliveSvc("b")
	m := newMgrWithServices(a, b)
	q, now := qualityMonitorForTest(t)
	q.mgr = m
	sel := newRouteSelector(m, []string{"a", "b"})
	sel.quality = q
	for i := 0; i < 3; i++ {
		qualityRound(q, routeProbeFailed, routeProbeNormal, now.Add(time.Duration(i)*time.Second))
	}
	if sel.preferred() != "b" || sel.candidates()[0] != b {
		t.Fatal("数据面与列表首选不一致")
	}
	q.probe = func(context.Context, Config, RouteQualityConfig) routeProbeResult {
		return routeProbeResult{status: routeProbeNormal}
	}
	q.round()
	m.lock.Lock()
	m.services["a"] = aliveSvc("a")
	m.lock.Unlock()
	q.round()
	if s := q.snapshot()["a"]; s.Samples != 1 {
		t.Fatalf("重建后不应沿用旧窗口: %+v", s)
	}
	q.cancel()
	before := q.snapshot()["a"]
	qualityRound(q, routeProbeSlow, routeProbeNormal, now.Add(time.Minute))
	if q.snapshot()["a"] != before {
		t.Fatal("关闭后不能提交探测")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				_ = q.snapshot()
				_ = sel.preferred()
			}
		}()
	}
	wg.Wait()
}

func TestRouteQualityConfigAndPersistence(t *testing.T) {
	base := RouteConfig{Name: "quality", Mode: RouteModeInput, Listen: "127.0.0.1:0", Members: []string{"a"}, Policy: "quality", Disabled: true}
	if err := base.validateBasic(); err != nil {
		t.Fatal(err)
	}
	if base.Quality.DownloadBytes != 16384 || base.Quality.IntervalSec != 30 {
		t.Fatal("默认值错误")
	}
	for name, change := range map[string]func(*RouteConfig){
		"非HTTPS": func(c *RouteConfig) { c.Quality.DownloadURL = "http://example.com" },
		"内嵌密码":   func(c *RouteConfig) { c.Quality.DownloadURL = "https://u:p@example.com" },
		"非法端口":   func(c *RouteConfig) { c.Quality.DownloadURL = "https://example.com:999999/file" },
		"字节越界":   func(c *RouteConfig) { c.Quality.DownloadBytes = 1048577 },
		"负数字节":   func(c *RouteConfig) { c.Quality.DownloadBytes = -1 },
		"间隔过短":   func(c *RouteConfig) { c.Quality.IntervalSec = 1 },
		"非法状态":   func(c *RouteConfig) { c.Quality.ConnectivityStatus = 500 },
		"超时关系":   func(c *RouteConfig) { c.Quality.NormalMS = 6000 },
		"出口模式":   func(c *RouteConfig) { c.Mode = RouteModeOutput; c.Listen = "tcp@127.0.0.1:0" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := base
			change(&cfg)
			if cfg.validateBasic() == nil {
				t.Fatal("应拒绝非法配置")
			}
		})
	}
	a := aliveSvc("a")
	a.cfg.Input = "socks5x@0.0.0.0:1080"
	m := newMgrWithServices(a)
	m.tunDir = t.TempDir()
	base.Quality.DownloadURL = "https://download.example.com/probe.bin"
	base.Quality.ConnectivityURL = "https://check.example.com/ok"
	if err := m.AddRoute(base, true); err != nil {
		t.Fatal(err)
	}
	cfgs, err := loadAllRouteFile(m.tunDir)
	if err != nil || len(cfgs) != 1 || cfgs[0].Quality != base.Quality {
		t.Fatalf("持久化失败: %+v %v", cfgs, err)
	}
	if err := m.RemoveRoute("quality"); err != nil {
		t.Fatal(err)
	}
	a.cfg.Input = "tcp@127.0.0.1:1080"
	if m.AddRoute(base, false) == nil {
		t.Fatal("非SOCKS5不能静默探测")
	}
}
