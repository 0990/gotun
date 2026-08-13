# 提案：add-smart-routing（智能路由）

## 为什么（Why）

目前每个 tunnel 只把一个输入监听器绑定到一个输出上。一旦该 tunnel 背后的链路
变得不可达（丢包、RTT 飙升、对端宕机），到达该输入的每条连接都会直接失败——
没有任何自动切换到更健康路径的能力。

运维通常会为同一个逻辑目的地、经不同中继线路运行多条 tunnel（例如
`goai_tcpmux` 与 `hk_tcpmux`）。他们希望有一个稳定的本地入口：优先使用最优的
tunnel，并在该路径劣化时自动切换，而不是在路径出问题时手工把客户端改指到别
处。gotun 已经在测量每条 tunnel 的健康度（通过现有探测机制得到 RTT / 丢包 /
up-down 状态），所以智能路由所需的信号已经在被计算——只是还没有被用来路由流
量。

实际使用中又有两类诉求：
- **出口容灾**：客户端固定一种协议（如 http 代理 / 裸 TCP），底下走多条**不同**
  线路（goai 走 tcp_mux、hk 走 kcp_mux），任选一条把流量发出去即可。
- **入口容灾**：客户端用 member tunnel 已经配好的某种协议入口（如 socks5x），
  希望在多个**同协议**的 member 入口之间做故障转移，入口本身不想再配协议。

## 变更内容（What Changes）

新增「智能路由」能力，以**两个独立的侧边栏标签页**提供两种转发模式：

- **「智能路由-出口」标签页（output 模式）**：入口 = 一个本地监听器 +
  一个跨 member 的**故障转移组合输出**。新连接按优先级 + 健康挑选某个 member，
  通过该 member 的 **output**（拨号/转发能力）把流量发出去。入口监听需配置协议
  （由客户端协议决定），各 member 的 output 线路**可以不同**。
- **「智能路由-入口」标签页（input 模式，透明转发）**：入口 = 一个**裸 TCP 透
  传监听器**。新连接按优先级 + 健康挑选某个 member，把字节**原样转发**到该
  member 的 **input 监听地址**，由 member 完成其协议（如 socks5x）协商与转发。
  入口不需要配置协议；要求所有 member 的 input 协议**一致**。

两种模式共享以下能力：

- **入口 CRUD**：创建 / 列表 / 编辑 / 启用禁用 / 删除入口。一个入口包含：名称、
  一个入口监听地址、一个**有序的 member tunnel 名称列表**（即优先级顺序）、一个
  故障转移策略（首版本仅支持：优先级 / 顺序故障转移）。
- **健康选址**：复用现有 `Service.QualitySummary()` 的按 tunnel 健康分类
  （up / degraded / down / unknown），被判定为 `down` 的 member 被自动跳过；健康
  为 `unknown`（未开帧头探测）的 member 仍可被选中、不被自动跳过。
- **按新连接选址 + 失败顺延**：每条新连接独立重新评估「优先级最高的健康
  member」；选中后拨号/连接失败则顺延到下一个 member。
- **持久化**：入口以 JSON 文件（`.route` 后缀）持久化，启动时加载运行，新增 /
  编辑 / 禁用时通过 manager 热生效。
- **管理 API**：提供两类入口各自的 CRUD 端点，遵循现有 `/api/...` +
  `authMgr.RequireAuth` 模式。

本次变更**明确不包含**（非目标 non-goals）：
- 连接中段故障转移（已建立的流断开时**不**迁移；只有*新*连接被路由）。已记录的
  限制。
- 跨 member 的负载均衡 / 按权重分流——只做顺序故障转移。
- UDP 入口监听（入口监听仅 TCP）。
- 为未设置 `frame_header_enable` 的 member 补齐健康探测（见「影响 → 前置条件」）。
- input 模式的协议感知终结（入口仅做透明转发，不终结 socks5/http）。

## 能力（Capabilities）

### 新增能力（New Capabilities）

- `smart-routing-output`：智能路由-出口模式。一个命名入口，含一个本地监听与一个
  跨 member 的故障转移组合输出；对入口新连接按优先级 + 健康，经所选 member 的
  output 转发；含管理标签页与 API。
- `smart-routing-input`：智能路由-入口模式（透明转发）。一个命名入口，含一个裸
  TCP 透传监听；对入口新连接按优先级 + 健康，把字节透明转发到所选 member 的
  input 监听地址；含管理标签页与 API。

### 修改能力（Modified Capabilities）

- 无。本次新增两个能力，不改变现有 tunnel 在需求层面的行为。（实现上会复用
  tunnel 的健康信号与 manager，但 tunnel 本身行为不变。）

## 影响（Impact）

- **数据模型 / 持久化**：新增智能路由入口配置类型（含 `Mode`：`output|input`），
  以 JSON 文件（`.route`）存放在 tun 目录，复用 `tun/config.go` 的原子写入 + 回
  滚机制。
- **运行时 / manager**：`tun.Manager` 增加对智能路由入口的管理（add /
  replace-by-uuid / set-disabled / remove / list），与 service 管理并列。
  - output 模式：新增实现现有未导出 `output` 接口（`tun/output.go:31`）的
    `failoverOutput`，按优先级委托各 member 的 `GetStream()`。
  - input 模式：新增透明转发 service，按优先级把字节 relay 到所选 member 的
    input 监听地址。
- **构造器串联**：`NewServer`（`tun/server.go:32`）目前构造 output 时不带
  manager 引用；故障转移选址需要 manager 按名称解析 member，因此需把 manager 串
  联进智能路由 service 的构造路径（两种模式都需要）。
- **健康信号**：两种模式统一使用 `Service.QualitySummary()`（`tun/service.go:10`），
  其在 `Server` 上委托给 output 的健康分类（`tun/quality.go:214-238`）。
- **管理 API**：在 `admin/route/route.go` 标记之间新增两组端点
  （`/api/route_output/*` 与 `/api/route_input/*`），并各配一个 controller 包。
- **管理界面**：在 `admin/view/layout/default.html` 的菜单/路由标记处新增**两个**
  标签页「智能路由-出口」「智能路由-入口」，各对应一个 `admin/view/route_*`
  下的页面（Vue SFC，经 `/render`）。需要重新编译 Go 二进制（embed FS）。
- **正确故障转移的前置条件**：member 应设置 `frame_header_enable` 才能真正测量
  RTT / 丢包 / 状态；未设置的 member 报告 `QualityStatusDisabled`（unknown），仍
  可被选中但永不被自动跳过。界面应暴露这一点，提示为 member 开启帧头探测。
- **input 模式前置条件**：所有 member 的 input 协议需一致，否则透明转发的连接在
  故障转移时会因协议不匹配而中断；创建/编辑时校验并提示。
- **对现有 tunnel、配置文件或 API 无破坏性变更。**
