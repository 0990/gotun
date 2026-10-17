package route

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/0990/gotun/admin/model"
	"github.com/0990/gotun/tun"
)

func reserveTCPAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve addr: %v", err)
	}
	addr := lis.Addr().String()
	if err := lis.Close(); err != nil {
		t.Fatalf("close reserved listener: %v", err)
	}
	return addr
}

// invokeSwitchLog 调用 SwitchLog handler 并解析响应（code != 200 时 data 为 nil）
func invokeSwitchLog(t *testing.T, handler func(http.ResponseWriter, *http.Request), query url.Values) (int, []model.RouteSwitchEvent) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/route_output/switch_log?"+query.Encode(), strings.NewReader(""))
	resp := httptest.NewRecorder()
	handler(resp, req)

	var raw struct {
		Code int                      `json:"code"`
		Msg  string                   `json:"msg"`
		Data []model.RouteSwitchEvent `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return raw.Code, raw.Data
}

func TestSwitchLogHandler(t *testing.T) {
	mgr := tun.NewManager(t.TempDir())

	// 两个真实 member（输出地址不可达不影响选址，选址只看生命周期状态/健康）
	memberNames := []string{"m1", "m2"}
	for _, name := range memberNames {
		cfg := tun.Config{Name: name, Input: "tcp@" + reserveTCPAddr(t), Output: "tcp@127.0.0.1:1"}
		if err := mgr.AddService(cfg, true); err != nil {
			t.Fatalf("add member %s: %v", name, err)
		}
	}
	t.Cleanup(func() {
		for _, name := range memberNames {
			if svc, ok := mgr.GetService(name); ok {
				_ = svc.Close()
			}
		}
	})

	cfg := tun.RouteConfig{
		Name:    "r1",
		Mode:    tun.RouteModeOutput,
		Listen:  "tcp@" + reserveTCPAddr(t),
		Members: memberNames,
	}
	if err := mgr.AddRoute(cfg, true); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	t.Cleanup(func() {
		if svc, ok := mgr.GetRoute("r1"); ok {
			_ = svc.Close()
		}
	})

	handler := SwitchLog(mgr, tun.RouteModeOutput)

	// 缺 name 参数
	if code, _ := invokeSwitchLog(t, handler, url.Values{}); code != http.StatusInternalServerError {
		t.Fatalf("缺 name 应返回 500, 得到 %d", code)
	}
	// 路由不存在
	if code, _ := invokeSwitchLog(t, handler, url.Values{"name": {"nope"}}); code != http.StatusInternalServerError {
		t.Fatalf("路由不存在应返回 500, 得到 %d", code)
	}
	// 模式不匹配（r1 是 output，用 input handler 查询）
	inputHandler := SwitchLog(mgr, tun.RouteModeInput)
	if code, _ := invokeSwitchLog(t, inputHandler, url.Values{"name": {"r1"}}); code != http.StatusInternalServerError {
		t.Fatalf("模式不匹配应返回 500, 得到 %d", code)
	}

	// 正常查询：watcher 启动后会记录初始选中（无 -> m1），轮询等待其出现
	deadline := time.Now().Add(5 * time.Second)
	for {
		code, events := invokeSwitchLog(t, handler, url.Values{"name": {"r1"}})
		if code != http.StatusOK {
			t.Fatalf("正常查询应返回 200, 得到 %d", code)
		}
		if len(events) > 0 {
			if events[0].To != "m1" || events[0].From != "" {
				t.Fatalf("初始选中应为 无->m1, 得到 %+v", events[0])
			}
			if events[0].Time == "" {
				t.Fatal("时间字段不应为空")
			}
			svc, _ := mgr.GetRoute("r1")
			want := svc.SwitchEvents()[0].Time.In(time.FixedZone("UTC+8", 8*60*60)).Format("2006-01-02 15:04:05")
			if events[0].Time != want {
				t.Fatalf("切换记录应使用北京时间：得到 %s，期望 %s", events[0].Time, want)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("等待初始选中记录超时")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

type routeTimeStub struct {
	tun.RouteService
	cfg tun.RouteConfig
}

func (s routeTimeStub) RouteCfg() tun.RouteConfig          { return s.cfg }
func (s routeTimeStub) Status() string                     { return "disabled" }
func (s routeTimeStub) MemberHealth() map[string]string    { return nil }
func (s routeTimeStub) PreferredMember() string            { return "" }
func (s routeTimeStub) QualitySummary() tun.QualitySummary { return tun.QualitySummary{} }

func TestRouteCreatedAtBeijing(t *testing.T) {
	rs := routeTimeStub{cfg: tun.RouteConfig{
		CreatedAt: time.Date(2026, 10, 9, 18, 6, 13, 0, time.UTC),
	}}
	if got := routeToModel(rs).CreatedAt; got != "2026-10-10 02:06:13" {
		t.Fatalf("创建时间应使用北京时间：%s", got)
	}
}

func TestQualityConfigAPIEditAndValidation(t *testing.T) {
	mgr := tun.NewManager(t.TempDir())
	member := tun.Config{Name: "socks", Input: "socks5x@127.0.0.1:1080", Disabled: true}
	if err := mgr.AddService(member, true); err != nil {
		t.Fatal(err)
	}
	cfg := tun.RouteConfig{Name: "quality", Mode: tun.RouteModeInput, Listen: "127.0.0.1:0", Members: []string{"socks"}, Policy: "quality", Disabled: true}
	if err := mgr.AddRoute(cfg, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.RemoveRoute("quality") })
	rs, _ := mgr.GetRoute("quality")
	view := routeToModel(rs)
	if view.Quality.DownloadBytes != 16384 || view.MemberQuality["socks"].Status != "evaluating" {
		t.Fatalf("默认配置和评估状态缺失: %+v", view)
	}
	view.Quality.DownloadURL = "https://download.example.com/file"
	view.Quality.ConnectivityURL = "https://check.example.com/ok"
	view.Quality.IntervalSec = 60
	invoke := func(v model.Route) int {
		body, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/api/route_input/edit", strings.NewReader(string(body)))
		recorder := httptest.NewRecorder()
		Edit(mgr, tun.RouteModeInput)(recorder, req)
		var response struct {
			Code int
			Msg  string
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Code
	}
	if code := invoke(view); code != 200 {
		t.Fatalf("保存失败: %d", code)
	}
	rs, _ = mgr.GetRoute("quality")
	if got := routeToModel(rs); got.Quality != view.Quality || got.Policy != "quality" {
		t.Fatalf("编辑往返失败: %+v", got)
	}
	view.Quality.DownloadURL = "http://insecure.example.com"
	if code := invoke(view); code == 200 {
		t.Fatal("不应接受非 HTTPS 目标")
	}
	rs, _ = mgr.GetRoute("quality")
	if rs.RouteCfg().Quality.DownloadURL != "https://download.example.com/file" {
		t.Fatal("保存失败应保留原配置")
	}
}
