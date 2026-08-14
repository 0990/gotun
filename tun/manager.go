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
	routes   map[string]*routeService
	lock     sync.RWMutex
	tunDir   string
}

func NewManager(tunDir string) *Manager {
	return &Manager{
		services: make(map[string]Service),
		routes:   make(map[string]*routeService),
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

	// 加载并启动智能路由入口（在 tunnel 之后，保证 member 已注册）
	routeCfgs, err := loadAllRouteFile(m.tunDir)
	if err != nil {
		return err
	}
	for _, v := range routeCfgs {
		if err := m.AddRoute(v, false); err != nil {
			// 单个入口加载失败不阻断整体启动（如 member 尚未就绪），仅记录
			logrus.WithError(err).WithField("route", v.Name).Error("load route failed")
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

// ---------------- 智能路由入口管理 ----------------

// validateRouteLocked 校验路由配置（含 member 存在性、协议一致性、监听冲突、名称唯一）
// excludeUUID 用于编辑时排除自身
func (m *Manager) validateRouteLocked(cfg *RouteConfig, excludeUUID string) error {
	if err := cfg.validateBasic(); err != nil {
		return err
	}

	// 名称唯一：不能与已有 route（除自身）或 tunnel 重名
	for name, r := range m.routes {
		if name == cfg.Name && r.cfg.UUID != excludeUUID {
			return errors.New("name already exist（已存在同名智能路由）")
		}
	}
	if _, ok := m.services[cfg.Name]; ok {
		return errors.New("name already exist（已存在同名 tunnel，避免混淆请改名）")
	}

	// member 必须存在
	for _, member := range cfg.Members {
		if _, ok := m.services[member]; !ok {
			return fmt.Errorf("member 不存在:%s", member)
		}
	}

	// 入口模式：所有 member 的 input 协议需一致
	if cfg.Mode == RouteModeInput {
		var want protocol = -1
		for i, member := range cfg.Members {
			svc := m.services[member]
			p, err := memberInputProto(svc)
			if err != nil {
				return fmt.Errorf("member %s 的 input 无法解析:%v", member, err)
			}
			if i == 0 {
				want = p
				continue
			}
			if p != want {
				return fmt.Errorf("入口模式要求所有 member 的 input 协议一致:%s 为 %s，与首个 member 的 %s 不一致", member, p.String(), want.String())
			}
		}
	}

	// 监听地址冲突：与现有 route 的监听比对
	for name, r := range m.routes {
		if r.cfg.UUID == excludeUUID {
			continue
		}
		if r.cfg.Listen == cfg.Listen {
			return fmt.Errorf("监听地址已被智能路由 %s 占用:%s", name, cfg.Listen)
		}
	}
	return nil
}

// AddRoute 新增智能路由入口
func (m *Manager) AddRoute(cfg RouteConfig, createFile bool) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if cfg.UUID == "" {
		cfg.UUID = util.NewUUID()
	}
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = time.Now()
	}

	if err := m.validateRouteLocked(&cfg, ""); err != nil {
		return err
	}

	svc, err := m.buildRouteServiceLocked(cfg)
	if err != nil {
		return err
	}
	if err := runPreparedService(svc); err != nil {
		return err
	}

	if createFile {
		if err := createRouteFile(m.tunDir, cfg); err != nil {
			_ = svc.Close()
			return err
		}
	}

	m.routes[cfg.Name] = svc
	return nil
}

// buildRouteServiceLocked 按配置构造 routeService（禁用时也不监听，Run 为空操作）
func (m *Manager) buildRouteServiceLocked(cfg RouteConfig) (*routeService, error) {
	rs, err := newRouteService(cfg, m)
	if err != nil {
		return nil, err
	}
	return rs, nil
}

// ReplaceRouteByUUID 按 UUID 替换智能路由入口（带回滚）
func (m *Manager) ReplaceRouteByUUID(cfg RouteConfig) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	oldSvc, oldName, ok := m.getRouteByUUIDLocked(cfg.UUID)
	if !ok {
		return errors.New("uuid not exist")
	}
	oldCfg := oldSvc.cfg

	cfg.UUID = oldCfg.UUID
	cfg.CreatedAt = oldCfg.CreatedAt

	if err := m.validateRouteLocked(&cfg, cfg.UUID); err != nil {
		return err
	}

	newSvc, err := m.buildRouteServiceLocked(cfg)
	if err != nil {
		return err
	}

	if err := oldSvc.Close(); err != nil {
		return err
	}
	if err := runPreparedService(newSvc); err != nil {
		return m.rollbackRouteLocked(oldName, oldCfg, err)
	}

	if err := replaceRouteFile(m.tunDir, oldCfg, cfg); err != nil {
		_ = newSvc.Close()
		return m.rollbackRouteLocked(oldName, oldCfg, err)
	}

	delete(m.routes, oldName)
	m.routes[cfg.Name] = newSvc
	return nil
}

// SetRouteDisabled 启用/禁用智能路由入口
func (m *Manager) SetRouteDisabled(name string, disabled bool) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	current, ok := m.routes[name]
	if !ok {
		return errors.New("route not exist")
	}

	oldCfg := current.cfg
	if oldCfg.Disabled == disabled {
		return nil
	}

	newCfg := oldCfg
	newCfg.Disabled = disabled

	if err := m.validateRouteLocked(&newCfg, newCfg.UUID); err != nil {
		return err
	}

	newSvc, err := m.buildRouteServiceLocked(newCfg)
	if err != nil {
		return err
	}

	if err := current.Close(); err != nil {
		return err
	}
	if err := runPreparedService(newSvc); err != nil {
		return m.rollbackRouteLocked(name, oldCfg, err)
	}

	if err := replaceRouteFile(m.tunDir, oldCfg, newCfg); err != nil {
		_ = newSvc.Close()
		return m.rollbackRouteLocked(name, oldCfg, err)
	}

	m.routes[name] = newSvc
	return nil
}

