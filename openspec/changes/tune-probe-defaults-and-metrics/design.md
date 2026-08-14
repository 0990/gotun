# 设计：tune-probe-defaults-and-metrics

## 背景

智能路由选址（`tun/route_selector.go` 的 `memberAlive`）依据 `Service.QualitySummary().Status`
剔除 `down` 的 member。状态由 `QualityTracker.calcStatusLocked`（`tun/quality.go:214`）基于
滑动窗口内的探测样本计算，样本由 `FrameProbeRunner`（`tun/frame_probe_runner.go`）按
`probe_interval_sec` 节流产生。

本次只调「探测节奏 / 失败判定 / 窗口大小」三个内置默认值，并保证可用性指标始终存在；
不改判定算法。

## 关键决策

### D1：默认值抽为命名常量，集中放在 quality 相关代码处

三处默认值目前硬编码且分散：

- `probe_interval_sec` → `frame_probe_runner.go` 的 `maybeProbe`（`interval = 10 * time.Second`）
- `probe_timeout_ms` → `frame_probe_runner.go` 的 `probeOnce`（`timeout = 5 * time.Second`）
- `probe_window_size` → `quality.go` 的 `NewQualityTracker`（`windowSize = 20`）

决策：在 `tun/quality.go`（或独立的常量块）定义：

```go
const (
    defaultProbeIntervalSec = 2
    defaultProbeTimeoutMS   = 1500
    defaultProbeWindowSize  = 5
)
```

`frame_probe_runner.go` 引用前两个常量，`NewQualityTracker` 引用第三个。
**理由**：消除魔法数字、单一事实来源，后续再调只改一处。

### D2：只改内置默认，不回填存量配置

`Extend` 三个字段（`config.go:40-42`）的「未设置」以零值（0）表示，运行时读到 0 才落到
内置默认。因此改默认值即对所有「未显式配置」的 tunnel 生效，**无需**改写任何 `.tun`
JSON 文件；已显式配置（>0）的 tunnel 维持原值。

**理由**：满足「对存量配置无破坏性变更」；零值即默认的语义保持不变。

### D3：默认值取 2s / 1500ms / 5 的依据

- interval=2s：mux 下单次探测为一次子流 ping/pong，开销可忽略；2s 使状态刷新足够快。
- timeout=1500ms：需大于典型 RTT 以免误判，又要显著小于旧 5s 以加快 hang 型失败的判定。
  1500ms 是两者的折中。
- windowSize=5：配合 interval=2s，「全失败 → down」最坏 ~10s；窗口过小会更敏感于抖动，
  5 是收敛速度与平滑性的折中。

**已知权衡**：窗口缩小后，degraded/up 对短期丢包更敏感。这是可接受的——故障转移更看重
「快速剔除死 member」，且 `degraded` 仍可被选中（`memberAlive` 只剔除 `down`）。

### D4：可用性指标「创建即初始化」，不新增指标名

现状 `gotun_probe_status` 仅在 `updatePrometheusLocked`（被 `RecordProbeSuccess/Failure`
调用）时写入，导致未探测 / 未开帧头的 tunnel 无序列。

决策：在 `QualityTracker` 创建（`NewQualityTracker`）时即调用一次状态写入，按 `enabled`：

- `enabled=false`（未开帧头）→ 写 `disabled`（-1）
- `enabled=true` 但尚无样本 → 写 `down`（0，与 `calcStatusLocked` 空样本判 down 一致）

**理由**：复用既有指标语义（up=1 / degraded=0.5 / down=0 / disabled=-1），不改变取值映射；
让「tunnel 消失」与「down」在监控中可区分。初始值与 `calcStatusLocked` 对空样本的判定
保持一致，避免语义分裂。

注意：初始化写入需读取 service/output 标签——`NewQualityTracker` 已接收这两个参数，
可直接用于 `WithLabelValues`。

## 风险与缓解

- **风险**：窗口 5 + interval 2s 下，短时网络抖动可能使 member 在 up/degraded 间抖动。
  **缓解**：`degraded` 不被 `memberAlive` 剔除，选址行为不受影响；仅 `down`（窗口内全失败）
  才剔除，5 个样本全失败的误判概率低。
- **风险**：`NewQualityTracker` 也被 `outputTracker`（`output.go:372`）以 `enabled=false`
  空调用（非 `*Output` 时的兜底）。初始化写入会为其产生无意义序列（标签为空字符串）。
  **缓解**：初始化写入仅在 `service`/`output` 标签非空时执行，跳过兜底 tracker。

## 测试策略

- 默认值：单测断言未配置时 `maybeProbe`/`probeOnce`/`NewQualityTracker` 落到新常量
  （2s / 1500ms / 5）。
- 指标初始化：构造 `NewQualityTracker`（enabled true/false）后，用 prometheus 测试
  收集器断言对应 `{service,output}` 序列已存在且初值正确（0 / -1）。
- 既有 `quality_test.go` 的端到端用例显式传了 probe 参数，不受默认值变更影响，应继续通过。
