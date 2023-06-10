package echo

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"net"
	"time"
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

func CheckUDP(targetAddr string, req string, timeout time.Duration) (string, error) {
	conn, err := net.DialTimeout("udp", targetAddr, timeout)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	_, err = conn.Write([]byte(req))
	if err != nil {
		return "", err
	}

	conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}

	return string(buf[0:n]), nil
}
