package tun

import (
	"context"
	"fmt"
	"github.com/0990/gotun/server/socks5x"
	"github.com/0990/socks5"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"
)

func Test_Socks5X(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	startSocks5X(7007)

	relayClientAddr := "127.0.0.1:6000"
	relayServerAddr := "127.0.0.1:6001"
	c, err := NewServer(Config{
		Name:   "tcp",
		Input:  fmt.Sprintf("socks5x@%s", relayClientAddr),
		Output: fmt.Sprintf("tcp@%s", relayServerAddr),
		//InProtoCfg:    inSocks5xStr(InProtoSocks5X{}),
		InDecryptKey:  "",
		InDecryptMode: "",
		OutCryptKey:   "goodweather",
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
		InDecryptKey:  "goodweather",
		InDecryptMode: "gcm",
		OutCryptKey:   "",
		OutCryptMode:  "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()
	time.Sleep(time.Second * 2)

	socks5ClientTest(socks5.ClientCfg{
		ServerAddr: relayClientAddr,
		UserName:   "",
		Password:   "",
		UDPTimout:  60,
		TCPTimeout: 60,
	}, t)
}

func startSocks5X(listenPort int) error {
	s, err := socks5x.NewServer(listenPort, 60, 60)
	if err != nil {
		return err
	}
	err = s.Run()
	if err != nil {
		return err
	}
	return nil
}

func socks5ClientTest(cfg socks5.ClientCfg, t *testing.T) {
	sc := socks5.NewSocks5Client(cfg)

	hc := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return sc.Dial(network, addr)
			},
		},
	}
	resp, err := hc.Get("http://whatismyip.akamai.com/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.FailNow()
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(string(b))
}
