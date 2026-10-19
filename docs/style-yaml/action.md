---
title: action：动作与时机
scope: rule
keys: [action, at, repeat, fail, refresh, reinject_on_compact]
spec: style/v0.3
sources:
  - core/spec/validate_rules.go
  - core/spec/refresh.go
  - core/spec/merge.go
  - core/plan/requirements.go
  - core/plan/plan.go
  - core/compile/entries.go
  - core/engine/process.go
  - core/state/dedup.go
  - core/state/state.go
---

# action：动作与时机

> 规则命中后做什么（注入、提示、阻止）、在调用前还是调用后执行、多久重复一次，以及处理器出错时是否放行。

## 概览

| 字段 | 作用 | 默认值 |
|---|---|---|
| `action` | 命中后的动作 | `inject` |
| `at` | tool / mcp / skill 规则在调用前还是调用后评估 | block 为 `before`，其余为 `after` |
| `repeat` | 去重粒度 | warn 为 `every`，inject 为 `session`，block 不去重 |
| `fail` | STYLE 处理器出错时放行还是阻止 | 未设置，等同 `open` |
| `refresh` | 每隔 N 轮重新注入 | 未设置，等同 `0`（不刷新） |
| `reinject_on_compact` | 上下文压缩后重新注入 | system 规则为 `true`，其余为 `false` |

```yaml
rules:
  - id: <id>
    target: tool
    tool: shell
    action: block
    at: before
    fail: closed
    text: <拒绝理由>
```

## 字段

### `action`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | string |
| 必填 | 否 |
| 默认值 | `inject` |
| 可选值 | `inject` / `warn` / `block` |
| 可写入 defaults | 是 |

| 动作 | 效果 | 需要的能力 |
|---|---|---|
| `inject` | 把正文作为上下文交给模型 | add-context |
| `warn` | 向用户显示完整正文（system-message），同时把 `model_text`（没有时取正文第一句）交给模型（add-context） | 两者至少一个 |
| `block` | 拒绝本次调用或提示，正文作为拒绝理由 | deny |

约束：

- `block` 只能用于 `tool`、`mcp`、`skill` 规则，以及 `event: tool.pre` / `prompt.submitted`（诊断码 `action-target`）。
- block 的拒绝理由建议不超过 40 tokens（诊断码 `T005`，warning）。

### `at`

| 属性 | 值 |
|---|---|
| 位置 | 规则（`target: tool` / `mcp` / `skill`） |
| 类型 | string |
| 必填 | 否 |
| 默认值 | `action: block` 时为 `before`，否则为 `after` |
| 可选值 | `before` / `after` |
| 可写入 defaults | 是 |

`before` 在工具调用前评估（`tool.pre`），`after` 在工具调用成功后评估（`tool.post`）。`fallback: degrade` 时，首选事件不可用会改用另一个，见 [fallback.md](fallback.md)。

约束：

- block 规则不能设为 `after`（诊断码 `block-after`）。

### `repeat`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | string |
| 必填 | 否 |
| 默认值 | warn 为 `every`，inject 为 `session`；block 不使用 |
| 可选值 | `every` / `turn` / `session` |
| 可写入 defaults | 是 |

| 取值 | 去重方式 |
|---|---|
| `every` | 不去重，每次命中都输出 |
| `turn` | 同一轮对话中，同一规则的同一内容只输出一次 |
| `session` | 同一会话中，同一规则的同一内容只输出一次 |

约束：

- block 规则的 `repeat` 会被忽略（诊断码 `block-repeat`，warning）。
- inject 规则使用 `every` 或 `turn` 时应写 `reason`（诊断码 `T007`，warning）。

### `fail`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | string |
| 必填 | 否 |
| 默认值 | 未设置，等同 `open` |
| 可选值 | `open` / `closed` |
| 可写入 defaults | 是 |

STYLE 自身的处理器在某个事件上出错（例如安装索引无效，无法可靠评估规则）时：

- `open`：放行，不输出。
- `closed`：拒绝该事件。保护范围是整个原生事件，`when` 条件不会缩小这个范围。

约束：

- `closed` 只能用于能直接拒绝的评估点：`at: before` 的 tool / mcp / skill 规则，或 `event: tool.pre` / `prompt.submitted`（诊断码 `fail-closed-target`）。
- 计划时，原评估点必须支持 deny，且不能经过降级或改为静态指令；否则为 `install-error`（原因 `fail-closed-unsupported`）。

### `refresh`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | integer |
| 必填 | 否 |
| 默认值 | 未设置，等同 `0`（不刷新） |
| 可选值 | 0 到 1000000 的整数 |
| 可写入 defaults | 是 |

正数表示每隔 N 个已提交的用户轮次重新输出一次。第一次命中立即输出，之后从上次输出起，每满 N 轮且规则再次命中时输出。计数以 `prompt.submitted` 为时钟；到期的输出要等规则原本的评估点再次命中时才发出。

约束：

- 必须是 0 到 1000000 的整数，小数和布尔值都无效（诊断码 `refresh-invalid`）。
- 正数时要求：动作是 inject 或 warn，`repeat: session`，不是 `mode: replace`，事件不是 `subagent.start`（诊断码 `refresh-incompatible`）。
- inject 规则使用正数 `refresh` 时应写 `reason`（诊断码 `T007`，warning）。

