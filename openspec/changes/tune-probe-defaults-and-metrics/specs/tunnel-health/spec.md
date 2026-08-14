# 规格增量：tunnel-health（修改能力）

## Purpose

每个 tunnel 的输出健康度（RTT / 丢包 / up-degraded-down 状态）由探测机制计算，供
智能路由选址与监控使用。本增量调整探测参数的**内置默认值**以加快故障收敛，并要求
可用性指标在 tunnel 生命周期内**始终存在**（含未探测与未开帧头情形）。健康判定算法
本身不变。

## ADDED Requirements

### Requirement: 探测参数的内置默认值

系统 SHALL 为未显式配置的探测参数提供内置默认值：`probe_interval_sec` 默认 2 秒、
`probe_timeout_ms` 默认 1500 毫秒、`probe_window_size` 默认 5。这些默认值 SHALL 以
命名常量定义，且仅在该参数未被显式配置（零值）时生效；已显式配置的 tunnel SHALL
维持其配置值。

#### Scenario: 未配置探测参数的 tunnel 使用新默认值

- **WHEN** 一个 tunnel 的 `Extend` 未设置 `probe_interval_sec` / `probe_timeout_ms` /
  `probe_window_size`（或为 0）
- **THEN** 运行时使用默认值 2s / 1500ms / 5 进行探测节奏、失败判定与窗口大小计算

#### Scenario: 显式配置的 tunnel 不受默认值变更影响

- **WHEN** 一个 tunnel 的 `Extend` 显式设置了上述任一参数（大于 0）
- **THEN** 该参数使用所配置的值，而非内置默认值

### Requirement: 可用性指标始终存在

系统 SHALL 在每个 tunnel 的健康跟踪器创建时即写入其 `gotun_probe_status` 序列，使该
tunnel 在 Prometheus 中始终可被查询，而不必等到首次探测。初值 SHALL 反映当前可判定
的状态：未开帧头探测（disabled）取值 -1；已开帧头但尚无探测样本取值 0（down）；后续
探测按真实状态刷新。取值映射 SHALL 保持 up=1 / degraded=0.5 / down=0 / disabled=-1 不变。

#### Scenario: 未开帧头的 tunnel 创建后即有 disabled 序列

- **WHEN** 一个未设置 `frame_header_enable` 的 tunnel 被加载/创建
- **THEN** 其 `{service, output}` 对应的 `gotun_probe_status` 序列存在且取值为 -1

#### Scenario: 已开帧头但未探测的 tunnel 创建后即为 down

- **WHEN** 一个设置了 `frame_header_enable` 的 tunnel 被加载，但尚未完成任何一次探测
- **THEN** 其 `gotun_probe_status` 序列存在且取值为 0，并在首次探测成功后刷新为真实状态

#### Scenario: 内部兜底跟踪器不产生无标签序列

- **WHEN** 运行时创建不带 service/output 标签的内部兜底健康跟踪器
- **THEN** 不写入任何 `gotun_probe_status` 序列
