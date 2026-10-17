# STYLE 分发格式 v0.3：包结构与顶层字段

规范总览和已定决定见 [STYLE-spec.md](STYLE-spec.md)；后续章节见 [规则定义](STYLE-spec-rules.md)。

## 1. 包结构和 md 文件

### 1.1 两种 md，按目录区分

目录包里的 md 只有两种角色，由所在目录决定，不靠内容猜：

| 位置 | 角色 | frontmatter | 怎么被用到 |
|---|---|---|---|
| `rules/**/*.md` | **规则文件**：一个文件就是一条规则 | 必须有，写规则字段 | 只能被 `STYLE.yaml` 里的 `from:` 引入（1.3） |
| 其他位置（习惯放 `prompts/`） | **正文文件**：只有要注入的文本 | **不允许**，开头是 `---` 就报错 | 只能被规则的 `file:` 引用 |

```
terse-reviewer/
├── STYLE.yaml              # 清单：包信息 + rules[]，所有规则都在这里列出
├── prompts/*.md            # 正文文件，被 file: 引用
└── rules/**/*.md           # 规则文件，用 from: 引入
```

两条硬约束，保证每个 md 只有一个角色：
- `file:` 不能指向 `rules/` 下的文件（报错 `file-into-rules`，应改用 `from:`）。
- 正文文件不能带 frontmatter（报错 `body-has-frontmatter`），否则作者以为写了一条规则，其实被当成了正文。

### 1.2 规则文件的格式

```markdown
---
target: tool
tool: [file.edit, file.write]
action: warn
when:
  conditions:
    - { field: tool.added, op: contains, pattern: "TODO" }
---
本仓库不在代码里留 TODO 注释，没做完的事列在回复里。
```

- **frontmatter**：第 3 节的任意规则字段，另外允许 `reads`（第 6 节）。`reads` 会并进包级 `reads`，这样规则文件拷进别的包也能用。不允许 `file`、`text`：正文就是文件本身。
- **正文**：`---` 之后的全部内容，去掉开头的空行和结尾的空白，其余原样保留。正文为空时报错 `empty-body`（`block` 也要写拒绝理由）。
- **`id`**：可以不写，默认取相对 `rules/` 的路径去掉 `.md`，例如 `rules/tool/no-todo.md` → `tool/no-todo`。写了就用写的，同样要在包内唯一。
- **不允许包级字段**（`name`、`version`、`settings`、`budget` 等），否则报错 `rule-file-pkg-field`。包级字段只能写在 `STYLE.yaml`，或者单文件包里（1.6）。

### 1.3 和 STYLE.yaml 的关系、字段优先级

**`STYLE.yaml` 的 `rules[]` 是包里全部规则的唯一清单。** 规则文件不会自动生效，必须用 `from:` 引入。这样安装和审核时只看 `STYLE.yaml`，就知道这个包会注入什么、会阻止什么。

`rules[]` 的每一项是下面两种之一：

- **内联规则**：直接写规则字段，加上 `file:` 或 `text:`。
- **引入规则文件**：写 `from:`，后面可以再写要覆盖的字段。

```yaml
rules:
  - id: base                        # 内联规则
    target: system
    file: prompts/system.md
  - from: rules/tool/no-todo.md     # 引入一个规则文件
    priority: 10                    # 只覆盖这里写出来的字段
  - from: rules/legacy.md
    enabled: false                  # 停用，但保留文件
  - from: rules/review/*.md         # glob：作者明确选择引入整个目录
```

- **两种写法互斥**：有 `from:` 的项不能再写 `id`、`file`、`text`；没有 `from:` 的项必须写 `id`，并且 `file` 和 `text` 二选一。违反时报错 `from-and-inline` / `inline-missing-id` / `file-and-text`。
- **`from:` 条目不能覆盖 `target`**（报错 `override-target`）：换了 target，规则文件里和旧 target 配套的字段（`tool`、`mcp`…）就对不上了。
- `from:` 可以写 glob，glob 上写的覆盖字段对匹配到的每个文件都生效。
- **同一个 `id` 出现两次就报错**（`dup-id`），不做隐式合并。同一个文件被两项 `from:` 匹配到（比如既写了 glob 又单独写了它）也算重复。
- `rules/` 下没被任何 `from:` 引入的 md，validate 给警告 `undeclared-rule-file`："未声明，不会生效"。
- 安装报告和 `style plan` 里标明每条规则是内联的，还是来自哪个文件。

