# 任务：tune-probe-defaults-and-metrics

## 1. 默认值常量与替换

- [x] 1.1 在 `tun/quality.go` 定义命名常量 `defaultProbeIntervalSec=2`、`defaultProbeTimeoutMS=1500`、`defaultProbeWindowSize=5`（中文注释说明用途）
- [x] 1.2 `tun/frame_probe_runner.go` 的 `maybeProbe`：interval 默认值改用 `defaultProbeIntervalSec`
- [x] 1.3 `tun/frame_probe_runner.go` 的 `probeOnce`：timeout 默认值改用 `defaultProbeTimeoutMS`
- [x] 1.4 `tun/quality.go` 的 `NewQualityTracker`：windowSize 默认值改用 `defaultProbeWindowSize`

## 2. 可用性指标创建即初始化

- [x] 2.1 `NewQualityTracker` 在创建时（service/output 标签非空时）写入一次 `gotun_probe_status` 初值：`enabled=false` → -1（disabled）；`enabled=true` → 0（down）
- [x] 2.2 确保 `outputTracker` 兜底路径（空标签）不产生无意义序列

## 3. 测试

- [x] 3.1 新增/更新单测：未配置 probe 参数时，三处默认值分别为 2s / 1500ms / 5（`tun/quality_defaults_test.go`）
- [x] 3.2 新增单测：`NewQualityTracker`（enabled true/false）创建后，对应 `{service,output}` 的 `gotun_probe_status` 序列已存在且初值正确（0 / -1）
- [x] 3.3 确认既有 `tun/quality_test.go` 端到端用例仍通过

## 4. 验证

- [x] 4.1 `go build ./...` 通过
- [x] 4.2 `go vet ./...` 通过（两处警告为既有问题，与本变更无关：`config.go:19` 的 `mode"` 标签笔误、`frpc_controller.go:68` 不可达代码）
- [x] 4.3 `go test ./tun/ -run 'Quality|Probe|FrameHeader'` 通过（全量 `go test ./...` 含 100s sleep 的慢用例，非本次改动范围）
