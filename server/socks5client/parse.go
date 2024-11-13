package socks5client

import (
	"github.com/0990/socks5"
	"net/url"
	"strings"
)

func ParseUrl(rawUrl string) (socks5.ClientCfg, string, error) {
	if !strings.HasPrefix(rawUrl, "socks5://") {
		rawUrl = "socks5://" + rawUrl
	}
	u, err := url.Parse(rawUrl)
	if err != nil {
		return socks5.ClientCfg{}, "", err
	}

	var url string
	if len(u.Path) > 0 {
		url = u.Path[1:]
	}

	password, _ := u.User.Password()
	clientCfg := socks5.ClientCfg{
		ServerAddr: u.Host,
		UserName:   u.User.Username(),
		Password:   password,
		UDPTimout:  60,
		TCPTimeout: 60,
	}

	return clientCfg, url, nil
}
