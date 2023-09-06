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

const defaultTestWebUrl = "http://ipinfo.io"

func CheckTCP(clientCfg socks5.ClientCfg, testWebUrl string, timeout time.Duration) (response string, err error) {
	sc := socks5.NewSocks5Client(clientCfg)

	hc := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return sc.Dial(network, addr)
			},
		},
	}

	if testWebUrl == "" {
		testWebUrl = defaultTestWebUrl
	}

	resp, err := hc.Get(testWebUrl)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status code:%d", resp.StatusCode)
	}

	if testWebUrl != defaultTestWebUrl {
		return fmt.Sprintf("rsp(%s):ok", testWebUrl), nil
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

	return fmt.Sprintf("rsp(ipinfo.io):%s", r.IP), nil
}
