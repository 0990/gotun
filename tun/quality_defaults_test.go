package tun

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// 未显式配置时，三处内置默认值应符合预期
func Test_ProbeDefaults(t *testing.T) {
	if defaultProbeIntervalSec != 2 {
		t.Fatalf("defaultProbeIntervalSec = %d, 期望 2", defaultProbeIntervalSec)
	}
	if defaultProbeTimeoutMS != 1500 {
		t.Fatalf("defaultProbeTimeoutMS = %d, 期望 1500", defaultProbeTimeoutMS)
	}
	if defaultProbeWindowSize != 5 {
		t.Fatalf("defaultProbeWindowSize = %d, 期望 5", defaultProbeWindowSize)
	}

	// windowSize 传 0 时应落到默认值
	tr := NewQualityTracker("", "", nil, nil, true, 0)
	if tr.windowSize != defaultProbeWindowSize {
		t.Fatalf("windowSize 兜底 = %d, 期望 %d", tr.windowSize, defaultProbeWindowSize)
	}
}

// 创建即写入 gotun_probe_status 初值：开帧头未探测 → down(0)
func Test_QualityTrackerInitMetric_Enabled(t *testing.T) {
	service, output := "init-enabled-svc", "init-enabled-out"
	NewQualityTracker(service, output, nil, nil, true, 0)

	got := testutil.ToFloat64(probeStatusGauge.WithLabelValues(service, output))
	if got != 0 {
		t.Fatalf("开帧头未探测初值 = %v, 期望 0(down)", got)
	}
}

// 创建即写入 gotun_probe_status 初值：未开帧头 → disabled(-1)
func Test_QualityTrackerInitMetric_Disabled(t *testing.T) {
	service, output := "init-disabled-svc", "init-disabled-out"
	NewQualityTracker(service, output, nil, nil, false, 0)

	got := testutil.ToFloat64(probeStatusGauge.WithLabelValues(service, output))
	if got != -1 {
		t.Fatalf("未开帧头初值 = %v, 期望 -1(disabled)", got)
	}
}
