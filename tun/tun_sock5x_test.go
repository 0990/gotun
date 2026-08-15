package tun

import (
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/0990/gotun/server/echo"
	"github.com/0990/gotun/server/socks5x"
	"github.com/0990/socks5"
)

// Test_Socks5X 验证完整链路：socks5 客户端 -> 隧道入口(socks5x) -> 加密中继
// -> 上级 socks5x 服务器 -> 最终目标（本地 echo 服务）。
// 用例自包含：所有端口动态分配，最终目标用本地 echo 代替外部服务。
func Test_Socks5X(t *testing.T) {
	// 最终目标：本地 echo 服务（替代原先依赖的外部服务 127.0.0.1:9999）
	targetAddr := reserveTCPAddr(t)
	if err := echo.StartTCPEchoServer(targetAddr); err != nil {
		t.Fatalf("start echo server: %v", err)
	}

	// 上级 socks5x 代理服务器（Windows 保留端口段可能导致 bind 失败，换端口重试）
	upstream, upstreamPort := startSocks5X(t)
	defer upstream.Close()

	relayClientAddr := reserveTCPAddr(t)
	relayServerAddr := reserveTCPAddr(t)

	s, err := NewServer(Config{
		Name:          "socks5x_relay_server",
		Input:         fmt.Sprintf("tcp@%s", relayServerAddr),
		Output:        fmt.Sprintf("tcp@127.0.0.1:%d", upstreamPort),
		InDecryptKey:  "goodweather",
		InDecryptMode: "gcm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Run(); err != nil {
		t.Fatalf("run relay server: %v", err)
	}
	defer s.Close()

	c, err := NewServer(Config{
		Name:         "socks5x_relay_client",
		Input:        fmt.Sprintf("socks5x@%s", relayClientAddr),
		Output:       fmt.Sprintf("tcp@%s", relayServerAddr),
		OutCryptKey:  "goodweather",
		OutCryptMode: "gcm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Run(); err != nil {
		t.Fatalf("run relay client: %v", err)
	}
	defer c.Close()

	time.Sleep(time.Second * 2)

	sc := socks5.NewSocks5Client(socks5.ClientCfg{
		ServerAddr: relayClientAddr,
		TCPTimeout: 5,
		UDPTimout:  5,
	})
	conn, err := sc.Dial("tcp", targetAddr)
	if err != nil {
		t.Fatalf("socks5 dial through tunnel: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(time.Second * 5))
	buf := make([]byte, 32)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if got := string(buf[:n]); got != "hello" {
		t.Fatalf("echo got %q, want %q", got, "hello")
	}
}

// startSocks5X 在动态空闲端口上启动 socks5x 服务器，bind 失败（如落入
// Windows 保留端口段）时换端口重试，返回服务器实例与实际监听的端口
func startSocks5X(t *testing.T) (*socks5x.Server, int) {
	t.Helper()
	var lastErr error
	for i := 0; i < 10; i++ {
		port := reserveTCPPort(t)
		s, err := socks5x.NewServer(port, 300, 200)
		if err != nil {
			lastErr = err
			continue
		}
		if err := s.Run(); err != nil {
			lastErr = err
			continue
		}
		return s, port
	}
	t.Fatalf("start socks5x server: %v", lastErr)
	return nil, 0
}

// reserveTCPPort 分配一个空闲 TCP 端口号（先监听再释放，存在微小竞争窗口）
func reserveTCPPort(t *testing.T) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(reserveTCPAddr(t))
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return port
}
