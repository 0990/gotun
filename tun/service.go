package tun

import "fmt"

type Service interface {
	Run() error
	Close() error
	Cfg() Config
	Status() string
	QualitySummary() QualitySummary
	QualityDetails() map[string]QualitySnapshot
	BandwidthSummary() BandwidthSummary
	BandwidthTest() (BandwidthSummary, error)
	Probe() bool
}

func NewService(cfg Config) (Service, error) {
	switch cfg.Mode {
	case "", "default":
		return NewServer(cfg)
	case "frpc":
		return NewFrpc(cfg)
	case "frps":
		return NewFrps(cfg)
	default:
		return nil, fmt.Errorf("invalid mode: %s", cfg.Mode)
	}
}

type disabledService struct {
	cfg Config
}

func NewDisabledService(cfg Config) Service {
	cfg.Disabled = true
	return &disabledService{cfg: cfg}
}

func (s *disabledService) Run() error {
	return nil
}

func (s *disabledService) Close() error {
	return nil
}

func (s *disabledService) Cfg() Config {
	return s.cfg
}

func (s *disabledService) Status() string {
	return "disabled"
}

func (s *disabledService) QualitySummary() QualitySummary {
	return QualitySummary{Status: QualityStatusDisabled, LastError: "tunnel disabled"}
}

func (s *disabledService) QualityDetails() map[string]QualitySnapshot {
	return map[string]QualitySnapshot{}
}

func (s *disabledService) BandwidthSummary() BandwidthSummary {
	return BandwidthSummary{Status: BandwidthStatusDisabled, LastError: "tunnel disabled"}
}

func (s *disabledService) BandwidthTest() (BandwidthSummary, error) {
	return s.BandwidthSummary(), nil
}

func (s *disabledService) Probe() bool {
	return false
}
