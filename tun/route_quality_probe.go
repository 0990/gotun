package tun

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/proxy"
)

const (
	routeProbeNormal = "normal"
	routeProbeSlow   = "slow"
	routeProbeFailed = "failed"
	routeProbeCommon = "common_failure"
)

type routeProbeResult struct {
	status       string
	duration     time.Duration
	err          string
	connectivity string
}

func newRouteProbeClient(parent context.Context, cfg Config, timeout time.Duration) (*http.Client, error) {
	addr, authCfg, err := routeQualityProxy(cfg)
	if err != nil {
		return nil, err
	}
	var auth *proxy.Auth
	if authCfg.UserName != "" || authCfg.Password != "" {
		auth = &proxy.Auth{User: authCfg.UserName, Password: authCfg.Password}
	}
	dialer, err := proxy.SOCKS5("tcp", addr, auth, &net.Dialer{Timeout: timeout})
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Transport 可能为连接复用剥离请求取消；显式关联路由关闭与协商超时。
			dialCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			stop := context.AfterFunc(parent, cancel)
			defer stop()
			return dialer.(proxy.ContextDialer).DialContext(dialCtx, network, addr)
		},
		DisableCompression: true,
		DisableKeepAlives:  true,
	}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

// routeProbeRequest 的超时涵盖代理协商、TLS、响应头及完整响应体。
func routeProbeRequest(ctx context.Context, client *http.Client, target string, status int, size int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "gotun-route-probe")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		return fmt.Errorf("响应状态码 %d，预期 %d", resp.StatusCode, status)
	}
	if size < 0 {
		return nil
	}
	if resp.Header.Get("Content-Encoding") != "" && resp.Header.Get("Content-Encoding") != "identity" {
		return fmt.Errorf("探测响应不应压缩")
	}
	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, size+1))
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}
	if n != size {
		return fmt.Errorf("响应正文 %d 字节，预期 %d", n, size)
	}
	return nil
}

func probeRouteMember(ctx context.Context, cfg Config, c RouteQualityConfig) routeProbeResult {
	client, err := newRouteProbeClient(ctx, cfg, time.Duration(c.TimeoutMS)*time.Millisecond)
	if err != nil {
		return routeProbeResult{status: routeProbeFailed, err: err.Error(), connectivity: "failed"}
	}
	defer client.CloseIdleConnections()
	return probeRouteWithClient(ctx, client, c)
}

func probeRouteWithClient(ctx context.Context, client *http.Client, c RouteQualityConfig) routeProbeResult {
	start := time.Now()
	err := routeProbeRequest(ctx, client, c.DownloadURL, c.DownloadStatus, c.DownloadBytes)
	r := routeProbeResult{status: routeProbeNormal, duration: time.Since(start)}
	if err == nil {
		if r.duration > time.Duration(c.NormalMS)*time.Millisecond {
			r.status = routeProbeSlow
		}
		return r
	}
	r.status, r.err = routeProbeFailed, err.Error()
	if ctx.Err() != nil {
		return r
	}
	if err := routeProbeRequest(ctx, client, c.ConnectivityURL, c.ConnectivityStatus, -1); err != nil {
		r.connectivity = "failed"
	} else {
		r.connectivity = "ok"
	}
	return r
}
