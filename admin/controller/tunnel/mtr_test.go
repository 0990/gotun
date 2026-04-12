package tunnel

import "testing"

func TestBuildMTRSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   string
		wantMode string
		wantHost string
		wantPort string
		wantErr  bool
	}{
		{name: "tcp", output: "tcp@1.2.3.4:80", wantMode: "tcp", wantHost: "1.2.3.4", wantPort: "80"},
		{name: "tcp_mux", output: "tcp_mux@1.2.3.4:443", wantMode: "tcp", wantHost: "1.2.3.4", wantPort: "443"},
		{name: "udp", output: "udp@1.2.3.4:53", wantMode: "udp", wantHost: "1.2.3.4", wantPort: "53"},
		{name: "quic", output: "quic@1.2.3.4:443", wantMode: "udp", wantHost: "1.2.3.4", wantPort: "443"},
		{name: "kcp_mux", output: "kcp_mux@1.2.3.4:9999", wantMode: "udp", wantHost: "1.2.3.4", wantPort: "9999"},
		{name: "kcpx_mux", output: "kcpx_mux@1.2.3.4:7777", wantMode: "udp", wantHost: "1.2.3.4", wantPort: "7777"},
		{name: "ipv6", output: "tcp@[2001:db8::1]:443", wantMode: "tcp", wantHost: "2001:db8::1", wantPort: "443"},
		{name: "bad format", output: "tcp-1.2.3.4:80", wantErr: true},
		{name: "bad proto", output: "socks5x@1.2.3.4:1080", wantErr: true},
		{name: "bad port", output: "tcp@1.2.3.4:notaport", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := buildMTRSpec("demo", tt.output)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for output %q", tt.output)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildMTRSpec(%q) error = %v", tt.output, err)
			}
			if got.Mode != tt.wantMode {
				t.Fatalf("mode = %q, want %q", got.Mode, tt.wantMode)
			}
			if got.Host != tt.wantHost {
				t.Fatalf("host = %q, want %q", got.Host, tt.wantHost)
			}
			if got.Port != tt.wantPort {
				t.Fatalf("port = %q, want %q", got.Port, tt.wantPort)
			}
		})
	}
}

func TestMTRArgs(t *testing.T) {
	t.Parallel()

	spec := mtrSpec{
		Tunnel:   "demo",
		Protocol: "tcp_mux",
		Mode:     "tcp",
		Host:     "43.134.250.33",
		Port:     "4000",
	}

	got := spec.args()
	want := []string{"--curses", "--no-dns", "--tcp", "-P", "4000", "43.134.250.33"}
	if len(got) != len(want) {
		t.Fatalf("args len = %d, want %d, got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q; got=%v", i, got[i], want[i], got)
		}
	}
}
