package service

import (
	"net"
	"testing"
	"time"
)

// waitListening 等待 addr 进入可连接状态（监听中），超时返回 false
func waitListening(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// waitClosed 等待 addr 不再可连接（端口已释放），超时返回 false
func waitClosed(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return true
		}
		c.Close()
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// TestSetDisabledReleasesPort 验证：新建 echo 服务监听端口 -> 停用后端口释放 -> 启用后端口恢复
func TestSetDisabledReleasesPort(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	addr := "127.0.0.1:23881"
	cfg := Config{
		Name:   "echo-test",
		Type:   TypeEcho,
		Listen: addr,
	}

	if err := mgr.AddService(cfg, true); err != nil {
		t.Fatalf("AddService: %v", err)
	}
	if !waitListening(addr, 3*time.Second) {
		t.Fatalf("port %s not listening after AddService", addr)
	}

	// 停用 -> 端口应释放
	if err := mgr.SetServiceDisabled("echo-test", true); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !waitClosed(addr, 3*time.Second) {
		t.Fatalf("port %s still listening after disable (Close 未释放端口)", addr)
	}

	// 启用 -> 端口应恢复
	if err := mgr.SetServiceDisabled("echo-test", false); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !waitListening(addr, 3*time.Second) {
		t.Fatalf("port %s not listening after enable", addr)
	}

	// 删除 -> 端口应释放
	if err := mgr.RemoveService("echo-test"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !waitClosed(addr, 3*time.Second) {
		t.Fatalf("port %s still listening after remove", addr)
	}
}

// TestSocks5CloseReleasesPort 验证标准 socks5 服务的停用释放端口
func TestSocks5CloseReleasesPort(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	port := 23882
	cfg := Config{
		Name:       "socks5-test",
		Type:       TypeSocks5,
		ListenPort: port,
		Username:   "u1",
		Password:   "p1",
	}
	addr := "127.0.0.1:23882"

	if err := mgr.AddService(cfg, true); err != nil {
		t.Fatalf("AddService: %v", err)
	}
	if !waitListening(addr, 3*time.Second) {
		t.Fatalf("socks5 port not listening after AddService")
	}

	if err := mgr.SetServiceDisabled("socks5-test", true); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !waitClosed(addr, 3*time.Second) {
		t.Fatalf("socks5 port still listening after disable")
	}

	if err := mgr.SetServiceDisabled("socks5-test", false); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !waitListening(addr, 3*time.Second) {
		t.Fatalf("socks5 port not listening after enable")
	}

	_ = mgr.RemoveService("socks5-test")
}

// TestHttpProxyCloseReleasesPort 验证 http_proxy 服务停用释放端口、启用恢复
func TestHttpProxyCloseReleasesPort(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	addr := "127.0.0.1:23128"
	cfg := Config{Name: "hp-test", Type: TypeHttpProxy, Listen: addr}

	if err := mgr.AddService(cfg, true); err != nil {
		t.Fatalf("AddService: %v", err)
	}
	if !waitListening(addr, 3*time.Second) {
		t.Fatalf("httpproxy not listening after AddService")
	}
	if err := mgr.SetServiceDisabled("hp-test", true); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !waitClosed(addr, 3*time.Second) {
		t.Fatalf("httpproxy still listening after disable")
	}
	if err := mgr.SetServiceDisabled("hp-test", false); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !waitListening(addr, 3*time.Second) {
		t.Fatalf("httpproxy not listening after enable")
	}
	_ = mgr.RemoveService("hp-test")
}

// TestHttpProxyStartConflict 验证端口被占用时 Run 能返回错误（错误不丢失）
func TestHttpProxyStartConflict(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	addr := "127.0.0.1:23129"

	first := Config{Name: "hp-a", Type: TypeHttpProxy, Listen: addr}
	if err := mgr.AddService(first, true); err != nil {
		t.Fatalf("first AddService: %v", err)
	}
	if !waitListening(addr, 3*time.Second) {
		t.Fatalf("first httpproxy not listening")
	}

	// 直接构造第二个实例监听同端口，Run 应返回监听错误
	second := newHTTPProxyService(Config{Name: "hp-b", Type: TypeHttpProxy, Listen: addr})
	if err := second.Run(); err == nil {
		t.Fatalf("expected listen conflict error, got nil")
	}
	_ = mgr.RemoveService("hp-a")
}
