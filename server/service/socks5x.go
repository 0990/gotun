package service

import (
	"sync"

	"github.com/0990/gotun/server/socks5x"
)

// socks5XService socks5x 服务（自定义协议），持有 socks5x.Server 句柄，Close 释放端口
type socks5XService struct {
	baseService
	srv  *socks5x.Server
	lock sync.Mutex
}

func newSocks5XService(cfg Config) Service {
	return &socks5XService{baseService: baseService{cfg: cfg, status: "running"}}
}

func (s *socks5XService) Run() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	tcpTimeout := s.cfg.TCPTimeout
	if tcpTimeout <= 0 {
		tcpTimeout = 300
	}
	udpTimeout := s.cfg.UDPTimeout
	if udpTimeout <= 0 {
		udpTimeout = 120
	}

	srv, err := socks5x.NewServer(s.cfg.ListenPort, tcpTimeout, udpTimeout)
	if err != nil {
		return err
	}
	if err := srv.Run(); err != nil {
		return err
	}
	s.srv = srv
	return nil
}

func (s *socks5XService) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.srv == nil {
		return nil
	}
	return s.srv.Close()
}
