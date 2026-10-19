---
title: fallback：落点不可用时的处理
scope: rule
keys: [fallback]
spec: style/v0.3
sources:
  - core/spec/validate_rules.go
  - core/spec/merge.go
  - core/spec/native.go
  - core/plan/requirements.go
  - core/plan/plan.go
  - core/plan/events.go
  - core/plan/policy.go
  - core/plan/system.go
  - core/plan/native.go
---

# fallback：落点不可用时的处理

> 规则想用的事件或输出能力在某个 harness 上不受支持时，`fallback` 决定是换一种方式安装、跳过这条规则，还是拒绝安装整个包。

## 概览

`style plan` 和 `style install` 会针对每个 harness 计算每条规则的落点（placement），也就是规则在哪个事件、用哪种输出能力生效。不同 harness 支持的能力不一样，例如 Codex 在 `session.start` 只能输出 add-context，Claude Code 在 `subagent.start` 没有任何输出能力。`style adapters` 可以查看当前机器上各 harness 的能力。

当首选落点不可用时，`fallback` 决定接下来怎么做：

- `degrade`（默认）：沿降级路径换一种落点安装。
- `skip`：不安装这条规则，其余规则照常安装。
- `error`：把这条规则标为安装错误，整个包都不能安装。

同一个包在不同 harness 上可能得到不同结果，计划按 harness 分别给出。

```yaml
defaults:
  fallback: degrade        # 包级默认值，见 defaults.md
rules:
  - id: <id>
    fallback: skip         # 规则级，覆盖默认值
native_hooks:
  - id: <id>
    fallback: error        # 原生 hook 只能取 skip 或 error
```

## 字段

### `fallback`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | string |
| 必填 | 否 |
| 默认值 | `degrade` |
| 可选值 | `degrade` / `skip` / `error` |
| 可写入 defaults | 是 |

决定这条规则在某个 harness 上缺少落点时的处理方式。各取值对应的计划状态如下：

| 取值 | 部分落点不可用 | 全部落点不可用 |
|---|---|---|
| `degrade` | 用可用的部分安装，状态为 `partial` 或 `degraded` | 尝试降级路径；仍无落点时为 `skipped`（system 规则为 `install-error`） |
| `skip` | 整条规则 `skipped` | `skipped` |
| `error` | `install-error`，包被拒绝安装 | `install-error`，包被拒绝安装 |

约束：

- 取值必须是 `degrade`、`skip`、`error` 之一（诊断码 `invalid-schema`）。

### `native_hooks[].fallback`

| 属性 | 值 |
|---|---|
| 位置 | native_hooks |
| 类型 | string |
| 必填 | 否 |
| 默认值 | `skip` |
| 可选值 | `skip` / `error` |
| 可写入 defaults | 否（`defaults` 只作用于规则） |

原生 hook 没有降级路径。adapter 无法写入该 hook 时，`skip` 跳过它，`error` 拒绝安装整个包。hook 没有当前 harness 的映射时总是跳过，与 `fallback` 无关。

约束：

- 只能取 `skip` 或 `error`（诊断码 `native-field`）。

## 行为

### 计划状态

| 状态 | 含义 |
|---|---|
| `native` | 按首选落点安装 |
| `degraded` | 经过降级后安装，落点上的 `degraded_from` 记录原本的事件或 `static` |
| `partial` | 只安装了一部分落点或输出能力 |
| `skipped` | 不安装这条规则 |
| `never-trigger` | 规则在该 harness 上永远不会触发，不安装 |
| `native-hook` | 原生 hook 已映射 |
| `install-error` | 安装错误；只要有一条，计划就显示 `Plan rejected`，整个包不能安装 |

### degrade 的降级路径

以下步骤只在 `fallback: degrade` 时生效。

1. **block 降为 warn**：首选事件不支持 deny 时，`action: block` 改为 `warn`，`repeat` 改为 `every`（`core/plan/policy.go` `evaluate`）。
2. **warn 只输出一部分**：warn 需要 add-context 或 system-message。只支持其中一种时，仍然安装，状态为 `partial`。`skip` 和 `error` 会把这种情况视为不可用（`core/plan/policy.go` `evaluate`）。
3. **tool / mcp / skill 换事件**：首选事件不可用时，改用另一个事件，状态为 `degraded`（`core/plan/events.go` `planHook`）。
   - `at: before`：先 `tool.pre`，再 `tool.post`。
   - `at: after`：先 `tool.post`，再 `tool.pre`。
   - `action: block` 没有备用事件。
4. **tool / mcp / skill 降为静态指令**：所有事件都不可用时，把规则写成常驻指令，并加上前缀，例如 `When using shell: `（`core/plan/events.go` `planEvents`）。只有同时满足以下条件才走这一步：
   - `action: inject`
   - 正文不含运行时变量
   - 没有 `when`
   - `refresh` 为 0
