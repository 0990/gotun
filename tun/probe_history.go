package tun

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/0990/gotun/pkg/probehistory"
	"github.com/sirupsen/logrus"
)

// OpenProbeHistory 只能在启动隧道之前调用，后续重建实例共用同一个历史库。
func (m *Manager) OpenProbeHistory() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.history != nil {
		return nil
	}
	if len(m.services) > 0 || len(m.routes) > 0 {
		return errors.New("探测历史需要在启动隧道前初始化")
	}
	history, err := probehistory.Open(filepath.Join(m.tunDir, "probe-history.db"))
	if err != nil {
		return err
	}
	m.history = history
	return nil
}

func (m *Manager) CloseProbeHistory() error {
	if m.history == nil {
		return nil
	}
	return m.history.Close()
}

func (m *Manager) QueryProbeHistory(ctx context.Context, filter probehistory.Filter) (probehistory.Page, error) {
	if m.history == nil {
		return probehistory.Page{}, errors.New("探测历史尚未启用")
	}
	return m.history.Query(ctx, filter)
}

func (m *Manager) recordProbeHistory(record probehistory.Record) {
	if m.history == nil {
		return
	}
	if err := m.history.Record(record); err != nil {
		logrus.WithError(err).WithField("tunnel_uuid", record.TunnelUUID).Error("记录探测历史失败")
	}
}

func (m *Manager) attachProbeHistory(s Service) {
	if m.history == nil {
		return
	}
	var out output
	switch svc := s.(type) {
	case *Server:
		out = svc.output
	case *Frpc:
		out = svc.output
	default:
		return
	}
	cfg := s.Cfg()
	outputTracker(out).recordProbe = func(at time.Time, success bool, ms int64, message string) {
		status := "normal"
		if !success {
			status = "failed"
		}
		m.recordProbeHistory(probehistory.Record{TunnelUUID: cfg.UUID, Time: at, Kind: "frame", Status: status, DurationMS: ms, Error: message})
	}
}
