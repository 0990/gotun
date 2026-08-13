# 设计：add-smart-routing（智能路由）

## 背景（Context）

动机见 `proposal.md` —— 为什么。相关现状（已对照代码核实）：

- tunnel 以 JSON 文件持久化（`<tunDir>/<name>.tun`），由 `tun.Manager`
  （`tun/manager.go:13`）以 tunnel **名称**为键在内存中管理；运行中的 tunnel 是
  一个 `tun.Service`（`tun/service.go:5`）。
- 默认 tunnel 是 `tun.Server` = 一个 `input`（本地监听，由 `newInput` 工厂构造，
  `tun/input.go:16`）+ 一个 `output`（远端拨号，`NewOutput`，`tun/output.go:46`）。
  新入站流在 `Server.handleInputStream`（`tun/server.go:90`）进入数据路径，调
  `s.output.GetStream()`（`server.go:119`）拨出后建管。
- 拨号侧是未导出的 `output` 接口（`tun/output.go:31-41`）：`Run / Close /
  GetStream / GetProbeStream / GetBandwidthStream / QualitySnapshot /
  QualitySummary / FrameHeaderEnabled / ProbeConfig`。
- 每个 tunnel 的健康由 `Service.QualitySummary()`（`tun/service.go:10`）暴露，在
  `Server` 上委托给 output 的 `QualityTracker`，分类为
  `up`/`degraded`/`down`/`disabled`（`tun/quality.go:214-238`）。探测仅当 output
  设置 `frame_header_enable` 时运行（`server.go:165 startProbe`），否则报告
  `QualityStatusDisabled`（即 unknown）。`disabledService` 恒报 disabled。
- `NewServer`（`tun/server.go:26-51`）构造 output 时**不带 `*Manager` 参数**，目
  前无法按名称看到同级 tunnel。
- 管理后台：原生 `net/http` ServeMux，路由注册在 `admin/route/route.go` 的
  `----Route-begin----`/`----Route-end----` 标记之间；controller 是捕获
  `*tun.Manager` 的闭包工厂；视图是 `admin/view/` 下经 `/render` 提供的 Vue
  SFC；侧边栏菜单与 vue-router 路由硬编码在
  `admin/view/layout/default.html` 的 `----Menus-Add-----` / `----Routes-Add-----`
  标记处。

## 目标 / 非目标（Goals / Non-Goals）

**目标：**
- 两种一等的智能路由入口，均以**两个独立标签页**提供：
  - **出口模式（output）**：入口监听 + 跨 member 的故障转移组合输出，经所选
    member 的 output 发出连接。
  - **入口模式（input，透明转发）**：裸 TCP 入口监听，把字节透明转发到所选
    member 的 input 监听地址。
- 两种模式**共用同一套健康选址逻辑**（`Service.QualitySummary()`）与 `Members`
  数组顺序即优先级。
- 复用现有 output / 质量机制与 manager 的原子持久化 + 回滚；不另起并行健康系统。

**非目标：**
- 连接中段迁移、加权负载均衡、UDP 入口（见 proposal 非目标）。
- input 模式的协议感知终结（入口只做透明转发，不终结 socks5/http）。
- 改变 member tunnel 自身的运行或探测方式。
- 通用可插拔路由策略框架——v1 只做优先级故障转移。

## 决策（Decisions）

### D1 —— 两种模式都是 `tun.Service`；共享选址，差别在「转发到哪一端」

把入口实现为受管 `Service`（与 `Server`、`Frpc`、`Frps` 并列）。两种模式共享：

- **member 解析**：经 `*Manager.GetService(name)` 按名称惰性解析 member（见 D3）。
- **选址策略**：见 D2，按 `Members` 顺序 + `QualitySummary()` 健康，失败顺延。

差别仅在数据面：

- **出口模式（`failoverOutput`）**：实现现有 `output` 接口（`tun/output.go:31`），
  自身不持有 socket，把每次 `GetStream()` 委托给所选 member 的活动 output。入口
  的 `input` 由 `newInput` 依据配置的协议监听构造，新流走与
  `Server.handleInputStream` 相同的 crypto/建管路径，只是 `output` 换成
  `failoverOutput`。**好处**：对 input 侧、accept 循环、crypto 包装、管道逻辑零改
  动；member 的 output 线路可彼此不同。
- **入口模式（透明转发 service）**：入口是一个裸 TCP 监听；每条新连接按选址结果
  `net.Dial` 到所选 member 的 **input 监听地址**，然后 `io.Copy` 双向转发字节。
  入口不做任何协议处理，member 自己完成 socks5/http 协商。**好处**：入口无需配协
  议；**代价**：要求所有 member 的 input 协议一致（见 D5 校验），且切换时若协议不
  同会断。

*曾考虑的替代方案：*
- *只做一个模式。* 否决：两类诉求（不同线路容灾 vs 复用同协议入口）都真实存在，
  且共享选址后增量成本可控。
- *input 模式做协议感知终结。* 否决：会退化为 output 模式的复杂度，丢掉「入口省
  事」的意义。

### D2 —— 统一选址策略：优先级列表 + 健康门控 + 失败顺延

两种模式共用一个选址函数：

1. 候选 = 按配置优先级排序的 member（`Members[0]` 最高）。
2. 跳过满足任一条件的 member：被禁用、不存在/未运行、或
   `QualitySummary().Status == down`（unknown/disabled 的 member 仍可选，见规
   格）。
3. 对第一个剩余 member 尝试「转发」（output 模式 = 调其 output `GetStream()`；
   input 模式 = `net.Dial` 其 input 地址）。失败则顺延到下一个 member，返回首个
   成功。
4. 全部失败则报错；入口据此关闭入站连接。

