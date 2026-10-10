# 验证记录

验证日期：2026-10-10。此文记录实际执行结果；尚未执行的检查不计为通过。

## 自动测试范围

Go 测试覆盖从 APK ZIP 到 Manifest/DEX 字符串、差分、压缩快照和 Markdown 的完整基础流程；比较仅压缩方式变化的包，确认构建噪声不会形成证据。测试包含同版本不同构建、ABI/签名拒绝条件、DEX 越界与 MUTF-8、腾讯 URL 边界、Manifest 资源 ID 噪声、AI 输入预算、有效/虚假证据 ID、HTTP 429/500 降级与不可信 Markdown 转义。

Python 的 5 项测试覆盖 Release API 失败、已发布版本不可覆盖、缺失附件不能公开、草稿重试和 digest 校验。本地 `go test -cover ./...`、`go vet ./...` 和 Python 测试通过；首轮远端 [CI](https://github.com/mcxiaochenn/WeChatDiff/actions/runs/38031023259) 的 race、coverage、vet、格式检查、Python 测试与构建全部通过。

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

代码差分、远端发布及幂等运行结果将在完成后补充。

## 限制

没有 DeepSeek Key 时不能证明真实 API 与模型效果，只能通过 mock HTTP 验证请求、预算、引用校验及失败降级。静态证据不代替功能运行验证；报告不会据此声称聊天、支付等用户功能已改变。
