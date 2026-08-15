package echo

import (
	"github.com/0990/gotun/core"
	"github.com/sirupsen/logrus"
	"net"
	"time"
)

func StartUDPEchoServer(address string) error {
	_, err := startUDPEcho(address)
	return err
}

// udpEchoServer 可关闭的 UDP echo 监听
type udpEchoServer struct {
	conn *net.UDPConn
}

// startUDPEcho 启动 UDP echo 监听并返回可关闭句柄
func startUDPEcho(address string) (*udpEchoServer, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	listen, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	s := &udpEchoServer{conn: listen}
	go func() {
		for {
			var data [core.MaxSegmentSize]byte
			n, addr, err := listen.ReadFromUDP(data[:])
			if err != nil {
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
	return s, nil
}

// Close 关闭 UDP 监听
func (s *udpEchoServer) Close() error {
	return s.conn.Close()
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
	buf := make([]byte, core.MaxSegmentSize)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}

	return string(buf[0:n]), nil
}
