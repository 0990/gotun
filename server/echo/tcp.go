package echo

import (
	"github.com/0990/gotun/core"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"time"
)

func StartTCPEchoServer(addr string) error {
	_, err := startTCPEcho(addr)
	return err
}

// tcpEchoServer 可关闭的 TCP echo 监听
type tcpEchoServer struct {
	listener net.Listener
}

// startTCPEcho 启动 TCP echo 监听并返回可关闭句柄
func startTCPEcho(addr string) (*tcpEchoServer, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	s := &tcpEchoServer{listener: listener}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go func(conn net.Conn) {
				defer conn.Close()

				log := logrus.WithFields(logrus.Fields{
					"address": conn.RemoteAddr(),
				})

				for {
					buf := make([]byte, core.MaxSegmentSize)
					n, err := conn.Read(buf)
					if err != nil {
						if err != io.EOF {
							log.WithError(err).Error("read from tcp")
						} else {
							log.WithError(err).Info("read from tcp")
						}
						return
					}

					log.WithField("data", string(buf[0:n])).Info("echoserver tcp receive")
					conn.Write(buf[0:n])
				}

			}(conn)
		}
	}()
	return s, nil
}

// Close 关闭 TCP 监听
func (s *tcpEchoServer) Close() error {
	return s.listener.Close()
}

func CheckTCP(targetAddr string, req string, timeout time.Duration) (string, error) {
	conn, err := net.DialTimeout("tcp", targetAddr, timeout)
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
