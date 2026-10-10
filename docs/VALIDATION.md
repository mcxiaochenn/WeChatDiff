# 验证记录

验证日期：2026-10-10。此文记录实际执行结果；尚未执行的检查不计为通过。

## 自动测试范围

Go 测试覆盖从 APK ZIP 到 Manifest/DEX 字符串、差分、压缩快照和 Markdown 的完整基础流程；比较仅压缩方式变化的包，确认构建噪声不会形成证据。测试包含同版本不同构建、ABI/签名拒绝条件、DEX 越界与 MUTF-8、腾讯 URL 边界、Manifest 资源 ID 噪声、AI 输入预算、有效/虚假证据 ID、HTTP 429/500 降级与不可信 Markdown 转义。

Go 当前包含 11 个测试入口及其子用例；Python 的 7 项测试覆盖 Release API 失败、已发布版本不可覆盖、缺失附件不能公开、草稿重试、重复草稿选择和 digest 校验。本地 `go test -cover ./...`、`go vet ./...` 和 Python 测试通过；[网关配置 CI](https://github.com/mcxiaochenn/WeChatDiff/actions/runs/38033777675) 的 race、coverage、vet、格式检查、Python 测试与构建全部通过。

## 真实官方 APK

| 身份 | 旧版本 | 新版本 |
| --- | --- | --- |
| versionName | 8.0.78 | 8.0.79 |
| versionCode | 3180 | 3200 |
| ABI | arm64-v8a | arm64-v8a |
| APK SHA-256 | 41f7dc1f720767fa78fa20dd13ea034b817bbf6ebd23dfd1324c647499c9c1ba | f2a725bc04302ad218a563533ae79356e66b64aa711b275bc919a8d432f7e15d |

来源为腾讯官网下载地址：

- 旧：[weixin8078android3180_0x28004e32_arm64.apk](https://dldir1v6.qq.com/weixin/android/weixin8078android3180_0x28004e32_arm64.apk)
- 新：[weixin8079android3200_0x28004f30_arm64_1.apk](https://dldir1v6.qq.com/weixin/android/weixin8079android3200_0x28004f30_arm64_1.apk)

首次本地基础流程成功生成 44,291 条静态证据，未调用 AI，未声称这些变化为已上线功能。随后发现大量 Manifest theme 地址变化，因此将其归类为资源引用噪声并排除出 AI 输入；这是根据真实 APK 结果做出的修正。

两个包均通过 Android SDK apksigner 校验，证书 SHA-256 一致：`0fe4ff85c215918396dadc7cd8ce6963339af33d37751a56e54c7206b63a3c7c`。

整包 JADX 在 3 GiB 堆下发生 OutOfMemoryError，因此正式实现改为逐 DEX 反编译，保持内存预算。部分方法无法反编译时报告明确标记覆盖不完整，只比较两边都已生成的类，不将覆盖缺失当作类新增或删除。

独立交叉核验：

- 使用 Android SDK `aapt2 dump xmltree` 检查两个包：`ChatroomSearchManagerUI`、`ExdeviceSportCalendarUI`、`AgentAuditScanResultActivity` 仅出现在新包 Manifest，与差分报告的新增声明一致。该事实不证明对应功能已经对用户启用。
- 使用 Android NDK `llvm-objcopy --dump-section .text` 独立提取 `lib/arm64-v8a/libAdvanceP2P.so` 代码节并计算 SHA-256，旧值 `504c6f21c6ec94e5f5c12548bfffca038a28fce249d1fcd3b0017707b80c89c2`、新值 `d672b5a95bd97ef66d8b8ea70ebf5c76d30f3a38e80f6cb7b4ddc5a140958fba`，均与 Go 差分结果完全一致。代码节变化本身不证明具体 P2P 功能修复。

## 远端分析、发布与幂等验收

[真实分析任务](https://github.com/mcxiaochenn/WeChatDiff/actions/runs/38031393874) 的下载、签名校验和新旧完整 APK 差分步骤成功，生成 177,406 条证据、新包 237,829 个反编译类索引及 80 个有界代码摘录。代码覆盖不完整，未将 173,865 个未共同匹配的路径声称为新增/删除。292 项 Manifest 资源 ID 地址变化与签名噪声单独标注。引用 ID 唯一；最终证据摘录没有 JADX 失败占位代码和临时目录注释。

该任务的旧发布脚本因 GitHub 按 tag 查询无法读取草稿而失败；问题已经修复，增加了按附件完整性选择草稿、按已校验 Release ID 公开和从可信 Artifact 恢复发布的入口。实际恢复测试还覆盖了同 tag 重复草稿场景，附件校验阻止了空草稿公开。

[发布恢复任务](https://github.com/mcxiaochenn/WeChatDiff/actions/runs/38032681044) 成功自动公开 [8.0.79 报告](https://github.com/mcxiaochenn/WeChatDiff/releases/tag/apk-f2a725bc04302ad218a563533ae79356e66b64aa711b275bc919a8d432f7e15d)。已从公开 Release 下载全部五个附件，逐一核验 GitHub 资产 digest，并核验 SHA256SUMS 的四条记录；全部匹配。新旧证书一致，前序 APK 身份与下载 SHA-256 一致，机器可读报告可以解压并解析。未调用真实 AI。

[重复普通监测](https://github.com/mcxiaochenn/WeChatDiff/actions/runs/38032938504) 成功恢复 Latest 快照，重新下载当前官网 APK，识别此 SHA-256 已发布，跳过后续反编译、AI 和发布。公开 Release 数量保持 1。新维护版 Actions 和固定 ubuntu-24.04 环境均完成实际运行验证。

一次验证产生的额外空草稿保留为未公开状态，不影响 Latest、已发布 SHA-256 去重或公开报告；脚本优先保护已公开版本，不会因同 tag 残留草稿覆盖它。

## 限制

静态证据不代替功能运行验证；报告不会据此声称聊天、支付等用户功能已改变。

## 真实 AI 网关验证

用户授权后，已配置 `DEEPSEEK_API_KEY` Repository Secret、`DEEPSEEK_BASE_URL=https://sub2api.mcxiaochen.top/v1` 和 `DEEPSEEK_MODEL=deepseek-flash`。模型列表接口返回 `deepseek-flash` 与 `deepseek-v4-pro`，真实 Chat Completions 请求支持当前 JSON 和关闭 thinking 参数。

直接使用已公开的 8.0.78 → 8.0.79 结构化报告验证同一个 Go AI 客户端，没有重新反编译或发送完整 APK/源码。人工调试共发出 4 次请求，其中 2 次因输出校验不通过降级；一个已定位原因是单条结论引用 12 个 ID，超过 8 个的上限。提示词补充引用上限、旧/新代码方向和禁止据纯混淆改名推测行为调整，未放宽校验或启用自动重试。

最终请求输入 35 条证据、23,943 字节证据 JSON，生成 8 条通过 ID、分类、长度与引用数量校验的中文推测。网关返回用量：prompt 8,640、completion 629、total 9,269 Token。核对引用后，新增组件与群聊退出结果文案都有对应 Manifest/资源变化；模型没有声称 Bug 已修复。保活等代码仍有混淆与截断，模型列出的内部变化不证明行为调整。ID 校验不能自动证明语义正确，生成结论始终标记为推测。

AI 验证报告保存于本地忽略目录 `.work/ai-validation/`，原已公开 Release 保持不变；后续新 SHA-256 的监测会使用已保存网关配置。凭据通过隐藏输入传递并保存为 GitHub Secret，没有写入项目文件或报告。
