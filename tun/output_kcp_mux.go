package tun

import (
	"github.com/xtaci/smux"
	"log"
	"time"
)

func dialKCPBuilder(config KCPConfig) func(addr string) (StreamMaker, error) {
	return func(addr string) (StreamMaker, error) {
		session, err := dialKCPConn(addr, config)
		if err != nil {
			return nil, err
		}

		smuxConfig := smux.DefaultConfig()
		smuxConfig.MaxReceiveBuffer = config.StreamBuf

		if err := smux.VerifyConfig(smuxConfig); err != nil {
			log.Fatalf("%+v", err)
		}

		smuxSess, err := smux.Client(session, smuxConfig)
		if err != nil {
			return nil, err
		}
		return &KCPsmuxSession{session: smuxSess}, nil
	}
}

type KCPsmuxSession struct {
	session *smux.Session
}

func (p *KCPsmuxSession) OpenStream() (Stream, error) {
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

func (p *KCPsmuxStream) ID() int64 {
	return int64(p.Stream.ID())
}

func (p *KCPsmuxStream) SetReadDeadline(t time.Time) error {
	return p.Stream.SetReadDeadline(t)
}
