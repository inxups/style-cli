---
title: diagnostics：诊断码总表
scope: concept
keys: []
spec: style/v0.3
sources:
  - core/spec/validate.go
  - core/spec/validate_package.go
  - core/spec/validate_rules.go
  - core/spec/validate_template.go
  - core/spec/validate_conditions.go
  - core/spec/validate_lints.go
  - core/spec/validate_files.go
  - core/spec/native.go
  - core/spec/load.go
  - core/spec/package_files.go
  - core/tokens/analyze.go
  - core/tokens/installed.go
  - internal/app/entry.go
  - internal/app/harness_requirement.go
---

# diagnostics：诊断码总表

> 汇总 `style validate`、`style plan` 和安装流程报告的全部诊断码，并指向详细说明它们的页面。

## 概览

诊断码分为三个阶段：

1. **加载**：读取文件、解析 YAML 和 front matter。遇到错误立即中止，一次只报告一个。
2. **校验**：检查字段结构、取值和跨字段约束，一次报告全部问题。
3. **计划与安装**：估算 token、检查 harness 版本和安装总量。

每条诊断都有级别：

- **error**：包不能安装。
- **warning**：包可以安装，但很可能不符合作者的本意。

## 行为

### 输出格式

文本输出每行一条：

```text
<诊断码> <位置>: <说明>
```

位置的常见写法：

| 写法 | 指向 |
|---|---|
| `STYLE.yaml` | 包级字段 |
| `STYLE.yaml#<id>` | STYLE.yaml 中的内联规则 |
| `STYLE.yaml#<id>:when.conditions[0]` | 规则的第一个条件 |
| `STYLE.yaml:settings[0]` | 第一个配置项 |
| `STYLE.yaml#native_hooks/<id>` | 原生 hook |
| `rules/<name>.md` | 规则文件 |
| `codex`、`claude-code` | 按 harness 计算的 token 诊断 |

`style validate --json` 输出数组，每项包含 `code`、`path`、`severity`、`message`。

### 退出码

| 退出码 | 含义 |
|---|---|
| 0 | 成功；只有 warning 时也是 0 |
| 1 | 加载失败、有 error 级诊断，或命令用法错误 |
| 3 | `style sync` 检测到变更 |
| 4 | 计划被拒绝：有规则为 `install-error`，或 `T002` / `T004` 为 error（`T001` 为 error 时校验就失败，退出码为 1） |

## 诊断码

### 加载

详见 [overview.md](overview.md)。

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-package` | error | 入口不是目录或 `.style.md` 文件；`rules` / `native_hooks` 不是列表或含非 mapping 项；`from` 为空 |
| `invalid-yaml` | error | YAML 无法解析、不是 mapping 或包含多个文档 |
| `invalid-frontmatter` | error | front matter 缺少开始或结束的 `---` |
| `bad-encoding` | error | 文件不是 UTF-8 文本或包含 NUL 字节 |
| `file-not-found` | error | 引用的文件不存在，或 `from` glob 没有匹配 |
| `file-read` | error | 文件无法读取，或不是普通文件 |
| `file-too-large` | error | 文件超过 256 KiB |
| `path-escape` | error | 路径不是包内相对路径 |
| `symlink` | error | 包入口或包内路径上有符号链接 |
| `bad-glob` | error | `from` glob 语法无效 |
| `file-into-rules` | error | `file` 指向 `rules/` 下的文件 |
| `body-has-frontmatter` | error | `file` 引用的正文文件带有 front matter |
| `native-format-removed` | error | 使用了已移除的 `target: native` |

### 包级字段

详见 [package.md](package.md)、[requires.md](requires.md)、[budget.md](budget.md)、[defaults.md](defaults.md)。

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-spec` | error | `spec` 不是 `style/v0.3` |
| `bad-name` | error | `name` 不符合命名规则 |
| `bad-version` | error | `version` 不是语义化版本 |
| `defaults-field` | error | `defaults` 中使用了不允许的键 |
| `sensitive-reserved` | error | 配置项使用了保留字段 `sensitive` |
| `undeclared-rule-file` | warning | `rules/` 下的 `.md` 文件没有被任何 `from` 引用 |

### 规则

