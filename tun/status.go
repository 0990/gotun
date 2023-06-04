package tun

type StatusX struct {
	status string
}

func (s *StatusX) SetStatus(status string) {
	s.status = status
}

func (s *StatusX) Status() string {
	return s.status
}
