---
title: package：包级元数据
scope: package
keys: [spec, name, version, description, authors, priority]
spec: style/v0.3
sources:
  - core/spec/validate_package.go
  - core/spec/load.go
  - core/spec/merge.go
  - core/compile/values.go
  - core/engine/process.go
---

# package：包级元数据

> 标识一个 STYLE 包的基本信息：规范版本、名称、版本号、描述、作者和默认优先级。

## 概览

这些字段写在 `STYLE.yaml` 顶层，单文件包写在 `.style.md` 的 front matter 里。`spec`、`name`、`version`、`description` 是必填项。`priority` 除了描述包本身，还是规则和原生 hook 的默认优先级。

包级字段一共有 13 个：本页的 6 个，加上 `requires`、`settings`、`recommends`（见 [requires.md](requires.md)）、`budget`（见 [budget.md](budget.md)）、`defaults`（见 [defaults.md](defaults.md)）、`rules`（见 [rules.md](rules.md)）和 `native_hooks`（见 [native-hooks.md](native-hooks.md)）。其他顶层键会报 `invalid-schema`。

```yaml
spec: style/v0.3
name: <name>
version: <semver>
description: <description>
authors: [<author>]
priority: 50
```

## 字段

### `spec`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | string |
| 必填 | 是 |
| 默认值 | 无 |
| 可选值 | `style/v0.3` |
| 可写入 defaults | 否 |

STYLE 规范版本。当前只接受 `style/v0.3`。

约束：

- 必须等于 `style/v0.3`（诊断码 `invalid-spec`）。

### `name`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | string |
| 必填 | 是 |
| 默认值 | 无 |
| 可选值 | — |
| 可写入 defaults | 否 |

包名，用于安装、更新、卸载等命令，以及 STYLE 数据目录中的文件名。单文件包的规则没有 `id` 时，规则 id 取包名。

约束：

- 必须匹配 `^[a-z0-9][a-z0-9-]{0,63}$`：小写字母、数字和连字符，以字母或数字开头，最长 64 个字符（诊断码 `bad-name`）。

### `version`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | string |
| 必填 | 是 |
| 默认值 | 无 |
| 可选值 | — |
| 可写入 defaults | 否 |

包版本号，使用语义化版本（SemVer 2.0），可以带预发布和构建后缀，例如 `1.2.0-rc.1+build.5`。

约束：

- 必须是语义化版本（诊断码 `bad-version`）。

### `description`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | string |
| 必填 | 是 |
| 默认值 | 无 |
| 可选值 | — |
| 可写入 defaults | 否 |

包的一句话描述。

约束：

- 必须是非空字符串（诊断码 `invalid-schema`）。

### `authors`

| 属性 | 值 |
|---|---|
| 位置 | 包级 |
| 类型 | string \| string[] |
| 必填 | 否 |
| 默认值 | 无 |
| 可选值 | — |
| 可写入 defaults | 否 |

作者信息，可以写一个字符串，也可以写字符串列表。

约束：

- 必须是字符串或字符串列表（诊断码 `invalid-schema`）。

### `priority`

| 属性 | 值 |
|---|---|
| 位置 | 包级；规则和原生 hook 也可以单独声明 |
| 类型 | number |
| 必填 | 否 |
| 默认值 | `50` |
| 可选值 | 任意有限数字，可以是小数 |
| 可写入 defaults | 是 |

包级 `priority` 是规则和原生 hook 的默认优先级。同一个事件上有多条规则输出时，按 `[priority, 包安装顺序, 规则序号]` 升序排列，数值小的排在前面。

约束：

- 包级 `priority` 必须是数字（诊断码 `invalid-schema`）。
- 规则级 `priority` 必须是非负数字（诊断码 `invalid-schema`）。
- `rules/*.md` 的 front matter 可以声明 `priority`，这是规则文件里唯一允许出现的包级字段名。

## 行为

1. **必填检查**：缺少或写错 `spec`、`name`、`version`、`description` 时，校验失败，包不能安装（`core/spec/validate_package.go` `validatePackage`）。
2. **至少一条声明**：目录包的 `rules` 和 `native_hooks` 不能同时为空（诊断码 `invalid-schema`）。
3. **priority 的默认值链**：规则的 `priority` 依次取规则字段、`defaults.priority`、包级 `priority`、`50`。原生 hook 的 `priority` 依次取 hook 字段、包级 `priority`、`50`，不读 `defaults`（`core/spec/merge.go` `Merge`）。
4. **排序**：编译和运行时都按 `[priority, 包安装顺序, 规则序号]` 升序排列条目（`core/compile/values.go`、`core/engine/process.go` `lessOrder`）。

## 示例

### 最小示例

```yaml
spec: style/v0.3
name: pkg-min
version: 0.1.0
description: package minimal example
rules:
  - id: house-style
    target: system
    text: Answer in English.
```

### 调整优先级

```yaml
spec: style/v0.3
name: pkg-priority
version: 1.2.0-rc.1
description: package priority example
authors:
  - Alice
  - Bob
priority: 70
rules:
  - id: house-style
    target: system
    text: Answer in English.
  - id: safety
    target: system
    priority: 10
    text: Never print secrets.
```

`house-style` 的优先级是 70（继承包级），`safety` 是 10。两条规则在同一个位置输出时，`safety` 排在前面。

## 校验与诊断

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-spec` | error | `spec` 不是 `style/v0.3` |
| `bad-name` | error | `name` 不符合命名规则 |
| `bad-version` | error | `version` 不是语义化版本 |
| `invalid-schema` | error | `description` 为空；`authors` 类型错误；`priority` 不是数字；未知的顶层字段；`rules` 和 `native_hooks` 都为空 |

## 相关

- [概览：包的形态与加载流程](overview.md)
- [requires：依赖、设置与推荐](requires.md)
- [budget：token 预算](budget.md)
- [defaults：包级默认值](defaults.md)
- [rules：规则来源与正文](rules.md)
