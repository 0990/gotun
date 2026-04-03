package tun

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0990/gotun/pkg/stats"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	probeRTTGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gotun_probe_rtt_ms",
		Help: "Average probe RTT in milliseconds.",
	}, []string{"service", "output"})

	probeLossGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gotun_probe_loss_ratio",
		Help: "Probe loss ratio in the rolling window.",
	}, []string{"service", "output"})

	probeJitterGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gotun_probe_jitter_ms",
		Help: "Average probe jitter in milliseconds.",
	}, []string{"service", "output"})

	probeStatusGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gotun_probe_status",
		Help: "Probe status: up=1, degraded=0.5, down=0, disabled=-1.",
	}, []string{"service", "output"})

	probeSuccessCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gotun_probe_success_total",
		Help: "Total successful probe count.",
	}, []string{"service", "output"})

	probeFailureCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gotun_probe_failure_total",
		Help: "Total failed probe count.",
	}, []string{"service", "output"})
)

const (
	QualityStatusDisabled = "disabled"
	QualityStatusUp       = "up"
	QualityStatusDegraded = "degraded"
	QualityStatusDown     = "down"
)

type QualitySummary struct {
	Status    string  `json:"status"`
	RTTMs     int64   `json:"rtt_ms"`
	LossPct   float64 `json:"loss_pct"`
	LastError string  `json:"last_error"`
}

type QualitySnapshot struct {
	Status              string    `json:"status"`
	RTTAvgMs            int64     `json:"rtt_avg_ms"`
	LossRatio           float64   `json:"loss_ratio"`
	LossPct             float64   `json:"loss_pct"`
	JitterMs            int64     `json:"jitter_ms"`
	LastOKAt            time.Time `json:"last_ok_at"`
	LastError           string    `json:"last_error"`
	ConsecutiveFailures int64     `json:"consecutive_failures"`
	ProbeSuccessTotal   int64     `json:"probe_success_total"`
	ProbeFailureTotal   int64     `json:"probe_failure_total"`
	OpenSuccessTotal    int64     `json:"open_success_total"`
	OpenFailureTotal    int64     `json:"open_failure_total"`
	ActiveStreams       int64     `json:"active_streams"`
	BytesUplinkTotal    int64     `json:"bytes_uplink_total"`
	BytesDownlinkTotal  int64     `json:"bytes_downlink_total"`
}

type probeSample struct {
	success bool
	rttMS   int64
}

type QualityTracker struct {
	service string
	output  string

	readCounter  stats.Counter
	writeCounter stats.Counter

	enabled    bool
	windowSize int

	openSuccessTotal atomic.Int64
	openFailureTotal atomic.Int64
	activeStreams    atomic.Int64

	probeSuccessTotal   atomic.Int64
	probeFailureTotal   atomic.Int64
	consecutiveFailures atomic.Int64

	mu        sync.RWMutex
	samples   []probeSample
	lastOKAt  time.Time
	lastError string
}

func NewQualityTracker(service, output string, readCounter, writeCounter stats.Counter, enabled bool, windowSize int) *QualityTracker {
	if windowSize <= 0 {
		windowSize = 20
	}
	return &QualityTracker{
		service:      service,
		output:       output,
		readCounter:  readCounter,
		writeCounter: writeCounter,
		enabled:      enabled,
		windowSize:   windowSize,
	}
}

func (q *QualityTracker) Enabled() bool {
	return q != nil && q.enabled
}

func (q *QualityTracker) RecordOpenSuccess() {
	if q == nil {
		return
	}
	q.openSuccessTotal.Add(1)
}

func (q *QualityTracker) RecordOpenFailure() {
	if q == nil {
		return
	}
	q.openFailureTotal.Add(1)
}

func (q *QualityTracker) RecordStreamOpen() {
	if q == nil {
		return
	}
	q.activeStreams.Add(1)
}

func (q *QualityTracker) RecordStreamClose() {
	if q == nil {
		return
	}
	q.activeStreams.Add(-1)
}

func (q *QualityTracker) RecordProbeSuccess(rtt time.Duration) {
	if q == nil {
		return
	}
	probeSuccessCounter.WithLabelValues(q.service, q.output).Inc()
	q.probeSuccessTotal.Add(1)
	q.consecutiveFailures.Store(0)

	q.mu.Lock()
	defer q.mu.Unlock()
	q.lastOKAt = time.Now()
	q.lastError = ""
	ms := rtt.Milliseconds()
	q.appendSample(probeSample{success: true, rttMS: ms})
	q.updatePrometheusLocked()
}

