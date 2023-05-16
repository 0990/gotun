package tun

import (
	"github.com/xtaci/kcp-go/v5"
	"log"
)

type KCPSession struct {
	*kcp.UDPSession
}

func (c *KCPSession) ID() int64 {
	return int64(1)
}

func dialKCP(config KCPConfig) func(addr string) (Stream, error) {
	return func(addr string) (Stream, error) {
		session, err := dialKCPConn(addr, config)
		if err != nil {
			return nil, err
		}
		return &KCPSession{UDPSession: session}, nil
	}
}

func dialKCPConn(addr string, config KCPConfig) (*kcp.UDPSession, error) {
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
