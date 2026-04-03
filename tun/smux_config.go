package tun

import "github.com/xtaci/smux"

func newSmuxConfig(streamBuf int) (*smux.Config, error) {
	cfg := smux.DefaultConfig()
	if streamBuf > 0 {
		cfg.MaxReceiveBuffer = streamBuf
		cfg.MaxStreamBuffer = streamBuf
	}
	if err := smux.VerifyConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
