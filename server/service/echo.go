package service

import (
	"sync"

	"github.com/0990/gotun/server/echo"
)

// echoService echo 测试服务，持有 echo.Server 句柄，Close 释放 TCP+UDP 监听
type echoService struct {
	baseService
	srv  *echo.Server
	lock sync.Mutex
}

func newEchoService(cfg Config) Service {
	return &echoService{baseService: baseService{cfg: cfg, status: "running"}}
}

func (s *echoService) Run() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	srv := echo.NewServer(s.cfg.Listen)
	if err := srv.Run(); err != nil {
		return err
	}
	s.srv = srv
	return nil
}

func (s *echoService) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.srv == nil {
		return nil
	}
	return s.srv.Close()
}
