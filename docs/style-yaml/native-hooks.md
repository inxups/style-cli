---
title: native_hooks：原生 hook
scope: native_hooks
keys: [native_hooks, event, matcher, command, timeout]
spec: style/v0.3
sources:
  - core/spec/native.go
  - core/spec/load.go
  - core/spec/merge.go
  - core/plan/native.go
  - internal/adapter/claude/native.go
  - internal/adapter/codex/native.go
---

# native_hooks：原生 hook

> 把一条 shell 命令直接注册为 harness 的原生 hook，例如编辑文件后自动运行格式化脚本。

## 概览

`rules` 声明的是面向模型的文本，由 STYLE 运行时统一评估。`native_hooks` 则把包作者提供的命令原样写入各 harness 的 hook 配置，由 harness 自己执行。它们的区别：

| | `rules` | `native_hooks` |
|---|---|---|
| 内容 | 给模型的文本 | shell 命令 |
| 事件名 | STYLE 统一事件（如 `tool.post`） | harness 原生事件（如 `PostToolUse`） |
| 跨 harness | 一份声明适配所有 harness | 每个 harness 单独写映射 |
| 降级 | 有降级路径 | 没有降级路径 |
| token 估算 | 计入预算 | 不估算 |

`native_hooks` 只能写在目录包的 STYLE.yaml 中，单文件包不能声明。

```yaml
native_hooks:
  - id: <id>
    description: <说明>
    claude-code:              # harness 名作为键
      event: PostToolUse
      matcher: "Edit|Write"
      command: ./scripts/format.sh
      timeout: 30
    codex:
      event: PostToolUse
      matcher: apply_patch
      command: ./scripts/format.sh
```

## 字段

### `native_hooks`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | mapping 列表 |
| 必填 | 目录包中 `rules` 和 `native_hooks` 至少有一个非空 |
| 默认值 | 无 |
| 可选值 | — |
| 可写入 defaults | 否 |

每一项是一个原生 hook 声明，可以包含以下字段：

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | string | 是 | — | 格式同规则 id，在包内的规则和 hook 之间唯一 |
| `description` | string | 否 | — | 说明 |
| `enabled` | bool | 否 | `true` | `false` 时不安装 |
| `priority` | number | 否 | 包级 `priority`，没有时为 `50` | 排序优先级 |
| `fallback` | string | 否 | `skip` | `skip` 或 `error`，见 [fallback.md](fallback.md) |
| `<harness>` | mapping | 至少一个 | — | 某个 harness 的映射，键是已注册的 harness 名 |

约束：

- 必须是列表，每项必须是 mapping（诊断码 `invalid-package`）。
- `id` 必须有效且唯一（诊断码 `bad-id`、`dup-id`）。
- `description`、`enabled`、`priority`、`fallback` 类型或取值错误（诊断码 `native-field`）。
- 不能使用规则字段或包级字段，例如 `target`、`text`（诊断码 `native-field`）。
- 至少要有一个 harness 映射（诊断码 `native-mapping`）。
- 其他键必须是已注册的 harness 名（诊断码 `native-field`）。

### `event`

| 属性 | 值 |
|---|---|
| 位置 | native_hooks.\<harness\> |
| 类型 | string |
| 必填 | 是 |
| 默认值 | 无 |
| 可选值 | 该 harness 的原生事件名，见下表 |
| 可写入 defaults | 否 |

| 原生事件 | Claude Code | Codex | 接受 matcher |
|---|---|---|---|
| `SessionStart`、`SessionEnd`、`UserPromptSubmit` | ✓ | ✓ | 否 |
| `PreToolUse`、`PostToolUse`、`PermissionRequest` | ✓ | ✓ | 是 |
| `PostToolUseFailure` | ✓ | | 是 |
| `Stop`、`SubagentStart`、`SubagentStop`、`PreCompact`、`PostCompact` | ✓ | ✓ | 否 |
| `UserPromptExpansion`、`Notification`、`FileChanged` | ✓ | | 否 |

约束：

- 必须是非空字符串（诊断码 `native-event`），且是该 harness 支持的事件（诊断码 `native-event`）。

### `matcher`

| 属性 | 值 |
|---|---|
| 位置 | native_hooks.\<harness\> |
| 类型 | string |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | 正则表达式；Claude Code 还接受 `*` |
| 可写入 defaults | 否 |

按原生工具名筛选，语法由 harness 决定。两个 harness 都按正则表达式匹配。

约束：

- 必须是非空单行字符串，不带首尾空白（诊断码 `native-matcher`）。
- 只能用于接受 matcher 的事件（诊断码 `native-matcher`）。
- 必须是有效的正则表达式（诊断码 `native-matcher`）。

### `command`

