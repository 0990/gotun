package socks5client

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/0990/socks5"
	"io"
	"net"
	"net/http"
	"time"
)

func CheckTCP(addr string, timeout time.Duration) (response string, err error) {
	sc := socks5.NewSocks5Client(socks5.ClientCfg{
		ServerAddr: addr,
		UserName:   "",
		Password:   "",
		UDPTimout:  60,
		TCPTimeout: 60,
	})

	hc := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return sc.Dial(network, addr)
			},
		},
	}
	resp, err := hc.Get("http://ipinfo.io/")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status code:%d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	type Response struct {
		IP string `json:"ip"`
	}

	var r Response
	err = json.Unmarshal(b, &r)
	if err != nil {
		return "", err
	}

	return r.IP, nil
}
