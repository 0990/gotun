package socks5client

import (
	"encoding/hex"
	"github.com/0990/socks5"
	"net"
)

func CheckUDP(addr string) (string, error) {
	sc := socks5.NewSocks5Client(socks5.ClientCfg{
		ServerAddr: addr,
		UserName:   "",
		Password:   "",
		UDPTimout:  60,
		TCPTimeout: 60,
	})

	conn, err := sc.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return "", err
	}

	// 发送一个dns请求包，返回dns服务器的ip
	b, err := hex.DecodeString("0001010000010000000000000a74787468696e6b696e6703636f6d0000010001")
	if err != nil {
		return "", err
	}
	if _, err := conn.Write(b); err != nil {
		return "", err
	}

	b = make([]byte, 2048)
	n, err := conn.Read(b)
	if err != nil {
		return "", err
	}
	b = b[:n]
	b = b[len(b)-4:]
	return net.IPv4(b[0], b[1], b[2], b[3]).String(), nil
}