### `reinject_on_compact`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | bool |
| 必填 | 否 |
| 默认值 | `target: system` 时为 `true`，否则为 `false` |
| 可选值 | `true` / `false` |
| 可写入 defaults | 是 |

`true` 时，上下文压缩后这条规则的 `session` 去重重新计数，下次命中时重新输出；`session.start` 规则还会自动把 `compact` 加入 `sources`。

约束：

- 只对 `repeat: session` 有效（诊断码 `reinject-unused`，warning）。

## 行为

1. **去重的键**：`repeat: session` 的键由会话代次、压缩轮次（仅 `reinject_on_compact: true` 时计入）、规则 id 和输出内容的哈希组成。`repeat: turn` 的键由会话代次、轮次 id（harness 不提供时用提示计数）、规则 id 和输出内容的哈希组成，不受压缩影响。两种键都包含输出内容，所以正文渲染结果变化时会再次输出（`core/state/dedup.go` `repeatName`）。
2. **会话来源**（`core/state/state.go`）：
   - `startup`：开始全新的作用域，清空去重记录。
   - `clear`：会话代次加一，`session` 去重重新开始。
   - `compact`：压缩轮次加一，只影响 `reinject_on_compact: true` 的规则。
3. **不去重的情况**：block 规则、`repeat: every`、`subagent.start` 事件永远不去重（`core/state/dedup.go` `Claim`）。
4. **warn 的输出**：评估点只支持一种输出能力时，只发出对应的那部分：只有 add-context 时用户看不到提示，只有 system-message 时模型收不到文本（`core/engine/process.go`）。
5. **block 降级**：`fallback: degrade` 且评估点不支持 deny 时，block 改为 warn，`repeat` 改为 `every`（`core/plan/policy.go`）。
6. **refresh 的计划**：system 规则使用 refresh 时必须有可输出的 `prompt.submitted` 落点；refresh 规则不会降为静态指令。`style plan` 会列出刷新策略及其限制（`core/plan/types.go` `DescribeRefresh`）。

## 示例

### 最小示例

```yaml
spec: style/v0.3
name: ac-min
version: 0.1.0
description: action minimal example
rules:
  - id: force-push-guard
    target: tool
    tool: shell
    action: block
    when:
      conditions:
        - field: tool.input.command
          op: shell_regex
          pattern: "^git push .*(--force|-f)"
    text: Force push is not allowed.
```

### 各种动作与重复策略

```yaml
spec: style/v0.3
name: ac-all
version: 0.1.0
description: actions example
rules:
  - id: lint-reminder
    target: tool
    tool: file.edit
    action: warn
    model_text: Run the linter after editing.
    text: Edited a file. Run the linter before committing; CI rejects lint errors.
  - id: force-push-guard
    target: tool
    tool: shell
    action: block
    fail: closed
    when:
      conditions:
        - field: tool.input.command
          op: shell_regex
          pattern: "^git push .*(--force|-f)"
    text: Force push is not allowed.
  - id: plan-reminder
    target: system
    refresh: 10
    reason: Long sessions drift from the plan format.
    when:
      model: "*"
    text: Keep plans to five steps or fewer.
  - id: per-turn
    target: event
    event: prompt.submitted
    repeat: turn
    reason: Remind on every turn.
    text: State assumptions before acting.
```

- `lint-reminder`：用户看到完整提示，模型只收到 `model_text`。
- `force-push-guard`：调用前评估；STYLE 处理器出错时也会拒绝这次 shell 调用。
- `plan-reminder`：带 `when` 和 `refresh`，是动态 system 规则，挂在 `session.start` 和 `prompt.submitted` 上，每 10 轮重新注入。
- `per-turn`：每轮最多注入一次。

`style plan ac-all --harness claude-code` 中 `plan-reminder` 的刷新策略：

```text
  refresh: every 10 submitted turns since last emission; first match immediately; matched points only; full per-emission budget
     prompt-count-fallback without native turn IDs
     no child cadence without child prompt callbacks
     due output waits for its original matching point
```

## 校验与诊断

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-schema` | error | `action`、`at`、`repeat`、`fail` 取值无效；`reinject_on_compact` 不是布尔值 |
| `action-target` | error | block 用在不支持的目标或事件上 |
| `block-after` | error | block 规则设为 `at: after` |
| `fail-closed-target` | error | `fail: closed` 用在不能直接拒绝的评估点 |
| `refresh-invalid` | error | `refresh` 不是 0 到 1000000 的整数 |
| `refresh-incompatible` | error | 正数 `refresh` 与动作、`repeat`、`mode` 或事件不兼容 |
| `block-repeat` | warning | block 规则声明了 `repeat` |
| `reinject-unused` | warning | `reinject_on_compact: true` 但 `repeat` 不是 `session` |
| `T005` | warning | block 拒绝理由超过 40 tokens |
| `T007` | warning | 重复注入的 inject 规则缺少 `reason` |

## 相关

- [target：作用对象](target.md)
- [when：条件匹配](when.md)
- [fallback：落点不可用时的处理](fallback.md)
- [rules：规则来源与正文](rules.md)（`model_text`、`reason`）
- [defaults：包级默认值](defaults.md)
