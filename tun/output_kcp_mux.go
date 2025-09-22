package tun

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/stats"
	"github.com/xtaci/smux"
	"log"
	"time"
)

func dialKCPBuilder(ctx context.Context, addr string, config string, readCounter, writeCounter stats.Counter) (core.IStreamMaker, error) {
	var cfg KCPConfig
	if config != "" {
		err := json.Unmarshal([]byte(config), &cfg)
		if err != nil {
			return nil, err
		}
	} else {
		cfg = defaultKCPConfig
	}

	session, err := dialKCPConn(ctx, addr, cfg)
	if err != nil {
		return nil, err
	}

	smuxConfig := smux.DefaultConfig()
	smuxConfig.MaxReceiveBuffer = cfg.StreamBuf

	if err := smux.VerifyConfig(smuxConfig); err != nil {
		log.Fatalf("%+v", err)
	}

	smuxSess, err := smux.Client(session, smuxConfig)
	if err != nil {
		return nil, err
	}
	return &KCPsmuxSession{session: smuxSess}, nil
}

type KCPsmuxSession struct {
	session *smux.Session
}

func (p *KCPsmuxSession) OpenStream() (core.IStream, error) {
	steam, err := p.session.OpenStream()
	return &KCPsmuxStream{Stream: steam}, err
}

func (p *KCPsmuxSession) IsClosed() bool {
	return p.session.IsClosed()
}

func (p *KCPsmuxSession) Close() error {
	return p.session.Close()
}

type KCPsmuxStream struct {
	*smux.Stream
}

func (p *KCPsmuxStream) ID() string {
	return fmt.Sprintf("kcpsmuxstream-%d", p.Stream.ID())
}

func (p *KCPsmuxStream) SetReadDeadline(t time.Time) error {
	return p.Stream.SetReadDeadline(t)
}
