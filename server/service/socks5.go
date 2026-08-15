package service

import (
	"sync"

	"github.com/0990/socks5"
)

// socks5Service 标准 socks5 服务，持有 socks5.Server 句柄，Close 关闭 TCP+UDP 监听
type socks5Service struct {
	baseService
	srv  socks5.Server
	lock sync.Mutex
}

func newSocks5Service(cfg Config) Service {
	return &socks5Service{baseService: baseService{cfg: cfg, status: "running"}}
}

func (s *socks5Service) Run() error {
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

	srv, err := socks5.NewServer(socks5.ServerCfg{
		ListenPort: s.cfg.ListenPort,
		UserName:   s.cfg.Username,
		Password:   s.cfg.Password,
		TCPTimeout: tcpTimeout,
		UDPTimout:  udpTimeout,
		LogLevel:   "error",
	})
	if err != nil {
		return err
	}
	if err := srv.Run(); err != nil {
		return err
	}
	s.srv = srv
	return nil
}

func (s *socks5Service) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.srv == nil {
		return nil
	}
	return s.srv.Close()
}
