---
title: rules：规则来源与正文
scope: rule
keys: [rules, id, file, text, from, model_text, enabled, reason]
spec: style/v0.3
sources:
  - core/spec/load.go
  - core/spec/validate_rules.go
  - core/spec/validate_template.go
  - core/template/template.go
  - core/template/event.go
---

# rules：规则来源与正文

> 说明一条规则从哪里来、正文怎么写，以及正文里可以使用的模板变量。

## 概览

`rules` 是 STYLE.yaml 顶层的列表，每一项声明一条或一组规则。一条规则由三部分组成：

- **标识**：`id`。
- **正文**：注入给模型的文本，来自 `text`、`file` 或规则文件。
- **行为字段**：`target`、`action`、`when` 等，分别见 [target.md](target.md)、[action.md](action.md)、[when.md](when.md)。

正文可以使用模板变量，例如 `{{settings.language}}`、`{{model}}`。

```yaml
rules:
  - id: <id>                 # 内联规则：id + text
    target: system
    text: <正文>
  - id: <id>                 # 内联规则：id + file
    target: system
    file: prompts/<name>.md
  - from: rules/*.md         # 规则文件：front matter + 正文
    enabled: true            # 覆盖值，作用于每个匹配的文件
```

## 字段

### `rules`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | mapping 列表 |
| 必填 | 目录包中 `rules` 和 `native_hooks` 至少有一个非空 |
| 默认值 | 无 |
| 可选值 | — |
| 可写入 defaults | 否 |

每一项是内联规则（含 `text` 或 `file`）或 `from` 条目。单文件包不能声明 `rules`。

约束：

- 必须是列表，每项必须是 mapping（诊断码 `invalid-package`）。
- 规则字段只能是以下 25 个之一（诊断码 `invalid-schema`）：`id` `target` `file` `text` `model_text` `action` `at` `when` `repeat` `fail` `refresh` `reinject_on_compact` `sources` `fallback` `priority` `max_tokens` `reason` `from` `enabled` `mode` `tool` `names` `mcp` `skill` `event`。

### `id`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | string |
| 必填 | 内联规则必填；规则文件和单文件包可省略 |
| 默认值 | 规则文件：`rules/` 之后去掉 `.md` 的路径；单文件包：包名 |
| 可选值 | — |
| 可写入 defaults | 否 |

规则在包内的唯一标识，用于计划、去重和诊断。

约束：

- 必须匹配 `^[a-z0-9][a-z0-9_./-]{0,127}$`，且不能包含 `..` 或 `//`，不能以 `/` 结尾（诊断码 `bad-id`）。
- 在同一个包的规则和原生 hook 之间不能重复（诊断码 `dup-id`）。
- 内联规则必须声明（诊断码 `inline-missing-id`）。

### `text`

| 属性 | 值 |
|---|---|
| 位置 | 规则（只用于 STYLE.yaml 中的内联规则） |
| 类型 | string |
| 必填 | 内联规则必须有 `text` 或 `file` |
| 默认值 | 无 |
| 可选值 | — |
| 可写入 defaults | 否 |

直接写在 STYLE.yaml 里的正文。

约束：

- 不能与 `file` 同时使用（诊断码 `file-and-text`）。
- 规则文件和单文件包不能使用（诊断码 `invalid-schema`）。

### `file`

| 属性 | 值 |
|---|---|
| 位置 | 规则（只用于 STYLE.yaml 中的内联规则） |
| 类型 | string |
| 必填 | 内联规则必须有 `text` 或 `file` |
| 默认值 | 无 |
| 可选值 | 包内相对路径 |
| 可写入 defaults | 否 |

正文文件的路径。文件内容整体作为正文。

约束：

