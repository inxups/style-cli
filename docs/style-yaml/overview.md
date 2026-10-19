---
title: 概览：包的形态与加载流程
scope: concept
keys: []
spec: style/v0.3
sources:
  - core/spec/load.go
  - core/spec/frontmatter.go
  - core/spec/package_files.go
  - core/spec/validate.go
  - core/spec/validate_files.go
  - core/spec/merge.go
---

# 概览：包的形态与加载流程

> STYLE 包有两种形态：目录包和单文件包。本页说明它们的结构，以及 `style` 读取一个包时依次做了什么。

## 概览

一个 STYLE 包描述一组面向模型的规则（`rules`）和原生 hook（`native_hooks`），由 `style install` 安装到 Codex、Claude Code 等 harness。包有两种形态：

| 形态 | 入口 | 适用场景 |
|---|---|---|
| 目录包 | 目录下的 `STYLE.yaml` | 多条规则、原生 hook、配置项 |
| 单文件包 | 一个 `*.style.md` 文件 | 只有一条规则 |

`style init <name>` 生成目录包骨架，`style init <name> --single` 生成单文件包。

```text
demo/                      # 目录包
├── STYLE.yaml             # 包级字段、rules、native_hooks
├── rules/
│   └── review.md          # 规则文件：front matter + 正文，由 from 引用
└── prompts/
    └── default.md         # 正文文件：纯文本，由 file 引用

tone.style.md              # 单文件包：front matter + 正文
```

## 行为

### 目录包

1. **读取 `STYLE.yaml`**：必须是单个 YAML 文档，顶层是 mapping（`core/spec/frontmatter.go` `parseYAMLMap`）。
2. **拆分字段**：`rules` 和 `native_hooks` 之外的顶层字段都是包级元数据（`core/spec/load.go` `loadDirectoryFrom`）。
3. **展开规则**：`rules` 中的每一项按以下三种方式之一读取（`core/spec/load.go` `loadDirectoryFrom`）。
   - **内联正文**：用 `text` 直接写正文。
   - **正文文件**：用 `file` 引用包内文件。文件不能有 front matter，也不能位于 `rules/` 下。
   - **规则文件**：用 `from` 引用 `rules/` 下的 `.md` 文件，支持 glob（`*`、`?`、`[...]`、`**`）。每个匹配的文件成为一条规则：front matter 是规则字段，正文是规则正文，没有 `id` 时取 `rules/` 之后、去掉 `.md` 的路径。`from` 条目上的其他字段作为覆盖值作用于每个匹配的文件。
4. **读取原生 hook**：`native_hooks` 中的每一项必须是 mapping，见 [native-hooks.md](native-hooks.md)。
5. **未声明的规则文件**：`rules/` 下没有被任何 `from` 引用的 `.md` 文件不会加载，校验时报 `undeclared-rule-file` 警告（`core/spec/validate_files.go`）。

### 单文件包

文件名必须以 `.style.md` 结尾，内容是 front matter 加正文（`core/spec/load.go` `loadSingleBytes`）：

- front matter 里的包级字段（如 `spec`、`name`）作为包元数据，规则字段（如 `target`、`action`）作为唯一一条规则的字段。`priority` 同时作用于两者。
- 正文就是这条规则的正文。
- 没有 `id` 时，规则 id 取包的 `name`。
- 不能声明 `rules`、`native_hooks`、`from`、`file`、`text`。

### 文件读取限制

包内所有文件都要满足以下限制（`core/spec/package_files.go`）：

- 路径必须是包内相对路径，不能以 `/` 开头，不能含 `..` 或反斜杠（诊断码 `path-escape`）。
- 路径上不能有符号链接，包入口本身也不能是符号链接（`symlink`）。
- 单个文件不超过 256 KiB（`file-too-large`）。
- 必须是 UTF-8 文本，不含 NUL 字节（`bad-encoding`）。
- front matter 必须以 `---` 开始并以 `---` 结束（`invalid-frontmatter`）。

### 加载之后

加载阶段的错误（如 `file-not-found`、`invalid-yaml`）会立即中止，一次只报告一个；修复后再运行才能看到下一个。

1. **校验**：依次检查包级字段、规则、原生 hook 和未声明的规则文件，生成诊断（`core/spec/validate.go`）。有 error 级诊断时，`style validate` 返回非零退出码，包不能安装。
2. **合并默认值**：为每条规则计算最终值，移除 `enabled: false` 的规则，见 [defaults.md](defaults.md)（`core/spec/merge.go`）。
3. **计划**：针对每个 harness 计算规则的落点和降级情况，见 [fallback.md](fallback.md)。用 `style plan` 查看。
4. **安装**：`style install` 按计划写入 harness 配置。

## 示例

### 最小示例

```yaml
spec: style/v0.3
name: overview-min
version: 0.1.0
description: overview minimal example
rules:
  - id: house-style
    target: system
    text: Answer in English.
```

### 三种规则写法

```yaml
spec: style/v0.3
name: overview-mixed
version: 0.1.0
description: inline, file and from rules
rules:
  - id: house-style          # 内联正文
    target: system
    text: Answer in English.
  - id: style                # 正文文件
    target: system
    file: prompts/default.md
  - from: rules/*.md         # 规则文件，每个匹配的文件一条规则
    priority: 40             # 覆盖每个规则文件的 priority
```

`rules/review.md`：

```markdown
---
target: system
---
Review changes before committing.
```

这条规则的 id 是 `review`。

### 单文件包

`tone.style.md`：

```markdown
---
spec: style/v0.3
name: tone
version: 0.1.0
description: single-file package example
target: system
---
Keep answers short.
```

这个包只有一条规则，id 是 `tone`。

## 校验与诊断

| 诊断码 | 级别 | 触发条件 |
|---|---|---|
| `invalid-package` | error | 入口既不是目录也不是 `.style.md` 文件；`rules` 或 `native_hooks` 不是列表 |
| `invalid-yaml` | error | YAML 无法解析、不是 mapping 或包含多个文档 |
| `invalid-frontmatter` | error | front matter 缺少开始或结束的 `---` |
| `body-has-frontmatter` | error | `file` 引用的正文文件带有 front matter |
| `file-into-rules` | error | `file` 指向 `rules/` 下的文件 |
| `file-not-found` | error | 引用的文件不存在，或 `from` glob 没有匹配任何文件 |
| `path-escape` | error | 路径不是包内相对路径 |
| `symlink` | error | 包入口或包内路径上有符号链接 |
| `file-too-large` | error | 文件超过 256 KiB |
| `bad-encoding` | error | 文件不是 UTF-8 文本或包含 NUL 字节 |
| `bad-glob` | error | `from` glob 语法无效 |
| `undeclared-rule-file` | warning | `rules/` 下的 `.md` 文件没有被任何 `from` 引用 |

全部诊断码见 [diagnostics.md](diagnostics.md)。

## 相关

- [package：包级元数据](package.md)
- [rules：规则来源与正文](rules.md)
- [defaults：包级默认值](defaults.md)
- [fallback：落点不可用时的处理](fallback.md)
- [native_hooks：原生 hook](native-hooks.md)
