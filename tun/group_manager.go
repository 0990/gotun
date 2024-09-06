package tun

import (
	"errors"
	"github.com/0990/gotun/pkg/util"
	"github.com/sirupsen/logrus"
	"sync"
)

type GroupManager struct {
	services map[string]Service
	groups   map[string]*Group
	lock     sync.RWMutex
	dir      string
	groupDir string
}

func NewGroupManager(dir string) *GroupManager {
	return &GroupManager{
		services: make(map[string]Service),
		lock:     sync.RWMutex{},
		dir:      dir,
	}
}

func (m *GroupManager) Run() error {
	groupsCfg, err := loadAllFile[GroupConfig](m.dir, GROUP_CONFIG_SUFFIX)
	if err != nil {
		return err
	}

	for _, v := range groupsCfg {
		err = m.AddGroup(v, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *GroupManager) AllGroups() map[string]*Group {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.groups
}

func (m *GroupManager) GroupFile(name string) string {
	return groupFile(m.dir, name)
}

func (m *GroupManager) AddGroup(config GroupConfig, createFile bool) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if config.UUID == "" {
		config.UUID = util.NewUUID()
	}

	_, ok := m.services[config.Name]
	if ok {
		return errors.New("name already exist")
	}

	s, err := newGroup(config)
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
		err := createGroupFile(m.dir, config)
		if err != nil {
			return err
		}
	}

	m.groups[config.Name] = s
	return nil
}

func (m *GroupManager) GetGroup(name string) (*Group, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	s, ok := m.groups[name]
	return s, ok
}

func (m *GroupManager) RemoveGroup(name string) error {
	err := deleteGroupFile(m.dir, name)
	if err != nil {
		return err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	s, ok := m.groups[name]
	if !ok {
		return nil
	}
	err = s.Close()
	if err != nil {
		return err
	}
	delete(m.groups, name)
	return nil
}

func (m *GroupManager) RemoveGroupByUUID(uuid string) error {
	s, ok := m.GetGroupByUUID(uuid)
	if !ok {
		return errors.New("uuid not exist")
	}

	return m.RemoveGroup(s.cfg.Name)
}

func (m *GroupManager) GetGroupByUUID(uuid string) (*Group, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, v := range m.groups {
		if v.cfg.UUID == uuid {
			return v, true
		}
	}
	return nil, false
}
