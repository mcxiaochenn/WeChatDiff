# 部署与维护

## 独立仓库

在 GitHub 创建新的空公开仓库后推送本项目。创建必须是独立仓库，不使用 Fork。仓库 Actions 必须启用；组织策略需要允许 workflow 的 `contents: write`，仅发布 job 请求该权限，CI 为只读。

## Secrets 和 Variables

| 名称 | 类型 | 必须 | 用途 |
| --- | --- | --- | --- |
| `DEEPSEEK_API_KEY` | Repository Secret | 否 | DeepSeek API 调用；缺失时仍会发布基础报告 |
| `DEEPSEEK_MODEL` | Repository Variable | 否 | 默认 `deepseek-flash`，需要支持 chat completions、JSON 与关闭 thinking 参数 |
| `DEEPSEEK_BASE_URL` | Repository Variable | 否 | 默认 `https://api.deepseek.com`；兼容网关通常填写 `https://网关域名/v1`，程序追加 `/chat/completions`。必须 HTTPS，不允许 URL 内嵌凭据 |
| `GITHUB_TOKEN` | Actions 内置 | 是，自动提供 | 发布 Release；不需要另外设置同名 Secret 或 PAT |

```bash
gh secret set DEEPSEEK_API_KEY --repo YOUR_NAME/WeChatDiff
gh workflow run monitor.yml --repo YOUR_NAME/WeChatDiff
gh run list --repo YOUR_NAME/WeChatDiff
```

`gh secret set` 交互输入 Key，不把 Key 写入命令、日志或 Git。不要把 API Key 用作 `DEEPSEEK_MODEL`。

## 首次运行与历史验证

第一次普通运行只发布基线。若希望立即对比当前版本与一个历史版本，手动触发时填入 `old_url`；`new_url` 留空，当前版本始终由官网发现。

若要生成指定两个历史 APK 的报告，可同时填入 `old_url` 和 `new_url`；这类报告发布时使用 `--latest=false`，避免改变日常监测基线。已发布 SHA-256 永远去重，包括没有启用 AI 时发布的报告；配置 Key 后不会自动覆盖旧报告。

## 状态与恢复

状态由 Releases 附件持久化。工作流列出全部已发布 SHA-256 tag，并下载 Latest 的 `snapshot.json.gz`。没有任何 Release 才允许首次建基线；已有状态但恢复失败时直接报错，避免静默丢失历史。

新包未变化：本次无新 Release，分析与发布步骤跳过。新包变化：取得旧 APK 并核验其 SHA-256；旧地址失效或返回不同内容时，改用历史快照进行结构差分并注明代码覆盖缺失。

发布事务：生成四个报告文件 → 生成 SHA256SUMS → 创建/恢复草稿 → 上传五个附件 → 校验远端大小及可用的资产 digest → 公开 Release。任何中间失败都不会公开尚未完整上传的草稿。重跑同一 workflow 即可恢复。不要手动把缺少附件的草稿公开，也不要把无关 Release 设为 Latest。

如果分析已经完成、只有发布失败，可运行「重试发布已生成报告」，填写原「监测与发布」的任务 ID。该入口校验任务来自本仓库 main 分支的监测工作流，再下载完整报告 Artifact，恢复草稿并发布；不会重新下载 APK、反编译或调用 AI。原任务填写了 `new_url` 时，必须勾选 historical，保持历史报告不设为 Latest。Artifact 只保留 14 天，过期后需重新运行分析。

工作流使用同一个 concurrency group 串行运行，且不取消进行中的发布。临时 APK、JADX 源码仅在 runner 工作目录内，不上传；诊断 Artifact 只保留报告与工具日志 14 天。

## 运行条件

- 固定 GitHub `ubuntu-24.04` hosted runner，Go 按 go.mod 安装、Temurin Java 17；避免 ubuntu-latest 迁移改变 Android SDK 布局。Actions 固定当前维护版本的提交 SHA。
- runner 的 Android SDK Build Tools 35.0.0 提供 apksigner。缺失即报错，需要维护工作流中的路径。
- JADX 1.5.6 的 URL 与 SHA-256 固定在 `scripts/install-jadx.sh`；升级时核验官方资产 digest 并同步修改两者。
- 单任务最大 55 分钟；Go 主流程最大 45 分钟；每个 APK 反编译最大 12 分钟。
- 下载重试 3 次，单次最多 10 分钟、包大小上限 1 GiB；AI 最多一次请求、90 秒。

## 停止与回退

可在 GitHub Actions 页面禁用「监测与发布」工作流以停止后续任务，不影响已发布历史。代码回退使用普通新提交或经人工审查的 revert；不需要重置远程历史或删除 Release。签名变更、官网结构变化、快照 schema 不兼容时应先停止发布并检查证据，不使用“无基线”方式绕过错误。
