# AGENTS.md

本文件面向所有在本仓库工作的编码 agent（Codex、Claude、Copilot 等）。Claude 专用
指引见 [CLAUDE.md](CLAUDE.md)。

## 智能体配置双侧同步

- CLAUDE.md 与 AGENTS.md 必须保持内容一致（仅首行标题分别为 # CLAUDE.md / # AGENTS.md）；修改任一文件必须在同一变更中同步另一文件。
- .claude/skills/ 与 .agents/skills/ 必须保持内容一致；新增、修改或删除任一侧 skill，必须在同一变更中镜像到另一侧。
- 发现两侧不一致时，以最近一次有意修改的一侧为准补齐另一侧，并在提交说明中注明。

## 代码约定

- Go；遵循现有代码风格。注释与日志信息使用中文，与现有代码一致。
- 提交信息用中文 + conventional commits 风格（如 `feat:...`、`fix:...`）。

## 项目速览

- 隧道以 JSON 文件持久化，由 `tun.Manager` 在内存中以名称（name）为键管理。
- 管理后台：原生 `net/http` ServeMux + Vue 2（http-vue-loader，经 `/render` 加载
  `admin/view/` 下的 SFC）。新增页面/路由/菜单需重新编译 Go 二进制（embed FS）。
