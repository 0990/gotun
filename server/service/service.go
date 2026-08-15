package service

import "fmt"

// Service 内置服务实例生命周期接口
// 四种实现均持有底层可关闭句柄，Close 会释放监听端口，支持 web 端热启停。
type Service interface {
	Run() error
	Close() error
	Cfg() Config
	Status() string // running|disabled|error
}

// NewService 按类型构造内置服务实例
func NewService(cfg Config) (Service, error) {
	switch cfg.Type {
	case TypeEcho:
		return newEchoService(cfg), nil
	case TypeHttpProxy:
		return newHTTPProxyService(cfg), nil
	case TypeSocks5:
		return newSocks5Service(cfg), nil
	case TypeSocks5X:
		return newSocks5XService(cfg), nil
	default:
		return nil, fmt.Errorf("invalid service type: %s", cfg.Type)
	}
}

// disabledService 禁用占位实例（不监听，仅保存配置）
type disabledService struct {
	cfg Config
}

func newDisabledService(cfg Config) Service {
	cfg.Disabled = true
	return &disabledService{cfg: cfg}
}

func (s *disabledService) Run() error   { return nil }
func (s *disabledService) Close() error { return nil }
func (s *disabledService) Cfg() Config  { return s.cfg }
func (s *disabledService) Status() string {
	return "disabled"
}

// baseService 四种实例的公共字段：配置与状态
type baseService struct {
	cfg    Config
	status string
}

func (s *baseService) Cfg() Config { return s.cfg }

func (s *baseService) Status() string {
	if s.status == "" {
		return "running"
	}
	return s.status
}
