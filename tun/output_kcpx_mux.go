package tun

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/stats"
	"github.com/xtaci/smux"
)

func dialKCPXBuilder(ctx context.Context, addr string, config string, readCounter, writeCounter stats.Counter) (core.IStreamMaker, error) {
	cfg, err := parseKCPXConfig(config)
	if err != nil {
		return nil, err
	}

	session, err := dialKCPXConn(ctx, addr, cfg)
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
	return &KCPXsmuxSession{session: smuxSess}, nil
}

type KCPXsmuxSession struct {
	session *smux.Session
}

func (p *KCPXsmuxSession) OpenStream() (core.IStream, error) {
	stream, err := p.session.OpenStream()
	return &KCPXsmuxStream{Stream: stream}, err
}

func (p *KCPXsmuxSession) IsClosed() bool {
	return p.session.IsClosed()
}

func (p *KCPXsmuxSession) Close() error {
	return p.session.Close()
}

type KCPXsmuxStream struct {
	*smux.Stream
}

func (p *KCPXsmuxStream) ID() string {
	return fmt.Sprintf("kcpxsmuxstream-%d", p.Stream.ID())
}

func (p *KCPXsmuxStream) SetReadDeadline(t time.Time) error {
	return p.Stream.SetReadDeadline(t)
}