func (q *QualityTracker) RecordProbeFailure(err error) {
	if q == nil {
		return
	}
	probeFailureCounter.WithLabelValues(q.service, q.output).Inc()
	q.probeFailureTotal.Add(1)
	q.consecutiveFailures.Add(1)

	q.mu.Lock()
	defer q.mu.Unlock()
	if err != nil {
		q.lastError = err.Error()
	}
	q.appendSample(probeSample{success: false})
	q.updatePrometheusLocked()
}

func (q *QualityTracker) appendSample(sample probeSample) {
	q.samples = append(q.samples, sample)
	if len(q.samples) > q.windowSize {
		q.samples = q.samples[len(q.samples)-q.windowSize:]
	}
}

func (q *QualityTracker) Summary() QualitySummary {
	snap := q.Snapshot()
	return QualitySummary{
		Status:    snap.Status,
		RTTMs:     snap.RTTAvgMs,
		LossPct:   snap.LossPct,
		LastError: snap.LastError,
	}
}

func (q *QualityTracker) Snapshot() QualitySnapshot {
	if q == nil {
		return QualitySnapshot{Status: QualityStatusDisabled}
	}

	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.snapshotLocked()
}

func (q *QualityTracker) calcStatusLocked() string {
	if !q.enabled {
		return QualityStatusDisabled
	}
	if len(q.samples) == 0 {
		return QualityStatusDown
	}

	var hasSuccess bool
	var hasFailure bool
	for _, sample := range q.samples {
		if sample.success {
			hasSuccess = true
			continue
		}
		hasFailure = true
	}
	if !hasSuccess {
		return QualityStatusDown
	}
	if hasFailure || q.consecutiveFailures.Load() > 0 {
		return QualityStatusDegraded
	}
	return QualityStatusUp
}

func (q *QualityTracker) updatePrometheusLocked() {
	snap := q.snapshotLocked()
	probeRTTGauge.WithLabelValues(q.service, q.output).Set(float64(snap.RTTAvgMs))
	probeLossGauge.WithLabelValues(q.service, q.output).Set(snap.LossRatio)
	probeJitterGauge.WithLabelValues(q.service, q.output).Set(float64(snap.JitterMs))

	statusValue := -1.0
	switch snap.Status {
	case QualityStatusUp:
		statusValue = 1
	case QualityStatusDegraded:
		statusValue = 0.5
	case QualityStatusDown:
		statusValue = 0
	}
	probeStatusGauge.WithLabelValues(q.service, q.output).Set(statusValue)
}

func (q *QualityTracker) snapshotLocked() QualitySnapshot {
	snap := QualitySnapshot{
		Status:              q.calcStatusLocked(),
		LastOKAt:            q.lastOKAt,
		LastError:           q.lastError,
		ConsecutiveFailures: q.consecutiveFailures.Load(),
		ProbeSuccessTotal:   q.probeSuccessTotal.Load(),
		ProbeFailureTotal:   q.probeFailureTotal.Load(),
		OpenSuccessTotal:    q.openSuccessTotal.Load(),
		OpenFailureTotal:    q.openFailureTotal.Load(),
		ActiveStreams:       q.activeStreams.Load(),
	}
	if q.readCounter != nil {
		snap.BytesDownlinkTotal = q.readCounter.Value()
	}
	if q.writeCounter != nil {
		snap.BytesUplinkTotal = q.writeCounter.Value()
	}

	var okCount int64
	var failCount int64
	var totalRTT int64
	var lastRTT int64
	var jitterSum int64
	var jitterSamples int64
	for _, sample := range q.samples {
		if !sample.success {
			failCount++
			continue
		}
		okCount++
		totalRTT += sample.rttMS
		if lastRTT > 0 {
			jitterSum += int64(math.Abs(float64(sample.rttMS - lastRTT)))
			jitterSamples++
		}
		lastRTT = sample.rttMS
	}
	totalCount := int64(len(q.samples))
	if okCount > 0 {
		snap.RTTAvgMs = totalRTT / okCount
	}
	if totalCount > 0 {
		snap.LossRatio = float64(failCount) / float64(totalCount)
		snap.LossPct = snap.LossRatio * 100
	}
	if jitterSamples > 0 {
		snap.JitterMs = jitterSum / jitterSamples
	}
	return snap
}
