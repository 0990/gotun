package socks5x

import (
	"errors"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/msg"
	"github.com/0990/socks5"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"time"
)

type Server struct {
	listenPort int
	tcpTimeout int
	udpTimeout int
}

func NewServer(listenPort int, tcpTimeout int, udpTimeout int) (*Server, error) {
	return &Server{
		listenPort: listenPort,
		tcpTimeout: tcpTimeout,
		udpTimeout: udpTimeout,
	}, nil
}

func (s *Server) Run() error {
	s5, err := socks5.NewServer(socks5.ServerCfg{
		ListenPort: s.listenPort,
		TCPTimeout: s.tcpTimeout,
		UDPTimout:  s.udpTimeout,
		LogLevel:   "debug",
	})

	if err != nil {
		return err
	}

	s5.SetCustomTcpConnHandler(s.handleConn)
	err = s5.Run()
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) handleConn(conn *net.TCPConn) {
	err := s.handleConnError(conn)
	if err != nil {
		if err != io.EOF {
			logrus.WithError(err).Error("handleConnError")
		}
	}
}

func (s *Server) handleConnError(conn *net.TCPConn) error {
	defer conn.Close()

	m, err := msg.ReadMsg(conn)
	if err != nil {
		return err
	}

	req, ok := m.(*msg.Socks5XReq)
	if !ok {
		return errors.New("msg type error")
	}

	targetAddr := req.TargetAddr
	dst, err := net.DialTimeout("tcp", targetAddr, time.Second*3)
	if err != nil {
		//errStr := err.Error()
		//var rep byte = socks5.RepHostUnreachable
		//if strings.Contains(errStr, "refused") {
		//	rep = socks5.RepConnectionRefused
		//} else if strings.Contains(errStr, "network is unreachable") {
		//	rep = socks5.RepNetworkUnreachable
		//}

		//msg.WriteMsg(conn, &msg.Socks5XResp{
		//	Rep: rep,
		//})
		logrus.WithError(err).Debugf("connect to %v failed", targetAddr)
		return nil
	}
	defer dst.Close()

	//err = msg.WriteMsg(conn, &msg.Socks5XResp{
	//	Rep: socks5.RepSuccess,
	//})
	//
	//if err != nil {
	//	return err
	//}

	timeout := time.Duration(s.tcpTimeout) * time.Second
	core.Pipe(&TCPConn{Conn: dst}, &TCPConn{Conn: conn}, timeout)
	return nil
}
