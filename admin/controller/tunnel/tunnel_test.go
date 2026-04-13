package tunnel

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/0990/gotun/admin/model"
	"github.com/0990/gotun/admin/response"
	"github.com/0990/gotun/tun"
)

func TestCreateReturnsErrorWithoutDirtyState(t *testing.T) {
	blockPath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blockPath, []byte("x"), 0o666); err != nil {
		t.Fatalf("write block file: %v", err)
	}

	mgr := tun.NewManager(blockPath)
	addr := reserveControllerTCPAddr(t)
	payload := model.Tunnel{
		Name:   "create-handler-fail",
		Input:  "tcp@" + addr,
		Output: "tcp@127.0.0.1:1",
	}

	ret := invokeTunnelHandler(t, Create(mgr), payload)
	if ret.Code != 500 {
		t.Fatalf("ret code = %d, want 500", ret.Code)
	}
	if _, ok := mgr.GetService(payload.Name); ok {
		t.Fatal("service should not remain after create failure")
	}
}

func TestEditReturnsErrorAndKeepsExistingTunnel(t *testing.T) {
	mgr := tun.NewManager(t.TempDir())
	originalAddr := reserveControllerTCPAddr(t)
	cfg := tun.Config{
		Name:   "edit-handler-keep",
		Input:  "tcp@" + originalAddr,
		Output: "tcp@127.0.0.1:1",
	}
	if err := mgr.AddService(cfg, true); err != nil {
		t.Fatalf("add service: %v", err)
	}

	originalSvc, ok := mgr.GetService(cfg.Name)
	if !ok {
		t.Fatal("service should exist")
	}
	defer closeControllerService(t, mgr, cfg.Name)

	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen blocker: %v", err)
	}
	defer blocker.Close()

	payload := Config2Model(originalSvc.Cfg())
	payload.Input = "tcp@" + blocker.Addr().String()

	ret := invokeTunnelHandler(t, Edit(mgr), payload)
	if ret.Code != 500 {
		t.Fatalf("ret code = %d, want 500", ret.Code)
	}

	currentSvc, ok := mgr.GetService(cfg.Name)
	if !ok {
		t.Fatal("original service should still exist")
	}
	if currentSvc.Cfg().Input != originalSvc.Cfg().Input {
		t.Fatal("original config should remain after edit failure")
	}
}

func TestSetDisabledTogglesTunnelState(t *testing.T) {
	mgr := tun.NewManager(t.TempDir())
	inputAddr := reserveControllerTCPAddr(t)
	cfg := tun.Config{
		Name:   "toggle-handler",
		Input:  "tcp@" + inputAddr,
		Output: "tcp@127.0.0.1:1",
	}
	if err := mgr.AddService(cfg, true); err != nil {
		t.Fatalf("add service: %v", err)
	}
	defer closeControllerService(t, mgr, cfg.Name)

	ret := invokeRawTunnelHandler(t, SetDisabled(mgr), map[string]interface{}{
		"name":     cfg.Name,
		"disabled": true,
	})
	if ret.Code != 200 {
		t.Fatalf("disable ret code = %d, want 200", ret.Code)
	}

	svc, ok := mgr.GetService(cfg.Name)
	if !ok || !svc.Cfg().Disabled {
		t.Fatal("service should be disabled after handler call")
	}

	ret = invokeRawTunnelHandler(t, SetDisabled(mgr), map[string]interface{}{
		"name":     cfg.Name,
		"disabled": false,
	})
	if ret.Code != 200 {
		t.Fatalf("enable ret code = %d, want 200", ret.Code)
	}

	svc, ok = mgr.GetService(cfg.Name)
	if !ok || svc.Cfg().Disabled {
		t.Fatal("service should be enabled after handler call")
	}
}

func invokeTunnelHandler(t *testing.T, handler func(http.ResponseWriter, *http.Request), payload model.Tunnel) response.Ret {
	t.Helper()

	return invokeRawTunnelHandler(t, handler, payload)
}

func invokeRawTunnelHandler(t *testing.T, handler func(http.ResponseWriter, *http.Request), payload interface{}) response.Ret {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/tunnel", bytes.NewReader(body))
	resp := httptest.NewRecorder()
	handler(resp, req)

	var ret response.Ret
	if err := json.Unmarshal(resp.Body.Bytes(), &ret); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return ret
}

func reserveControllerTCPAddr(t *testing.T) string {
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

func closeControllerService(t *testing.T, mgr *tun.Manager, name string) {
	t.Helper()

	svc, ok := mgr.GetService(name)
	if !ok {
		return
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("close service %s: %v", name, err)
	}
}
