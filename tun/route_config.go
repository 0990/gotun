package tun

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// ROUTE_CONFIG_SUFFIX 智能路由入口的持久化文件后缀（与 .tun 区分，互不影响）
const ROUTE_CONFIG_SUFFIX = ".route"

// 智能路由模式
const (
	RouteModeOutput = "output" // listen -> member 的 output（经 member 的转发能力发出）
	RouteModeInput  = "input"  // listen -> member 的 input（透明转发到 member 的入口监听地址）
)

// RouteConfig 智能路由入口配置
// Members 数组顺序即优先级，索引 0 最高
type RouteConfig struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Disabled bool   `json:"disabled"`
	Mode     string `json:"mode"`   // output|input
	Listen   string `json:"listen"` // output 模式: "proto@host:port"；input 模式: "host:port"
	// InProtoCfg 入口监听协议的扩展配置（如 socks5x 的认证/超时），仅 output 模式使用；
	// input 模式为裸 TCP 透明转发，无此配置
	InProtoCfg string    `json:"in_proto_cfg"`
	Members    []string  `json:"members"` // member tunnel 名称，按优先级排序
	Policy     string    `json:"policy"`  // 故障转移策略，v1 仅支持 "priority"
	CreatedAt  time.Time `json:"create_at"`
}

// normalize 填充默认值并规范化
func (r *RouteConfig) normalize() {
	if r.Policy == "" {
		r.Policy = "priority"
	}
	if r.Mode == "" {
		r.Mode = RouteModeOutput
	}
}

// validateBasic 做与 manager 无关的基础校验（名称/模式/监听/member/策略）
func (r *RouteConfig) validateBasic() error {
	r.normalize()

	if strings.TrimSpace(r.Name) == "" {
		return errors.New("名称不能为空")
	}
	if r.Mode != RouteModeOutput && r.Mode != RouteModeInput {
		return fmt.Errorf("未知模式:%s（仅支持 %s|%s）", r.Mode, RouteModeOutput, RouteModeInput)
	}
	if r.Policy != "priority" {
		return fmt.Errorf("未知策略:%s（v1 仅支持 priority）", r.Policy)
	}
	if len(r.Members) == 0 {
		return errors.New("member 不能为空")
	}
	for _, m := range r.Members {
		if strings.TrimSpace(m) == "" {
			return errors.New("member 名称不能为空")
		}
	}

	// 监听地址形态按模式校验
	if r.Mode == RouteModeOutput {
		// 需带协议，且协议可被 input 工厂解析
		if _, _, err := parseProtocol(r.Listen); err != nil {
			return fmt.Errorf("出口模式监听地址需带协议(如 tcp@0.0.0.0:1000):%v", err)
		}
	} else {
		// 入口模式为裸 host:port，且透明转发仅支持 TCP 目标，不允许带协议
		if strings.Contains(r.Listen, "@") {
			return errors.New("入口模式监听地址为裸 host:port，不要带协议(如 0.0.0.0:1000)")
		}
		if _, _, err := net.SplitHostPort(r.Listen); err != nil {
			return fmt.Errorf("入口模式监听地址需为 host:port:%v", err)
		}
	}
	return nil
}

// memberInputProto 返回某 member 的 input 协议（用于入口模式协议一致性校验）
func memberInputProto(svc Service) (protocol, error) {
	proto, _, err := parseProtocol(svc.Cfg().Input)
	return proto, err
}
