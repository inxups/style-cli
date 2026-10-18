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
style count rules.txt --count=exact --count-provider=anthropic --count-model=claude-haiku-5-5 --json
```

这条命令会发送所选文本。调用时从 `ANTHROPIC_API_KEY` 读取独立 API key，固定 HTTPS Count tokens 接口和版本 `2023-06-01`，封成一个 user message；不读取 Claude 订阅凭据。请通过本地安全环境配置 key，不放进命令、仓库或报告。

结果 `scope: anthropic-messages-request` 包含该最小消息请求的封装开销，披露模型、接口版本和请求摘要；它不是纯正文增量或两份请求的差值。文件空文本拒绝。没有自定义 endpoint、自动重试或重定向；15 秒超时、响应上限 1 MiB。缺 key/参数、网络、HTTP 或无效计数均报错，不静默退回估算，错误不回显服务端正文或 key。

[离线 A15 报告](review/M3-A15-2026-10-18.json)覆盖独立 Python golden、两套完整词表摘要、API transport fixture 和命令兼容。Anthropic 官方 API 已于 2026-10-18 补测通过，见[真实验收](review/M3-TOKEN-OFFICIAL-2026-10-18.json)与[可复跑生产 CLI 检查](review/M3-TOKEN-OFFICIAL-VERIFY-2026-10-18.py)：count/plan/explain 共 7 项，计数 17、21、583、17，重复一致、缺 key 失败与输入保全通过。官方估算校准 CARRY-01 现为 Anthropic 40/40、OpenAI 0/40，详见下节。许可见[第三方说明](../third_party/README.md)。

2026-10-18 用户已撤回中转方案，改用 Anthropic 官方 API；独立中转入口、配置和私有临时测试材料已清理，见[官方接入准备](review/M3-TOKEN-OFFICIAL-SETUP-2026-10-18.json)。此前[中转兼容结果](review/M3-TOKEN-RELAY-2026-10-18.json)仅保留历史事实，不作为当前接入步骤或官方验收。官方真实产品计数现已通过，REAL-03/TOKEN-01 关闭；历史中转结果不计入官方校准。

## 固定语料估算校准

2026-10-18 按用户“先只测试claudecode”的范围，用官方 `claude-haiku-5-5` 完成固定 40 条规则的校准。估算取自已验生产 CLI `plan`；官方请求保留相同 user message 和 system 前缀，只加入单条规则，以“加入规则后计数 − 空规则基线计数”作为参考。基线为 24；40 条计数与基线/中英文首尾共 5 项复测均成功，46 次请求全部 HTTP 200，复测值一致。原始语料与生产估算系数保持不变。

| 语料 | 条数 | STYLE 估算合计 | 官方增量合计 | 合计相对偏差 | 逐条平均绝对误差 |
|---|---:|---:|---:|---:|---:|
| 英文 | 20 | 626 | 643 | -2.64% | 6.21% |
| 中文 | 20 | 579 | 651 | -11.06% | 10.87% |
| 全部 | 40 | 1,205 | 1,294 | -6.88% | 8.54% |

合计相对偏差为 `(估算合计 − 官方增量合计) / 官方增量合计`，负数表示低估。逐条平均绝对误差先对每条计算 `abs(估算 − 官方增量) / 官方增量` 再平均；最大为 18.92%，p95 为 17.95%。28 条低估、8 条高估、4 条一致，中文 20 条全部低估。结果只描述这组短规则及指定模型，没有约定准确率通过阈值，不据此承诺其他语料的误差范围或自动调整预算系数。

逐条文本/请求/响应摘要与计数见[实测报告](review/M3-CARRY-01-ANTHROPIC-2026-10-18.json)，重跑方式见[采集器说明](../testdata/manual/README.md#m3-carry-01claude-官方校准)。此项使用官方计数接口，不生成回复，思考强度不参与计数；不作为新的 Claude Code hook 投递验收。OpenAI 官方计数仍为 0/40；后续自定义服务的原生对照见下节，CARRY-01 整体继续部分完成。

## Codex 自定义 Responses 原生对照

2026-10-18 用户选择自己的 API 地址，并指定隔离的原生 codex-cli、直接 Responses。固定 40 条语料全部尝试：39 条取得计数，1 条被服务策略拒绝（`en-concurrency`）。拒绝项没有计数，正文保留；下表只统计有效样本。这是自定义服务经 Codex CLI 返回的用量对照，不能计入 OpenAI 官方计数接口的 0/40。模型配置为 `gpt-6.1-sol` / low，CLI 为 `codex-cli 0.159.3`；上游模型身份没有独立核实。

所有样本共用新 HOME/CODEX_HOME/XDG/tmp、同一个空工作目录和固定用户消息 `Continue.`。只有规则指令文件内容变化；CLI 实际加载的是去首尾空白后的正文，原生日志逐条核对。额外的固定开发者指令要求只回复 OK，关闭 shell/agent 工具、搜索和原生重试。原生 `token_usage_record.usage` 按 response 身份取得首个请求用量，并与 `token_count.last_token_usage` 交叉核对；减去空基线 5,174 得到规则增量。缓存输入单列，不从输入总量中扣除。

| 语料 | 有效条数 | STYLE 估算合计 | 原生首请求输入增量 | 合计相对偏差 | 逐条平均绝对误差 |
|---|---:|---:|---:|---:|---:|
| 英文 | 19 | 598 | 361 | 65.65% | 66.25% |
| 中文 | 20 | 579 | 439 | 31.89% | 32.92% |
| 全部有效样本 | 39 | 1,177 | 800 | 47.12% | 49.16% |

合计偏差定义与上节一致；正数表示高估。有效样本最大绝对误差为 83.33%（`en-cost-report`），p95 为 78.95%；0 条低估、39 条高估、0 条一致。6 项空基线/中英文首尾复测全部一致，没有准确率通过阈值，生产估算系数保持原样。未取得计数的样本不纳入误差均值或合计。

最终组共 49 次原生 CLI 启动，46 次取得可核对的首请求用量；服务策略拒绝 1 次、零响应传输失败 2 次均保留，传输恢复另记。初始可见消息、实际规则正文、模型/配置模板和输入文件保全已核对；没有截获完整序列化请求，不作完整请求/账单保证。[正式对照报告](review/M3-CARRY-01-CODEX-2026-10-18.json)区分 `suite_executed` / `available_measurements_verified` 与仍为 false 的完整通过状态。

最初的 [turn 汇总法](review/M3-CARRY-01-CODEX-TURN-2026-10-18.json)、[首尾空白守卫](review/M3-CARRY-01-CODEX-TRIM-2026-10-18.json)和[传输中断预检](review/M3-CARRY-01-CODEX-TRANSPORT-2026-10-18.json)保持未通过，不混入正式统计。重跑与离线核对见[手册](../testdata/manual/README.md#m3-carry-01codex-原生误差对照)。
