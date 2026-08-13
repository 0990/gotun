# 任务：add-smart-routing（智能路由）

按 `specs/smart-routing-output/spec.md` 与 `specs/smart-routing-input/spec.md`
实现两个能力，采用 `design.md` 的方案（共享健康选址 + 出口模式 `failoverOutput`
组合输出 + 入口模式透明转发 + `.route` 持久化 + 两个独立标签页/API）。

## 1. 入口配置与持久化

- [x] 1.1 新增 `tun/route_config.go`：`RouteConfig`（`UUID, Name, Disabled, Mode, Listen, Members, Policy, CreatedAt`），`Mode` 取 `output|input`。
- [x] 1.2 `tun/config.go` 新增 `.route` 持久化（`routeFile/createRouteFile/deleteRouteFile/writeRouteFileAtomic/replaceRouteFile/loadAllRouteFile`），复用原子写入 + 备份/回滚；并修正 `loadAllFile` 后缀过滤 bug（原 `suffix != suffix` 恒假，会导致 `.route` 误载 `.tun`）。
- [x] 1.3 `RouteConfig.validateBasic()`：名称/Listen/Member 非空、Policy 默认 `priority`；按 Mode 校验 Listen 形态（output 带协议可解析、input 为裸 `host:port`）。

## 2. 共享健康选址逻辑

- [x] 2.1 `tun/route_selector.go`：`routeSelector.candidates()` 按 `Members` 顺序、经 `Manager.GetService` 惰性解析，`memberAlive` 跳过禁用/非 running/`down`。
- [x] 2.2 `tryForward(fn)`：对候选依次尝试「转发」，失败顺延，返回首个成功，全部失败报错。
- [x] 2.3 `preferred()` 计算当前首选 member，供管理列表显示。

## 3. 出口模式（failoverOutput 组合输出）

- [x] 3.1 `tun/output_failover.go`：`failoverOutput` 实现 `output` 接口，`GetStream()` 用选址+顺延委托给所选 member 的活动 output（经 `streamDialer`/`dialerOf` 访问 member 的 output）。
- [x] 3.2 `GetProbeStream`/`GetBandwidthStream` 委托所选 member；`QualitySummary`/`QualitySnapshot`/`FrameHeaderEnabled`/`ProbeConfig` 报告首选 member 状态；`Run()`/`Close()` 空操作。
- [x] 3.3 出口模式数据面：`routeService.buildOutputMode()` 用 `newInput` 按 `Listen`（带协议）构造输入，`handleOutputStream` 走与 `Server.handleInputStream` 同构的 crypto/建管流程。

## 4. 入口模式（透明转发）

- [x] 4.1 入口模式数据面在 `tun/route.go`：`runInputMode`/`serveInput`/`handleInputConn` 裸 TCP 监听，每条新连接按选址 `net.Dial` 到所选 member 的 input 监听地址，`relayConn` 双向透明转发。
- [x] 4.2 协议一致性校验：`manager.validateRouteLocked` 解析各 member `Cfg().Input` 协议，input 模式要求全部一致，否则报错。

## 5. 入口 service 与 manager 接线

- [x] 5.1 `routeService` 构造接收 `*Manager`（`newRouteService(cfg, mgr)`），惰性解析 member；未改动现有 `NewServer` 签名。
- [x] 5.2 `Manager` 新增 `AddRoute/ReplaceRouteByUUID/SetRouteDisabled/RemoveRoute/RemoveRouteByUUID/GetRoute/GetRouteByUUID/AllRoute`，共用锁，带回滚持久化 `.route`；`AllRoute` 返回 `RouteService` 只读视图供跨包使用。
- [x] 5.3 `Manager.Run()` 一并加载并按 Mode 启动 `.route` 入口；`validateRouteLocked` 强制名称唯一（含与 tunnel 重名）与监听冲突校验。

## 6. 管理 API

- [x] 6.1+6.2 新建 `admin/controller/route` 包（两模式共用，按 mode 参数化：`List/Create/Edit/Delete/SetDisabled` 闭包工厂，返回 `response.Ret`）。
- [x] 6.3 `admin/model/route.go`：`model.Route` DTO，含各 member 健康、当前首选、无健康数据 member 列表；`routeToModel`/`modelToRoute` 互转。
- [x] 6.4 `admin/route/route.go` 注册 `/api/route_output/*` 与 `/api/route_input/*`（各 5 个端点），均 `authMgr.RequireAuth`；校验错误经 panic→500 携带中文信息返回。

## 7. 管理界面（两个独立标签页）

- [x] 7.1 `admin/view/layout/default.html`：`----Menus-Add-----` 加「智能路由-出口」「智能路由-入口」两菜单项，`----Routes-Add-----` 加 `route_output_list`/`route_input_list` 两路由。
- [x] 7.2 `admin/view/route_output/list.html`：列表（名称/状态/监听/有序 member+健康/当前首选/操作），创建/编辑弹窗监听带协议、member 有序多选（取自 `/api/tunnel/list`）。
- [x] 7.3 `admin/view/route_input/list.html`：同上，监听为裸 `host:port`，并对 member 协议一致性、无健康数据 member 给出可见提示。
- [x] 7.4 两页面接入各自 `/api/route_*/*`，复用 layout 的 axios 401 拦截跳登录。

## 8. 测试与验证

- [x] 8.1 `tun/route_test.go` 单测共享选址：首选优先、跳过 `down`、unknown 仍可选、非 running 跳过、失败顺延、全部失败报错、无候选报错、恢复回切首选。（8 个用例全过）
- [x] 8.2 出口模式 `failoverOutput` 选址/顺延经共享选址单测覆盖（取流委托即选址回调）；入口模式 relay 选址经 `tryForward` 单测覆盖（真实 relay 需活 tunnel，见 8.5 手工验证）。
- [x] 8.3 `Manager` route CRUD + 持久化往返（含 `.route`/`.tun` 互不干扰）+ 协议一致性校验单测全过；`TestManager_RouteCRUD`/`TestRoutePersistence_RoundTrip`/`TestManager_InputModeProtoConsistency` 通过。
- [x] 8.4 校验逻辑经 `validateBasic`/`validateRouteLocked` 单测覆盖（认证由 `authMgr.RequireAuth` 中间件统一保证，与现有 tunnel 端点一致）。
- [x] 8.5 `go build ./...` 通过；`go vet ./tun/ ./admin/...` 无 route 相关问题（仓库存在 `json:"mode""` 等 pre-existing vet 警告，与本次无关）。
  - 说明：`tun` 包若干**网络依赖的既有测试**（`Test_TcpMuxTun`/`Test_Frp_*`/`Test_Socks5X`/`TestManagerAddServiceClosesServiceWhenCreateFileFails`/`TestManagerReplaceServiceByUUIDRollsBackOnRunFailure`）在本沙箱环境在 master 上即失败/挂起，属 pre-existing，与本次改动无关；本次新增测试全部通过。
  - 手工端到端验证（建 `http@0.0.0.0:1000 → [goai_tcpmux, hk_tcpmux]` 出口入口与 `0.0.0.0:1001 → [同协议 members]` 入口入口，验证首选路由/首选下线自动切换/恢复回切）需在真实运行环境进行，本沙箱无法绑真实远端 tunnel。
