# STYLE.yaml 文档

本目录说明 STYLE 包（`spec: style/v0.3`）的写法。所有页面都按 [_template.md](_template.md) 的格式编写。

## 目录结构

| 文件 | 内容 | 关键字 | 状态 |
|---|---|---|---|
| [_template.md](_template.md) | 标准页面模板 | — | 已完成 |
| [style-yaml/overview.md](style-yaml/overview.md) | 包的形态（目录包、单文件包）、加载流程、默认值合并顺序 | — | 已完成 |
| [style-yaml/package.md](style-yaml/package.md) | 包级元数据 | `spec` `name` `version` `description` `authors` `priority` | 已完成 |
| [style-yaml/requires.md](style-yaml/requires.md) | 依赖、设置与推荐 | `requires` `settings` `recommends` | 已完成 |
| [style-yaml/budget.md](style-yaml/budget.md) | token 预算 | `budget`（`per_rule` `per_event` `always_on` `on`） `max_tokens` | 已完成 |
| [style-yaml/defaults.md](style-yaml/defaults.md) | 包级默认值与推导默认值 | `defaults` | 已完成 |
| [style-yaml/rules.md](style-yaml/rules.md) | 规则来源、正文与模板语法 | `rules` `id` `file` `text` `from` `model_text` `enabled` `reason` | 已完成 |
| [style-yaml/target.md](style-yaml/target.md) | 规则作用对象 | `target` `tool` `names` `mcp` `skill` `event` `sources` `mode` | 已完成 |
| [style-yaml/action.md](style-yaml/action.md) | 规则动作与时机 | `action` `at` `repeat` `fail` `refresh` `reinject_on_compact` | 已完成 |
| [style-yaml/fallback.md](style-yaml/fallback.md) | 落点不可用时的处理与降级路径 | `fallback`（`degrade` `skip` `error`） | 已完成 |
| [style-yaml/when.md](style-yaml/when.md) | 条件匹配 | `when`（`model` `thinking_effort` `thinking_effort_approx` `harness` `agent` `conditions` `provider`） | 已完成 |
| [style-yaml/native-hooks.md](style-yaml/native-hooks.md) | 原生 hook 声明 | `native_hooks`（`event` `matcher` `command` `timeout`） | 已完成 |
| [style-yaml/diagnostics.md](style-yaml/diagnostics.md) | 诊断码总表 | — | 已完成 |

## 写作约定

- **一页一组关键字**：同一个关键字只在一个页面里详细说明，其他页面通过链接引用。
- **以代码为准**：页面内容必须和源码一致。front matter 的 `sources` 列出定义这些关键字的文件，修改这些文件时同步更新对应页面。
- **示例可验证**：以 `spec:` 开头的完整 YAML 示例都要用 `style validate` 验证。正常示例必须没有诊断；故意演示警告或错误的示例，要在正文写出预期的诊断码。
- **链接用相对路径**：页面之间用 `../`、`./` 引用，不写绝对路径。
- **新增页面**：复制 `_template.md`，把文件名和状态登记到上面的目录表。
- **文件命名**：小写字母加连字符，例如 `native-hooks.md`。
