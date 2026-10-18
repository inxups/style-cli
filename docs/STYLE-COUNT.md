# 文本计数

`count` 读取 UTF-8 普通文件；`plan` 计编译后的各条模型文本；`explain` 计离线重放最终输出。默认预算、session_cost、运行时硬上限和 stats 继续使用原有估算。

```sh
style count rules.txt --json
style count rules.txt --count=exact --count-provider=openai --encoding=o200k_base --json
style plan ./my-style --harness=codex --count=exact --count-provider=openai --encoding=cl100k_base --json
style explain --harness=codex --event=PreToolUse --payload=payload.json --index=index.json --count=exact --count-provider=openai --encoding=o200k_base --json
```

本地编码明确选择 `o200k_base` 或 `cl100k_base`。词表随二进制内置，无需 key、Python 或运行时下载。实现固定为 `github.com/tiktoken-go/tokenizer@v0.7.0`；特殊 token 拼写按普通文字计。结果报告 encoding、实现版本、UTF-8 摘要、字节/字符和 token 数，不推断模型编码，也不声称完整请求或账单精确。

文件最多 256 KiB，保留换行和全部字节，拒绝 symlink、非普通文件、无效 UTF-8。本地 exact 还拒绝超过 4,096 UTF-8 字节的连续空白或非空白段：固定版 BPE 在超长段内使用二次复杂度合并。程序明确报错，不分段或截断后宣称精确。estimate 和 Anthropic 路径不受该局部编码限制。

`plan` 在解析 vars/settings、warn 的 model_text/首句与正常编译后分别计 placement。编译器既有去重规则保持，同包歧义重复项仍分别保留；不能相加后称完整请求计数。任一保留的运行时模板会在联网前拒绝整次 exact，提示用 `explain`。native 命令输出、宿主管理标记与外层请求排除。

`explain` 沿原有私有状态和显式 facts 求值，计 cap/去重/合并后的 additional context；工具 block 计拒绝原因，prompt/skill block 不计模型输入。无输入报告 `scope: not-model-input` 和 0，不读取 key 或请求网络。原生 wire 保持，计数失败使诊断命令失败。

## Anthropic 接口

```sh
style count rules.txt --count=exact --count-provider=anthropic --count-model=YOUR_MODEL_ID --json
```

这条命令会发送所选文本。调用时从 `ANTHROPIC_API_KEY` 读取独立 API key，固定 HTTPS Count tokens 接口和版本 `2023-06-01`，封成一个 user message；不读取 Claude 订阅凭据。请通过本地安全环境配置 key，不放进命令、仓库或报告。

结果 `scope: anthropic-messages-request` 包含该最小消息请求的封装开销，披露模型、接口版本和请求摘要；它不是纯正文增量或两份请求的差值。文件空文本拒绝。没有自定义 endpoint、自动重试或重定向；15 秒超时、响应上限 1 MiB。缺 key/参数、网络、HTTP 或无效计数均报错，不静默退回估算，错误不回显服务端正文或 key。

[离线 A15 报告](review/M3-A15-2026-10-18.json)覆盖独立 Python golden、两套完整词表摘要、API transport fixture 和命令兼容。Anthropic 真实 API 验收已由用户延期；官方校准 CARRY-01 仍两家各 0/40。许可见[第三方说明](../third_party/README.md)。

2026-10-18 的 [sub2api 中转兼容测试](review/M3-TOKEN-RELAY-2026-10-18.json)使用仓库外的私有配置，对短英文、短中文、较长混合文本与重复请求取得 16、24、3216、16；有效整数、非空正数、长度变化和重复稳定检查通过。[探针](review/M3-TOKEN-RELAY-PROBE-2026-10-18.py)可使用自己的私有配置复跑。该结果不修改生产命令的固定 endpoint，也未验证中转的官方上游来源；REAL-03/TOKEN-01 保持部分完成，CARRY-01 仍各 0/40。
