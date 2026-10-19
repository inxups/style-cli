---
title: defaults：包级默认值
scope: defaults
keys: [defaults]
spec: style/v0.3
sources:
  - core/spec/validate_package.go
  - core/spec/merge.go
  - core/spec/validate_rules.go
  - core/spec/refresh.go
  - core/spec/validate_lints.go
---

# defaults：包级默认值

> 给包里所有规则统一设置默认值，避免在每条规则里重复同一个字段。

## 概览

`defaults` 是 STYLE.yaml 顶层的一个 mapping。它设置的值会作用于包里的每一条规则，规则自己声明的同名字段优先。`defaults` 只作用于 `rules`，不作用于 `native_hooks`。

规则字段的最终值（effective value）按以下顺序合并，后面的覆盖前面的：

1. 内置默认值
2. `defaults`
3. 规则自身的字段（STYLE.yaml 里的内联规则，或 `rules/*.md` 的 front matter）
4. `from` 条目上的覆盖值

合并之后，再根据结果推导 `at`、`repeat`、`reinject_on_compact`、`mode` 这几个字段的默认值。

```yaml
spec: style/v0.3
name: <name>
version: <version>
description: <description>
priority: 50               # 也是规则 priority 的内置默认值
budget:
  per_rule: 400            # 也是规则 max_tokens 的内置默认值
defaults:
  fallback: skip
  max_tokens: 200
rules:
  - id: <id>
    fallback: degrade      # 覆盖 defaults.fallback
```

## 字段

### `defaults`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | mapping |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | 键只能是下表中的 10 个 |
| 可写入 defaults | — |

为包里的所有规则提供默认值。

| 键 | 内置默认值 | 说明 |
|---|---|---|
| `action` | `inject` | 规则动作 |
| `at` | 推导：`action` 为 `block` 时是 `before`，否则是 `after` | 规则在目标调用前还是调用后生效 |
| `repeat` | 推导：`warn` 为 `every`，`inject` 为 `session`；`block` 不使用 | 去重粒度 |
| `fail` | 未设置，等同 `open` | handler 失败时放行还是阻止 |
| `refresh` | 未设置，等同 `0`（不刷新） | 重新注入的间隔，0 到 1000000 的整数 |
| `reinject_on_compact` | 推导：`target` 为 `system` 时是 `true`，否则是 `false` | 上下文压缩后是否重新注入 |
| `sources` | `[startup, clear, fork]` | 哪些会话来源触发 session.start 规则 |
| `fallback` | `degrade` | 落点不可用时的处理，见 [fallback.md](fallback.md) |
| `priority` | 包级 `priority`，没有时为 `50` | 规则排序优先级 |
| `max_tokens` | `budget.per_rule`，没有时为 `400` | 单条规则的 token 上限 |

约束：

- 必须是 mapping（诊断码 `invalid-schema`）。
- 只能使用上表中的键。`id`、`target`、`file`、`text`、`model_text`、`when`、`mode`、`reason`、`from`、`enabled`、`tool`、`names`、`mcp`、`skill`、`event` 都不能写进 `defaults`（诊断码 `defaults-field`）。
- 值的类型和取值范围与规则字段相同（诊断码 `invalid-schema`；`refresh` 为 `refresh-invalid`）。
- `rules/*.md` 的 front matter 不能声明 `defaults`（诊断码 `rule-file-pkg-field`，同时报 `invalid-schema`）。

## 行为

1. **合并顺序**：内置默认值 → `defaults` → 规则字段 → `from` 覆盖值（`core/spec/merge.go` `Merge`）。
2. **两个包级字段充当内置默认值**：包级 `priority` 和 `budget.per_rule` 分别作为规则 `priority` 和 `max_tokens` 的内置默认值。`defaults.priority` 和 `defaults.max_tokens` 会覆盖它们（`core/spec/merge.go` `Merge`）。
3. **合并后推导**：只有字段在前面各层都没有设置时，才使用推导默认值。推导依据的是合并后的值，所以 `defaults.action: block` 也会让 `at` 推导为 `before`（`core/spec/merge.go` `Merge`）。
   - `at`：`block` 为 `before`，其余为 `after`。
   - `reinject_on_compact`：`target: system` 时为 `true`。
   - `mode`：`target: system` 时为 `append`。
   - `repeat`：`warn` 为 `every`，其余为 `session`。
