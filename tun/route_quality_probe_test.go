package tun

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// 真实路径：带认证 SOCKS5 input -> 第一跳 mux -> 第二跳 mux -> SOCKS5X -> TLS 目标。
func TestRouteQualityProbeMultiHop(t *testing.T) {
	var downloads, confirms atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			confirms.Add(1)
			w.WriteHeader(204)
		case "/short":
			io.WriteString(w, "short")
		case "/large":
			io.WriteString(w, strings.Repeat("x", 32768))
		case "/status":
			w.WriteHeader(503)
		case "/redirect":
			http.Redirect(w, r, "/download", 302)
		case "/compressed":
			w.Header().Set("Content-Encoding", "gzip")
			io.WriteString(w, "bad")
		case "/slow":
			time.Sleep(35 * time.Millisecond)
			io.WriteString(w, strings.Repeat("x", 16384))
		case "/stall":
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			downloads.Add(1)
			io.WriteString(w, strings.Repeat("x", 16384))
		}
	}))
	defer target.Close()
	upstream, port := startSocks5X(t)
	defer upstream.Close()
	frame := `{"frame_header_enable":true}`
	outFrame := `{"mux_conn":1,"frame_header_enable":true}`
	lastAddr, middleAddr, entryAddr := reserveTCPAddr(t), reserveTCPAddr(t), reserveTCPAddr(t)
	start := func(cfg Config) *Server {
		t.Helper()
		s, err := NewServer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Run(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	}
	last := start(Config{Name: "quality-final", Input: "tcp_mux@" + lastAddr, Output: fmt.Sprintf("tcp@127.0.0.1:%d", port), InExtend: frame})
	start(Config{Name: "quality-middle", Input: "tcp_mux@" + middleAddr, Output: "tcp_mux@" + lastAddr, InExtend: frame, OutExtend: outFrame})
	cfg := Config{Name: "quality-entry", Input: "socks5x@" + entryAddr, InProtoCfg: `{"username":"probe","password":"secret"}`, Output: "tcp_mux@" + middleAddr, OutExtend: outFrame}
	entry := start(cfg)
	client, err := newRouteProbeClient(context.Background(), cfg, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	client.Transport.(*http.Transport).TLSClientConfig = target.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	c := RouteQualityConfig{}
	c.normalize()
	c.DownloadURL = target.URL + "/download"
	c.ConnectivityURL = target.URL + "/ok"
	r := probeRouteWithClient(context.Background(), client, c)
	if r.status != routeProbeNormal || downloads.Load() != 1 || confirms.Load() != 0 {
		t.Fatalf("应到最终 TLS 目标并完成正文: %+v downloads=%d confirms=%d", r, downloads.Load(), confirms.Load())
	}
	for _, path := range []string{"/short", "/large", "/status", "/redirect", "/compressed"} {
		t.Run(path, func(t *testing.T) {
			c.DownloadURL = target.URL + path
			r := probeRouteWithClient(context.Background(), client, c)
			if r.status != routeProbeFailed || r.connectivity != "ok" || r.err == "" {
				t.Fatalf("应失败且辅助确认连通: %+v", r)
			}
		})
	}
	c.DownloadURL = target.URL + "/slow"
	c.NormalMS = 10
	if r := probeRouteWithClient(context.Background(), client, c); r.status != routeProbeSlow {
		t.Fatalf("应识别慢请求: %+v", r)
	}
	c.DownloadURL = target.URL + "/stall"
	client.Timeout = 120 * time.Millisecond
	startAt := time.Now()
	r = probeRouteWithClient(context.Background(), client, c)
	if r.status != routeProbeFailed || time.Since(startAt) > time.Second {
		t.Fatalf("响应体必须受总超时限制: %+v", r)
	}
	// 保持生产默认 TLS 验证，只有测试客户端显式信任本地测试证书。
	untrusted, err := newRouteProbeClient(context.Background(), cfg, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer untrusted.CloseIdleConnections()
	if err := routeProbeRequest(context.Background(), untrusted, target.URL+"/download", 200, 16384); err == nil {
		t.Fatal("不应接受未信任证书")
	}
	wrong := cfg
	wrong.InProtoCfg = `{"username":"probe","password":"wrong"}`
	authClient, err := newRouteProbeClient(context.Background(), wrong, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer authClient.CloseIdleConnections()
	if err := routeProbeRequest(context.Background(), authClient, target.URL+"/download", 200, 16384); err == nil {
		t.Fatal("错误认证不能绕过 SOCKS")
	}
	t.Run("优先级入口发现第一跳之后的故障", func(t *testing.T) {
		backup := start(Config{Name: "priority-backup", Input: "socks5x@" + reserveTCPAddr(t), Output: fmt.Sprintf("tcp@127.0.0.1:%d", port)})
		m := newMgrWithServices(entry, backup)
		route, err := newRouteService(RouteConfig{
			Name: "priority-input", Mode: RouteModeInput, Listen: "127.0.0.1:0", Policy: "priority",
			Members: []string{entry.Cfg().Name, backup.Cfg().Name},
		}, m)
		if err != nil {
			t.Fatal(err)
		}
		defer route.Close()
		q := route.selector.quality
		q.cfg.Quality.DownloadURL, q.cfg.Quality.ConnectivityURL = target.URL+"/download", target.URL+"/ok"
		q.cfg.Quality.TimeoutMS = 500
		q.probe = func(ctx context.Context, cfg Config, quality RouteQualityConfig) routeProbeResult {
			client, err := newRouteProbeClient(ctx, cfg, time.Duration(quality.TimeoutMS)*time.Millisecond)
			if err != nil {
				t.Error(err)
				return routeProbeResult{status: routeProbeFailed}
			}
			defer client.CloseIdleConnections()
			client.Transport.(*http.Transport).TLSClientConfig = target.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			return probeRouteWithClient(ctx, client, quality)
		}
		q.round()
		q.round()
		if route.PreferredMember() != cfg.Name || route.MemberQuality()[cfg.Name].ConsecutiveNormal != 2 {
			t.Fatal("完整链路正常时应优先使用主线路")
		}
		// 断开最后一跳，保留入口到中间节点的帧探测应答。
		_ = last.Close()
		entry.probeRunner.maybeProbe(time.Now(), true)
		if entry.QualitySummary().Status != QualityStatusUp {
			t.Fatal("测试条件不成立：第一跳帧探测仍应正常")
		}
		for i := 0; i < 3; i++ {
			q.round()
		}
		if route.PreferredMember() != backup.Cfg().Name || route.MemberQuality()[cfg.Name].ConsecutiveFailures != 3 {
			t.Fatalf("后续链路故障应切到备用：%+v", route.MemberQuality())
		}
		entry.probeRunner.maybeProbe(time.Now(), true)
		if entry.QualitySummary().Status != QualityStatusUp {
			t.Fatal("完整链路切换不应依赖第一跳变为 down")
		}
	})
}

func TestRouteQualityCancelDuringSOCKSHandshake(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	accepted := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		conn, e := lis.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		close(accepted)
		_, _ = io.Copy(io.Discard, conn)
		close(closed)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := newRouteProbeClient(ctx, Config{Input: "socks5x@" + lis.Addr().String()}, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	done := make(chan error, 1)
	go func() { done <- routeProbeRequest(ctx, client, "https://example.com", 200, 16) }()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("代理未接受连接")
	}
	cancel()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("取消应返回错误")
		}
	case <-time.After(time.Second):
		t.Fatal("取消未中断代理协商")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("取消后遗留 SOCKS 连接")
	}
}
