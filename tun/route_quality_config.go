package tun

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// RouteQualityConfig 控制完整业务路径探测；字节数是未压缩响应正文长度。
type RouteQualityConfig struct {
	DownloadURL        string `json:"download_url"`
	ConnectivityURL    string `json:"connectivity_url"`
	DownloadStatus     int    `json:"download_status"`
	ConnectivityStatus int    `json:"connectivity_status"`
	DownloadBytes      int64  `json:"download_bytes"`
	IntervalSec        int    `json:"interval_sec"`
	TimeoutMS          int    `json:"timeout_ms"`
	NormalMS           int    `json:"normal_ms"`
}

func (c *RouteQualityConfig) normalize() {
	if c.DownloadURL == "" {
		c.DownloadURL = "https://speed.cloudflare.com/__down?bytes=16384"
	}
	if c.ConnectivityURL == "" {
		c.ConnectivityURL = "https://www.gstatic.com/generate_204"
	}
	if c.DownloadStatus == 0 {
		c.DownloadStatus = 200
	}
	if c.ConnectivityStatus == 0 {
		c.ConnectivityStatus = 204
	}
	if c.DownloadBytes == 0 {
		c.DownloadBytes = 16384
	}
	if c.IntervalSec == 0 {
		c.IntervalSec = 30
	}
	if c.TimeoutMS == 0 {
		c.TimeoutMS = 5000
	}
	if c.NormalMS == 0 {
		c.NormalMS = 2000
	}
}

func (c RouteQualityConfig) validate() error {
	for _, target := range []string{c.DownloadURL, c.ConnectivityURL} {
		u, err := url.Parse(target)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return errors.New("探测目标必须是有效的 HTTPS URL，不能包含账号或片段")
		}
		if u.Port() != "" {
			if _, err := net.LookupPort("tcp", u.Port()); err != nil {
				return errors.New("探测目标端口无效")
			}
		}
	}
	if c.DownloadStatus < 200 || c.DownloadStatus > 299 || c.ConnectivityStatus < 200 || c.ConnectivityStatus > 299 {
		return errors.New("探测预期状态码必须在 200 到 299 之间")
	}
	if c.DownloadBytes < 1 || c.DownloadBytes > 1024*1024 {
		return errors.New("探测下载字节数必须在 1 到 1048576 之间")
	}
	if c.IntervalSec < 10 || c.IntervalSec > 3600 {
		return errors.New("探测间隔必须在 10 到 3600 秒之间")
	}
	if c.TimeoutMS < 100 || c.TimeoutMS > 30000 || c.NormalMS < 1 || c.NormalMS > c.TimeoutMS {
		return errors.New("总超时必须在 100 到 30000 毫秒之间，正常时间必须大于零且不超过总超时")
	}
	return nil
}

// routeQualityProxy 只接受可直接协商的 SOCKS5 入口，并将通配监听转换为回环地址。
func routeQualityProxy(cfg Config) (string, InProtoSocks5X, error) {
	p, addr, err := parseProtocol(cfg.Input)
	if err != nil || p != Socks5X {
		return "", InProtoSocks5X{}, errors.New("质量策略要求 member 的 input 为 socks5x")
	}
	if cfg.InDecryptMode != "" && cfg.InDecryptMode != "none" {
		return "", InProtoSocks5X{}, errors.New("质量策略要求 member 的 input 不加密")
	}
	ext, err := parseExtend(cfg.InExtend)
	if err != nil {
		return "", InProtoSocks5X{}, fmt.Errorf("input 扩展配置无效: %w", err)
	}
	if ext.FrameHeaderEnable {
		return "", InProtoSocks5X{}, errors.New("质量策略要求 member 的 input 不启用帧头")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", InProtoSocks5X{}, err
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	var auth InProtoSocks5X
	if strings.TrimSpace(cfg.InProtoCfg) != "" {
		if err := json.Unmarshal([]byte(cfg.InProtoCfg), &auth); err != nil {
			return "", auth, fmt.Errorf("SOCKS5 认证配置无效: %w", err)
		}
	}
	return net.JoinHostPort(host, port), auth, nil
}
