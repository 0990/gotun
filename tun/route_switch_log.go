package tun

import (
	"sync"
	"time"
)

// routeSwitchLogMax 每条智能路由最多保留的切换记录条数
const routeSwitchLogMax = 20

// RouteSwitchEvent 记录一次首选 member 切换（管理 API 只读）
type RouteSwitchEvent struct {
	Time time.Time `json:"time"`
	From string    `json:"from"` // 原首选 member，空串表示此前无可用 member（含启动时的初始选中）
	To   string    `json:"to"`   // 新首选 member，空串表示全部 member 不可用
}

// routeSwitchLog 首选 member 切换记录的环形缓冲（只保留最近 routeSwitchLogMax 条）
type routeSwitchLog struct {
	mu     sync.RWMutex
	events []RouteSwitchEvent
}

func (l *routeSwitchLog) record(from, to string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, RouteSwitchEvent{Time: time.Now(), From: from, To: to})
	if len(l.events) > routeSwitchLogMax {
		l.events = l.events[len(l.events)-routeSwitchLogMax:]
	}
}

// list 返回事件副本，最新一条在前
func (l *routeSwitchLog) list() []RouteSwitchEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]RouteSwitchEvent, 0, len(l.events))
	for i := len(l.events) - 1; i >= 0; i-- {
		out = append(out, l.events[i])
	}
	return out
}