| 属性 | 值 |
|---|---|
| 位置 | native_hooks.\<harness\> |
| 类型 | string |
| 必填 | 是 |
| 默认值 | 无 |
| 可选值 | 单行 shell 命令 |
| 可写入 defaults | 否 |

harness 触发事件时执行的命令。安装时会在命令前加上一个所有权前缀（`: --owner=…;`），用于识别和卸载 STYLE 写入的 hook。

约束（诊断码 `native-command`）：

- 不能为空，必须是单行，不能包含 NUL，不能带首尾空白。
- 不能以 `;`、`&`、`|`、`)`、`#` 开头。
- 引号必须闭合，不能以未转义的反斜杠结尾。

这些限制保证加上所有权前缀后，命令的含义不变。

### `timeout`

| 属性 | 值 |
|---|---|
| 位置 | native_hooks.\<harness\> |
| 类型 | integer |
| 必填 | 否 |
| 默认值 | `60`（秒） |
| 可选值 | 1 到 600 的整数 |
| 可写入 defaults | 否 |

约束：

- 必须是 1 到 600 的整数（诊断码 `native-timeout`）。

## 行为

1. **只支持四个映射字段**：`event`、`matcher`、`command`、`timeout`。`async`、`onFailure`、`type`、`url`、`prompt` 等 harness 扩展字段都不支持，诊断信息会说明原因（`core/spec/native.go` `nativeExtensionReason`）。
2. **不读 defaults**：原生 hook 的默认值固定为 `enabled: true`、`priority` 取包级值、`fallback: skip`（`core/spec/merge.go`）。
3. **计划**（`core/plan/native.go`）：
   - 没有当前 harness 的映射：`skipped`，与 `fallback` 无关。
   - adapter 能写入：`native-hook`，并显示事件、matcher 和最终的超时时间。
   - adapter 不能写入（例如在 Windows 上，原生命令需要 POSIX 环境）：`fallback: skip` 时为 `skipped`，`fallback: error` 时为 `install-error`。
4. **计划不显示命令**：`style plan` 的输出只包含事件、matcher 和超时，不包含命令内容。
5. **不估算 token**：原生 hook 的输出不计入预算，`plan` 的 `Token` 段会列出 `native hook output not estimated`。

## 示例

### 最小示例

```yaml
spec: style/v0.3
name: nh-min
version: 0.1.0
description: native hook minimal example
native_hooks:
  - id: format-on-edit
    claude-code:
      event: PostToolUse
      matcher: "Edit|Write"
      command: ./scripts/format.sh
```

### 多个 harness 与 fallback

```yaml
spec: style/v0.3
name: nh-ok
version: 0.1.0
description: native hooks example
native_hooks:
  - id: format-on-edit
    description: Run the formatter after file edits
    claude-code:
      event: PostToolUse
      matcher: "Edit|Write"
      command: ./scripts/format.sh
      timeout: 30
    codex:
      event: PostToolUse
      matcher: apply_patch
      command: ./scripts/format.sh
  - id: session-log
    fallback: error
    claude-code:
      event: SessionStart
      command: echo "session started" >> /tmp/style-session.log
```

`style plan nh-ok --harness codex,claude-code` 的结果（节选）：

```text
Harness: codex
Native hooks:
- format-on-edit: native-hook (native)
  -> native PostToolUse matcher apply_patch; timeout 60s
- session-log: skipped (native)
  ! no native mapping for this harness

Harness: claude-code
Native hooks:
- format-on-edit: native-hook (native)
  -> native PostToolUse matcher Edit|Write; timeout 30s
- session-log: native-hook (native)
  -> native SessionStart; timeout 60s
```

`session-log` 没有 Codex 映射，所以即使是 `fallback: error`，在 Codex 上也只是跳过。

## 校验与诊断

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-package` | error | `native_hooks` 不是列表，或某项不是 mapping |
| `bad-id` | error | `id` 缺失或格式无效 |
| `dup-id` | error | `id` 与规则或其他 hook 重复 |
| `native-field` | error | 字段类型错误；使用了规则或包级字段；不支持的扩展字段；未注册的 harness |
| `native-mapping` | error | 没有任何 harness 映射，或映射不是 mapping |
| `native-event` | error | 缺少 `event`，或该 harness 不支持这个事件 |
| `native-matcher` | error | matcher 格式无效、正则无效，或用在不接受 matcher 的事件上 |
| `native-command` | error | 命令为空或违反格式限制 |
| `native-timeout` | error | `timeout` 不是 1 到 600 的整数 |
| `native-format-removed` | error | 使用了已移除的 `target: native` 规则写法 |

## 相关

- [fallback：落点不可用时的处理](fallback.md)
- [rules：规则来源与正文](rules.md)
- [target：作用对象](target.md)
- [diagnostics：诊断码总表](diagnostics.md)
