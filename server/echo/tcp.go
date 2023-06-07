package echo

import (
	"github.com/sirupsen/logrus"
	"io"
	"net"
)

func StartTCPEchoServer(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

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
					buf := make([]byte, 65535)
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
	return nil
}
