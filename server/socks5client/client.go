package socks5client

import (
	"fmt"
)

func CheckServer(addr string) {
	response, err := CheckTCP(addr)
	if err != nil {
		fmt.Printf("check tcp failed:%s \n", err.Error())
	} else {
		fmt.Printf("check tcp success,response(ipinfo.io):%s \n", response)
	}

	response, err = CheckUDP(addr)
	if err != nil {
		fmt.Printf("check udp failed:%s \n", err.Error())
	} else {
		fmt.Printf("check udp success,response(8.8.8.8):%s \n", response)
	}
}
