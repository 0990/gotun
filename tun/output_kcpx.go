package tun

import (
	"context"
	"fmt"

	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/stats"
	kcpx "github.com/0990/kcpx-go"
)

type KCPXConn struct {
	*kcpx.Conn
}

func (c *KCPXConn) ID() string {
	return fmt.Sprintf("kcpx-%d", c.Conv())
}

func dialKCPX(addr string, config string, readCounter, writeCounter stats.Counter) (core.IStream, error) {
	cfg, err := parseKCPXConfig(config)
	if err != nil {
		return nil, err
	}

	conn, err := dialKCPXConn(context.Background(), addr, cfg)
	if err != nil {
		return nil, err
	}
	return &KCPXConn{Conn: conn}, nil
}

func dialKCPXConn(ctx context.Context, addr string, cfg KCPXConfig) (*kcpx.Conn, error) {
	_ = ctx

	conn, err := kcpx.DialWithOptions(addr, cfg.ToOptions())
	if err != nil {
		return nil, err
	}

	applyKCPXSessionConfig(conn.Session(), cfg)
	applyKCPXDialSocketConfig(conn.Session(), cfg)
	return conn, nil
}
