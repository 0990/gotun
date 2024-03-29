package tun

import (
	"errors"
	"github.com/0990/gotun/pkg/util"
	"github.com/sirupsen/logrus"
	"sync"
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

	if config.UUID == "" {
		config.UUID = util.NewUUID()
	}

	_, ok := m.services[config.Name]
	if ok {
		return errors.New("name already exist")
	}

	s, err := NewService(config)
	if err != nil {
		return err
	}

	go func() {
		err := s.Run()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"name": config.Name,
			}).WithError(err).Error("tun run error")
		}
	}()

	if createFile {
		err := createServiceFile(m.tunDir, config)
		if err != nil {
			return err
		}
	}

	m.services[config.Name] = s
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
