package tun

import (
	"sync"
	"time"
)

type FrameProbeRunner struct {
	channel *ProbeChannel
	tracker *QualityTracker
	cfg     Extend
	close   chan struct{}
	wake    chan struct{}

	lastProbe time.Time
	probeMu   sync.Mutex
}

func NewFrameProbeRunner(channel *ProbeChannel, tracker *QualityTracker, cfg Extend) *FrameProbeRunner {
	return &FrameProbeRunner{
		channel: channel,
		tracker: tracker,
		cfg:     cfg,
		close:   make(chan struct{}),
		wake:    make(chan struct{}, 1),
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
			r.maybeProbe(time.Now(), false)
		case <-r.wake:
			r.maybeProbe(time.Now(), true)
		case <-r.close:
			return
		}
	}
}

func (r *FrameProbeRunner) TriggerProbe() bool {
	select {
	case r.wake <- struct{}{}:
	default:
	}
	return true
}

func (r *FrameProbeRunner) maybeProbe(now time.Time, force bool) {
	r.probeMu.Lock()
	defer r.probeMu.Unlock()

	interval := time.Duration(r.cfg.ProbeIntervalSec) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}

	if !force && !r.lastProbe.IsZero() && now.Sub(r.lastProbe) < interval {
		return
	}

	r.lastProbe = now
	r.probeOnce()
}

func (r *FrameProbeRunner) probeOnce() {
	timeout := time.Duration(r.cfg.ProbeTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	rtt, err := r.channel.Probe(timeout)
	if err != nil {
		r.tracker.RecordProbeFailure(err)
		return
	}
	r.tracker.RecordProbeSuccess(rtt)
}
