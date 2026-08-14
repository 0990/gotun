package model

// Route 智能路由入口（管理 API DTO）
type Route struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Disabled bool   `json:"disabled"`
	Mode     string `json:"mode"` // output|input
	Listen   string `json:"listen"`
	// InProtoCfg 出口模式的入口监听协议扩展配置（如 socks5x 认证/超时）；input 模式为空
	InProtoCfg string   `json:"in_proto_cfg"`
	Members    []string `json:"members"`
	Policy     string   `json:"policy"`

	// 运行态（仅 List 返回）
	Status          string            `json:"status"`           // running/init/... 或 disabled
	PreferredMember string            `json:"preferred_member"` // 当前首选 member
	MemberHealth    map[string]string `json:"member_health"`    // member 名称 -> 健康状态(up/degraded/down/disabled/unknown)
	QualitySummary  QualitySummary    `json:"quality_summary"`  // 入口整体健康（=首选 member）
	CreatedAt       string            `json:"created_at"`
	MemberNoHealth  []string          `json:"member_no_health"` // 无健康数据(未开帧头探测)的 member，供 UI 提示
}

// RouteSwitchEvent 一次首选 member 切换记录（管理 API DTO）
type RouteSwitchEvent struct {
	Time string `json:"time"`
	From string `json:"from"` // 原首选 member，空串表示此前无可用 member
	To   string `json:"to"`   // 新首选 member，空串表示全部 member 不可用
}
