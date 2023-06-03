package tun

import (
	"fmt"
	"github.com/0990/gotun/echoserver"
	"github.com/sirupsen/logrus"
	"testing"
	"time"
)

func TestFrp_Run(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	echoserver.StartTCPEchoServer(targetAddr)

	relayClientAddr := "127.0.0.1:6000"
	workerRemoteAddr := "127.0.0.1:6001"
	c, err := NewService(Config{
		Name:          "tcp",
		Mode:          "frps",
		Input:         fmt.Sprintf("tcp@%s", relayClientAddr),
		Output:        fmt.Sprintf("tcp@%s", workerRemoteAddr),
		InDecryptKey:  "",
		InDecryptMode: "",
		OutCryptKey:   "111111",
		OutCryptMode:  "gcm",
	})
	if err != nil {
		t.Fatal(err)
	}

	c.Run()

	s, err := NewService(Config{
		Name:          "tcp",
		Mode:          "frpc",
		Input:         fmt.Sprintf("tcp@%s", workerRemoteAddr),
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
	echo(t, relayClientAddr)
}

func Test_Frp_KCPMuxTun(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	echoserver.StartTCPEchoServer(targetAddr)

	relayClientAddr := "127.0.0.1:6000"
	workerServerAddr := "127.0.0.1:6001"

	s, err := NewService(Config{
		Name:          "",
		Mode:          "frpc",
		Input:         fmt.Sprintf("kcp_mux@%s", workerServerAddr),
		Output:        fmt.Sprintf("tcp@%s", targetAddr),
		InDecryptKey:  "111111",
		InDecryptMode: "gcm",
		InExtend:      muxConnExtend(10),

		OutCryptKey:  "",
		OutCryptMode: "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()

	c, err := NewService(Config{
		Name:          "",
		Mode:          "frps",
		Input:         fmt.Sprintf("tcp@%s", relayClientAddr),
		Output:        fmt.Sprintf("kcp_mux@%s", workerServerAddr),
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
	echo(t, relayClientAddr)
}

func Test_Frp_UDPTun(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	targetAddr := "127.0.0.1:7007"
	echoserver.StartUDPEchoServer(targetAddr)

	relayClientAddr := "127.0.0.1:6000"
	workerServerAddr := "127.0.0.1:6001"
	c, err := NewService(Config{
		Name:          "udp_tun_client",
		Mode:          "frps",
		Input:         fmt.Sprintf("udp@%s", relayClientAddr),
		Output:        fmt.Sprintf("udp@%s", workerServerAddr),
		InDecryptKey:  "",
		InDecryptMode: "",
		OutCryptKey:   "",
		OutCryptMode:  "",
	})
	if err != nil {
		t.Fatal(err)
	}

	c.Run()
	time.Sleep(time.Second)

	s, err := NewService(Config{
		Name:          "udp_tun_server",
		Mode:          "frpc",
		Input:         fmt.Sprintf("udp@%s", workerServerAddr),
		Output:        fmt.Sprintf("udp@%s", targetAddr),
		InDecryptKey:  "",
		InDecryptMode: "",
		OutCryptKey:   "",
		OutCryptMode:  "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()
	time.Sleep(time.Second)
	err = echoUDP(relayClientAddr)
	if err != nil {
		t.Fatal(err)
	}
}
