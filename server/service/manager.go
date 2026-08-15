package service

import (
	"errors"
	"sync"
	"time"

	"github.com/0990/gotun/pkg/util"
	"github.com/sirupsen/logrus"
)

// Manager 内置服务实例管理器：负责加载、增删改、启停与持久化
type Manager struct {
	services map[string]Service
	lock     sync.RWMutex
	dir      string // tunnel 目录
}

func NewManager(dir string) *Manager {
	return &Manager{
		services: make(map[string]Service),
		lock:     sync.RWMutex{},
		dir:      dir,
	}
}

// Run 启动时加载所有 .service 配置并启动
func (m *Manager) Run() error {
	cfgs, err := loadAllServiceFile(m.dir)
	if err != nil {
		return err
	}

	for _, v := range cfgs {
		if err := m.AddService(v, false); err != nil {
			// 单个实例加载失败不阻断整体启动，仅记录
			logrus.WithError(err).WithField("service", v.Name).Error("load service failed")
		}
	}
	return nil
}

// CreateFileOnly 仅把配置写入 .service 文件，不启动实例（供启动前迁移使用；实例由 Run 统一加载启动）
func (m *Manager) CreateFileOnly(config Config) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	config = normalizeConfig(config)
	if _, ok := m.services[config.Name]; ok {
		return errors.New("name already exist")
	}
	return createServiceFile(m.dir, config)
}

// ServiceExistFile 判断指定名称的 .service 文件是否已存在
func (m *Manager) ServiceExistFile(name string) bool {
	return isFileExist(serviceFile(m.dir, name))
}

// AddService 新增内置服务实例
func (m *Manager) AddService(config Config, createFile bool) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	config = normalizeConfig(config)

	if _, ok := m.services[config.Name]; ok {
		return errors.New("name already exist")
	}

	s, err := prepareManagedService(config)
	if err != nil {
		return err
	}
	if err := runPreparedService(s); err != nil {
		return err
	}

	if createFile {
		if err := createServiceFile(m.dir, config); err != nil {
			_ = s.Close()
			return err
		}
	}

	m.services[config.Name] = s
	return nil
}

// RemoveService 删除内置服务实例（删文件 + 移除运行实例）
func (m *Manager) RemoveService(name string) error {
	if err := deleteServiceFile(m.dir, name); err != nil {
		return err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	s, ok := m.services[name]
	if !ok {
		return nil
	}
	if err := s.Close(); err != nil {
		return err
	}
	delete(m.services, name)
	return nil
}

// ReplaceServiceByUUID 按 UUID 替换实例（编辑）
func (m *Manager) ReplaceServiceByUUID(config Config) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	oldService, oldName, ok := m.getServiceByUUIDLocked(config.UUID)
	if !ok {
		return errors.New("uuid not exist")
	}

	oldCfg := oldService.Cfg()
	config = normalizeConfig(config)
	config.UUID = oldCfg.UUID
	config.CreatedAt = oldCfg.CreatedAt

	if config.Name != oldName {
		if _, nameExists := m.services[config.Name]; nameExists {
			return errors.New("name already exist")
		}
	}

	newService, err := prepareManagedService(config)
	if err != nil {
		return err
	}

	if err := oldService.Close(); err != nil {
		return err
	}
	if err := runPreparedService(newService); err != nil {
		return err
	}

	if err := replaceServiceFile(m.dir, oldCfg, config); err != nil {
		_ = newService.Close()
		return err
	}

	delete(m.services, oldName)
	m.services[config.Name] = newService
	return nil
}

// SetServiceDisabled 启用/禁用实例
func (m *Manager) SetServiceDisabled(name string, disabled bool) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	current, ok := m.services[name]
	if !ok {
		return errors.New("service not exist")
	}

	oldCfg := current.Cfg()
	if oldCfg.Disabled == disabled {
		return nil
	}

	newCfg := oldCfg
	newCfg.Disabled = disabled

	newService, err := prepareManagedService(newCfg)
	if err != nil {
		return err
	}

	if err := current.Close(); err != nil {
		return err
	}
	if err := runPreparedService(newService); err != nil {
		return err
	}

	if err := replaceServiceFile(m.dir, oldCfg, newCfg); err != nil {
		_ = newService.Close()
		return err
	}

	m.services[name] = newService
	return nil
}

// AllService 返回全部实例（按名称）
func (m *Manager) AllService() map[string]Service {
	m.lock.RLock()
	defer m.lock.RUnlock()

	out := make(map[string]Service, len(m.services))
	for k, v := range m.services {
		out[k] = v
	}
	return out
}

// GetService 按名称取实例
func (m *Manager) GetService(name string) (Service, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	s, ok := m.services[name]
	return s, ok
}

func (m *Manager) getServiceByUUIDLocked(uuid string) (Service, string, bool) {
	for name, v := range m.services {
		if v.Cfg().UUID == uuid {
			return v, name, true
		}
	}
	return nil, "", false
}

func prepareManagedService(config Config) (Service, error) {
	if config.Disabled {
		return newDisabledService(config), nil
	}
	return NewService(config)
}

func runPreparedService(s Service) error {
	go func() {
		if err := s.Run(); err != nil {
			logrus.WithError(err).WithField("name", s.Cfg().Name).Error("runPreparedService")
		}
	}()
	return nil
}

func normalizeConfig(config Config) Config {
	if config.CreatedAt.IsZero() {
		config.CreatedAt = time.Now()
	}
	if config.UUID == "" {
		config.UUID = util.NewUUID()
	}
	return config
}
