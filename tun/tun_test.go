package tun

import (
	"fmt"
	"github.com/0990/gotun/server/echo"
	"net"
	"testing"
	"time"
)

func Test_Tcp(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	echo.StartTCPEchoServer(targetAddr)

	relayAddr := "127.0.0.1:6000"
	s, err := NewServer(Config{
		Name:          "tcp",
		Input:         fmt.Sprintf("tcp@%s", relayAddr),
		Output:        fmt.Sprintf("tcp@%s", targetAddr),
		InDecryptKey:  "",
		InDecryptMode: "",
		OutCryptKey:   "",
		OutCryptMode:  "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()
	time.Sleep(time.Second * 2)
	echoTCP(t, relayAddr)
}

func Test_TcpTun(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	echo.StartTCPEchoServer(targetAddr)

	relayClientAddr := "127.0.0.1:6000"
	relayServerAddr := "127.0.0.1:6001"
	c, err := NewServer(Config{
		Name:          "tcp",
		Input:         fmt.Sprintf("tcp@%s", relayClientAddr),
		Output:        fmt.Sprintf("tcp@%s", relayServerAddr),
		InDecryptKey:  "",
		InDecryptMode: "",
		OutCryptKey:   "111111",
		OutCryptMode:  "gcm",
	})
	if err != nil {
		t.Fatal(err)
	}

	c.Run()

	s, err := NewServer(Config{
		Name:          "tcp",
		Input:         fmt.Sprintf("tcp@%s", relayServerAddr),
		Output:        fmt.Sprintf("tcp@%s", targetAddr),
		InDecryptKey:  "111111",
		InDecryptMode: "gcm",
		OutCryptKey:   "",
		OutCryptMode:  "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()
	time.Sleep(time.Second * 2)
	echoTCP(t, relayClientAddr)
}

func Test_TcpMuxTun(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	echo.StartTCPEchoServer(targetAddr)

	relayClientAddr := "127.0.0.1:6000"
	relayServerAddr := "127.0.0.1:6001"

	s, err := NewServer(Config{
		Name:          "tcp",
		Input:         fmt.Sprintf("tcp_mux@%s", relayServerAddr),
		Output:        fmt.Sprintf("tcp@%s", targetAddr),
		InDecryptKey:  "111111",
		InDecryptMode: "gcm",
		OutCryptKey:   "",
		OutCryptMode:  "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()

	c, err := NewServer(Config{

		Name:          "tcp",
		Input:         fmt.Sprintf("tcp@%s", relayClientAddr),
		Output:        fmt.Sprintf("tcp_mux@%s", relayServerAddr),
		InDecryptKey:  "",
		InDecryptMode: "",

		OutCryptKey:  "111111",
		OutCryptMode: "gcm",
	})
	if err != nil {
		t.Fatal(err)
	}

	c.Run()

	time.Sleep(time.Second * 2)
	echoTCP(t, relayClientAddr)
}

func Test_QUICTun(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	echo.StartTCPEchoServer(targetAddr)

	relayClientAddr := "127.0.0.1:6000"
	relayServerAddr := "127.0.0.1:6001"

	s, err := NewServer(Config{

		Name:          "quic_tun_client",
		Input:         fmt.Sprintf("quic@%s", relayServerAddr),
		Output:        fmt.Sprintf("tcp@%s", targetAddr),
		InDecryptKey:  "111111",
		InDecryptMode: "gcm",

		OutCryptKey:  "",
		OutCryptMode: "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()

	c, err := NewServer(Config{

		Name:          "quic_tun_client",
		Input:         fmt.Sprintf("tcp@%s", relayClientAddr),
		Output:        fmt.Sprintf("quic@%s", relayServerAddr),
		InDecryptKey:  "",
		InDecryptMode: "",

		OutCryptKey:  "111111",
		OutCryptMode: "gcm",
		OutExtend:    muxConnExtend(10),
	})
	if err != nil {
		t.Fatal(err)
	}

	c.Run()

	time.Sleep(time.Second * 2)
	echoTCP(t, relayClientAddr)
}

func Test_KCPTun(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	echo.StartTCPEchoServer(targetAddr)

	relayClientAddr := "127.0.0.1:6000"
	relayServerAddr := "127.0.0.1:6001"

	s, err := NewServer(Config{
		Name:          "",
		Input:         fmt.Sprintf("kcp@%s", relayServerAddr),
		Output:        fmt.Sprintf("tcp@%s", targetAddr),
		InDecryptKey:  "111111",
		InDecryptMode: "gcm",
		OutCryptKey:   "",
		OutCryptMode:  "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()

	c, err := NewServer(Config{

		Name:          "",
		Input:         fmt.Sprintf("tcp@%s", relayClientAddr),
		Output:        fmt.Sprintf("kcp@%s", relayServerAddr),
		InDecryptKey:  "",
		InDecryptMode: "",

		OutCryptKey:  "111111",
		OutCryptMode: "gcm",
	})

	if err != nil {
		t.Fatal(err)
	}

	c.Run()

	time.Sleep(time.Second * 2)
	echoTCP(t, relayClientAddr)
}

func Test_KCPMuxTun(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	echo.StartTCPEchoServer(targetAddr)

	relayClientAddr := "127.0.0.1:6000"
	relayServerAddr := "127.0.0.1:6001"

	s, err := NewServer(Config{

		Name:          "",
		Input:         fmt.Sprintf("kcp_mux@%s", relayServerAddr),
		Output:        fmt.Sprintf("tcp@%s", targetAddr),
		InDecryptKey:  "111111",
		InDecryptMode: "gcm",

		OutCryptKey:  "",
		OutCryptMode: "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()

	c, err := NewServer(Config{
		Name:          "",
		Input:         fmt.Sprintf("tcp@%s", relayClientAddr),
		Output:        fmt.Sprintf("kcp_mux@%s", relayServerAddr),
		InDecryptKey:  "",
		InDecryptMode: "",

		OutCryptKey:  "111111",
		OutCryptMode: "gcm",
		OutExtend:    muxConnExtend(10),
	})

	if err != nil {
		t.Fatal(err)
	}

	c.Run()

	time.Sleep(time.Second * 2)
	echoTCP(t, relayClientAddr)
}

func echoTCP(t *testing.T, clientAddr string) error {
	conn, err := net.Dial("tcp", clientAddr)
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now()
	err = checkEchoReplyTCP(conn)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("latency:", time.Since(before).Milliseconds())
	return nil
}

func checkEchoReplyTCP(conn net.Conn) error {
	send1 := "hello"
	_, err := conn.Write([]byte(send1))
	if err != nil {
		return err
	}

	send2 := "world"
	_, err = conn.Write([]byte(send2))
	if err != nil {
		return err
	}

	var data []byte
	for {
		conn.SetReadDeadline(time.Now().Add(time.Second * 2))
		buf := make([]byte, 65535)
		n, err := conn.Read(buf)
		if err != nil {
			break
		}
		data = append(data, buf[0:n]...)
	}

	if string(data) != "helloworld" {
		return fmt.Errorf("echo send:%s %s receive:%s", send1, send2, string(data))
	}

	return nil
}
