# 版本发布流程

将代码提交后，推送 `v主版本.次版本.补丁版本` 格式的标签，例如：

```sh
git tag -a v0.3.5 -m "发布 v0.3.5"
git push --atomic origin master refs/tags/v0.3.5
```

`.github/workflows/release.yml` 会自动执行测试，按标签对应的 `go.mod` 配置 Go，编译以下 7 种组合：

| 系统 | 架构 | 附件格式 |
| --- | --- | --- |
| Linux | 386、amd64、arm64 | `.tar.gz` |
| Windows | 386、amd64 | `.zip` |
| macOS | amd64、arm64 | `.tar.gz` |

所有平台构建成功后，工作流自动创建并发布 GitHub Release，上传压缩包、各压缩包的 `.md5` 文件和汇总的 `SHA256SUMS`，无需手动创建草稿。压缩包名称保持 `gotun-版本-系统-架构`，二进制的 `-version` 输出与标签一致。

正式版本标记为 Latest；包含预发布后缀的标签（如 `v0.4.0-rc.1`）发布为预发布版本，不覆盖 Latest。任一测试或构建失败，都不会进入 Release 发布步骤。

已推送的旧标签不会因工作流更新而自动补发。在 Actions → Publish Go Releases → Run workflow 中选择 `master`，填写现有标签即可补发；补发同样检出该标签的源码。失败后可以重跑，已有 Release 会补充或更新同名附件。若仓库启用了不可变 Release，已公开的附件应通过新版本修复。

工作流使用内置 `GITHUB_TOKEN`，仅发布任务拥有 `contents: write` 权限，不需要新增个人访问令牌。Docker 镜像仍由独立的标签工作流发布。
