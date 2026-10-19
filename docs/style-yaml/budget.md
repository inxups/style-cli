---
title: budget：token 预算
scope: package
keys: [budget, max_tokens]
spec: style/v0.3
sources:
  - core/spec/validate_package.go
  - core/spec/validate_lints.go
  - core/tokens/analyze.go
  - core/tokens/budget.go
  - core/tokens/installed.go
  - internal/app/command.go
---

# budget：token 预算

> 限制规则注入给模型的文本量：单条规则、单个事件、常驻内容各有上限，超出时报警告或错误。

## 概览

STYLE 会离线估算每条规则输出给模型的 token 数，并与预算比较。预算写在包级 `budget` 里；单条规则可以用 `max_tokens` 单独设置上限。

| 预算 | 比较对象 | 默认值 | 诊断码 |
|---|---|---|---|
| `per_rule` | 单条规则的估算值（规则的 `max_tokens` 默认取这个值） | 400 | `T001` |
| `per_event` | 同一个事件上所有规则的合计 | 1200 | `T002` |
| `always_on` | 常驻内容的合计 | 600 | `T004` |

`budget.on` 决定超出预算时报 warning 还是 error。

```yaml
budget:
  per_rule: 400
  per_event: 1200
  always_on: 600
  on: warn
rules:
  - id: <id>
    max_tokens: 200      # 只影响这一条规则
```

## 字段

### `budget`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | mapping |
| 必填 | 否 |
| 默认值 | `per_rule: 400`、`per_event: 1200`、`always_on: 600`、`on: warn` |
| 可选值 | 键只能是 `per_rule`、`per_event`、`always_on`、`on` |
| 可写入 defaults | 否 |

- `per_rule`：单条规则的默认上限，也是规则 `max_tokens` 的内置默认值。
- `per_event`：同一个事件（例如 Claude Code 的 `PostToolUse Bash`）上所有规则估算值的合计上限。
- `always_on`：常驻内容的合计上限。常驻内容包括写入静态指令位置的规则，以及没有 `when` 的 `session.start` 规则。
- `on`：`warn`（默认）或 `error`。

约束：

- 必须是 mapping，不能有其他键（诊断码 `invalid-schema`）。
- `per_rule`、`per_event`、`always_on` 必须是正数（诊断码 `invalid-schema`）。
- `on` 只能是 `warn` 或 `error`（诊断码 `invalid-schema`）。

### `max_tokens`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | number |
| 必填 | 否 |
| 默认值 | `budget.per_rule`，没有时为 `400` |
| 可选值 | 非负数字 |
| 可写入 defaults | 是 |

这条规则估算值的上限。

约束：

- 必须是非负数字（诊断码 `invalid-schema`）。

## 行为

1. **估算对象**：估算的是模型实际收到的文本（`core/spec/validate_lints.go` `modelEstimate`）。
   - inject 规则：渲染后的正文。
   - warn 规则：`model_text`；没有 `model_text` 时取正文的第一句。
   - block 规则：拒绝理由，即正文。`target: event` 且事件不是 `tool.pre` 的 block 规则估算为 0。
   - `event.*` 模板变量按最坏情况估算（512 个字符，每个字符取最大转义宽度），单个变量约 1800 tokens。使用事件内容的规则通常需要放宽 `max_tokens` 和 `per_event`。
2. **规则上限**：`max_tokens` 依次取规则字段、`defaults.max_tokens`、`budget.per_rule`、`400`。超出时报 `T001`（`core/spec/validate_lints.go`、`core/tokens/analyze.go`）。
3. **事件和常驻合计**：按 harness 分别计算。`per_event` 按原生事件（和工具）分桶累加，超出时报 `T002`；常驻内容累加，超出时报 `T004`（`core/tokens/analyze.go`）。
4. **级别**：`on: warn` 时上述诊断都是 warning；`on: error` 时都是 error。
5. **命令行为**（`internal/app/command.go`）：
   - `style validate` 检查 `T001`，并对每个已注册的 harness 估算 `T002`、`T004`。出现 error 时退出码为 1。
   - `style plan` 先做校验：`T001` 为 error 时校验失败，退出码为 1，不输出计划。校验通过后在 `Token` 段报告估算值和诊断；`T002`、`T004` 为 error 时退出码为 4，计划被拒绝。
6. **安装后的总量**：安装、更新、同步、卸载等需要重新编译已安装包的操作，会把同一 harness 上所有已安装包的输出按事件合计。超过 adapter 输入上限的 80% 时报 `T003`（error），操作被阻止。adapter 上限可用 `style adapters` 查看，例如 Codex 约 2500 tokens，Claude Code 10000 字符（`core/tokens/installed.go`）。
7. **估算是近似值**：估算是离线计算的，输出标注 `estimated; calibration pending`，与模型实际计费的 token 数可能不同。

## 示例

### 最小示例

```yaml
spec: style/v0.3
name: bg-min
version: 0.1.0
description: budget minimal example
budget:
  per_rule: 200
rules:
  - id: house-style
    target: system
    text: Answer in English.
```

### 超出预算的警告

```yaml
spec: style/v0.3
name: bg-warn
version: 0.1.0
description: budget example
budget:
  per_rule: 10
  per_event: 15
  always_on: 12
rules:
  - id: house-style
    target: system
    text: Answer in English and keep every answer short, direct and free of filler words.
  - id: tests
    target: system
    max_tokens: 50
    text: Run the full test suite before you report that a change is complete.
```

`style validate bg-warn` 通过（退出码 0），但报告警告：

```text
T001 STYLE.yaml#house-style: estimated model input exceeds max_tokens (24 > 10)
T004 claude-code: always-on estimate 45 exceeds always_on 12
T004 codex: always-on estimate 45 exceeds always_on 12
```

`tests` 用 `max_tokens: 50` 放宽了自己的上限，所以没有 `T001`。两条都是静态 system 规则，属于常驻内容，不计入 `per_event`。

### 超出事件预算时阻止安装

```yaml
spec: style/v0.3
name: bg-event
version: 0.1.0
description: per-event budget example
budget:
  per_event: 30
  on: error
rules:
  - id: search-tips
    target: tool
    tool: shell
    text: Prefer rg over grep and fd over find when searching the repository.
  - id: output-tips
    target: tool
    tool: shell
    text: Summarize long command output instead of pasting it back verbatim.
```

两条规则都在 shell 工具调用后注入。`style plan bg-event --harness claude-code` 的 `Token` 段（节选）：

```text
  PostToolUse Bash: 40 tokens / 135 chars
  T002: event estimate 40 exceeds per_event 30
```

因为 `on: error`，退出码为 4，计划被拒绝。

## 校验与诊断

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-schema` | error | `budget` 结构、键或取值无效；`max_tokens` 不是非负数字 |
| `T001` | 由 `on` 决定 | 单条规则估算值超过 `max_tokens` |
| `T002` | 由 `on` 决定 | 同一事件的合计超过 `per_event` |
| `T003` | error | 所有已安装包在某事件上的合计超过 adapter 输入上限的 80% |
| `T004` | 由 `on` 决定 | 常驻内容合计超过 `always_on` |

## 相关

- [package：包级元数据](package.md)
- [defaults：包级默认值](defaults.md)（`defaults.max_tokens`）
- [action：动作与时机](action.md)（warn 的 `model_text`）
- [diagnostics：诊断码总表](diagnostics.md)
