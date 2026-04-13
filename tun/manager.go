package tun

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/0990/gotun/pkg/util"
	"github.com/sirupsen/logrus"
)

type Manager struct {
	services map[string]Service
	lock     sync.RWMutex
	tunDir   string
}

func NewManager(tunDir string) *Manager {
	return &Manager{
		services: make(map[string]Service),
		lock:     sync.RWMutex{},
		tunDir:   tunDir,
	}
}

func (m *Manager) Run() error {
	cfgs, err := loadAllServiceFile(m.tunDir)
	if err != nil {
		return err
	}

	for _, v := range cfgs {
		err = m.AddService(v, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) GetService(name string) (Service, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	s, ok := m.services[name]
	return s, ok
}

func (m *Manager) RemoveService(name string) error {
	err := deleteServiceFile(m.tunDir, name)
	if err != nil {
		return err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	s, ok := m.services[name]
	if !ok {
		return nil
	}
	err = s.Close()
	if err != nil {
		return err
	}
	delete(m.services, name)
	return nil
}

func (m *Manager) RemoveServiceByUUID(uuid string) error {
	s, ok := m.GetServiceByUUID(uuid)
	if !ok {
		return errors.New("uuid not exist")
	}

	return m.RemoveService(s.Cfg().Name)
}

func (m *Manager) GetServiceByUUID(uuid string) (Service, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, v := range m.services {
		if v.Cfg().UUID == uuid {
			return v, true
		}
	}
	return nil, false
}

func (m *Manager) AddService(config Config, createFile bool) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	config = normalizeConfig(config)

	_, ok := m.services[config.Name]
	if ok {
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
		if err := createServiceFile(m.tunDir, config); err != nil {
			_ = s.Close()
			return err
		}
	}

	m.services[config.Name] = s
	return nil
}

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
		return m.rollbackReplaceLocked(oldName, oldCfg, err)
	}

	if err := replaceServiceFile(m.tunDir, oldCfg, config); err != nil {
		_ = newService.Close()
		return m.rollbackReplaceLocked(oldName, oldCfg, err)
	}

	delete(m.services, oldName)
	m.services[config.Name] = newService
	return nil
}

func (m *Manager) SetServiceDisabled(name string, disabled bool) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	current, ok := m.services[name]
	if !ok {
		return errors.New("tun not exist")
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
		return m.rollbackReplaceLocked(name, oldCfg, err)
	}

	if err := replaceServiceFile(m.tunDir, oldCfg, newCfg); err != nil {
		_ = newService.Close()
		return m.rollbackReplaceLocked(name, oldCfg, err)
	}

	m.services[name] = newService
	return nil
}

func (m *Manager) AllService() map[string]Service {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.services
}

func (m *Manager) ServiceFile(name string) string {
	return serviceFile(m.tunDir, name)
}

func (m *Manager) getServiceByUUIDLocked(uuid string) (Service, string, bool) {
	for name, v := range m.services {
		if v.Cfg().UUID == uuid {
			return v, name, true
		}
	}
	return nil, "", false
}

func (m *Manager) rollbackReplaceLocked(oldName string, oldCfg Config, cause error) error {
	oldService, rollbackErr := prepareManagedService(oldCfg)
	if rollbackErr != nil {
		delete(m.services, oldName)
		return fmt.Errorf("%w; rollback failed: %v", cause, rollbackErr)
	}
	if rollbackErr := runPreparedService(oldService); rollbackErr != nil {
		delete(m.services, oldName)
		return fmt.Errorf("%w; rollback failed: %v", cause, rollbackErr)
	}

	m.services[oldName] = oldService
	return cause
}

func prepareManagedService(config Config) (Service, error) {
	if config.Disabled {
		return NewDisabledService(config), nil
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
