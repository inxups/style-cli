---
title: requires：依赖、设置与推荐
scope: package
keys: [requires, settings, recommends]
spec: style/v0.3
sources:
  - core/spec/validate_package.go
  - core/spec/validate_template.go
  - internal/app/install.go
  - internal/app/harness_requirement.go
  - core/compile/values.go
---

# requires：依赖、设置与推荐

> 声明包对 STYLE 运行时和 harness 版本的要求、安装时由用户填写的配置项，以及推荐给各 harness 的配置。

## 概览

这三个字段都写在包级：

- `requires`：版本要求。STYLE 运行时不满足时拒绝安装；某个 harness 版本不满足时跳过该 harness。
- `settings`：安装时填写的配置项，在规则正文里用 `{{settings.<name>}}` 引用。
- `recommends`：按 harness 分组的推荐配置。当前只做结构校验，安装流程不读取。

```yaml
requires:
  runtime: ">=0.1.0"
  harness:
    claude-code: ">=2.0.0"
settings:
  - name: language
    description: Answer language
    default: English
recommends:
  claude-code: {}
```

## 字段

### `requires`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | mapping |
| 必填 | 否 |
| 默认值 | 无（不限制版本） |
| 可选值 | 键只能是 `runtime`、`harness` |
| 可写入 defaults | 否 |

- `requires.runtime`：STYLE 运行时的版本要求。当前运行时版本是 `0.1.0`。
- `requires.harness.<harness>`：某个 harness 的版本要求。键是已注册的 harness 名，例如 `codex`、`claude-code`。

版本要求的写法是「运算符 + 语义化版本」，运算符可以省略：

| 写法 | 含义 |
|---|---|
| `1.2.3` 或 `=1.2.3` | 等于 |
| `>=1.2.3`、`>1.2.3`、`<=1.2.3`、`<1.2.3` | 比较 |
| `^1.2.3` | 不低于 1.2.3，且主版本相同 |
| `~1.2.3` | 不低于 1.2.3，且主版本和次版本都相同 |

约束：

- 必须是 mapping，只能包含 `runtime` 和 `harness`（诊断码 `invalid-schema`）。
- 版本要求必须是上表中的格式（诊断码 `invalid-schema`）。
- `harness` 的键必须是已注册的 harness（诊断码 `invalid-schema`）。

### `settings`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | mapping 列表 |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | 每项的键只能是 `name`、`description`、`default` |
| 可写入 defaults | 否 |

每一项声明一个配置项：

- `name`（必填）：配置项名称，在模板里用 `{{settings.<name>}}` 引用。
- `description`（可选）：说明文字。
- `default`（可选）：默认值。没有默认值的配置项，安装时必须用 `--set <name>=<value>` 提供。

约束：

- 必须是列表，每项必须是 mapping（诊断码 `invalid-schema`）。
- `name` 必须是非空字符串，且不能重复（诊断码 `invalid-schema`）。
- `description` 和 `default` 必须是字符串（诊断码 `invalid-schema`）。
- `sensitive` 是保留字段，不能使用（诊断码 `sensitive-reserved`）。
- 模板引用了未声明的配置项时报错（诊断码 `undeclared-var`）。

### `recommends`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | mapping |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | 键是已注册的 harness，值是 mapping |
| 可写入 defaults | 否 |

按 harness 分组的推荐配置。当前版本只校验结构，`plan` 和 `install` 都不读取它的内容。

约束：

- 必须是 mapping（诊断码 `invalid-schema`）。
- 键必须是已注册的 harness，值必须是 mapping（诊断码 `invalid-schema`）。

## 行为

1. **运行时版本**：`style install` 在读取配置项之前检查 `requires.runtime`。不满足时直接失败，不写入任何文件。`style plan` 不检查这一项（`internal/app/install.go` `validateRuntimeRequirement`）。
2. **harness 版本**：`style plan` 和 `style install` 都会检查 `requires.harness`。检测到的版本不满足时，报 `harness-version` 警告并跳过该 harness，其他 harness 照常处理。检测不到版本时也视为不满足（`internal/app/harness_requirement.go`）。
3. **配置项取值**：安装时依次取 `--set` 提供的值和 `default`。两者都没有时安装失败；`--set` 了未声明的配置项也会失败（`internal/app/install.go` `collectSettings`）。
4. **配置项保存**：安装后的取值保存在 STYLE 数据目录（默认 `~/.style`，可用 `STYLE_HOME` 指定）的 `settings/<包名>.yaml` 中，权限 0600，更新时沿用。
5. **模板渲染**：`{{settings.<name>}}` 在编译时替换为安装时的取值，属于静态内容，不会让规则变成动态规则（`core/compile/values.go` `withSettings`）。

## 示例

### 最小示例

```yaml
spec: style/v0.3
name: rq-min
version: 0.1.0
description: settings minimal example
settings:
  - name: language
    default: English
rules:
  - id: house-style
    target: system
    text: Answer in {{settings.language}}.
```

### 版本要求与必填配置项

```yaml
spec: style/v0.3
name: rq-ok
version: 0.1.0
description: requires settings recommends example
requires:
  runtime: ">=0.1.0"
  harness:
    claude-code: ">=2.0.0"
    codex: ">=999.0.0"
settings:
  - name: language
    description: Answer language
    default: English
  - name: team
    description: Team name
recommends:
  claude-code:
    note: anything
rules:
  - id: house-style
    target: system
    text: Answer in {{settings.language}} for team {{settings.team}}.
```

`style plan rq-ok --harness codex,claude-code` 会先输出：

```text
harness-version codex: skipping codex for package rq-ok: version 0.159.3 does not satisfy requires.harness.codex >=999.0.0
```

然后只给出 claude-code 的计划。`team` 没有默认值，所以安装时必须提供：

```text
$ style install rq-ok --harness claude-code
style install: setting "team" requires --set team=value
```

改为 `style install rq-ok --harness claude-code --set team=platform` 即可。

## 校验与诊断

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-schema` | error | 三个字段的结构、类型、未知键或版本格式错误；配置项重名；未注册的 harness |
| `sensitive-reserved` | error | 配置项使用了保留字段 `sensitive` |
| `undeclared-var` | error | 模板引用了未声明的 `settings.<name>` |
| `harness-version` | warning | harness 版本不满足 `requires.harness`，该 harness 被跳过（`plan`、`install` 时报告） |

## 相关

- [package：包级元数据](package.md)
- [rules：规则来源与正文](rules.md)（模板语法）
- [diagnostics：诊断码总表](diagnostics.md)
