package service

import (
	"sync"
	"time"

	"github.com/0990/httpproxy"
)

// httpProxyService http 代理服务，持有 httpproxy.Server 句柄，Close 关闭监听使 ListenAndServe 返回
type httpProxyService struct {
	baseService
	srv  httpproxy.Server
	lock sync.Mutex
}

func newHTTPProxyService(cfg Config) Service {
	return &httpProxyService{baseService: baseService{cfg: cfg, status: "running"}}
}

func (s *httpProxyService) Run() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	srv := httpproxy.NewServer(httpproxy.Config{
		BindAddr: s.cfg.Listen,
		Hosts:    []string{"*"},
		Verbose:  false,
	})

	// 库的 ListenAndServe 包含 监听+accept，启动失败（如端口被占用）会通过 errCh 返回；
	// 监听建立后进入 accept 循环不再返回（除非 Close）。用短等待区分"立即失败"与"监听已建立"。
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		// 立即返回，视为启动失败（正常 accept 循环不会这么快返回）
		return err
	case <-time.After(200 * time.Millisecond):
		// 监听已建立并进入 accept 循环
	}

	s.srv = srv
	return nil
}

func (s *httpProxyService) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.srv == nil {
		return nil
	}
	return s.srv.Close()
}
