package tun

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0990/gotun/pkg/probehistory"
)

func TestProbeHistoryCapturesBothKindsAndSurvivesRename(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.OpenProbeHistory(); err != nil {
		t.Fatal(err)
	}
	defer m.CloseProbeHistory()
	cfg := Config{UUID: "member-id", Name: "member-old", Input: "tcp@127.0.0.1:0", Output: "tcp@127.0.0.1:1"}
	service, err := m.prepareManagedService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := service.(*Server)
	s.SetStatus("running")
	m.services[cfg.Name] = s
	tracker := outputTracker(s.output)
	tracker.RecordProbeSuccess(123 * time.Millisecond)
	tracker.RecordProbeFailure(errors.New("帧探测超时"))
	for i, name := range []string{"入口甲", "入口乙"} {
		q := newRouteQualityMonitor(RouteConfig{UUID: name + "-id", Name: name, Policy: []string{"quality", "priority"}[i], Members: []string{cfg.Name}}, m, &routeSwitchLog{})
		q.probe = func(context.Context, Config, RouteQualityConfig) routeProbeResult {
			return routeProbeResult{status: routeProbeSlow, duration: 2500 * time.Millisecond}
		}
		q.round()
		q.cancel()
		q.round() // 关闭后的旧探测不应写入。
	}
	if err := m.history.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	page, err := m.QueryProbeHistory(context.Background(), probehistory.Filter{TunnelUUID: cfg.UUID})
	if err != nil || len(page.Records) != 4 {
		t.Fatalf("应有两条帧探测和两个入口各一条链路探测：%+v %v", page, err)
	}
	if page.Records[0].Kind != "route" || page.Records[0].RouteName != "入口乙" || page.Records[0].DurationMS != 2500 || page.Records[2].Error != "帧探测超时" || page.Records[3].DurationMS != 123 {
		t.Fatalf("原始结果或来源入口丢失：%+v", page.Records)
	}
	// 重命名后仍以相同 UUID 关联历史；同名新建的隧道不能继承旧记录。
	delete(m.services, cfg.Name)
	cfg.Name = "member-new"
	newService, err := m.prepareManagedService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.services[cfg.Name] = newService
	if found, ok := m.GetServiceByUUID(cfg.UUID); !ok || found.Cfg().Name != cfg.Name {
		t.Fatal("重命名后 UUID 应保持可查")
	}
	page, err = m.QueryProbeHistory(context.Background(), probehistory.Filter{TunnelUUID: "another-id"})
	if err != nil || len(page.Records) != 0 {
		t.Fatal("不同 UUID 不能读到旧历史")
	}
}

func TestCommonFailureIsRecordedWithoutPollutingWindow(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.OpenProbeHistory(); err != nil {
		t.Fatal(err)
	}
	defer m.CloseProbeHistory()
	for _, name := range []string{"a", "b"} {
		svc := aliveSvc(name)
		svc.cfg.UUID = name + "-id"
		m.services[name] = svc
	}
	q := newRouteQualityMonitor(RouteConfig{UUID: "r-id", Name: "入口", Members: []string{"a", "b"}}, m, &routeSwitchLog{})
	defer q.cancel()
	q.probe = func(context.Context, Config, RouteQualityConfig) routeProbeResult {
		return routeProbeResult{status: routeProbeFailed, connectivity: "ok", err: "下载失败", duration: time.Second}
	}
	q.round()
	if err := m.history.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	page, err := m.QueryProbeHistory(context.Background(), probehistory.Filter{TunnelUUID: "a-id"})
	if err != nil || len(page.Records) != 1 || page.Records[0].Status != routeProbeCommon || page.Records[0].Connectivity != "ok" || q.snapshot()["a"].Samples != 0 {
		t.Fatalf("共同失败应完整记录但不改变评分窗口：%+v %v", page, err)
	}
}