选址发生在**每次新连接**，因此每条连接都重新评估健康——免费满足「仅对新连接路
由」与「恢复后回到首选」。

*曾考虑的替代方案：* 后台 goroutine 预计算「当前最优 member」。否决：按连接选址
更简单、无滞后窗口、对探测翻转即时反应。

### D3 —— 把 `*Manager` 串联进两种入口的构造

两种模式都要把 member 名称解析为活动 `Service`（`GetService(name)`），但
`NewServer` 拿不到 manager。因此入口的 service 构造器接收 `*Manager`，并在每次
选址时**惰性解析** member（member 重启/编辑后无需重建入口）。

这是对现有构造路径唯一的外科式改动：为智能路由新增 manager 感知的构造器 /
`NewService` 分发分支（或独立 manager 方法），不改写现有 tunnel 的
`NewServer` 签名。

*曾考虑的替代方案：* 入口启动时一次性解析并缓存 `Service` 指针。否决：member 重
启/替换后会指向已关闭的 service。惰性解析既正确又廉价（一次 RWMutex 保护的 map
命中）。

### D4 —— 持久化：JSON 文件、`.route` 后缀、含 Mode 字段、复用原子/回滚

入口存为 `<tunDir>/<entryName>.route` JSON（与 `.tun` 区分，不影响现有
`loadAllServiceFile` 的 glob）。复用 `tun/config.go` 的原子写入 + 备份/回滚。配
置类型：

```
RouteConfig {
  UUID, Name, Disabled,
  Mode     string   // "output" | "input"
  Listen   string   // output 模式: "http@0.0.0.0:1000"；input 模式: "0.0.0.0:1000"
  Members  []string // 优先级顺序，Members[0] 最高
  Policy   string   // "priority"（v1 唯一支持值）
  CreatedAt
}
```

`Manager` 增加 `AddRoute / ReplaceRouteByUUID / SetRouteDisabled /
RemoveRoute(ByUUID) / AllRoute`，共用同一把锁。`Manager.Run()` 一并加载并启动
`.route` 入口，按 `Mode` 分派到出口/入口两种 service 构造。

### D5 —— 创建/编辑校验

两种模式都拒绝（清晰错误）：名称为空、member 为空、监听地址无法解析、引用不存
在的 member、监听地址已被占用、名称重复。

**出口模式**额外校验 `Listen` 的协议可被 `newInput` 解析（tcp/http/socks5x 等受
支持协议）。

**入口模式**额外校验所有 member 的 **input 协议一致**（解析各 member 的
`Cfg().Input` 协议部分比对）；不一致时报错并提示——这是透明转发的硬约束。

入口与 member 之间**不可能成环**（member 必须是已存在的 tunnel，而入口不是
tunnel），无需环检测。

### D6 —— 管理界面：两个独立标签页 + 两组 API

- 路由（`admin/route/route.go` 标记之间），均 `authMgr.RequireAuth` 包裹，各由一
  个捕获 `*tun.Manager` 的 controller 包处理：
  - 出口：`/api/route_output/list|create|edit|delete|set_disabled`
  - 入口：`/api/route_input/list|create|edit|delete|set_disabled`
- 入口列表行额外报告**当前首选 member**（用同一选址逻辑计算）与各 member 健康，
  供页面展示实时路由状态。
- 侧边栏 + 路由：在 `default.html` 的 `----Menus-Add-----` 加两个菜单项「智能路
  由-出口」「智能路由-入口」，在 `----Routes-Add-----` 加对应两条路由；新增两个
  页面 `admin/view/route_output/list.html` 与 `admin/view/route_input/list.html`
  （Vue SFC，`module.exports` 模式）。需重新编译 Go 二进制（embed FS）。
- 出口模式编辑页：监听地址带协议；入口模式编辑页：监听为裸 `host:port`，并对
  member 协议一致性、未开帧头探测的 member 给出可见提示。

## 风险 / 权衡（Risks / Trade-offs）

- **无健康数据的 member 不被自动跳过** → 未开 `frame_header_enable` 却实际已死的
  member 会被优先尝试，仅靠失败顺延才转移，给首条连接加一次失败开销。→ 缓解：
  失败顺延仍能正确转移；UI 提示「无健康数据」并引导开帧头探测；文档写明前置条
  件。
- **入口模式 member 协议不一致即断** → 透明转发下切换到一个协议不同的 member
  会使连接中断。→ 缓解：创建/编辑强校验协议一致（D5）+ UI 提示。
- **每个 member 额外占用本地监听** → member 是完整 tunnel，各自绑定入口可能用不
  到的端口（出口模式用不到 member 的 input，入口模式用不到 member 的 output）。
  → 缓解：无害且不在数据路径；记为后续「仅输出/仅输入 member」优化点。
- **按连接查询 manager** → 每条新连接 `GetService` 取一次 RWMutex。→ 缓解：预期
  速率下可忽略；避免悬空指针。
- **恢复时蜂拥回切** → 首选恢复后所有新连接一次性切回。→ 缓解：顺序故障转移可
  接受；v1 不加防抖（探测窗口已平滑 down/up）。
- **无连接中段故障转移** → 承载活动流的 member 宕机则该流断开。→ 缓解：记录的
  限制；新连接立即自愈。

## 迁移计划（Migration Plan）

- 纯增量；不改任何现有配置、文件或 API。部署 = 重新编译 Go 二进制（嵌入新视图与
  路由）并重启。现有 tunnel 不受影响。
- 回滚 = 还原二进制；`.route` 文件被旧版本忽略（旧版本只 glob `.tun`）。

## 开放问题（Open Questions）

无会改变规格、方案或任务拆分的开放问题。（后续是否加「仅输出/仅输入 member」、
负载均衡策略属未来变更，而非 v1 未知项。）
