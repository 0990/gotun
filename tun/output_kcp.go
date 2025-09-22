package tun

import (
	"context"
	"encoding/json"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/stats"
	"github.com/xtaci/kcp-go/v5"
	"log"
)

type KCPSession struct {
	*kcp.UDPSession
}

func (c *KCPSession) ID() string {
	return "kcpsession"
}

func dialKCP(addr string, config string, readCounter, writeCounter stats.Counter) (core.IStream, error) {
	var cfg KCPConfig
	if config != "" {
		err := json.Unmarshal([]byte(config), &cfg)
		if err != nil {
			return nil, err
		}
	} else {
		cfg = defaultKCPConfig
	}

	session, err := dialKCPConn(context.Background(), addr, cfg)
	if err != nil {
		return nil, err
	}
	return &KCPSession{UDPSession: session}, nil
}

func dialKCPConn(ctx context.Context, addr string, config KCPConfig) (*kcp.UDPSession, error) {
	kcpConn, err := kcp.DialWithOptions(addr, nil, config.DataShard, config.ParityShard)
	if err != nil {
		return nil, err
	}
	kcpConn.SetStreamMode(false)
	kcpConn.SetWriteDelay(config.WriteDelay)
	kcpConn.SetMtu(config.MTU)
	kcpConn.SetACKNoDelay(config.AckNodelay)
	kcpConn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
	kcpConn.SetWindowSize(config.SndWnd, config.RcvWnd)

	if err := kcpConn.SetDSCP(config.DSCP); err != nil {
		log.Println("SetDSCP:", err)
	}
	if err := kcpConn.SetReadBuffer(config.SockBuf); err != nil {
		log.Println("SetReadBuffer:", err)
	}
	if err := kcpConn.SetWriteBuffer(config.SockBuf); err != nil {
		log.Println("SetWriteBuffer:", err)
	}

	return kcpConn, nil
}
