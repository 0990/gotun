package tun

import (
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0990/gotun/server/echo"
)

func Test_TcpMuxTunWithFrameHeader(t *testing.T) {
	targetAddr := "127.0.0.1:7207"
	echo.StartTCPEchoServer(targetAddr)

	relayClientAddr := "127.0.0.1:6200"
	relayServerAddr := "127.0.0.1:6201"

	server, err := NewServer(Config{
		Name:          "frame-server",
		Input:         fmt.Sprintf("tcp_mux@%s", relayServerAddr),
		Output:        fmt.Sprintf("tcp@%s", targetAddr),
		InDecryptKey:  "111111",
		InDecryptMode: "gcm",
		InExtend:      `{"frame_header_enable":true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}

	client, err := NewServer(Config{
		Name:         "frame-client",
		Input:        fmt.Sprintf("tcp@%s", relayClientAddr),
		Output:       fmt.Sprintf("tcp_mux@%s", relayServerAddr),
		OutCryptKey:  "111111",
		OutCryptMode: "gcm",
		OutExtend:    `{"mux_conn":10,"frame_header_enable":true,"probe_interval_sec":1,"probe_timeout_ms":1000,"probe_window_size":5}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Run(); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("tcp", relayClientAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	if _, err := conn.Read(buf); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	details := client.QualityDetails()["output"]
	if details.ProbeSuccessTotal == 0 {
		t.Fatalf("expected probe success, got %+v", details)
	}
}

func Test_ProbeDoesNotReachBackend(t *testing.T) {
	targetAddr := "127.0.0.1:7208"
	var accepted atomic.Int64
	listener, err := net.Listen("tcp", targetAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			_ = conn.Close()
		}
	}()

	relayServerAddr := "127.0.0.1:6203"
	server, err := NewServer(Config{
		Name:          "probe-server",
		Input:         fmt.Sprintf("tcp@%s", relayServerAddr),
		Output:        fmt.Sprintf("tcp@%s", targetAddr),
		InDecryptKey:  "111111",
		InDecryptMode: "gcm",
		InExtend:      `{"frame_header_enable":true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}

	client, err := NewServer(Config{
		Name:         "probe-client",
		Input:        "tcp@127.0.0.1:6202",
		Output:       fmt.Sprintf("tcp@%s", relayServerAddr),
		OutCryptKey:  "111111",
		OutCryptMode: "gcm",
		OutExtend:    `{"frame_header_enable":true,"probe_interval_sec":1,"probe_timeout_ms":1000,"probe_window_size":5}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Run(); err != nil {
		t.Fatal(err)
	}

	businessConn, err := net.Dial("tcp", "127.0.0.1:6202")
	if err != nil {
		t.Fatal(err)
	}
	defer businessConn.Close()
	if _, err := businessConn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)
	if accepted.Load() != 1 {
		t.Fatalf("expected only one backend connection for business stream, got %d", accepted.Load())
	}
}