// RemoveRoute 删除智能路由入口
func (m *Manager) RemoveRoute(name string) error {
	if err := deleteRouteFile(m.tunDir, name); err != nil {
		return err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	svc, ok := m.routes[name]
	if !ok {
		return nil
	}
	if err := svc.Close(); err != nil {
		return err
	}
	delete(m.routes, name)
	return nil
}

// RemoveRouteByUUID 按 UUID 删除智能路由入口
func (m *Manager) RemoveRouteByUUID(uuid string) error {
	svc, ok := m.GetRouteByUUID(uuid)
	if !ok {
		return errors.New("uuid not exist")
	}
	return m.RemoveRoute(svc.cfg.Name)
}

// GetRoute 按名称取智能路由入口
func (m *Manager) GetRoute(name string) (*routeService, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	svc, ok := m.routes[name]
	return svc, ok
}

// GetRouteByUUID 按 UUID 取智能路由入口
func (m *Manager) GetRouteByUUID(uuid string) (*routeService, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	for _, v := range m.routes {
		if v.cfg.UUID == uuid {
			return v, true
		}
	}
	return nil, false
}

func (m *Manager) getRouteByUUIDLocked(uuid string) (*routeService, string, bool) {
	for name, v := range m.routes {
		if v.cfg.UUID == uuid {
			return v, name, true
		}
	}
	return nil, "", false
}

// RouteService 是智能路由入口对外暴露的只读视图（供管理 API 使用）
type RouteService interface {
	Service
	RouteCfg() RouteConfig
	PreferredMember() string
	MemberHealth() map[string]string
	SwitchEvents() []RouteSwitchEvent
}

// AllRoute 返回全部智能路由入口（按模式过滤，mode 为空则全部）
func (m *Manager) AllRoute(mode string) []RouteService {
	m.lock.RLock()
	defer m.lock.RUnlock()

	out := []RouteService{}
	for _, v := range m.routes {
		if mode == "" || v.cfg.Mode == mode {
			out = append(out, v)
		}
	}
	return out
}

// rollbackRouteLocked 路由替换失败时回滚到旧配置
func (m *Manager) rollbackRouteLocked(oldName string, oldCfg RouteConfig, cause error) error {
	oldSvc, rollbackErr := m.buildRouteServiceLocked(oldCfg)
	if rollbackErr != nil {
		delete(m.routes, oldName)
		return fmt.Errorf("%w; rollback failed: %v", cause, rollbackErr)
	}
	if rollbackErr := runPreparedService(oldSvc); rollbackErr != nil {
		delete(m.routes, oldName)
		return fmt.Errorf("%w; rollback failed: %v", cause, rollbackErr)
	}
	m.routes[oldName] = oldSvc
	return cause
}
