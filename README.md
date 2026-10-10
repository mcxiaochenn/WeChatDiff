# WeChatDiff

[![测试](https://github.com/mcxiaochenn/WeChatDiff/actions/workflows/ci.yml/badge.svg)](https://github.com/mcxiaochenn/WeChatDiff/actions/workflows/ci.yml)
[![监测与发布](https://github.com/mcxiaochenn/WeChatDiff/actions/workflows/monitor.yml/badge.svg)](https://github.com/mcxiaochenn/WeChatDiff/actions/workflows/monitor.yml)

自动监测微信 Android 官方完整 APK，以实际差分证据生成中文报告，并发布到 [GitHub Releases](https://github.com/mcxiaochenn/WeChatDiff/releases)。支持 DeepSeek 分析；没有 API Key 时仍可运行完整基础差分与发布流程。

这是新建的独立项目，不是 Fork。主要语言为 Go，使用 JADX 生成临时代码索引，以 Releases 保存历史状态。没有 Web 前端，也不追踪热补丁、动态配置或服务端灰度。

## 已实现能力

| 范围 | 实际分析内容 |
| --- | --- |
| 官方版本监测 | 解析 `weixin.qq.com` 中最高版本的 arm64 下载链接；从 APK Manifest 读取版本和 versionCode，以完整 SHA-256 区分同版本不同构建 |
| Manifest | 权限、组件、intent-filter、SDK 与属性的结构化差分；将纯资源引用地址变化单独标注 |
| DEX | 文件字节哈希、跨 DEX 分包的字符串集合变化、中文文字与可解释标识；忽略字符串索引重排与部分编译器标识 |
| SO | 文件哈希、ELF `.text` 哈希、已定义动态导出符号集合；不以 build-id 作为语义依据 |
| 资源与 assets | 解压内容 SHA-256 与大小、资源表全局字符串池变化，涵盖微信混淆后的 `r/` 资源目录 |
| 反编译代码 | JADX simple 模式，规范化逐文件哈希；为部分命名明确的微信类生成新增/移除行摘录 |
| AI | 有界结构化证据输入、JSON 输出、证据 ID 校验、错误降级；所有 AI 功能解释均为推测 |
| 历史与发布 | SHA-256 tag、前序 APK 身份、签名连续性、压缩快照、草稿附件校验后公开、失败重试与去重 |

报告分开列出**已确认静态变化**和**AI 推测**。字符串出现、组件新增、SO 字节改变可以确认；“新功能已上线”“Bug 已修复”无法仅凭静态差分确认。本项目不会把模型自报的置信度升级为事实。有效证据引用仍不保证模型的解释正确，发布报告中的 AI 结论需人工或运行时验证。

## 快速部署

完整步骤见 [部署指南](docs/DEPLOYMENT.md)。

1. 新建一个公开 GitHub 仓库并上传本项目（无需 Fork）。
2. 可选添加 Repository Secret：`DEEPSEEK_API_KEY`。自动发布使用 GitHub 提供的 `GITHUB_TOKEN`，无需 PAT。
3. 在 Actions 中手动运行「监测与发布」。首次默认建立基线，不声称有更新内容。也可提供旧 APK 官方地址 `old_url`，首次直接生成真实版本对比。
4. 定时任务每天 UTC 01:23、13:23（北京时间 09:23、21:23）执行。GitHub 调度可能延迟。

默认 AI 模型为低成本 `deepseek-flash`，关闭 thinking。模型名称可通过 Repository Variable `DEEPSEEK_MODEL` 调整。名称与价格以 [DeepSeek 官方文档](https://api-docs.deepseek.com/quick_start/pricing/) 为准。

使用兼容 DeepSeek Chat Completions 的网关时，设置 Repository Variable `DEEPSEEK_BASE_URL`（例如 `https://网关域名/v1`）；Key 仍只放在 `DEEPSEEK_API_KEY` Secret。请求禁止跟随重定向，避免认证信息被转发。

## 本地运行

需要 Go 1.26.4 或更新版本。基础差分不需要 Java；代码差分需要 Java 17+ 和 JADX 1.5.6。签名验证使用 Android SDK Build Tools 的 `apksigner`。

```bash
go test ./...
go build -o wechatdiff ./cmd/wechatdiff

# 本地两个完整 APK：不向 Git 提交 APK。
./wechatdiff --old old.apk --new new.apk --out out

# 从官网监测；有旧快照时自动恢复旧 APK。
./wechatdiff --baseline previous/snapshot.json.gz --out out \
  --jadx /path/to/jadx/bin/jadx \
  --apksigner /path/to/android-sdk/build-tools/35.0.0/apksigner

# 首次历史对比：两个 URL 均须来自腾讯 HTTPS 域名，版本由 APK 读取。
./wechatdiff --old-url 'https://dldir1v6.qq.com/weixin/android/OLD.apk' \
  --new-url 'https://dldir1v6.qq.com/weixin/android/NEW.apk' --out out
```

Windows 可使用 `.bat` 工具入口并设置 `JAVA_HOME`。无工具时基础差分会注明覆盖缺失；Actions 强制启用 JADX 与 apksigner。临时目录为 `.work/`，默认每次使用独立子目录，避免混入前一次反编译文件。

## 发布附件与证据

- `CHANGELOG.md`：中文可读报告，包含有界证据摘录。
- `diff.json.gz`：完整结构化变化与证据 ID、路径、前后哈希或值、AI 解释及用量。
- `snapshot.json.gz`：下次分析的结构化基线，保存文件哈希、Manifest、筛选字符串、SO 信息、代码哈希；不含完整反编译源码。
- `metadata.json`：当前身份、前序身份及发布 tag。
- `SHA256SUMS`：上述四个附件的校验和。

tag 使用 `apk-<完整 SHA-256>`。同一包重复检测不会重复反编译、调用 AI 或创建 Release。全部已发布 tag 会分页读取；不依赖易失缓存，也不由 Actions 修改 Git 历史。机器记录在每个 Release 内形成前序身份链。

## 准确性、成本与失败行为

下载必须来自腾讯 HTTPS 域名，重定向仍受同样约束；检查 ZIP、包名 `com.tencent.mm`、主 DEX、Manifest 身份及 ABI。Actions 通过 apksigner 验证签名，并拒绝与旧基线证书不一致的包。首次证书信任来源是官方 HTTPS 下载，不内置硬编码证书信任库；合法签名轮换也会停止发布，需要人工审查。

ZIP 时间戳、压缩方式、顺序、签名条目和版本元数据不进入功能推测。DEX 文本集合跨分包比较；SO `.text` 和导出符号不包含 ELF build-id。资源 ID、编译器生成内容和混淆名称仍可能产生噪声；没有宣称完全消除混淆。

JADX 逐 DEX 反编译，合并类路径索引，避免整包交叉引用分析耗尽内存；跨 DEX 类型解析可能不完整。每个 APK 总计最长 12 分钟，2 个线程、3 GiB Java 堆。部分失败或超时时仅比较两边都有输出的类，其他路径标注为覆盖差异，不声称为新增/删除。代码摘录采用有界行集合差分，不做完整控制流分析，也不保证所有修改都进入 AI 输入。纯行序变化会改变代码哈希，但可能没有可解释行摘录。

每次 AI 最多 64 条证据、24,000 字节 JSON 证据输入、1,800 个输出 Token、90 秒超时；固定提示词另计。这是字节预算而非精确 Token 计数，用量由 API 返回后记录。每次分析最多一次 AI 请求，避免自动重试造成重复计费。无 Key、限流、API 失败、输出截断或无效证据 ID 时保留基础报告。APK 内容被视为不可信数据，模型不执行工具；生成文本经过 Markdown/HTML 转义。

旧 APK URL 失效或内容被替换时，使用已发布快照并注明无法恢复旧代码片段。新包下载/身份/签名失败，跨 ABI 对比，以及状态恢复失败会终止，不公开错误报告。首次只建立基线；首次历史对比必须能够取得旧 APK。发布中途失败保留草稿；重跑补齐附件后再公开。已公开的 Release 不会被自动覆盖。

GitHub 免费资源额度、定时任务停用规则与外网下载可用性仍适用。每天两次下载完整包用于 SHA-256 检查，不仅依赖不可靠的 HEAD/ETag；未变化时省略反编译和 AI。当前主线仅监测 arm64，不做多架构版本矩阵。

## 来源、许可证与鸣谢

本项目代码使用 [MIT](LICENSE)。微信 APK 及其内部内容属于原权利人，MIT 不授予这些内容的再分发权；项目不把 APK 或完整反编译源码提交到 Git，也不将 APK 作为自己的 Release 附件。

参考了以下项目的官方来源发现和历史索引思路，没有复制其实现代码、页面或完整版本列表：

- [Rodert/wechat-android-versions](https://github.com/Rodert/wechat-android-versions)：Apache-2.0；启发官网来源发现与归档设计。如未来复制实现，必须遵守 Apache-2.0 并保留所需声明与 NOTICE。
- [DJB-Developer/wechat-android-history-versions](https://github.com/DJB-Developer/wechat-android-history-versions)：参考历史官方 APK 地址；检查时 GitHub 未识别许可证，不默认允许复制代码或数据集。
- [canc3s/wechat-versions](https://github.com/canc3s/wechat-versions)：参考微信版本归档思路；检查时 GitHub 未识别许可证，不默认允许复制代码或数据集。

工具与依赖：

- [shogo82148/androidbinary](https://github.com/shogo82148/androidbinary)，MIT，用于二进制 Manifest 解码。完整版权与许可声明保存在 [third_party/androidbinary-LICENSE](third_party/androidbinary-LICENSE)。
- [skylot/jadx](https://github.com/skylot/jadx)，Apache-2.0，独立命令行工具；工作流下载固定版本并核验官方资产 SHA-256，不把工具源码并入本项目。
- Android SDK `apksigner`：由运行环境提供，遵守 Android SDK 及相关工具许可。

分析方法与验证证据见 [验证记录](docs/VALIDATION.md)。
