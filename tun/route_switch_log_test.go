package tun

import (
	"sync"
	"testing"
	"time"
)

func TestRouteSwitchLog_RecordAndOrder(t *testing.T) {
	l := &routeSwitchLog{}
	l.record("", "a") // 初始选中
	l.record("a", "b")

	events := l.list()
	if len(events) != 2 {
		t.Fatalf("应有 2 条记录, 得到 %d", len(events))
	}
	// 最新在前
	if events[0].From != "a" || events[0].To != "b" {
		t.Fatalf("最新一条应为 a->b, 得到 %+v", events[0])
	}
	if events[1].From != "" || events[1].To != "a" {
		t.Fatalf("最早一条应为初始选中 a, 得到 %+v", events[1])
	}
}

func TestRouteSwitchLog_KeepLast20(t *testing.T) {
	l := &routeSwitchLog{}
	for i := 0; i < 30; i++ {
		from := "m" + string(rune('a'+i%2))
		to := "m" + string(rune('a'+(i+1)%2))
		l.record(from, to)
	}

	events := l.list()
	if len(events) != routeSwitchLogMax {
		t.Fatalf("应只保留最近 %d 条, 得到 %d", routeSwitchLogMax, len(events))
	}
	if events[0].Time.Before(events[len(events)-1].Time) {
		t.Fatal("应按时间倒序（最新在前）")
	}
}

// switchFakeService 并发安全的假 member Service（watcher 会并发读健康状态）
type switchFakeService struct {
	fakeService
	mu      sync.Mutex
	health  QualitySummary
}

func newSwitchFake(name string, status string) *switchFakeService {
	s := &switchFakeService{fakeService: fakeService{cfg: Config{Name: name}, status: "running"}}
	s.health = QualitySummary{Status: status}
	return s
}

func (f *switchFakeService) setHealth(status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.health = QualitySummary{Status: status}
}

func (f *switchFakeService) QualitySummary() QualitySummary {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.health
}

// waitEvents 轮询等待切换记录达到 n 条（watcher 秒级 tick，给足余量）
func waitEvents(t *testing.T, svc *routeService, n int) []RouteSwitchEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		events := svc.SwitchEvents()
		if len(events) >= n {
			return events
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("等待切换记录达到 %d 条超时, 当前: %+v", n, svc.SwitchEvents())
	return nil
}

func TestRouteService_WatchSwitch(t *testing.T) {
	a := newSwitchFake("a", QualityStatusUp)
	b := newSwitchFake("b", QualityStatusUp)
	m := newMgrWithServices(a, b)

	svc := &routeService{
		selector:  newRouteSelector(m, []string{"a", "b"}),
		switchLog: &routeSwitchLog{},
		watchStop: make(chan struct{}),
	}
	go svc.watchSwitch()
	defer close(svc.watchStop)

	// 初始选中 a
	events := waitEvents(t, svc, 1)
	if events[0].From != "" || events[0].To != "a" {
		t.Fatalf("初始选中应为 无->a, 得到 %+v", events[0])
	}

	// a 宕 → 切到 b
	a.setHealth(QualityStatusDown)
	events = waitEvents(t, svc, 2)
	if events[0].From != "a" || events[0].To != "b" {
		t.Fatalf("a down 后应切到 b, 得到 %+v", events[0])
	}

	// a 恢复 → 切回 a
	a.setHealth(QualityStatusUp)
	events = waitEvents(t, svc, 3)
	if events[0].From != "b" || events[0].To != "a" {
		t.Fatalf("a 恢复后应切回 a, 得到 %+v", events[0])
	}

	// 全部宕 → 切到 无
	a.setHealth(QualityStatusDown)
	b.setHealth(QualityStatusDown)
	events = waitEvents(t, svc, 4)
	if events[0].From != "a" || events[0].To != "" {
		t.Fatalf("全部 down 后应切到无可用, 得到 %+v", events[0])
	}
}
