package tun

import (
	"errors"
	"testing"
	"time"
)

// fakeService 用于选址逻辑单测的假 Service
type fakeService struct {
	cfg     Config
	status  string
	quality QualitySummary
}

func (f *fakeService) Run() error                                    { return nil }
func (f *fakeService) Close() error                                  { return nil }
func (f *fakeService) Cfg() Config                                   { return f.cfg }
func (f *fakeService) Status() string                                { return f.status }
func (f *fakeService) QualitySummary() QualitySummary                { return f.quality }
func (f *fakeService) QualityDetails() map[string]QualitySnapshot    { return nil }
func (f *fakeService) BandwidthSummary() BandwidthSummary            { return BandwidthSummary{} }
func (f *fakeService) BandwidthTest() (BandwidthSummary, error)      { return BandwidthSummary{}, nil }
func (f *fakeService) Probe() bool                                   { return false }

func newMgrWithServices(svcs ...Service) *Manager {
	m := NewManager("")
	for _, s := range svcs {
		m.services[s.Cfg().Name] = s
	}
	return m
}

func aliveSvc(name string) *fakeService {
	return &fakeService{
		cfg:     Config{Name: name},
		status:  "running",
		quality: QualitySummary{Status: QualityStatusUp},
	}
}

func TestRouteSelector_PriorityOrder(t *testing.T) {
	a, b := aliveSvc("a"), aliveSvc("b")
	m := newMgrWithServices(a, b)
	sel := newRouteSelector(m, []string{"a", "b"})

	cands := sel.candidates()
	if len(cands) != 2 || cands[0].Cfg().Name != "a" || cands[1].Cfg().Name != "b" {
		t.Fatalf("应按优先级顺序返回 member: %+v", cands)
	}
	if got := sel.preferred(); got != "a" {
		t.Fatalf("首选应为 a, 得到 %s", got)
	}
}

func TestRouteSelector_SkipDownAndDisabled(t *testing.T) {
	a := aliveSvc("a")
	a.quality = QualitySummary{Status: QualityStatusDown} // down 被跳过
	b := aliveSvc("b")
	c := aliveSvc("c")
	c.cfg.Disabled = true // 禁用被跳过
	m := newMgrWithServices(a, b, c)
	sel := newRouteSelector(m, []string{"a", "b", "c"})

	if got := sel.preferred(); got != "b" {
		t.Fatalf("a=down 应切到 b, 得到 %s", got)
	}
}

func TestRouteSelector_UnknownHealthStillEligible(t *testing.T) {
	a := aliveSvc("a")
	a.quality = QualitySummary{Status: QualityStatusDisabled} // 未开帧头=unknown，仍可选
	m := newMgrWithServices(a)
	sel := newRouteSelector(m, []string{"a"})

	if got := sel.preferred(); got != "a" {
		t.Fatalf("unknown(disabled) 健康仍应可选, 得到 %q", got)
	}
}

func TestRouteSelector_NotRunningSkipped(t *testing.T) {
	a := aliveSvc("a")
	a.status = "output run: dial fail" // 生命周期非 running
	m := newMgrWithServices(a)
	sel := newRouteSelector(m, []string{"a"})

	if got := sel.preferred(); got != "" {
		t.Fatalf("非 running 应被跳过, 得到 %q", got)
	}
}

func TestRouteSelector_Recovery(t *testing.T) {
	a := aliveSvc("a")
	b := aliveSvc("b")
	m := newMgrWithServices(a, b)
	sel := newRouteSelector(m, []string{"a", "b"})

	a.quality = QualitySummary{Status: QualityStatusDown}
	if got := sel.preferred(); got != "b" {
		t.Fatalf("a down 应为 b, 得到 %s", got)
	}
	// a 恢复后回到首选
	a.quality = QualitySummary{Status: QualityStatusUp}
	if got := sel.preferred(); got != "a" {
		t.Fatalf("a 恢复应回到 a, 得到 %s", got)
	}
}

