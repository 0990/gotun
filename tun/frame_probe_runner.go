package tun

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type FrameStreamRegistry struct {
	mu      sync.RWMutex
	streams []*FrameStream
	next    int
}

func (r *FrameStreamRegistry) Add(stream *FrameStream) {
	if r == nil || stream == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streams = append(r.streams, stream)
}

func (r *FrameStreamRegistry) Remove(stream *FrameStream) {
	if r == nil || stream == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.streams {
		if s != stream {
			continue
		}
		r.streams = append(r.streams[:i], r.streams[i+1:]...)
		if r.next >= len(r.streams) {
			r.next = 0
		}
		return
	}
}

func (r *FrameStreamRegistry) Probe(timeout time.Duration) (time.Duration, error) {
	stream := r.Next()
	if stream == nil {
		return 0, errors.New("no active frame stream")
	}
	return stream.Probe(timeout)
}

func (r *FrameStreamRegistry) Next() *FrameStream {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.streams) == 0 {
		return nil
	}
	if r.next >= len(r.streams) {
		r.next = 0
	}
	stream := r.streams[r.next]
	r.next++
	if r.next >= len(r.streams) {
		r.next = 0
	}
	return stream
}

type FrameProbeRunner struct {
	registry *FrameStreamRegistry
	tracker  *QualityTracker
	cfg      Extend
	close    chan struct{}
	wake     chan struct{}

	quickUntil atomic.Int64
	lastProbe  atomic.Int64
	probeMu    sync.Mutex
}

func NewFrameProbeRunner(registry *FrameStreamRegistry, tracker *QualityTracker, cfg Extend) *FrameProbeRunner {
	return &FrameProbeRunner{
		registry: registry,
		tracker:  tracker,
		cfg:      cfg,
		close:    make(chan struct{}),
		wake:     make(chan struct{}, 1),
	}
}

func (r *FrameProbeRunner) Run() {
	go r.loop()
}

func (r *FrameProbeRunner) Close() {
	select {
	case <-r.close:
		return
	default:
		close(r.close)
	}
}

func (r *FrameProbeRunner) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.maybeProbe(time.Now())
		case <-r.wake:
			r.maybeProbe(time.Now())
		case <-r.close:
			return
		}
	}
}

func (r *FrameProbeRunner) TriggerQuickProbe(duration time.Duration) bool {
	if duration <= 0 {
		return false
	}
	r.quickUntil.Store(time.Now().Add(duration).UnixNano())
	select {
	case r.wake <- struct{}{}:
	default:
	}
	return true
}

func (r *FrameProbeRunner) maybeProbe(now time.Time) {
	r.probeMu.Lock()
	defer r.probeMu.Unlock()

	interval := time.Duration(r.cfg.ProbeIntervalSec) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}

	quickUntil := time.Unix(0, r.quickUntil.Load())
	if now.Before(quickUntil) {
		interval = time.Second
	}

	last := time.Unix(0, r.lastProbe.Load())
	if !last.IsZero() && now.Sub(last) < interval {
		return
	}

	r.lastProbe.Store(now.UnixNano())
	r.probeOnce()
}

func (r *FrameProbeRunner) probeOnce() {
	timeout := time.Duration(r.cfg.ProbeTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	rtt, err := r.registry.Probe(timeout)
	if err != nil {
		r.tracker.RecordProbeFailure(err)
		return
	}
	r.tracker.RecordProbeSuccess(rtt)
}