- 不能与 `text` 同时使用（诊断码 `file-and-text`）。
- 不能指向 `rules/` 下的文件（诊断码 `file-into-rules`）。
- 文件不能有 front matter（诊断码 `body-has-frontmatter`）。
- 需满足包内文件的通用限制，见 [overview.md](overview.md#文件读取限制)。

### `from`

| 属性 | 值 |
|---|---|
| 位置 | 规则（只用于 STYLE.yaml） |
| 类型 | string |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | `rules/` 下 `.md` 文件的路径或 glob |
| 可写入 defaults | 否 |

引用规则文件。支持 `*`、`?`、`[...]` 和跨目录的 `**`，匹配结果按路径排序，每个文件成为一条规则。`from` 条目上的其他字段是覆盖值，优先级高于规则文件的 front matter。

约束：

- 必须指向 `rules/` 下的 `.md` 文件（诊断码 `invalid-schema`）；glob 必须至少匹配一个文件（`file-not-found`）。
- 不能同时写 `id`、`file`、`text`（诊断码 `from-and-inline`）。
- 不能覆盖 `target` 和 `id`（诊断码 `override-target`）。
- 规则文件的 front matter 里不能写 `from`、`file`、`text`，也不能写除 `priority` 外的包级字段（诊断码 `invalid-schema`、`rule-file-pkg-field`）。

### `model_text`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | string |
| 必填 | 否；warn 规则引用 `event.*` 时必填 |
| 默认值 | 无（warn 规则取正文第一句） |
| 可选值 | — |
| 可写入 defaults | 否 |

warn 规则给模型看的文本。warn 会把完整正文作为提示显示给用户，把 `model_text` 作为上下文交给模型；没有 `model_text` 时，模型只收到正文的第一句。

约束：

- 只对 warn 规则有效，其他动作上写了会报警告（诊断码 `model-text-unused`）。
- 和正文一样做模板校验。

### `enabled`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | bool |
| 必填 | 否 |
| 默认值 | `true` |
| 可选值 | `true` / `false` |
| 可写入 defaults | 否 |

`false` 时规则不安装，但仍然参与校验。常用在 `from` 条目上临时关闭一组规则。

### `reason`

| 属性 | 值 |
|---|---|
| 位置 | 规则 |
| 类型 | string |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | — |
| 可写入 defaults | 否 |

写给维护者看的说明，解释为什么这条规则需要重复注入。不会发送给模型。

约束：

- inject 规则使用 `repeat: every` / `turn` 或正数 `refresh` 时，缺少 `reason` 会报警告（诊断码 `T007`）。

## 行为

### 正文

1. **正文来源**：`text`、`file` 指向的文件内容，或规则文件 front matter 之后的部分。规则文件和单文件包的正文会去掉开头的空行和结尾的空白（`core/spec/frontmatter.go`）。
2. **正文不能为空**（诊断码 `empty-body`），也不能包含保留标记 `<!-- style:`（`marker-in-body`）。
3. **静态与动态**：正文只含文本和 `settings.*` 变量时是静态正文，安装时就能渲染。含运行时变量时是动态正文，只能在事件发生时渲染。静态 system 规则可以写入 harness 的静态指令位置；动态规则只能挂在 hook 上（见 [fallback.md](fallback.md)）。

### 模板语法

| 写法 | 含义 |
|---|---|
| `{{name}}` | 替换为变量值；不可用时替换为空字符串 |
| `{{name \| default: "value"}}` | 不可用时使用默认值；默认值必须加引号，单引号也可以 |
| `\{{` | 输出字面量 `{{` |

可用的变量（`core/spec/validate_template.go`）：

| 变量 | 类型 | 说明 |
|---|---|---|
| `settings.<name>` | 安装时 | 配置项的取值，见 [requires.md](requires.md) |
| `model`、`provider`、`thinking_effort`、`thinking_effort_raw` | 运行时 | 当前模型、服务商、思考强度（归一化值和原始值） |
| `harness.name`、`harness.version`、`harness.cwd`、`harness.permission_mode`、`harness.agent`、`harness.session_id`、`harness.turn_id` | 运行时 | harness 信息 |
| `event.prompt`、`event.tool.name`、`event.tool.input`、`event.tool.input.<path>`、`event.tool.added`、`event.tool.output`、`event.mcp.server`、`event.mcp.tool`、`event.skill.name` | 运行时（事件内容） | 触发事件中的内容 |

规则：

- 未声明的变量报 `undeclared-var`；模板语法错误报 `invalid-template`。
- 运行时变量没有默认值、`when` 也不能保证它存在时，报 `var-no-default` 警告。例如 `when.model` 不含 `unknown` 时可以保证 `{{model}}` 存在。
- `harness.session_id` 和 `harness.turn_id` 每次事件都会变，会让 `repeat` 去重失效，报 `T006` 警告。
- 事件内容只能通过 `event.` 前缀引用。直接写 `{{prompt}}`、`{{tool.input}}` 会报 `event-var-in-template`。
- 事件内容必须在规则的事件上可用，否则报 `event-var-unavailable`。例如 `event.tool.output` 只在 `tool.post` 上可用。
- warn 规则引用事件内容时必须写 `model_text`（诊断码 `event-warn-model-text`）。
- 事件内容被视为不可信数据。每个值最多保留 512 个字符，编码为单行 JSON 字符串，并包在 `<style-untrusted-event field="..." truncated="...">` 边界内输出，不会被当作模板或指令解析（`core/template/event.go` `FenceEvent`）。

## 示例

### 最小示例

```yaml
spec: style/v0.3
name: rules-min
version: 0.1.0
description: rules minimal example
rules:
  - id: house-style
    target: system
    text: Answer in English.
```

### 规则文件与覆盖值

```yaml
spec: style/v0.3
name: rules-from
version: 0.1.0
description: from with overrides
rules:
  - from: rules/*.md
    priority: 30
  - from: rules/drafts/**/*.md
    enabled: false
```

`rules/` 下有 `review.md` 和 `drafts/wip.md` 两个文件。`*` 不跨目录，所以第一条 `from` 只匹配 `review.md`，生成 id 为 `review`、`priority` 为 30 的规则。第二条匹配 `drafts/` 下的所有文件，生成 id 为 `drafts/wip` 的规则并关闭它。

> 同一个文件被两个 `from` 引用时会生成两条同 id 的规则，报 `dup-id`。写多个 glob 时要让它们互不重叠。

### 模板变量

```yaml
spec: style/v0.3
name: rules-template
version: 0.1.0
description: template variables
budget:
  per_event: 2000
settings:
  - name: language
    default: English
rules:
  - id: house-style
    target: system
    text: Answer in {{settings.language}}.
  - id: model-note
    target: system
    when:
      model: "claude-*"
    text: "You are running as {{model}} in {{harness.cwd | default: 'the project'}}."
  - id: edit-review
    target: tool
    tool: file.edit
    max_tokens: 2000
    text: "Check this edit for style problems: {{event.tool.input}}"
```

- `house-style` 是静态正文。
- `model-note` 是动态正文；`when.model` 保证 `model` 存在，`harness.cwd` 有默认值，所以没有 `var-no-default` 警告。
- `edit-review` 把编辑内容作为不可信数据附在提示后面。token 估算按事件内容的最坏情况计算（512 个字符，每个字符取最大转义宽度），单个 `event.*` 变量就约 1800 tokens，超过默认的 `per_rule: 400` 和 `per_event: 1200`。所以这里放宽了 `max_tokens` 和 `budget.per_event`，见 [budget.md](budget.md)。

## 校验与诊断

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-package` | error | `rules` 不是列表，或某项不是 mapping；`from` 为空 |
| `invalid-schema` | error | 未知规则字段；字段类型错误；内联规则缺少 `text` 和 `file`；`from` 路径无效 |
| `bad-id` | error | `id` 格式无效或不是字符串 |
| `dup-id` | error | `id` 与其他规则或原生 hook 重复 |
| `inline-missing-id` | error | 内联规则缺少 `id` |
| `file-and-text` | error | 同时写了 `file` 和 `text` |
| `from-and-inline` | error | `from` 条目写了 `id`、`file` 或 `text` |
| `override-target` | error | `from` 条目覆盖 `target` 或 `id` |
| `rule-file-pkg-field` | error | 规则文件 front matter 写了除 `priority` 外的包级字段 |
| `empty-body` | error | 正文为空 |
| `marker-in-body` | error | 正文包含 `<!-- style:` |
| `invalid-template` | error | 模板语法错误 |
| `undeclared-var` | error | 未声明或不支持的变量 |
| `event-var-in-template` | error | 没有用 `event.` 前缀引用事件内容 |
| `event-var-unavailable` | error | 事件内容在该规则的事件上不可用 |
| `event-warn-model-text` | error | warn 规则引用事件内容但没有 `model_text` |
| `var-no-default` | warning | 运行时变量可能为空 |
| `T006` | warning | 使用了 `harness.session_id` 或 `harness.turn_id` |
| `T007` | warning | 重复注入的 inject 规则缺少 `reason` |
| `model-text-unused` | warning | 非 warn 规则写了 `model_text` |

## 相关

- [概览：包的形态与加载流程](overview.md)
- [target：作用对象](target.md)
- [action：动作与时机](action.md)
- [when：条件匹配](when.md)
- [requires：依赖、设置与推荐](requires.md)（`settings`）
- [budget：token 预算](budget.md)（`max_tokens`）
- [package：包级元数据](package.md)（`priority`）
