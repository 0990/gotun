package tun

import (
	"net"
	"testing"
	"time"

	kcpx "github.com/0990/kcpx-go"
)

func Test_KCPXReconnectMigratesSession(t *testing.T) {
	opts := kcpx.DefaultOptions()
	opts.HeartbeatInterval = 100 * time.Millisecond
	opts.HeartbeatTimeout = 500 * time.Millisecond

	listener, err := kcpx.ListenWithOptions("127.0.0.1:0", opts)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverConnCh := make(chan *kcpx.Conn, 1)
	go func() {
		conn, err := listener.AcceptKCP()
		if err == nil {
			serverConnCh <- conn
		}
	}()

	clientConn, err := kcpx.DialWithOptions(listener.Addr().String(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	var serverConn *kcpx.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(3 * time.Second):
		t.Fatal("accept timeout")
	}
	defer serverConn.Close()

	oldPort := clientConn.LocalAddr().(*net.UDPAddr).Port
	if err := clientConn.Reconnect(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		currentPort := clientConn.LocalAddr().(*net.UDPAddr).Port
		serverPort := serverConn.RemoteAddr().(*net.UDPAddr).Port
		if currentPort != oldPort && currentPort == serverPort {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server remote addr not migrated, client=%v server=%v", clientConn.LocalAddr(), serverConn.RemoteAddr())
		}
		time.Sleep(10 * time.Millisecond)
	}

	serverConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := clientConn.Write([]byte("after-reconnect")); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 32)
	n, err := serverConn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "after-reconnect" {
		t.Fatalf("server read got %q", got)
	}

	clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := serverConn.Write([]byte("ack")); err != nil {
		t.Fatal(err)
	}
	n, err = clientConn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "ack" {
		t.Fatalf("client read got %q", got)
	}
}
