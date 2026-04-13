package tun

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerAddServiceClosesServiceWhenCreateFileFails(t *testing.T) {
	blockPath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blockPath, []byte("x"), 0o666); err != nil {
		t.Fatalf("write block file: %v", err)
	}

	mgr := NewManager(blockPath)
	inputAddr := reserveTCPAddr(t)
	cfg := testManagerConfig("create-file-fail", inputAddr)

	err := mgr.AddService(cfg, true)
	if err == nil {
		t.Fatal("expected add service to fail")
	}

	if _, ok := mgr.GetService(cfg.Name); ok {
		t.Fatal("service should not be registered after create failure")
	}

	assertTCPAddrAvailable(t, inputAddr)
}

func TestManagerReplaceServiceByUUIDKeepsOldServiceOnBuildFailure(t *testing.T) {
	mgr := NewManager(t.TempDir())
	oldAddr := reserveTCPAddr(t)
	cfg := testManagerConfig("replace-build-fail", oldAddr)
	if err := mgr.AddService(cfg, true); err != nil {
		t.Fatalf("add service: %v", err)
	}
	defer closeServiceByName(t, mgr, cfg.Name)

	oldService, ok := mgr.GetService(cfg.Name)
	if !ok {
		t.Fatal("service should exist")
	}

	originalFile := mustReadFile(t, mgr.ServiceFile(cfg.Name))
	newCfg := oldService.Cfg()
	newCfg.Input = "invalid@127.0.0.1:12345"

	err := mgr.ReplaceServiceByUUID(newCfg)
	if err == nil {
		t.Fatal("expected replace to fail")
	}

	keptService, ok := mgr.GetService(cfg.Name)
	if !ok {
		t.Fatal("old service should still exist")
	}
	if keptService.Cfg().UUID != oldService.Cfg().UUID {
		t.Fatal("old service uuid should be preserved")
	}

	if data := mustReadFile(t, mgr.ServiceFile(cfg.Name)); string(data) != string(originalFile) {
		t.Fatal("config file should stay unchanged on build failure")
	}

	assertTCPAddrBusy(t, oldAddr)
}

func TestManagerReplaceServiceByUUIDRollsBackOnRunFailure(t *testing.T) {
	mgr := NewManager(t.TempDir())
	oldAddr := reserveTCPAddr(t)
	cfg := testManagerConfig("replace-run-fail", oldAddr)
	if err := mgr.AddService(cfg, true); err != nil {
		t.Fatalf("add service: %v", err)
	}
	defer closeServiceByName(t, mgr, cfg.Name)

	oldService, ok := mgr.GetService(cfg.Name)
	if !ok {
		t.Fatal("service should exist")
	}
	originalFile := mustReadFile(t, mgr.ServiceFile(cfg.Name))

	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen blocker: %v", err)
	}
	defer blocker.Close()

	newCfg := oldService.Cfg()
	newCfg.Input = "tcp@" + blocker.Addr().String()

	err = mgr.ReplaceServiceByUUID(newCfg)
	if err == nil {
		t.Fatal("expected replace to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "use") && !strings.Contains(strings.ToLower(err.Error()), "listen") {
		t.Fatalf("unexpected replace error: %v", err)
	}

	keptService, ok := mgr.GetService(cfg.Name)
	if !ok {
		t.Fatal("old service should still exist after rollback")
	}
	if keptService.Cfg().Input != oldService.Cfg().Input {
		t.Fatal("old service config should be restored after rollback")
	}

	if data := mustReadFile(t, mgr.ServiceFile(cfg.Name)); string(data) != string(originalFile) {
		t.Fatal("config file should stay unchanged on run failure")
	}

	assertTCPAddrBusy(t, oldAddr)
}

