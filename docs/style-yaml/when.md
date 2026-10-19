---
title: when：条件匹配
scope: when
keys: [when, model, provider, thinking_effort, thinking_effort_approx, harness, agent, conditions]
spec: style/v0.3
sources:
  - core/spec/validate_conditions.go
  - core/when/normalize.go
  - core/when/match.go
  - core/when/unknown.go
  - core/when/model.go
  - core/when/effort.go
  - core/when/predicate.go
  - core/when/shell.go
  - core/plan/plan.go
---

# when：条件匹配

> 给规则加上运行时条件：只在特定模型、思考强度、harness、代理，或事件内容满足条件时才生效。

## 概览

`when` 是规则上的 mapping。规则先按 `target` 的选择器筛选，再评估 `when`：

- 声明的所有子字段都必须满足（AND）。
- 子字段取列表时，命中任意一项即可（OR）。
- `conditions` 里的每一条都必须满足（AND）。

带 `when` 的规则是动态规则：不能写入 harness 的静态指令位置，也不能降级为静态指令。

```yaml
when:
  model: ["claude-opus-*", "**/gpt-5*"]
  provider: anthropic
  thinking_effort: ">=high"
  thinking_effort_approx: false
  harness:
    name: claude-code
    permission_mode: plan
  agent: any
  conditions:
    - field: tool.input.command
      op: shell_regex
      pattern: "^git push"
```

## 字段

### `when`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | mapping |
| 必填 | 否 |
| 默认值 | 无（只限制 `agent`，见下） |
| 可选值 | 键只能是 `model`、`provider`、`thinking_effort`、`thinking_effort_approx`、`harness`、`agent`、`conditions` |
| 可写入 defaults | 否 |

约束：

- 必须是 mapping，不能有其他键（诊断码 `invalid-schema`）。

### `model`

| 属性 | 值 |
|---|---|
| 位置 | when |
| 类型 | string \| string[] |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | 模型名 glob，或 `unknown` |
| 可写入 defaults | 否 |

按模型名匹配，不区分大小写。使用路径 glob 语法：`*` 不跨越 `/`；开头的 `**/` 可以匹配零个或多个完整的路径段，例如 `**/gpt-5*` 能匹配 `gpt-5` 和 `openai/gpt-5-mini`。`unknown` 在 harness 无法提供模型名时匹配。

约束：

- glob 语法必须有效（诊断码 `bad-model-glob`）。

### `provider`

| 属性 | 值 |
|---|---|
| 位置 | when |
| 类型 | string \| string[] |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | 服务商标识，或 `unknown` |
| 可写入 defaults | 否 |

按模型服务商精确匹配，例如 `anthropic`、`openai`。`unknown` 在无法确定服务商时匹配。

约束：

- 必须匹配 `^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`（诊断码 `bad-provider-id`）。

### `thinking_effort`

| 属性 | 值 |
|---|---|
| 位置 | when |
| 类型 | string \| string[] |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | 档位、档位列表、`>=档位`、`<=档位`，或 `open`、`unknown` |
| 可写入 defaults | 否 |

按思考强度匹配。档位从低到高为 `none` < `minimal` < `low` < `medium` < `high` < `xhigh` < `max`，不区分大小写。

- `>=high` 展开为 `high`、`xhigh`、`max`；`<=low` 展开为 `none`、`minimal`、`low`。
- `open` 是一个没有高低顺序的档位，映射表可以把某个原始值映射为 `open`。它只能按名称匹配，不参与 `>=` / `<=` 比较。
- `unknown` 在强度不可用时匹配：harness 没有报告强度，或报告的原始值不在映射表中。

各 harness 的原始强度值先按内置映射表转换为档位（不区分大小写），例如 Codex 的 `ultra` 映射为 `max`。内置映射表在 `internal/adapter/<harness>/effort.yaml`；用户也可以配置自己的映射，优先于内置映射。

约束：

- 必须是上述格式之一（诊断码 `bad-effort-expr`）。

### `thinking_effort_approx`

| 属性 | 值 |
|---|---|
| 位置 | when |
| 类型 | bool |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | `true` / `false` |
| 可写入 defaults | 否 |

按强度档位的来源匹配：通过内置映射表得到的档位为 `false`（精确），通过用户自定义映射得到的档位为 `true`（近似）。强度不可用时这个条件不满足。

约束：

