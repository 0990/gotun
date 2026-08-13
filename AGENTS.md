# AGENTS.md

本文件面向所有在本仓库工作的编码 agent（Codex、Claude、Copilot 等）。Claude 专用
指引见 [CLAUDE.md](CLAUDE.md)；OpenSpec 工作流细则见
[openspec/config.yaml](openspec/config.yaml)。

## 智能体配置双侧同步

- CLAUDE.md 与 AGENTS.md 必须保持内容一致（仅首行标题分别为 # CLAUDE.md / # AGENTS.md）；修改任一文件必须在同一变更中同步另一文件。
- .claude/skills/ 与 .agents/skills/ 必须保持内容一致；新增、修改或删除任一侧 skill，必须在同一变更中镜像到另一侧。
- 发现两侧不一致时，以最近一次有意修改的一侧为准补齐另一侧，并在提交说明中注明。

## 语言约定（强制）

**所有 OpenSpec 规划文档一律使用【简体中文】撰写。** 包括：

- 变更产物：`openspec/changes/<name>/` 下的 `proposal.md`、`design.md`、`tasks.md`
- 规格：`openspec/specs/**/spec.md` 及各变更下的增量 spec

### 例外（保持英文/原样，不得翻译）

以下结构性关键字是 OpenSpec 校验（`openspec validate`）所依赖的，**必须保持英文**：

- spec 增量小节标题：`## ADDED Requirements`、`## MODIFIED Requirements`、
  `## REMOVED Requirements`、`## RENAMED Requirements`
- 标签：`### Requirement:`、`#### Scenario:`（**恰好 4 个 `#`**）
- 场景标记：`- **WHEN**`、`- **THEN**`
- 任务复选框：`- [ ]`（apply 阶段据此解析进度）

代码标识符、文件路径、API 路由、配置键、Go 结构体/接口/函数名等技术内容也保持
原文，不译。

## OpenSpec 工作流

- 规划产物由 `openspec` CLI 脚手架生成；用 `openspec status --change "<name>" --json`
  与 `openspec instructions <artifact> --change "<name>" --json` 获取构建顺序与模板。
- **规划边界**：propose 工作流只创建规划产物，不要在此阶段改动项目代码；实现需显式
  进入 apply 工作流。
- 每个产物写完后用 `openspec validate "<name>"`（必要时 `--strict`）校验，必须保持
  valid。
- 归档：`openspec archive "<name>"`（变更名是位置参数；**不要**加 `--change`，那是
  `status`/`instructions` 的 flag）。

## 代码约定

- Go；遵循现有代码风格。注释与日志信息使用中文，与现有代码一致。
- 提交信息用中文 + conventional commits 风格（如 `feat:...`、`fix:...`）。

## 项目速览

- 隧道以 JSON 文件持久化，由 `tun.Manager` 在内存中以名称（name）为键管理。
- 管理后台：原生 `net/http` ServeMux + Vue 2（http-vue-loader，经 `/render` 加载
  `admin/view/` 下的 SFC）。新增页面/路由/菜单需重新编译 Go 二进制（embed FS）。