详见 [rules.md](rules.md)、[target.md](target.md)、[action.md](action.md)。

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-schema` | error | 通用结构错误：未知字段、类型错误、取值不在枚举中、缺少必填字段等 |
| `bad-id` | error | `id` 格式无效 |
| `dup-id` | error | `id` 与其他规则或原生 hook 重复 |
| `inline-missing-id` | error | 内联规则缺少 `id` |
| `file-and-text` | error | 同时写了 `file` 和 `text` |
| `from-and-inline` | error | `from` 条目写了 `id`、`file` 或 `text` |
| `override-target` | error | `from` 条目覆盖 `target` 或 `id` |
| `rule-file-pkg-field` | error | 规则文件 front matter 写了除 `priority` 外的包级字段 |
| `empty-body` | error | 正文为空 |
| `marker-in-body` | error | 正文包含保留标记 `<!-- style:` |
| `action-target` | error | block 用在不支持的目标或事件上 |
| `block-after` | error | block 规则设为 `at: after` |
| `fail-closed-target` | error | `fail: closed` 用在不能直接拒绝的评估点 |
| `replace-non-system` | error | 非 system 规则使用 `mode: replace` |
| `refresh-invalid` | error | `refresh` 不是 0 到 1000000 的整数 |
| `refresh-incompatible` | error | 正数 `refresh` 与动作、`repeat`、`mode` 或事件不兼容 |
| `block-repeat` | warning | block 规则声明了 `repeat` |
| `reinject-unused` | warning | `reinject_on_compact: true` 但 `repeat` 不是 `session` |
| `sources-unused` | warning | `sources` 声明在不经过 `session.start` 的规则上 |
| `model-text-unused` | warning | 非 warn 规则写了 `model_text` |

### 模板

详见 [rules.md](rules.md#模板语法)。

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-template` | error | 模板语法错误，例如缺少 `}}`、默认值没有加引号 |
| `undeclared-var` | error | 未声明或不支持的变量 |
| `event-var-in-template` | error | 没有用 `event.` 前缀引用事件内容 |
| `event-var-unavailable` | error | 事件内容在该规则的事件上不可用 |
| `event-warn-model-text` | error | warn 规则引用事件内容但没有 `model_text` |
| `var-no-default` | warning | 运行时变量可能为空，且没有默认值 |

### when 条件

详见 [when.md](when.md)。

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `bad-model-glob` | error | `model` glob 语法无效 |
| `bad-provider-id` | error | `provider` 标识格式无效 |
| `bad-effort-expr` | error | `thinking_effort` 格式无效 |
| `bad-regex` | error | 正则表达式无效 |
| `empty-pattern` | error | 子串类运算符的 `pattern` 为空 |
| `condition-field-wrong-event` | error | 条件字段在规则的评估点上不可用 |
| `negated-optional-field` | warning | 对可能缺失的字段使用 `not_contains` |
| `whole-tool-input` | warning | 条件字段是整个 `tool.input` |

### 原生 hook

详见 [native-hooks.md](native-hooks.md)。

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `native-field` | error | 字段类型错误；使用了规则或包级字段；不支持的扩展字段；未注册的 harness |
| `native-mapping` | error | 没有任何 harness 映射，或映射不是 mapping |
| `native-event` | error | 缺少 `event`，或该 harness 不支持这个事件 |
| `native-matcher` | error | matcher 无效，或用在不接受 matcher 的事件上 |
| `native-command` | error | 命令为空或违反格式限制 |
| `native-timeout` | error | `timeout` 不是 1 到 600 的整数 |

### token 预算

详见 [budget.md](budget.md)。`T001`、`T002`、`T004` 的级别由 `budget.on` 决定，默认是 warning。

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `T001` | 由 `budget.on` 决定 | 单条规则估算值超过 `max_tokens` |
| `T002` | 由 `budget.on` 决定 | 同一事件的合计超过 `per_event` |
| `T003` | error | 所有已安装包在某事件上的合计超过 adapter 输入上限的 80% |
| `T004` | 由 `budget.on` 决定 | 常驻内容合计超过 `always_on` |
| `T005` | warning | block 拒绝理由超过 40 tokens |
| `T006` | warning | 模板使用了 `harness.session_id` 或 `harness.turn_id` |
| `T007` | warning | inject 规则重复注入（`repeat: every` / `turn` 或正数 `refresh`）但缺少 `reason` |

### 计划与安装

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `harness-version` | warning | harness 版本不满足 `requires.harness`，该 harness 被跳过，见 [requires.md](requires.md) |

`style plan` 还会在规则状态下给出原因说明。以下两种原因会让规则变为 `install-error` 或 `skipped`，见 [action.md](action.md)：

- `fail-closed-unsupported`：`fail: closed` 规则的原评估点不支持 deny，或经过了降级。
- `refresh-unsupported-platform`：当前平台不支持 refresh 所需的会话锁。

计划状态（`native`、`degraded`、`partial`、`skipped`、`never-trigger`、`native-hook`、`install-error`）的含义见 [fallback.md](fallback.md#计划状态)。

## 示例

```yaml
spec: style/v0.3
name: diag-demo
version: 0.1.0
description: diagnostics example
defaults:
  repeat: turn
rules:
  - id: shell-guidance
    target: tool
    tool: shell
    text: Prefer rg over grep.
```

`style validate diag-demo --json`：

```json
[
  {
    "code": "T007",
    "path": "STYLE.yaml#shell-guidance",
    "severity": "warning",
    "message": "inject with repeat every or turn should include reason"
  }
]
```

只有 warning，退出码为 0。给规则加上 `reason` 后警告消失。

## 相关

- [概览：包的形态与加载流程](overview.md)
- [fallback：落点不可用时的处理](fallback.md)
- [budget：token 预算](budget.md)