**字段取值的优先级**，从低到高：

1. 规范默认值（3.1 的表）
2. 包级 `defaults:`
3. 规则文件的 frontmatter
4. `STYLE.yaml` 里 `from:` 条目上写的字段

覆盖按**顶层字段整体替换**：`when` 写了就整个替换，不和 frontmatter 里的 `when` 逐项合并。`reads` 是例外，各层取并集。

**`defaults:` 只能写这些字段**（其他报错 `defaults-field`）：`action`、`at`、`repeat`、`reinject_on_compact`、`sources`、`fallback`、`priority`、`max_tokens`。`defaults` 对内联规则同样生效。`defaults.priority` 和包级 `priority` 同时写时，`defaults.priority` 优先。

### 1.4 顺序

- **没有自动发现**。只有 `rules[]` 里列出的规则会被编译。
- **包内顺序**就是 `rules[]` 的书写顺序。一个 glob 匹配到多个文件时，这些文件在 glob 所在的位置按相对路径的字节序展开。
- **最终拼接顺序**（同一次 hook 调用命中多条规则时）：先按 `priority` 从小到大，相同时按包的安装顺序，再相同时按包内顺序。这个顺序编译时就定好，写进 `index.json` 的 `order`（第 9 节），运行时不再排序。
- 排在后面的文字在上下文里更靠近末尾。想让某条规则"最后说了算"，就把它的 `priority` 设大。

### 1.5 模板

规则文件正文、正文文件、`text:` 和 `model_text:` 用同一套模板。frontmatter 和 `STYLE.yaml` 的其他字段**不做模板展开**。

- **语法只有替换，没有逻辑**：`{{name}}`，以及带默认值的 `{{name | default: "未知"}}`。要输出字面的 `{{`，写 `\{{`。分支和循环一律不支持；"条件不同、文字不同"就拆成两条规则，各写各的 `when`。
- **可用变量**：

  | 变量 | 何时确定 | M1 能不能进模板 | 前提 |
  |---|---|---|---|
  | `vars.*`、`settings.*`、`package.name`、`package.version`、`rule.id` | 安装时 | 能 | 已定义；`sensitive` 的 settings 在 M1 不可用（8.4） |
  | `model`、`thinking_effort`、`thinking_effort_raw` | 运行时 | 能 | `reads` 里声明 |
  | `harness.name`、`harness.version`、`harness.cwd`、`harness.permission_mode`、`harness.agent` | 运行时 | 能 | `reads.harness` 里声明 |
  | `harness.session_id`、`harness.turn_id` | 运行时 | 能，但 lint `T006` 告警：每次都变，破坏去重和缓存 | 同上 |
  | `prompt`、`tool.name`、`tool.input.*`、`tool.added`、`tool.output`、`mcp.server`、`mcp.tool`、`skill.name` | 运行时，取自当前事件 | **不能**，报错 `event-var-in-template`；只能在 `when.conditions` 里用 | — |

- **取不到值时**：有 `default` 就用默认值，没有就是空字符串。想"取不到就别注入"，就在 `when` 里写条件，不要靠模板。运行时变量没写 `default`、又没有被 `when` 保证有值时，validate 给警告 `var-no-default`。"被 `when` 保证"指：`when.model` 保证 `model`；`when.thinking_effort` 保证 `thinking_effort` 和 `thinking_effort_raw`；`when.harness.<键>` 保证 `harness.<键>`。
- **静态和动态**：只用到安装时变量的正文叫**静态正文**，编译时直接渲染成最终文本；用到运行时变量的叫**动态正文**，编译时只解析，运行时再填值。这个区分决定 `system` 规则能不能写进静态位置（3.3、第 10 节）。
- **validate 会检查**：每个变量都有定义（`undeclared-var`）；运行时变量在 `reads` 里声明过（`reads-missing`）。

### 1.6 单文件包

