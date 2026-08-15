package echo

import (
	"sync"
)

// Server 可关闭的 echo 服务（同时监听 TCP 与 UDP，同地址回声）
type Server struct {
	address string

	tcpL  *tcpEchoServer
	udpL  *udpEchoServer
	once  sync.Once
}

// NewServer 构造 echo 服务，但不开始监听；调用 Run 才开始
func NewServer(address string) *Server {
	return &Server{address: address}
}

// Run 启动 TCP 与 UDP echo 监听，启动失败立即返回错误
func (s *Server) Run() error {
	tcpL, err := startTCPEcho(s.address)
	if err != nil {
		return err
	}
	s.tcpL = tcpL

	udpL, err := startUDPEcho(s.address)
	if err != nil {
		_ = s.tcpL.Close()
		return err
	}
	s.udpL = udpL
	return nil
}

// Close 关闭 TCP 与 UDP 监听，可重复调用
func (s *Server) Close() error {
	var err error
	s.once.Do(func() {
		if s.tcpL != nil {
			err = s.tcpL.Close()
		}
		if s.udpL != nil {
			if e := s.udpL.Close(); err == nil {
				err = e
			}
		}
	})
	return err
}