- 必须是布尔值（诊断码 `invalid-schema`）。

### `harness`

| 属性 | 值 |
|---|---|
| 位置 | when |
| 类型 | mapping |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | 键只能是 `name`、`permission_mode` |
| 可写入 defaults | 否 |

- `harness.name`：harness 名称或列表，例如 `claude-code`、`codex`。在计划阶段处理：不包含当前 harness 时，规则在该 harness 上为 `skipped`。
- `harness.permission_mode`：harness 当前的权限模式，例如 Claude Code 的 `plan`，在运行时匹配。`unknown` 在无法获取权限模式时匹配。

约束：

- 两个子键都必须是字符串或字符串列表（诊断码 `invalid-schema`）。
- `name` 必须是已注册的 harness（诊断码 `invalid-schema`）。

### `agent`

| 属性 | 值 |
|---|---|
| 位置 | when |
| 类型 | string \| string[] |
| 必填 | 否 |
| 默认值 | `main`；`event: subagent.start` 规则默认为 `any` |
| 可选值 | `any`、`main`，或子代理类型名 |
| 可写入 defaults | 否 |

限制规则作用于哪些代理：

- `main`：只作用于主代理。
- 子代理类型名（例如 `Explore`）：只作用于该类型的子代理。
- `any`：作用于所有代理。

没有声明 `agent` 时，规则只在主代理上生效。

约束：

- 必须是非空字符串或非空字符串列表（诊断码 `invalid-schema`）。

### `conditions`

| 属性 | 值 |
|---|---|
| 位置 | when |
| 类型 | mapping 列表 |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | 每项包含 `field`、`op`、`pattern` |
| 可写入 defaults | 否 |

对事件内容做文本匹配。每一项包含：

- `field`：要检查的字段。
- `op`：运算符。
- `pattern`：模式字符串。

**字段**：

| 字段 | 内容 | 可用的评估点 |
|---|---|---|
| `prompt` | 用户提示 | `prompt.submitted`、`skill.explicit` |
| `tool.name` | 原生工具名 | 工具事件 |
| `tool.canonical` | 规范工具分类 | 工具事件 |
| `tool.input` | 整个工具输入，压缩后的 JSON 文本 | 工具事件 |
| `tool.input.<路径>` | 工具输入中某个字段的值，例如 `tool.input.command` | 工具事件 |
| `tool.added` | 编辑或写入新增的内容 | `tool.pre`、`tool.post`（`file.edit` / `file.write` 或 `names` 规则） |
| `tool.output` | 工具输出 | `tool.post` |
| `mcp.server`、`mcp.tool` | MCP 服务器和工具名 | `mcp` 规则，或工具事件上的 event 规则 |
| `skill.name` | skill 名称 | `skill` 规则，或 `skill.explicit` |
| `cwd` | 当前工作目录 | 所有评估点 |

**运算符**：

| 运算符 | 含义 |
|---|---|
| `equals` | 完全相等 |
| `contains` / `not_contains` | 包含 / 不包含 |
| `starts_with` / `ends_with` | 前缀 / 后缀 |
| `regex` | RE2 正则表达式，部分匹配 |
| `shell_regex` | 把值当作 shell 脚本拆成一条条简单命令，任意一条匹配正则即可 |

`shell_regex` 会去掉引号，并展开命令替换、子 shell、`sh -c` / `bash -c` 脚本、`eval` 参数、送入 shell 的 heredoc、管道送入 shell 的 `echo` / `printf` / `cat` 输出、`sudo` / `xargs` 等包装器运行的命令，以及 `find -exec`。因此 `^git push` 也能匹配 `cd repo && git push`、`bash -c "git push"`。

约束：

- 每项必须是 mapping，只能有 `field`、`op`、`pattern`，三者都是字符串（诊断码 `invalid-schema`）。
- `op` 必须是上表中的运算符（诊断码 `invalid-schema`）。
- `contains`、`starts_with`、`ends_with`、`not_contains` 的 `pattern` 不能为空，否则条件恒真或恒假（诊断码 `empty-pattern`）。
- `regex` 和 `shell_regex` 的 `pattern` 必须是有效的 RE2 正则（诊断码 `bad-regex`）。
- `field` 必须是已知字段（诊断码 `invalid-schema`），并且在规则的评估点上可用（诊断码 `condition-field-wrong-event`）。
- 对可能缺失的字段使用 `not_contains` 会报警告。可能缺失的字段包括 `tool.added`、`tool.input.<路径>`，system 和 skill 规则上的 `prompt`，以及 event 规则上的 `mcp.*`。字段缺失时条件不成立，不会命中（诊断码 `negated-optional-field`）。
- 对整个 `tool.input` 写条件会报警告，建议改用解码后的 `tool.input.<路径>`（诊断码 `whole-tool-input`）。