5. **skill 的两个落点**：skill 规则同时挂在隐式事件和 `skill.explicit` 上。只有一个可用时，状态为 `partial`；`skip` 会去掉整条规则（`core/plan/events.go` `planEvents`）。
6. **静态 system 规则**（inject、正文不含运行时变量、没有 `when`、`refresh` 为 0）：
   - 优先写入 harness 的静态指令位置。
   - 没有静态位置时，改用 `session.start`，`degraded_from` 为 `static`。
   - `session.start` 也不可用时，改用 `prompt.submitted`，并设 `repeat: session`。
   - `mode: replace` 不降级，因为 hook 只能追加内容，做不到替换（`core/plan/system.go` `planSystem`、`planStaticSystemFallback`）。
7. **动态 system 规则**：需要 `session.start` 和 `prompt.submitted` 两个落点。只有一个可用时，用它安装，状态为 `degraded`。两个都不可用时为 `install-error`，即使是 `degrade` 也不会悄悄丢掉规则；只有 `skip` 会跳过（`core/plan/system.go` `planSystem`）。
8. **event 规则**（`target: event`）：只有一个事件，没有备用事件；除第 1、2 步外不降级。

### 不受 fallback 控制的情况

- **条件数据缺失**：`when` 需要的数据在该事件上取不到时，结果是 `never-trigger`；只有 `fallback: error` 时为 `install-error`（`core/plan/events.go`、`core/plan/system.go`）。
- **harness 不匹配**：`when.harness` 不包含当前 harness 时，结果是 `skipped`（`core/plan/plan.go` `planRule`）。
- **adapter 盲区**：adapter 观察不到规则的目标时，结果是 `never-trigger`。例如 Codex 只观察 `shell`、`file.edit`、`mcp` 等固定工具（`core/plan/plan.go` `planRule`）。
- **`fail: closed`**：只要落点经过降级、是静态指令，或原事件不支持 deny，就是 `install-error`。降级不能削弱失败保护（`core/plan/plan.go` `planRule`）。
- **`refresh` 大于 0**：不会降为静态指令。system 规则必须有可输出的 `prompt.submitted` 落点。当前平台不支持 refresh 会话锁时，规则按 `fallback` 处理：`error` 为 `install-error`，其他取值为 `skipped`。

## 示例

### 最小示例

```yaml
spec: style/v0.3
name: fb-min
version: 0.1.0
description: fallback minimal example
rules:
  - id: shell-guidance
    target: tool
    tool: shell
    fallback: skip
    text: Prefer rg over grep.
```

这条规则只在 harness 支持 `tool.post` 注入时安装，否则直接跳过，不会改到 `tool.pre`。

### 三种取值的对比

```yaml
spec: style/v0.3
name: fb-warn
version: 0.1.0
description: warn at session start with each fallback
rules:
  - id: warn-degrade
    target: event
    event: session.start
    action: warn
    text: Run the test suite before committing.
  - id: warn-skip
    target: event
    event: session.start
    action: warn
    fallback: skip
    text: Run the test suite before committing.
  - id: warn-error
    target: event
    event: session.start
    action: warn
    fallback: error
    text: Run the test suite before committing.
```

`style plan fb-warn --harness codex,claude-code` 在 Codex 0.159.3 上的结果（节选）：

```text
- warn-degrade: partial (warn event)
  -> session.start warn (partial); capabilities: add-context
     only one warn output capability is supported
- warn-skip: skipped (warn event)
  ! warn output is partial at session.start
  ! fallback:skip removes the rule because one or more placements are unavailable
- warn-error: install-error (warn event)
  ! warn output is partial at session.start
Plan rejected: one or more rules prevent installation.
```

Codex 在 `session.start` 只支持 add-context，所以三条规则得到三种结果。Claude Code 2.1.301 两种输出都支持，三条规则都是 `native`。

### 没有降级路径的事件

```yaml
rules:
  - id: subagent-degrade
    target: event
    event: subagent.start
    text: Keep subagent reports short.
```

Claude Code 2.1.301 在 `subagent.start` 没有输出能力，计划结果为 `skipped`，原因是 `no supported placement`。把 `fallback` 改为 `error` 后为 `install-error`，整个包不能安装。Codex 支持这个事件，结果为 `native`。

> 计划结果取决于本机检测到的 harness 版本。以上输出来自 Codex 0.159.3 和 Claude Code 2.1.301。

## 校验与诊断

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-schema` | error | 规则或 `defaults` 中的 `fallback` 不是 `degrade`、`skip`、`error` |
| `native-field` | error | `native_hooks[].fallback` 不是 `skip` 或 `error` |

`install-error` 和 `skipped` 是计划状态，不是校验诊断，`style validate` 不会报告它们。用 `style plan` 查看。

## 相关

- [defaults：包级默认值](defaults.md)
- [action：动作与时机](action.md)
- [target：作用对象](target.md)
- [when：条件匹配](when.md)
- [native_hooks：原生 hook](native-hooks.md)
