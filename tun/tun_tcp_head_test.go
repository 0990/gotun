package tun

import (
	"encoding/json"
	"fmt"
	"github.com/0990/gotun/server/echo"
	"testing"
	"time"
)

func Test_TcpTunHead(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	echo.StartTCPEchoServer(targetAddr)

	inHead := []byte("GET / HTTP/1.1\r\nHost:")
	outHead := []byte("GET / HTTP/1.1\r\nHost:")
	in, out, err := prepareHeadInOutCfg(inHead, outHead)
	if err != nil {
		t.Fatal(err)
		return
	}

	relayClientAddr := "127.0.0.1:6000"
	relayServerAddr := "127.0.0.1:6001"
	c, err := NewServer(Config{
		Name:          "tcp",
		Input:         fmt.Sprintf("tcp@%s", relayClientAddr),
		Output:        fmt.Sprintf("tcp@%s", relayServerAddr),
		InDecryptKey:  "",
		InDecryptMode: "",
		OutProtoCfg:   string(out),
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
		InProtoCfg:    string(in),
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

func Test_TcpMuxTunHead(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	echo.StartTCPEchoServer(targetAddr)

	inHead := []byte("ABCDE")
	outHead := []byte("ABCDE")
	in, out, err := prepareHeadInOutCfg(inHead, outHead)
	if err != nil {
		t.Fatal(err)
		return
	}

	relayClientAddr := "127.0.0.1:6000"
	relayServerAddr := "127.0.0.1:6001"

	s, err := NewServer(Config{
		Name:          "tcp",
		Input:         fmt.Sprintf("tcp_mux@%s", relayServerAddr),
		Output:        fmt.Sprintf("tcp@%s", targetAddr),
		InProtoCfg:    string(in),
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
		OutProtoCfg:   string(out),
		OutCryptKey:   "111111",
		OutCryptMode:  "gcm",
		OutExtend:     muxConnExtend(1),
	})
	if err != nil {
		t.Fatal(err)
	}

	c.Run()

	time.Sleep(time.Second * 2)
	echoTCP(t, relayClientAddr)
}

func muxConnExtend(count int) string {
	e := Extend{MuxConn: count}
	data, _ := json.Marshal(e)
	return string(data)
}

func prepareHeadInOutCfg(inHead []byte, outHead []byte) (string, string, error) {
	cfg := InProtoTCP{Head: inHead}
	in, err := json.Marshal(cfg)
	if err != nil {
		return "", "", err
	}

	outCfg := OutProtoTCP{HeadAppend: outHead}
	out, err := json.Marshal(outCfg)
	if err != nil {
		return "", "", err
	}

	return string(in), string(out), nil
}
