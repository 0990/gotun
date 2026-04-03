package tun

import "testing"

func TestNewSmuxConfigUsesStreamBufForPerStreamWindow(t *testing.T) {
	cfg, err := newSmuxConfig(4 * 1024 * 1024)
	if err != nil {
		t.Fatalf("newSmuxConfig returned error: %v", err)
	}
	if cfg.MaxReceiveBuffer != 4*1024*1024 {
		t.Fatalf("MaxReceiveBuffer = %d", cfg.MaxReceiveBuffer)
	}
	if cfg.MaxStreamBuffer != 4*1024*1024 {
		t.Fatalf("MaxStreamBuffer = %d", cfg.MaxStreamBuffer)
	}
}