## 行为

1. **求值顺序**：按 `provider`、`model`、`thinking_effort`、`thinking_effort_approx`、`harness`、`agent`、`conditions` 的顺序求值，遇到不满足的立即停止（`core/when/match.go`）。
2. **数据缺失**：运行时取不到某个字段的值时，该条件不满足；只有显式写了 `unknown` 的 `model`、`provider`、`thinking_effort`、`harness.permission_mode` 会在缺失时命中（`core/when/unknown.go`）。
3. **计划阶段的数据检查**：某个条件需要的数据在该 harness 的评估点上永远取不到时，规则为 `never-trigger`；`fallback: error` 时为 `install-error`（见 [fallback.md](fallback.md)）。
4. **与模板的关系**：`when.model`、`when.provider`、`when.thinking_effort`、`when.harness.*` 不含 `unknown` 时，能保证对应的模板变量存在，正文里引用它们不会报 `var-no-default`（见 [rules.md](rules.md)）。
5. **`tool.input` 与 `tool.input.<路径>`**：`tool.input` 是整个输入的 JSON 文本，引号、换行、反斜杠保持转义，键名也在文本里。匹配具体参数时应使用解码后的 `tool.input.command`、`tool.input.file_path` 等字段。

## 示例

### 最小示例

```yaml
spec: style/v0.3
name: wh-min
version: 0.1.0
description: when minimal example
rules:
  - id: deep-think
    target: system
    when:
      thinking_effort: ">=high"
    text: Think through edge cases before answering.
```

### 常见组合

```yaml
spec: style/v0.3
name: wh-all
version: 0.1.0
description: when example
rules:
  - id: deep-think
    target: system
    when:
      model: ["claude-opus-*", "**/gpt-5*"]
      thinking_effort: ">=high"
    text: Think through edge cases before answering.
  - id: claude-only
    target: system
    when:
      harness:
        name: claude-code
    text: Use the TodoWrite tool for multi-step work.
  - id: plan-mode
    target: event
    event: prompt.submitted
    when:
      harness:
        permission_mode: plan
    text: Do not edit files in plan mode.
  - id: subagent-brief
    target: event
    event: tool.pre
    when:
      agent: any
    text: Keep tool calls minimal.
  - id: rm-guard
    target: tool
    tool: shell
    action: block
    when:
      conditions:
        - field: tool.input.command
          op: shell_regex
          pattern: "^rm -rf /"
        - field: cwd
          op: starts_with
          pattern: /Users/
    text: Refusing recursive delete from the filesystem root.
```

- `deep-think`：模型名命中任意一个 glob，且思考强度不低于 high。
- `claude-only`：在 Codex 上计划结果为 `skipped`（`when.harness.name does not include this adapter`）。它带 `when`，所以即使正文是静态的，在 Claude Code 上也挂在 hook 上，而不是写入静态指令位置。
- `subagent-brief`：主代理和子代理的工具调用前都会注入。
- `rm-guard`：两个条件同时满足才拒绝。

## 校验与诊断

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-schema` | error | `when` 或子字段的结构、类型、未知键错误；未注册的 harness；未知的条件字段或运算符 |
| `bad-model-glob` | error | `model` glob 语法无效 |
| `bad-provider-id` | error | `provider` 标识格式无效 |
| `bad-effort-expr` | error | `thinking_effort` 格式无效 |
| `bad-regex` | error | 正则表达式无效 |
| `empty-pattern` | error | 子串类运算符的 `pattern` 为空 |
| `condition-field-wrong-event` | error | 条件字段在规则的评估点上不可用 |
| `negated-optional-field` | warning | 对可能缺失的字段使用 `not_contains` |
| `whole-tool-input` | warning | 条件字段是整个 `tool.input` |

## 相关

- [target：作用对象](target.md)
- [action：动作与时机](action.md)
- [rules：规则来源与正文](rules.md)（模板变量）
- [fallback：落点不可用时的处理](fallback.md)
