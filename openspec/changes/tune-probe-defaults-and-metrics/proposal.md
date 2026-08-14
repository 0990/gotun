# 提案：tune-probe-defaults-and-metrics（探测默认值调优与可用性指标）

## 为什么（Why）

智能路由依赖 `Service.QualitySummary()` 的健康分类做故障转移选址，但当前健康判定
收敛过慢，导致「member 已挂却迟迟不被剔除」：

- **探测节奏慢**：`probe_interval_sec` 默认 10s，一次探测间隔过长。
- **失败判定慢**：`probe_timeout_ms` 默认 5s，对端 hang 住时每次失败要等满 5s。
- **窗口过大**：`probe_window_size` 默认 20，需把窗口内成功样本全部刷掉才判 down。

三者叠加，「全成功 → down」最坏约 **200s**（20 × 10s），对端 hang 时甚至更久
（被 timeout 主导）。对故障转移场景而言，这段时间内流量持续被打到死 member 上。

此外，Prometheus 指标 `gotun_probe_status` 目前只在 `RecordProbeSuccess/Failure`
被调用时才写入，导致：

- tunnel 刚启动、尚未探过一次时，在 Prometheus 中**没有任何序列**；
- 未开帧头探测（`frame_header_enable=false`，状态 disabled）的 tunnel **始终没有**
  该序列。

监控系统里「tunnel 消失」与「tunnel down」无法区分，不利于「展示每个 tunnel 可用性」。

## 变更内容（What Changes）

- **调优探测默认值**（仅改代码内置默认，不改 JSON 配置语义；已显式配置上述参数的
  tunnel 不受影响）：
  - `probe_interval_sec`：10s → **2s**
  - `probe_timeout_ms`：5s → **1500ms**
  - `probe_window_size`：20 → **5**
  - 三处硬编码默认抽为命名常量，便于统一调整与阅读。
- **保证可用性指标始终存在**：每个 tunnel 的 `QualityTracker` 在注册/创建时即初始化
  一条 `gotun_probe_status` 序列（按 `enabled` 给初值 disabled/down），而不是等到首次
  探测才有序列。不新增指标名，仅保证既有指标的「始终存在、可监控」。

本变更**明确不包含**（非目标 non-goals）：

- 修改健康状态判定算法（`calcStatusLocked` 的 up/degraded/down 规则不变）。
- 新增 Prometheus 指标名或改变既有指标的语义与取值映射。
- 强制改写既有 tunnel 的持久化 JSON（不补写 probe 参数到存量配置）。
- 基于真实连接质量（open 成功/失败）的被动健康信号——留作后续单独变更。

## 能力（Capabilities）

### 新增能力（New Capabilities）

- 无。

### 修改能力（Modified Capabilities）

- `tunnel-health`：调整探测参数的**内置默认值**；要求可用性指标在 tunnel 生命周期内
  始终存在（含未探测与未开帧头情形）。健康判定算法本身不变。

## 影响（Impact）

- **运行时行为**：
  - 默认探测频率提高（10s → 2s），单 tunnel 探测开销极小（mux 下为一次子流 ping/pong），
    可忽略；多 tunnel 部署下探测 QPS 上升 5 倍，但基数很小。
  - 默认窗口缩小（20 → 5），「全失败 → down」最坏收敛由 ~200s 降至 ~10s（5 × 2s）；
    同时状态对短期抖动更敏感， degraded/up 翻转更频繁。
- **代码**：
  - `tun/frame_probe_runner.go`：interval/timeout 默认值改为命名常量。
  - `tun/quality.go`：windowSize 默认值改为命名常量；`NewQualityTracker`（或注册路径）
    增加「创建即写入初始 `gotun_probe_status`」逻辑。
- **可观测性**：所有 tunnel 注册后即出现在 `gotun_probe_status` 中；disabled（未开帧头）
  取值 -1，未探测取值 0（down），探测后按真实状态刷新。
- **对存量配置无破坏性变更**：已显式设置 probe 参数的 tunnel 维持原值。