func TestRouteSelector_TryForwardFallthrough(t *testing.T) {
	a := aliveSvc("a")
	b := aliveSvc("b")
	m := newMgrWithServices(a, b)
	sel := newRouteSelector(m, []string{"a", "b"})

	tried := []string{}
	err := sel.tryForward(func(svc Service) (bool, error) {
		tried = append(tried, svc.Cfg().Name)
		if svc.Cfg().Name == "a" {
			return false, errors.New("a 拨号失败") // a 失败顺延
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("b 成功则不应报错: %v", err)
	}
	if len(tried) != 2 || tried[0] != "a" || tried[1] != "b" {
		t.Fatalf("应先 a 失败后顺延 b: %v", tried)
	}
}

func TestRouteSelector_TryForwardAllFail(t *testing.T) {
	a := aliveSvc("a")
	m := newMgrWithServices(a)
	sel := newRouteSelector(m, []string{"a"})

	err := sel.tryForward(func(svc Service) (bool, error) {
		return false, errors.New("fail")
	})
	if err == nil {
		t.Fatal("全部失败应返回错误")
	}
}

func TestRouteSelector_TryForwardNoCandidate(t *testing.T) {
	a := aliveSvc("a")
	a.quality = QualitySummary{Status: QualityStatusDown}
	m := newMgrWithServices(a)
	sel := newRouteSelector(m, []string{"a"})

	err := sel.tryForward(func(svc Service) (bool, error) {
		return true, nil
	})
	if err == nil {
		t.Fatal("无可用 member 应返回错误")
	}
}

func TestRouteConfig_ValidateBasic(t *testing.T) {
	cases := []struct {
		name    string
		cfg     RouteConfig
		wantErr bool
	}{
		{"output ok", RouteConfig{Name: "x", Mode: RouteModeOutput, Listen: "tcp@0.0.0.0:1000", Members: []string{"a"}}, false},
		{"input ok", RouteConfig{Name: "x", Mode: RouteModeInput, Listen: "0.0.0.0:1001", Members: []string{"a"}}, false},
		{"空名称", RouteConfig{Name: "", Mode: RouteModeOutput, Listen: "tcp@0.0.0.0:1000", Members: []string{"a"}}, true},
		{"空member", RouteConfig{Name: "x", Mode: RouteModeOutput, Listen: "tcp@0.0.0.0:1000", Members: nil}, true},
		{"output 缺协议", RouteConfig{Name: "x", Mode: RouteModeOutput, Listen: "0.0.0.0:1000", Members: []string{"a"}}, true},
		{"input 带协议", RouteConfig{Name: "x", Mode: RouteModeInput, Listen: "tcp@0.0.0.0:1001", Members: []string{"a"}}, true},
		{"未知模式", RouteConfig{Name: "x", Mode: "bogus", Listen: "tcp@0.0.0.0:1000", Members: []string{"a"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.validateBasic()
			if c.wantErr && err == nil {
				t.Fatalf("应报错但通过了: %+v", c.cfg)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("不应报错但失败了: %v", err)
			}
		})
	}
}

func TestRoutePersistence_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := RouteConfig{
		UUID: "u1", Name: "web", Mode: RouteModeOutput,
		Listen: "tcp@0.0.0.0:1000", Members: []string{"a", "b"},
		Policy: "priority", CreatedAt: time.Now(),
	}
	if err := createRouteFile(dir, cfg); err != nil {
		t.Fatalf("createRouteFile 失败: %v", err)
	}

	cfgs, err := loadAllRouteFile(dir)
	if err != nil {
		t.Fatalf("loadAllRouteFile 失败: %v", err)
	}
	if len(cfgs) != 1 || cfgs[0].Name != "web" || cfgs[0].Mode != RouteModeOutput || len(cfgs[0].Members) != 2 {
		t.Fatalf("持久化往返不一致: %+v", cfgs)
	}

	// .tun 的加载不应看到 .route 文件，反之亦然
	tuns, err := loadAllServiceFile(dir)
	if err != nil {
		t.Fatalf("loadAllServiceFile 失败: %v", err)
	}
	if len(tuns) != 0 {
		t.Fatalf(".route 文件不应被 .tun 加载: %+v", tuns)
	}
}

func TestManager_RouteCRUD(t *testing.T) {
	a, b := aliveSvc("m1"), aliveSvc("m2")
	m := NewManager(t.TempDir())
	m.services[a.Cfg().Name] = a
	m.services[b.Cfg().Name] = b

	cfg := RouteConfig{Name: "r1", Mode: RouteModeOutput, Listen: "tcp@127.0.0.1:0", Members: []string{"m1", "m2"}}
	if err := m.AddRoute(cfg, true); err != nil {
		t.Fatalf("AddRoute 失败: %v", err)
	}
	if _, ok := m.GetRoute("r1"); !ok {
		t.Fatal("AddRoute 后应能 GetRoute")
	}

	// 引用不存在的 member 应报错
	bad := RouteConfig{Name: "r2", Mode: RouteModeOutput, Listen: "tcp@127.0.0.1:0", Members: []string{"nope"}}
	if err := m.AddRoute(bad, true); err == nil {
		t.Fatal("引用不存在 member 应报错")
	}

	// 名称与已有 route 冲突
	dup := RouteConfig{Name: "r1", Mode: RouteModeOutput, Listen: "tcp@127.0.0.1:0", Members: []string{"m1"}}
	if err := m.AddRoute(dup, true); err == nil {
		t.Fatal("重名应报错")
	}

	// set_disabled
	if err := m.SetRouteDisabled("r1", true); err != nil {
		t.Fatalf("SetRouteDisabled 失败: %v", err)
	}
	rs, _ := m.GetRoute("r1")
	if !rs.RouteCfg().Disabled {
		t.Fatal("禁用后 Disabled 应为 true")
	}

	// delete
	rs, _ = m.GetRoute("r1")
	if err := m.RemoveRouteByUUID(rs.RouteCfg().UUID); err != nil {
		t.Fatalf("RemoveRouteByUUID 失败: %v", err)
	}
	if _, ok := m.GetRoute("r1"); ok {
		t.Fatal("删除后不应再存在")
	}
}

func TestManager_InputModeProtoConsistency(t *testing.T) {
	// member input 协议不一致时，入口模式应报错
	s1 := aliveSvc("s1")
	s1.cfg.Input = "socks5x@0.0.0.0:1081"
	s2 := aliveSvc("s2")
	s2.cfg.Input = "tcp@0.0.0.0:1082"
	m := newMgrWithServices(s1, s2)

	cfg := RouteConfig{Name: "in1", Mode: RouteModeInput, Listen: "127.0.0.1:0", Members: []string{"s1", "s2"}}
	if err := m.AddRoute(cfg, false); err == nil {
		t.Fatal("input 协议不一致应报错")
	}

	// 协议一致则应通过
	s3 := aliveSvc("s3")
	s3.cfg.Input = "socks5x@0.0.0.0:1083"
	m2 := newMgrWithServices(s1, s3)
	cfg2 := RouteConfig{Name: "in2", Mode: RouteModeInput, Listen: "127.0.0.1:0", Members: []string{"s1", "s3"}}
	if err := m2.AddRoute(cfg2, false); err != nil {
		t.Fatalf("input 协议一致应通过: %v", err)
	}
}
