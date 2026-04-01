package tun

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"time"

	kcp "github.com/0990/kcp-go"
	kcpx "github.com/0990/kcpx-go"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type KCPXConfig struct {
	WriteDelay   bool `json:"write_delay"`
	MTU          int  `json:"mtu"`
	SndWnd       int  `json:"sndwnd"`
	RcvWnd       int  `json:"rcvwnd"`
	DataShard    int  `json:"datashard"`
	ParityShard  int  `json:"parityshard"`
	DSCP         int  `json:"dscp"`
	AckNodelay   bool `json:"acknodelay"`
	NoDelay      int  `json:"nodelay"`
	Interval     int  `json:"interval"`
	Resend       int  `json:"resend"`
	NoCongestion int  `json:"nc"`
	SockBuf      int  `json:"sockbuf"`
	StreamBuf    int  `json:"streambuf"`

	HandshakeTimeoutMS       int  `json:"handshake_timeout_ms"`
	HandshakeRetryIntervalMS int  `json:"handshake_retry_interval_ms"`
	HandshakeMaxRetries      int  `json:"handshake_max_retries"`
	HeartbeatIntervalMS      int  `json:"heartbeat_interval_ms"`
	HeartbeatTimeoutMS       int  `json:"heartbeat_timeout_ms"`
	ReconnectRetryIntervalMS int  `json:"reconnect_retry_interval_ms"`
	ReconnectMaxRetries      int  `json:"reconnect_max_retries"`
	EnableAutoReconnect      bool `json:"enable_auto_reconnect"`
}

var defaultKCPXConfig = func() KCPXConfig {
	opts := kcpx.DefaultOptions()
	return KCPXConfig{
		WriteDelay:               defaultKCPConfig.WriteDelay,
		MTU:                      defaultKCPConfig.MTU,
		SndWnd:                   defaultKCPConfig.SndWnd,
		RcvWnd:                   defaultKCPConfig.RcvWnd,
		DataShard:                defaultKCPConfig.DataShard,
		ParityShard:              defaultKCPConfig.ParityShard,
		DSCP:                     defaultKCPConfig.DSCP,
		AckNodelay:               defaultKCPConfig.AckNodelay,
		NoDelay:                  defaultKCPConfig.NoDelay,
		Interval:                 defaultKCPConfig.Interval,
		Resend:                   defaultKCPConfig.Resend,
		NoCongestion:             defaultKCPConfig.NoCongestion,
		SockBuf:                  defaultKCPConfig.SockBuf,
		StreamBuf:                defaultKCPConfig.StreamBuf,
		HandshakeTimeoutMS:       int(opts.HandshakeTimeout / time.Millisecond),
		HandshakeRetryIntervalMS: int(opts.HandshakeRetryInterval / time.Millisecond),
		HandshakeMaxRetries:      opts.HandshakeMaxRetries,
		HeartbeatIntervalMS:      int(opts.HeartbeatInterval / time.Millisecond),
		HeartbeatTimeoutMS:       int(opts.HeartbeatTimeout / time.Millisecond),
		ReconnectRetryIntervalMS: int(opts.ReconnectRetryInterval / time.Millisecond),
		ReconnectMaxRetries:      opts.ReconnectMaxRetries,
		EnableAutoReconnect:      opts.EnableAutoReconnect,
	}
}()

func parseKCPXConfig(extra string) (KCPXConfig, error) {
	cfg := defaultKCPXConfig
	if extra == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(extra), &cfg); err != nil {
		return KCPXConfig{}, err
	}
	return cfg, nil
}

func (c KCPXConfig) ToOptions() kcpx.Options {
	return kcpx.Options{
		DataShards:             c.DataShard,
		ParityShards:           c.ParityShard,
		HandshakeTimeout:       time.Duration(c.HandshakeTimeoutMS) * time.Millisecond,
		HandshakeRetryInterval: time.Duration(c.HandshakeRetryIntervalMS) * time.Millisecond,
		HandshakeMaxRetries:    c.HandshakeMaxRetries,
		HeartbeatInterval:      time.Duration(c.HeartbeatIntervalMS) * time.Millisecond,
		HeartbeatTimeout:       time.Duration(c.HeartbeatTimeoutMS) * time.Millisecond,
		ReconnectRetryInterval: time.Duration(c.ReconnectRetryIntervalMS) * time.Millisecond,
		ReconnectMaxRetries:    c.ReconnectMaxRetries,
		EnableAutoReconnect:    c.EnableAutoReconnect,
	}
}

func applyKCPXSessionConfig(session *kcp.UDPSession, cfg KCPXConfig) {
	if session == nil {
		return
	}
	session.SetStreamMode(true)
	session.SetWriteDelay(cfg.WriteDelay)
	session.SetMtu(cfg.MTU)
	session.SetACKNoDelay(cfg.AckNodelay)
	session.SetNoDelay(cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NoCongestion)
	session.SetWindowSize(cfg.SndWnd, cfg.RcvWnd)
}

func applyKCPXDialSocketConfig(session *kcp.UDPSession, cfg KCPXConfig) {
	if session == nil {
		return
	}
	if err := session.SetDSCP(cfg.DSCP); err != nil {
		log.Println("SetDSCP:", err)
	}
	if err := session.SetReadBuffer(cfg.SockBuf); err != nil {
		log.Println("SetReadBuffer:", err)
	}
	if err := session.SetWriteBuffer(cfg.SockBuf); err != nil {
		log.Println("SetWriteBuffer:", err)
	}
}

func configureKCPXListenSocket(conn *net.UDPConn, cfg KCPXConfig) {
	if conn == nil {
		return
	}
	if err := conn.SetReadBuffer(cfg.SockBuf); err != nil {
		log.Println("SetReadBuffer:", err)
	}
	if err := conn.SetWriteBuffer(cfg.SockBuf); err != nil {
		log.Println("SetWriteBuffer:", err)
	}
	if err := setSocketDSCP(conn, cfg.DSCP); err != nil {
		log.Println("SetDSCP:", err)
	}
}

func setSocketDSCP(conn net.Conn, dscp int) error {
	if conn == nil {
		return errors.New("nil conn")
	}

	var succeed bool
	if err := ipv4.NewConn(conn).SetTOS(dscp << 2); err == nil {
		succeed = true
	}
	if err := ipv6.NewConn(conn).SetTrafficClass(dscp); err == nil {
		succeed = true
	}
	if succeed {
		return nil
	}
	return errors.New("set dscp failed")
}
