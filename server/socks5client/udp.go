package socks5client

import (
	"encoding/hex"
	"github.com/0990/socks5"
	"net"
	"time"
)

func CheckUDP(clientCfg socks5.ClientCfg, timeout time.Duration) (advertisedAddr string, response string, err error) {
	sc := socks5.NewSocks5Client(clientCfg)

	sc.SetHandShakeCallback(func(cmd byte, reply *socks5.Reply) {
		if cmd == socks5.CmdUDP {
			advertisedAddr = reply.Address()
		}
	})

	conn, err := sc.DialTimeout("udp", "8.8.8.8:53", timeout)
	if err != nil {
		return advertisedAddr, "", err
	}

	// 发送一个dns请求包，返回dns服务器的ip
	b, err := hex.DecodeString("0001010000010000000000000a74787468696e6b696e6703636f6d0000010001")
	if err != nil {
		return advertisedAddr, "", err
	}
	if _, err := conn.Write(b); err != nil {
		return advertisedAddr, "", err
	}

	conn.SetReadDeadline(time.Now().Add(timeout))
	b = make([]byte, 2048)
	n, err := conn.Read(b)
	if err != nil {
		return advertisedAddr, "", err
	}

	b = b[:n]
	b = b[len(b)-4:]
	return advertisedAddr, net.IPv4(b[0], b[1], b[2], b[3]).String(), nil
}
