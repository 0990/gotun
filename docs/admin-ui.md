# 管理后台界面

界面使用浅灰背景、白色内容区与深灰文字，绿色用于按钮、开关和状态，覆盖隧道、智能路由入口/出口、内置服务及登录页面。

文字样式已应用确认的方案 02：名称使用 `#1f2937`，地址使用 `#263443`；桌面地址 14px、手机地址 13px；表头 13px 加粗，协议和辅助说明使用 `#475569` / `#526171`。

- 大于 900px：左侧导航与表格。
- 761–900px：底部导航与双列卡片。
- 不大于 760px：底部导航与单列卡片，弹窗从底部展开。

搜索和状态筛选作用于当前已加载列表；隧道管理默认显示「运行中」。运行状态与连接健康状态分开展示。隧道的详情、探测历史、带宽、MTR、导出、克隆和删除位于每条记录的更多菜单中，导入、探测和检测地址位于检测工具中。探测历史同时记录隧道帧探测与完整链路探测，保留最近 7 天，支持筛选及分页，详见[逐次探测历史](probe-history.md)。路由编辑支持显式调整成员优先级。

## 构建与使用

继续使用现有 Go + Vue 2 + http-vue-loader，不需要前端构建或在线 CDN。共享界面资源在 `admin/resource/dist/css/admin-ui.css` 和 `admin/resource/dist/js/admin-ui.js`，页面在 `admin/view/`。

```powershell
go build -o bin/gotun.exe ./cmd
```

页面和资源使用 embed 打包。部署时需要替换并重启对应二进制，浏览器刷新页面；已有配置文件和 API 格式保持兼容。

## 验证

```powershell
go test ./... -timeout 120s
```

浏览器回归脚本使用真实启动包与动态模板，并提供内存接口，不会修改运行实例的隧道或服务。需本机 Chrome（可用 `PLAYWRIGHT_CHANNEL` 改为 `msedge`）和可解析的 Playwright 模块：

```powershell
# 将以下模块路径替换为本机 Playwright 安装路径；不要改生产配置。
$env:PLAYWRIGHT_MODULE_PATH = '<Playwright 安装目录>/node_modules/playwright'
# 可选：保存测试截图。
$env:ADMIN_UI_SCREENSHOTS = '<截图输出目录>'
node scripts/test-admin-ui.cjs
```

脚本覆盖多种屏幕宽度、可见文字与背景对比度、地址字号、长 IPv6、搜索筛选、表单字段、保存失败重试、成员顺序、启停失败回退、MTR 流关闭和探测离页清理。实际网络传输与认证由现有 Go 测试覆盖；浏览器不访问生产连接。
