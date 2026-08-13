# CLAUDE.md

本文件是 Claude Code 在本仓库的工作指引。通用 agent 约定见 [AGENTS.md](AGENTS.md)；
OpenSpec 流程与产物模板以 `openspec` CLI 与 [openspec/config.yaml](openspec/config.yaml)
为准。

## 智能体配置双侧同步

- CLAUDE.md 与 AGENTS.md 必须保持内容一致（仅首行标题分别为 # CLAUDE.md / # AGENTS.md）；修改任一文件必须在同一变更中同步另一文件。
- .claude/skills/ 与 .agents/skills/ 必须保持内容一致；新增、修改或删除任一侧 skill，必须在同一变更中镜像到另一侧。
- 发现两侧不一致时，以最近一次有意修改的一侧为准补齐另一侧，并在提交说明中注明。

## 语言约定（强制）

**OpenSpec 的所有规划文档必须用【简体中文】撰写**：`proposal.md`、`design.md`、
`tasks.md`，以及 `openspec/specs/**` 下的所有 `spec.md`（含增量 delta）。

**保持英文/原样**（否则 `openspec validate` 失败或 apply 无法解析）：

- spec 增量小节标题 `## ADDED/MODIFIED/REMOVED/RENAMED Requirements`
- `### Requirement:` 与 `#### Scenario:`（恰好 4 个 `#`）标签
- `- **WHEN**` / `- **THEN**` 场景标记
- `- [ ]` 任务复选框
- 代码标识符、文件路径、API 路由、配置键等技术内容

## OpenSpec 要点

- 用 CLI 驱动：`openspec status --change "<name>" --json` 看构建顺序，
  `openspec instructions <artifact> --change "<name>" --json` 取模板与规则。
- propose 只产出规划文档、不改项目代码；实现走 apply 工作流（显式触发）。
- 每个产物写完运行 `openspec validate "<name>"` 保持 valid。
- 归档用 `openspec archive "<name>"`（位置参数；`--change` 只用于
  `status`/`instructions`）。

## 代码约定

- Go；注释与日志用中文，遵循现有风格。
- 提交信息用中文 + conventional commits（如 `feat:...`）。

## 常用命令

- 构建：`go build ./...`
- 静态检查：`go vet ./...`
- 测试：`go test ./...`
- OpenSpec 校验：`openspec validate "<change-name>"`
