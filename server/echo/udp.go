package echo

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"net"
)

func StartUDPEchoServer(address string) error {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}
	listen, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	go func() {
		for {
			var data [65535]byte
			n, addr, err := listen.ReadFromUDP(data[:])
			if err != nil {
				fmt.Println(err)
				break
			}

			log := logrus.WithFields(logrus.Fields{
				"address": addr,
				"data":    string(data[:n]),
			})
			log.Info("echoserver udp receive")
			_, err = listen.WriteToUDP(data[:n], addr)
			if err != nil {
				log.WithError(err).Error("write to udp")
				continue
			}
		}
	}()
	return nil
}
