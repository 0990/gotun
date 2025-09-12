package tun

import (
	"github.com/0990/gotun/core"
	"github.com/sirupsen/logrus"
	"sync"
	"sync/atomic"
	"time"
)

/*
streamMakerContainer 的一个线程安全实现（将其加入本文件或替换你已有的实现）。
主要功能：
- GetMaker() 返回当前 maker 是否可用
- SetMaker(maker, autoExpire) 设置 maker（会更新创建时间）
- TryStartCreate(fn) 如果没有创建任务在进行，就启动一次创建任务（返回 true 表示它启动了 goroutine）
- Close() 关闭当前 maker（如果有）
注意：我在实现中允许 SetMaker(nil, ...) 来表示创建失败 / 不可用场景；GetMaker 会在 maker 为 nil 时返回 ok=false。
*/

type streamMakerContainer struct {
	maker      core.IStreamMaker
	expireDate time.Time
	autoExpire int
	mu         sync.RWMutex

	created  time.Time
	creating int32
	closed   int32
}

func (s *streamMakerContainer) SetMaker(m core.IStreamMaker, autoExpire int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.maker != nil {
		_ = s.maker.Close()
	}

	s.maker = m
	s.autoExpire = autoExpire

	if m != nil {
		s.created = time.Now()
	} else {
		atomic.StoreInt32(&s.creating, 0)
	}
}

func (s *streamMakerContainer) GetMaker() (core.IStreamMaker, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.maker == nil {
		return nil, false
	}

	if s.maker.IsClosed() {
		logrus.Warningf("streamMakerContainer is closed")
		return nil, false
	}

	// 如果设置了 autoExpire，且已过期，则认为不可用（并让调用者触发重建）
	if s.autoExpire > 0 && time.Since(s.created) > time.Duration(s.autoExpire)*time.Second {
		return nil, false
	}

	return s.maker, true
}

// TryStartCreate 会尝试把 creating 从 0 -> 1；
// 如果成功，则在新的 goroutine 中执行 provided fn，并在 fn 返回后把 creating 置回 0（确保失败时也能重试）。
// 返回值表示：是否 **成功启动** 了 goroutine（即之前没有创建任务正在进行）。
func (s *streamMakerContainer) TryStartCreate(fn func()) bool {
	if atomic.LoadInt32(&s.closed) > 0 {
		return false
	}

	if !atomic.CompareAndSwapInt32(&s.creating, 0, 1) {
		// 已经有创建任务在进行
		return false
	}

	go func() {
		defer atomic.StoreInt32(&s.creating, 0)
		// 如果被 Close 掉了，就不执行创建逻辑
		if atomic.LoadInt32(&s.closed) > 0 {
			return
		}

		// 执行上层提供的创建逻辑（上层会调用 waitCreateStreamMaker 并最终调用 SetMaker）
		fn()
	}()
	return true
}

func (s *streamMakerContainer) Close() error {
	atomic.StoreInt32(&s.closed, 1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.maker != nil {
		err := s.maker.Close()
		s.maker = nil
		return err
	}
	return nil
}
