# 规格：smart-routing-input

## Purpose

智能路由-入口模式（透明转发）：让运维暴露一个稳定的本地裸 TCP 入口，绑定一组有
序的现有 tunnel，把每条新的入站连接按优先级 + 健康，**透明转发**到所选 member
tunnel 的 **input 监听地址**，由 member 完成其协议（如 socks5x）协商与转发。入
口本身不配置协议；当首选 member 不可达时自动故障转移到下一个 member。

## Requirements

### Requirement: 入口模式入口管理

系统 SHALL 允许运维创建、列出、编辑、启用/禁用、删除入口模式入口。一个入口
SHALL 具有唯一名称、恰好一个裸 TCP 入口监听地址（如 `0.0.0.0:1000`）、一个有序
（按优先级）的一或多个 member tunnel 名称列表，以及一个故障转移策略。入口配置
SHALL 被持久化，以便进程重启后保留。

#### Scenario: 创建绑定多个 tunnel 的入口入口

- **WHEN** 运维创建名为 `socks-in` 的入口模式入口，监听 `0.0.0.0:1000`，有序
  member 为 `[goai_tcpmux, hk_tcpmux]`
- **THEN** 该入口被持久化、出现在入口列表，并（若已启用）开始在 `0.0.0.0:1000`
  上做裸 TCP 监听

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

### Requirement: member input 协议一致性

入口模式做透明转发，客户端只在被选中的 member 上完成一次协议协商。系统 SHALL
要求同一入口的所有 member 的 input 协议一致（例如全为 socks5x），否则故障转移
会因协议不匹配而失败。创建/编辑入口时 SHALL 校验并在不一致时给出错误提示。

#### Scenario: member input 协议一致时被接受

- **WHEN** 入口的所有 member 的 input 均为 `socks5x` 协议
- **THEN** 校验通过，入口可被创建/启用

#### Scenario: member input 协议不一致时被拒绝

- **WHEN** 入口的 member 中既有 `socks5x` 又有 `http` 协议的 input
- **THEN** 系统拒绝该变更，并返回指明协议不一致的错误

### Requirement: 到 member input 的按优先级透明故障转移

对入口监听器上的每条新入站 TCP 连接，系统 SHALL 透明转发到当前可用、优先级最
高的 member tunnel 的 **input 监听地址**。被禁用、未运行、或其健康被判定为
`down` 的 member SHALL 被跳过，改用优先级顺序中的下一个 member。若连接所选
member 的 input 失败，系统 SHALL 在放弃前尝试下一个 member。

#### Scenario: 优先透明转发到健康首选 member 的 input

- **WHEN** 入口收到新连接，且第一优先级 member `goai_tcpmux` 健康
- **THEN** 该连接的字节被透明转发到 `goai_tcpmux` 的 input 监听地址

#### Scenario: 自动故障转移到下一 member 的 input

- **WHEN** 收到新连接，`goai_tcpmux` 被判定为 `down`，而 `hk_tcpmux` 健康
- **THEN** 无需运维介入，该连接被透明转发到 `hk_tcpmux` 的 input 监听地址

#### Scenario: 连接 member input 失败顺延到下一 member

- **WHEN** 收到新连接，`goai_tcpmux` 被选中，但连接其 input 失败，且
  `hk_tcpmux` 可用
- **THEN** 系统改用 `hk_tcpmux` 重试后再报告结果

#### Scenario: 所有 member 均不可用

- **WHEN** 收到新连接，且每个 member 均被禁用、未运行或为 `down`，或每次连接其
  input 都失败
- **THEN** 该连接被拒绝/关闭，并记录失败日志

#### Scenario: 恢复后回到首选 member

- **WHEN** `goai_tcpmux` 在先前为 `down` 之后恢复健康
- **THEN** 其后的新连接再次被透明转发到 `goai_tcpmux` 的 input

### Requirement: 用于入口选址的健康信号

入口选址 SHALL 使用现有按 tunnel 的健康分类（`Service.QualitySummary()`：
`up`/`degraded`/`down`/`unknown`）。健康为 `unknown` 的 member SHALL 保持可选
（不被自动跳过），并 SHALL 与 `up`/`degraded` 的 member 一起按优先级参与选址。
入口配置界面 SHOULD 在 member 缺少健康数据时给出可见提示。

#### Scenario: 无健康数据的 member 仍可选

- **WHEN** 某 member 没有健康探测数据（健康为 `unknown`），且它是优先级最高的在
  运行 member
- **THEN** 新连接被透明转发到它，而不是被跳过

#### Scenario: 即便更优先，down 的 member 仍被跳过

- **WHEN** 优先级最高的 member 被判定为 `down`，而某个较低优先级的 member 为
  `up`、`degraded` 或 `unknown`
- **THEN** 选中该较低优先级的 member

### Requirement: 入口模式仅对新连接路由

故障转移决策 SHALL 按每条新入站连接生效。当承载某条已建立连接的 member 之后劣
化时，系统 SHALL NOT 迁移该连接。每条新连接 SHALL 在建立时独立重新评估 member
健康。

#### Scenario: 已建立连接不被迁移

- **WHEN** 某连接已被透明转发到 `goai_tcpmux`，之后 `goai_tcpmux` 变为 `down`
- **THEN** 现有连接保留在 `goai_tcpmux`（不迁移），新连接被路由到健康 member

### Requirement: 入口模式管理标签页与 API

系统 SHALL 在管理后台侧边栏提供「智能路由-入口」标签页，列出入口模式入口及其
运行状态；并 SHALL 通过遵循现有管理 API 约定的、需认证的 API 端点暴露入口模式
入口的 CRUD 操作。

#### Scenario: 侧边栏显示「智能路由-入口」标签

- **WHEN** 已认证的运维打开管理界面
- **THEN** 侧边栏包含「智能路由-入口」标签，可导航到入口模式入口列表

#### Scenario: 入口入口列表显示实时状态

- **WHEN** 运维打开「智能路由-入口」页面
- **THEN** 每个入口显示名称、监听地址、有序 member 及各自健康、启用状态、当前首
  选 member

#### Scenario: API 需要认证

- **WHEN** 在无有效会话的情况下调用任一入口模式 API 端点
- **THEN** 系统返回认证错误（401），与现有管理端点一致