func TestManagerReplaceServiceByUUIDRenamesAndPreservesIdentity(t *testing.T) {
	mgr := NewManager(t.TempDir())
	oldAddr := reserveTCPAddr(t)
	cfg := testManagerConfig("rename-old", oldAddr)
	if err := mgr.AddService(cfg, true); err != nil {
		t.Fatalf("add service: %v", err)
	}
	defer closeServiceByName(t, mgr, "rename-new")

	oldService, ok := mgr.GetService(cfg.Name)
	if !ok {
		t.Fatal("service should exist")
	}
	oldCfg := oldService.Cfg()

	newAddr := reserveTCPAddr(t)
	newCfg := oldCfg
	newCfg.Name = "rename-new"
	newCfg.Input = "tcp@" + newAddr

	if err := mgr.ReplaceServiceByUUID(newCfg); err != nil {
		t.Fatalf("replace service: %v", err)
	}

	if _, ok := mgr.GetService(cfg.Name); ok {
		t.Fatal("old service name should be removed after rename")
	}
	newService, ok := mgr.GetService(newCfg.Name)
	if !ok {
		t.Fatal("renamed service should exist")
	}

	actualCfg := newService.Cfg()
	if actualCfg.UUID != oldCfg.UUID {
		t.Fatal("uuid should be preserved across rename")
	}
	if !actualCfg.CreatedAt.Equal(oldCfg.CreatedAt) {
		t.Fatal("created_at should be preserved across rename")
	}

	if _, err := os.Stat(mgr.ServiceFile(cfg.Name)); !os.IsNotExist(err) {
		t.Fatalf("old service file should be removed, err=%v", err)
	}

	fileCfg := readConfigFile(t, mgr.ServiceFile(newCfg.Name))
	if fileCfg.UUID != oldCfg.UUID {
		t.Fatal("file should keep original uuid")
	}
	if !fileCfg.CreatedAt.Equal(oldCfg.CreatedAt) {
		t.Fatal("file should keep original created_at")
	}

	assertTCPAddrBusy(t, newAddr)
	assertTCPAddrAvailable(t, oldAddr)
}

func TestManagerAddDisabledServiceDoesNotStartListener(t *testing.T) {
	mgr := NewManager(t.TempDir())
	inputAddr := reserveTCPAddr(t)
	cfg := testManagerConfig("disabled-create", inputAddr)
	cfg.Disabled = true

	if err := mgr.AddService(cfg, true); err != nil {
		t.Fatalf("add disabled service: %v", err)
	}
	defer closeServiceByName(t, mgr, cfg.Name)

	svc, ok := mgr.GetService(cfg.Name)
	if !ok {
		t.Fatal("disabled service should be registered")
	}
	if !svc.Cfg().Disabled {
		t.Fatal("disabled flag should be preserved")
	}
	if svc.Status() != "disabled" {
		t.Fatalf("status = %s, want disabled", svc.Status())
	}

	assertTCPAddrAvailable(t, inputAddr)
}

func TestManagerSetServiceDisabledStopsAndRestartsService(t *testing.T) {
	mgr := NewManager(t.TempDir())
	inputAddr := reserveTCPAddr(t)
	cfg := testManagerConfig("toggle-disabled", inputAddr)
	if err := mgr.AddService(cfg, true); err != nil {
		t.Fatalf("add service: %v", err)
	}
	defer closeServiceByName(t, mgr, cfg.Name)

	if err := mgr.SetServiceDisabled(cfg.Name, true); err != nil {
		t.Fatalf("disable service: %v", err)
	}

	disabledSvc, ok := mgr.GetService(cfg.Name)
	if !ok {
		t.Fatal("disabled service should exist")
	}
	if !disabledSvc.Cfg().Disabled {
		t.Fatal("service should be disabled")
	}
	assertTCPAddrAvailable(t, inputAddr)

	if err := mgr.SetServiceDisabled(cfg.Name, false); err != nil {
		t.Fatalf("enable service: %v", err)
	}

	enabledSvc, ok := mgr.GetService(cfg.Name)
	if !ok {
		t.Fatal("enabled service should exist")
	}
	if enabledSvc.Cfg().Disabled {
		t.Fatal("service should be enabled")
	}
	assertTCPAddrBusy(t, inputAddr)
}

func testManagerConfig(name string, inputAddr string) Config {
	return Config{
		Name:   name,
		Input:  "tcp@" + inputAddr,
		Output: "tcp@127.0.0.1:1",
	}
}

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

func assertTCPAddrAvailable(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		lis, err := net.Listen("tcp", addr)
		if err == nil {
			_ = lis.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("addr %s should be available: %v", addr, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func assertTCPAddrBusy(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		_ = lis.Close()
		if time.Now().After(deadline) {
			t.Fatalf("addr %s should be busy", addr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func closeServiceByName(t *testing.T, mgr *Manager, name string) {
	t.Helper()

	svc, ok := mgr.GetService(name)
	if !ok {
		return
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("close service %s: %v", name, err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return data
}

func readConfigFile(t *testing.T, path string) Config {
	t.Helper()

	data := mustReadFile(t, path)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config file: %v", err)
	}
	return cfg
}
