package tun

import (
	"github.com/0990/gotun/core"
	"sync"
	"time"
)

type streamMakerContainer struct {
	maker      core.IStreamMaker
	expireDate time.Time
	autoExpire int

	lock sync.RWMutex
}

func (w *streamMakerContainer) SetMaker(maker core.IStreamMaker, autoExpire int) {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.autoExpire = autoExpire
	if autoExpire > 0 {
		w.expireDate = time.Now().Add(time.Duration(autoExpire) * time.Second)
	}

	w.maker = maker
}

func (w *streamMakerContainer) GetMaker() (core.IStreamMaker, bool) {
	w.lock.RLock()
	defer w.lock.RUnlock()

	if w.maker.IsClosed() {
		return nil, false
	}

	if w.autoExpire > 0 && time.Now().After(w.expireDate) {
		return nil, false
	}

	return w.maker, true
}

func (w *streamMakerContainer) Close() error {
	return w.maker.Close()
}
