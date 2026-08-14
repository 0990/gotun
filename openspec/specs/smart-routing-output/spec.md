# 规格：smart-routing-output

## Purpose

智能路由-出口模式：让运维暴露一个稳定的本地入口监听器，绑定一组有序的现有
tunnel，把每条新的入站连接按优先级 + 健康，经所选 member tunnel 的 **output**
（拨号/转发能力）发出去——当首选 member 宕机或不可达时自动故障转移到下一条。
各 member 的 output 线路可以不同。

## Requirements

### Requirement: 出口模式入口管理

系统 SHALL 允许运维创建、列出、编辑、启用/禁用、删除出口模式入口。一个入口
SHALL 具有唯一名称、恰好一个带协议的入口监听地址（如 `http@0.0.0.0:1000` 或
`tcp@0.0.0.0:1000`）、一个有序（按优先级）的一或多个 member tunnel 名称列表，
以及一个故障转移策略。入口配置 SHALL 被持久化，以便进程重启后保留。

#### Scenario: 创建绑定多个 tunnel 的出口入口

- **WHEN** 运维创建名为 `web-out` 的出口模式入口，入口监听为
  `http@0.0.0.0:1000`，有序 member 为 `[goai_tcpmux, hk_tcpmux]`
- **THEN** 该入口被持久化、出现在入口列表，并（若已启用）开始在 `0.0.0.0:1000`
  上按所配协议监听

#### Scenario: 入口监听地址与现有监听冲突

- **WHEN** 运维创建或编辑的入口，其监听地址已被另一个 tunnel 输入或另一个入口
  占用
- **THEN** 系统拒绝该变更并返回错误，且不中断现有监听

#### Scenario: 入口引用不存在的 member

- **WHEN** 运维创建或编辑的入口，其 member 列表包含不存在的 tunnel 名称
- **THEN** 系统拒绝该变更，并返回指明未知 member 的错误

#### Scenario: 入口在重启后保留

- **WHEN** 已创建入口且进程重启
- **THEN** 所有已启用的入口按其配置的 member 与优先级顺序恢复监听

### Requirement: 经 member output 的按优先级故障转移转发

对入口监听器上的每条新入站连接，系统 SHALL 通过当前可用、优先级最高的 member
tunnel 的 **output** 转发该连接。被禁用、未运行、或其健康被判定为 `down` 的
member SHALL 被跳过，改用优先级顺序中的下一个 member。若通过所选 member 的
output 打开流失败，系统 SHALL 在放弃前尝试下一个 member。各 member 的 output
配置（协议/远端地址）可以不同。

#### Scenario: 优先经健康首选 member 的 output 转发

- **WHEN** 入口收到新连接，且第一优先级 member `goai_tcpmux` 健康
- **THEN** 该连接经 `goai_tcpmux` 的 output 拨出

#### Scenario: 自动故障转移到下一 member 的 output

- **WHEN** 收到新连接，`goai_tcpmux` 被判定为 `down`，而 `hk_tcpmux` 健康
- **THEN** 无需运维介入，该连接经 `hk_tcpmux` 的 output 拨出

#### Scenario: member 间可使用不同 output 线路

- **WHEN** `goai_tcpmux` 的 output 为 `tcp_mux@远端A`，`hk_tcpmux` 的 output 为
  `kcp_mux@远端B`，且首选 member 不可用
- **THEN** 系统仍可通过另一 member 的不同 output 线路转发连接

#### Scenario: 打开流失败顺延到下一 member

- **WHEN** 收到新连接，`goai_tcpmux` 被选中，但经其 output 打开流失败，且
  `hk_tcpmux` 可用
- **THEN** 系统改用 `hk_tcpmux` 重试该连接后再报告结果

#### Scenario: 所有 member 均不可用

- **WHEN** 收到新连接，且每个 member 均被禁用、未运行或为 `down`，或每次经其
  output 打开流都失败
- **THEN** 该连接被拒绝/关闭，并记录失败日志

#### Scenario: 恢复后回到首选 member

- **WHEN** `goai_tcpmux` 在先前为 `down` 之后恢复健康
- **THEN** 其后的新连接再次经 `goai_tcpmux` 的 output 路由

### Requirement: 用于出口选址的健康信号

出口选址 SHALL 使用现有按 tunnel 的健康分类（`Service.QualitySummary()`：
`up`/`degraded`/`down`/`unknown`）。健康为 `unknown` 的 member SHALL 保持可选
（不被自动跳过），并 SHALL 与 `up`/`degraded` 的 member 一起按优先级参与选址。
入口配置界面 SHOULD 在 member 缺少健康数据时给出可见提示。

#### Scenario: 无健康数据的 member 仍可选

- **WHEN** 某 member 没有健康探测数据（健康为 `unknown`），且它是优先级最高的在
  运行 member
- **THEN** 新连接经它路由，而不是被跳过

#### Scenario: 即便更优先，down 的 member 仍被跳过

- **WHEN** 优先级最高的 member 被判定为 `down`，而某个较低优先级的 member 为
  `up`、`degraded` 或 `unknown`
- **THEN** 选中该较低优先级的 member

### Requirement: 出口模式仅对新连接路由

故障转移决策 SHALL 按每条新入站连接生效。当承载某条已建立连接的 member 之后劣
化时，系统 SHALL NOT 迁移该连接。每条新连接 SHALL 在建立时独立重新评估 member
健康。

#### Scenario: 已建立连接不被迁移

- **WHEN** 某连接经 `goai_tcpmux` 的 output 建立，之后 `goai_tcpmux` 变为 `down`
- **THEN** 现有连接保留在 `goai_tcpmux`（不迁移），新连接被路由到健康 member

### Requirement: 出口模式管理标签页与 API

系统 SHALL 在管理后台侧边栏提供「智能路由-出口」标签页，列出口模式入口及其运
行状态；并 SHALL 通过遵循现有管理 API 约定的、需认证的 API 端点暴露出口模式入
口的 CRUD 操作。

#### Scenario: 侧边栏显示「智能路由-出口」标签

- **WHEN** 已认证的运维打开管理界面
- **THEN** 侧边栏包含「智能路由-出口」标签，可导航到出口模式入口列表

#### Scenario: 出口入口列表显示实时状态

- **WHEN** 运维打开「智能路由-出口」页面
- **THEN** 每个入口显示名称、监听地址、有序 member 及各自健康、启用状态、当前首
  选 member

#### Scenario: API 需要认证

- **WHEN** 在无有效会话的情况下调用任一出口模式 API 端点
- **THEN** 系统返回认证错误（401），与现有管理端点一致
