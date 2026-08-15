# Prometheus 指标说明

本文档列出 gotun 暴露的全部自定义 Prometheus 指标、含义、标签与常用查询示例。

## 暴露方式

- 端点：`GET /metrics`
- 监听地址由配置项 `metrics_listen` 指定（pprof 与 Prometheus 共用该地址，通过路径区分）。
  **为空则不开启监控**。配置示例：

  ```yaml
  metrics_listen: "127.0.0.1:6060"
  ```

- 同一地址下另有 pprof 调试端点：`/debug/pprof/*`。
- 除下表自定义指标外，`/metrics` 还会暴露 Go 运行时的默认指标（`go_*`、`process_*` 等）。

## 命名规范

所有自定义指标统一使用 `Namespace = "gotun"`，按业务划分子系统（Subsystem），
完整指标名 = `gotun_<subsystem>_<name>`。

| Subsystem | 含义 |
|---|---|
| `probe` | 链路质量探测 |
| `conn`  | 隧道流量字节计数 |
| `stream` | 多路复用流打开性能 |
| `output` | 底层连接池取流分布 |

## 指标总览

| 指标名 | 类型 | 标签 | 说明 |
|---|---|---|---|
| `gotun_probe_rtt_ms` | Gauge | `service`, `output` | 滑动窗口内的平均探测 RTT（毫秒） |
| `gotun_probe_loss_ratio` | Gauge | `service`, `output` | 滑动窗口丢包率（0~1） |
| `gotun_probe_jitter_ms` | Gauge | `service`, `output` | 平均探测抖动（毫秒） |
| `gotun_probe_status` | Gauge | `service`, `output` | 健康状态：`up=1` / `degraded=0.5` / `down=0` / `disabled=-1` |
| `gotun_probe_success_total` | Counter | `service`, `output` | 探测成功累计次数 |
| `gotun_probe_failure_total` | Counter | `service`, `output` | 探测失败累计次数 |
| `gotun_conn_bytes_uplink_total` | Counter | `name`, `io` | 上行累计字节数（连接写出方向） |
| `gotun_conn_bytes_downlink_total` | Counter | `name`, `io` | 下行累计字节数（连接读入方向） |
| `gotun_conn_bytes_common_total` | Counter | `name`, `io` | 不区分方向的通用字节计数（FRPC/FRPS 使用） |
| `gotun_stream_open_duration_seconds` | Histogram | `status` | 打开一条 mux stream 的耗时分布 |
| `gotun_output_pool_get_total` | Gauge | `idx` | 连接池各槽位成功取流次数 |

## 分组说明

### 1. 链路质量探测（probe）

数据来自帧头探测 runner，每次探测成功/失败时刷新。是 tunnel 列表 up/down 健康徽标背后的数据源。

- 标签 `service`：tunnel 名称；`output`：出口地址。
- `gotun_probe_status` 取值：
  - `1`：up（健康）
  - `0.5`：degraded（抖动/丢包导致降级）
  - `0`：down（已开启帧头探测且已有样本，但当前不可用）
  - `-1`：disabled（未开启帧头探测，或已开启但尚无探测样本/刚启用未探测）

每个 tunnel 的健康跟踪器创建时即写入一次 `gotun_probe_status` 初值，因此未探测/未开帧头的 tunnel 也会在指标中可见。刚启用、首次探测尚未完成的 tunnel 处于"未探测"中间态，记为 `-1`，待首次探测写入样本后再按真实状态刷新，避免误报为 down。

### 2. 隧道流量字节计数（conn）

由 `StatsConn` / `StatsPacketConn` 包装 TCP/UDP 连接，在 Read/Write 时累加。

- 标签 `name`：tunnel 名/UUID；`io`：input/output 地址。
- 普通 Server 用 uplink/downlink 成对计数；FRPC/FRPS 用 common 计数（暂未严格区分方向）。

### 3. 多路复用流打开性能（stream / output）

- `gotun_stream_open_duration_seconds`：打开 mux stream 的耗时分布，bucket 为
  `0.01, 0.05, 0.2, 0.5, 2, 4, 10`（秒）。标签 `status` 区分取流路径：
  - `openStream`：命中已有连接
  - `makeStream`：新建连接
  - `waitMaker`：等待建连完成
  - `returnError`：取流失败
  - 以及 `probe_*`、`bandwidth_*` 前缀的探测、带宽测试取流路径。
- `gotun_output_pool_get_total`：按连接池槽位 `idx` 统计成功取流次数，用于观察多条底层连接的负载均衡情况。

## 常用 PromQL 示例

```promql
# 各隧道实时上行/下行带宽（字节/秒）
rate(gotun_conn_bytes_uplink_total[1m])
rate(gotun_conn_bytes_downlink_total[1m])

# 隧道健康状态（1=up, 0.5=degraded, 0=down, -1=disabled）
gotun_probe_status

# 隧道中断告警：状态为 down
gotun_probe_status == 0

# 探测丢包率超过 10% 的隧道
gotun_probe_loss_ratio > 0.1

# 取流耗时 P99（按路径区分）
histogram_quantile(0.99, sum by (status, le) (rate(gotun_stream_open_duration_seconds_bucket[5m])))
```