一个 md 就是一个完整的包：frontmatter 里同时写包级字段（`spec`、`name`、`version`、`description`，可选 `requires`、`reads`、`vars`、`settings`、`recommends`、`priority`、`budget`）和**一条**规则的字段，正文是这条规则的文本。规则 `id` 默认等于 `name`；`priority` 同时作为包级和规则级。文件名约定为 `<name>.style.md`，用 `style install foo.style.md` 安装。

需要第二条规则时，改成目录包：包级字段移到 `STYLE.yaml`，规则字段和正文留在 `rules/<name>.md`，再在 `rules[]` 里加一行 `- from: rules/<name>.md`。

### 1.7 名字、路径和文件的限制

这些名字会变成文件名或标记，路径会被安装器读取，所以都有硬限制（违反即报错）：

| 对象 | 限制 | 错误码 |
|---|---|---|
| 包 `name` | `^[a-z0-9][a-z0-9-]{0,63}$` | `bad-name` |
| 规则 `id` | `^[a-z0-9][a-z0-9_./-]{0,127}$`，不含 `..`、`//`，不以 `/` 开头或结尾 | `bad-id` |
| `file:`、`from:`（含 glob 展开结果） | 相对路径；解析符号链接后仍在包根目录内；不允许绝对路径和逃出包根的 `..` | `path-escape` |
| 符号链接 | 复制安装时遇到任何符号链接都拒绝 | `symlink` |
| 每个文件 | 不超过 256 KiB；合法 UTF-8；不含 NUL | `file-too-large`、`bad-encoding` |
| 正文内容 | 不能出现 STYLE 的静态块标记 `<!-- style:`（卸载时靠它定位） | `marker-in-body` |

## 2. 顶层字段

| 字段 | 必填 | 说明 |
|---|---|---|
| `spec` | 是 | `style/v0.3` |
| `name` / `version` / `description` | 是 | `name` 见 1.7，`version` 用 semver |
| `authors` | 否 | |
| `requires.runtime` | 否 | `style` 二进制的版本范围。`install` 和 `sync` 时检查，运行时不查 |
| `requires.harness` | 否 | 每个 harness 的最低版本，键是 harness 名：`claude-code`、`codex`。没列出的 harness 也能装，按第 5 节降级；版本不满足时跳过这个 harness 并告警 |
| `reads` | 否 | 要读的运行时数据（第 6 节）。不写就是什么都不读 |
| `vars` | 否 | 包内常量，模板里写 `{{vars.x}}` |
| `settings` | 否 | 安装时向用户要的值：`name`、`description`、`default`。值都是字符串。`sensitive` 是保留字段，M1 写了就报错 `sensitive-reserved`（8.4） |
| `recommends` | 否 | 建议调整的 harness 自带风格层，例如 `codex: { personality: none }`。安装器只提示，不改 |
| `priority` | 否 | 默认 50 |
| `budget` | 否 | token 预算（7.2）。不写就用默认值 |
| `defaults` | 否 | 所有规则共用的默认字段，白名单见 1.3 |
| `rules` | 是 | 包里全部规则的清单，至少一条 |

### 2.1 M3-D0 名称校验补充（M3-T00 已实现）

[M3-D0 设计](STYLE-cli-design.md#31-m3-d0-注册表校验边界) 由当前 Service 的注册表统一校验 `requires.harness`、`recommends` 的 harness 键，与 `when.harness.name` 使用同一名称全集。M3-T00 已实现，当前生产默认注册两家；不会因 agenthooks 还有其他 Provider 常量而自动支持其他 harness。

目录包与单文件包共用完整入口。`requires.harness` 的值继续为现有版本约束，版本未满足的安装行为不变；`recommends` 的值继续要求 mapping，安装器仍只提示。注册且暂未检测到的名称不属于未知名；已注册但无 native 能力也可用于这两个字段，不能因此接受该名称的 native 映射。`spec.Validate` 仅做结构/lint 校验；完整有效性使用显式目录的 `ValidateWithCatalog`。旧诊断及 API 兼容选择见详细设计，结果见 [M3-A0](../review/M3-A0-2026-10-17.json)。