4. **block 规则不使用 repeat**：最终 `action` 为 `block` 时，合并结果会删除 `repeat`，即使它来自 `defaults`。block 决策永不去重（`core/spec/merge.go` `Merge`）。
5. **禁用的规则**：`enabled: false` 的规则在合并后被移除，但仍然参与校验。`enabled` 不能写进 `defaults`。
6. **native_hooks 不读 defaults**：原生 hook 的默认值固定为 `enabled: true`、`priority` 取包级 `priority`、`fallback: skip`（`core/spec/merge.go` `Merge`）。
7. **逐条校验**：校验看的是每条规则的最终值。`defaults` 里的一个值可能让多条规则同时报错或警告（`core/spec/validate_rules.go` `validateRuleFields`）。
   - `defaults.action: block`：不能 block 的规则（如 `target: system`）报 `action-target`。
   - `defaults.fail: closed`：不在可阻止落点上的规则报 `fail-closed-target`。
   - `defaults.refresh` 大于 0：不满足 refresh 条件的规则报 `refresh-incompatible`。
   - `defaults.repeat`：block 规则报 `block-repeat` 警告；取 `every` 或 `turn` 时，没有 `reason` 的 inject 规则报 `T007` 警告。
   - `defaults.sources`：非 session.start 规则报 `sources-unused` 警告。内置的 `sources` 默认值不会触发这个警告。
   - 值本身无效时，包级和每条继承它的规则都会报错。

## 示例

### 最小示例

```yaml
spec: style/v0.3
name: df-min
version: 0.1.0
description: defaults minimal example
defaults:
  fallback: skip
rules:
  - id: shell-guidance
    target: tool
    tool: shell
    text: Prefer rg over grep.
```

`shell-guidance` 没有声明 `fallback`，最终值取 `defaults` 里的 `skip`。

### 默认值与逐条覆盖

```yaml
spec: style/v0.3
name: df-ok
version: 0.1.0
description: package defaults example
priority: 60
budget:
  per_rule: 300
defaults:
  fallback: skip
  max_tokens: 200
rules:
  - id: shell-guidance
    target: tool
    tool: shell
    text: Prefer rg over grep.
  - id: force-push-guard
    target: tool
    tool: shell
    action: block
    when:
      conditions:
        - field: tool.input.command
          op: contains
          pattern: "git push --force"
    text: Force push is not allowed.
  - id: house-style
    target: system
    fallback: degrade
    priority: 90
    text: Answer in English.
```

各规则的最终值：

| 规则 | `fallback` | `priority` | `max_tokens` | 推导值 |
|---|---|---|---|---|
| `shell-guidance` | `skip`（defaults） | `60`（包级） | `200`（defaults，覆盖 `budget.per_rule`） | `at: after`、`repeat: session` |
| `force-push-guard` | `skip`（defaults） | `60`（包级） | `200`（defaults） | `at: before`，没有 `repeat` |
| `house-style` | `degrade`（规则） | `90`（规则） | `200`（defaults） | `reinject_on_compact: true`、`mode: append` |

### 常见误用

```yaml
defaults:
  repeat: turn
  sources: [startup]
rules:
  - id: shell-guidance
    target: tool
    tool: shell
    text: Prefer rg over grep.
  - id: force-push-guard
    target: tool
    tool: shell
    action: block
    text: Force push is not allowed.
```

`style validate` 会通过，但报告 4 条警告：

```text
T007 STYLE.yaml#shell-guidance: inject with repeat every or turn should include reason
block-repeat STYLE.yaml#force-push-guard: block rules are never deduplicated; repeat is ignored
sources-unused STYLE.yaml#force-push-guard: sources only applies to session.start rules
sources-unused STYLE.yaml#shell-guidance: sources only applies to session.start rules
```

只对部分规则有意义的字段，应该写在那些规则上，而不是写进 `defaults`。

## 校验与诊断

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-schema` | error | `defaults` 不是 mapping，或某个值的类型、取值无效 |
| `defaults-field` | error | 使用了不允许写进 `defaults` 的键 |
| `refresh-invalid` | error | `defaults.refresh` 不是 0 到 1000000 的整数 |
| `rule-file-pkg-field` | error | `rules/*.md` 的 front matter 声明了 `defaults` |
| `action-target` | error | `defaults.action: block` 作用到不能 block 的规则上 |
| `fail-closed-target` | error | `defaults.fail: closed` 作用到不在可阻止落点上的规则上 |
| `refresh-incompatible` | error | `defaults.refresh` 大于 0，作用到不满足 refresh 条件的规则上 |
| `block-repeat` | warning | `defaults.repeat` 作用到 block 规则上 |
| `T007` | warning | `defaults.repeat` 为 `every` 或 `turn`，作用到没有 `reason` 的 inject 规则上 |
| `sources-unused` | warning | `defaults.sources` 作用到非 session.start 规则上 |

## 相关

- [fallback：落点不可用时的处理](fallback.md)
- [package：包级元数据](package.md)
- [budget：token 预算](budget.md)
- [action：动作与时机](action.md)
- [rules：规则来源与正文](rules.md)
- [native_hooks：原生 hook](native-hooks.md)
