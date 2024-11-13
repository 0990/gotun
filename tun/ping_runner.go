package tun

import (
	"errors"
	"strings"
	"sync"
	"time"
)

type PingRunner struct {
	ping IPing
	cfg  PingConfig
	sync.RWMutex

	close chan struct{}

	pingMS             int
	pingError          error
	pingErrorStartTime time.Time
}

func newPingRunner(cfg PingConfig) (*PingRunner, error) {
	ss := strings.Split(cfg.Addr, "@")
	if len(ss) != 2 {
		return nil, errors.New("invalid addr")
	}
	ping, err := NewPing(ss[0], ss[1])
	if err != nil {
		return nil, err
	}
	return &PingRunner{
		cfg:    cfg,
		ping:   ping,
		pingMS: -1,
		close:  make(chan struct{}, 1),
	}, nil
}

func (s *PingRunner) Run() {
	go s.run()
}
func (s *PingRunner) run() {
	ticker := time.NewTicker(time.Second * time.Duration(s.cfg.Interval))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.Ping()
		case <-s.close:
			return
		}
	}
}

func (s *PingRunner) Ping() {
	ping, err := s.ping.Ping(10, time.Second)
	s.Lock()
	defer s.Unlock()
	if err != nil {
		s.pingError = err
		s.pingMS = -1
		s.pingErrorStartTime = time.Now()
	} else {
		s.pingError = nil
		s.pingMS = ping
		s.pingErrorStartTime = time.Time{}
	}
}

func (s *PingRunner) Close() {
	close(s.close)
}

func (s *PingRunner) GetPing() (int, error) {
	s.RLock()
	defer s.RUnlock()
	return s.pingMS, s.pingError
}

func (s *PingRunner) GetPingErrorTime() time.Time {
	s.RLock()
	defer s.RUnlock()
	return s.pingErrorStartTime
}
