package socks5client

import (
	"fmt"
	"time"
)

func CheckServer(rawUrl string) {
	clientCfg, testWebUrl, err := ParseUrl(rawUrl)
	if err != nil {
		fmt.Printf("parse url failed:%s \n", err.Error())
		return
	}

	response, err := CheckTCP(clientCfg, testWebUrl, time.Second*2)
	if err != nil {
		fmt.Printf("check tcp failed:%s \n", err.Error())
	} else {
		fmt.Printf("check tcp success,response(ipinfo.io):%s \n", response)
	}

	_, response, err = CheckUDP(clientCfg, time.Second*2)
	if err != nil {
		fmt.Printf("check udp failed:%s \n", err.Error())
	} else {
		fmt.Printf("check udp success,response(8.8.8.8):%s \n", response)
	}
}
