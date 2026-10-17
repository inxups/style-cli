# STYLE CLI 使用教程

更新时间：2026-10-18。当前里程碑为 M3，基线、全部功能实现与 A0–A18 离线/升级验收已通过；Windows 原生自动化三平台结果见 [A17](review/M3-A17-2026-10-18.json)。真实 B/C 与最终 CI 的进度以 [M3 任务](STYLE-M3-TASKS.md)和[测试矩阵](STYLE-M3-TESTS.md)为准。M1/M2 已关闭并保留只读历史；官方固定语料计数仍两家各 0/40、非阻塞，精确本地编码不替代模型校准。

本教程面向第一次使用 STYLE 的人，从构建到卸载走完整生命周期。早期示例输出来自隔离沙箱，路径已缩写；M3 命令和边界在第 16 节汇总，验证依据链接到各功能指南。规则语义以[规范](spec/STYLE-spec.md)为准，实现职责见 [adapter 总览](STYLE-ADAPTERS-CURRENT.md)，当前真实环境步骤见 [M3 手册](STYLE-M3-MANUAL.md)。

## 目录

1. [STYLE 是什么](#1-style-是什么)
2. [准备与构建](#2-准备与构建)
3. [先在沙箱里试一遍](#3-先在沙箱里试一遍)
4. [写一个规则包](#4-写一个规则包)
5. [validate 与 plan：安装前先看](#5-validate-与-plan安装前先看)
6. [install：写入 Claude Code / Codex](#6-install写入-claude-code--codex)
7. [验证规则真的生效](#7-验证规则真的生效)
8. [修改规则：copy、link、sync、trust](#8-修改规则copylinksynctrust)
9. [uninstall](#9-uninstall)
10. [日常检查：list、doctor、adapters、stats](#10-日常检查listdoctoradaptersstats)
11. [命令速查与退出码](#11-命令速查与退出码)
12. [目录与文件布局](#12-目录与文件布局)
13. [常见场景](#13-常见场景)
14. [排错](#14-排错)
15. [当前不支持的功能](#15-当前不支持的功能)
16. [M3 工作流程](#16-m3-工作流程)

---

## 1. STYLE 是什么

STYLE 把一组**短规则**写进 Claude Code 和 Codex，用来微调它们的行为，例如“评审先给结论”“禁止强制推送”“新增代码里出现 TODO 就提醒”。规则放在一个**包**里，用一份 `STYLE.yaml` 清单加若干 Markdown 正文描述。

整个流程分四步：

```
写包 ──▶ validate / plan ──▶ install ──▶ 运行时 hook
(STYLE.yaml)  (只读，安装前预览)  (写入 harness 配置)  (style 二进制被 harness 调用)
```

要点：

- **`style` 同时是管理 CLI 和 hook 入口。** 普通规则由 Claude Code / Codex 调用 `style … agenthooks run --provider=…`，决定是否注入文字、警告或阻断；`target: native` 直接执行作者声明的命令。
- **运行时不读规则源码。** 安装时把规则编译成 `$STYLE_HOME/build/index.json`，hook 只读这份编译产物和状态目录。
- **规则有三种动作：** `inject`（给模型追加上下文）、`warn`（用户看全文，模型只看一句）、`block`（阻断工具调用或提示词）。
- **注入文字可能随历史重复进入后续请求。** 缓存命中、压缩和上下文管理会改变保留范围与费用；`plan` 提供规则级情景估算，`inject` 默认每个会话只注入一次。
- **做不到就说。** 某个 harness 做不到的规则会按 `fallback` 降级、跳过或拒绝安装，并写进报告。

当前支持的 harness：`claude-code`、`codex`。

---

## 2. 准备与构建

需要 Go 1.25 或更新版本（见 `go.mod`）。**把二进制放在固定位置**，因为安装时生成的 hook 保存的是它的绝对路径；不要用 `go run`，也不要在安装后移动或删除它。

```bash
cd /path/to/style-cli
mkdir -p ~/.local/bin
go build -o ~/.local/bin/style ./cmd/style
~/.local/bin/style --help
```

仓库的测试按 [AGENTS.md](../AGENTS.md) 使用临时缓存目录和清理后的环境变量，贡献代码时用 `make test-go`。本教程只需要上面这条构建命令。

三个环境变量决定 `style` 操作哪里（都不设时用默认值）：

| 变量 | 作用 | 默认值 |
|---|---|---|
| `STYLE_HOME` | STYLE 自己的数据：已装包、编译产物、状态、lock | `~/.style` |
| `CLAUDE_CONFIG_DIR` | Claude Code 配置目录 | `~/.claude` |
| `CODEX_HOME` | Codex 配置目录 | `~/.codex` |

`style` 是否“检测到”某个 harness，取决于它的可执行文件在 `PATH` 中，或者配置目录里已有它的配置文件（Codex：`config.toml` / `hooks.json`；Claude：`settings.json` / `CLAUDE.md` / `rules/`）。

---

## 3. 先在沙箱里试一遍

第一次用建议把三个目录都指到临时位置，这样不会碰到你真实的 Claude / Codex 配置：

```bash
export STYLE_HOME=$(mktemp -d)/style-home
export CLAUDE_CONFIG_DIR=$(mktemp -d)
export CODEX_HOME=$(mktemp -d)
echo '{}' > "$CLAUDE_CONFIG_DIR/settings.json"   # 让 style 认为 Claude 配置存在
touch "$CODEX_HOME/config.toml"                  # 同上，Codex

style adapters      # 确认两个 harness 都 detected=true
style init demo     # 生成一个可用的包骨架
style validate demo
style plan demo
style install demo --route codex=codex-user --yes   # 不指定路线时 Codex 会选 codex-plugin，见 6.3
style list
style uninstall --all --yes
```

沙箱的好处：随时 `rm -rf` 即可重来。确认流程理解后，再取消这些环境变量，对真实目录操作（见第 6 节的备份建议）。

---

## 4. 写一个规则包

### 4.1 创建骨架

```bash
style init team-style          # 目录包：team-style/
style init --single quick      # 单文件包：quick.style.md
```

目录包生成的文件：

```
team-style/
├── STYLE.yaml          # 清单：包信息 + rules[]
├── prompts/default.md  # 正文文件（只有文本，没有 frontmatter）
└── rules/review.md     # 规则文件（frontmatter + 正文）
```

`style init` 不会覆盖已有内容；目标已存在时报错并以退出码 1 结束：

```
style init: app: init destination already exists: …/my-style
```

不带名字时，在**当前空目录**里就地初始化。包名只能是小写字母、数字和连字符。

### 4.2 一个完整的例子

下面这个 `team-style` 包包含四条规则：静态 system 指令、依赖运行时模型名的动态指令、阻断强制推送、新增 TODO 时警告。把它们建出来：

```bash
mkdir -p team-style/prompts team-style/rules
```

`team-style/STYLE.yaml`：

```yaml
spec: style/v0.3
name: team-style
version: 0.1.0
description: 团队评审风格：先给结论，禁止强推，提示 TODO
reads:
  model: true                     # 声明要读运行时的 model，模板里才能用 {{model}}
settings:
  - name: max_findings            # 安装时可用 --set max_findings=5 覆盖
    description: 每次评审最多列几条问题
    default: "10"
rules:
  - id: style                     # 内联规则：正文来自 prompts/style.md
    target: system
    file: prompts/style.md
  - id: model-info                # 动态 system：用到了运行时变量
    target: system
    file: prompts/model-info.md
  - id: no-force-push             # 阻断：条件写在 when 里，text 是拒绝理由
    target: tool
    tool: shell
    action: block
    when:
      conditions:
        - {field: tool.input.command, op: shell_regex, pattern: '^git\s+push\b.*(--force|\s-f\b)'}
    text: 不要强制推送，请新建提交。
  - from: rules/no-todo.md        # 引入一个规则文件
```

`team-style/prompts/style.md`（正文文件，**不能**以 `---` 开头）：

```markdown
本仓库评审先给结论，每条问题带 file:line，最多 {{settings.max_findings}} 条。
```

`team-style/prompts/model-info.md`：

```markdown
当前会话模型：{{model | default: "未知"}}。
```

`team-style/rules/no-todo.md`（规则文件：frontmatter 写规则字段，正文就是注入文本）：

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

> 上面的 `no-force-push` 正则只是教学用的简化版，会漏掉 `git -C repo push --force` 之类的写法。生产用请参考 [example/terse-reviewer/STYLE.yaml](../example/terse-reviewer/STYLE.yaml) 里的 `shell_regex` 写法，它逐条简单命令匹配并处理了全局选项、引号和 `bash -c`。

### 4.3 核心概念速查

**包里有什么。** `STYLE.yaml` 的 `rules[]` 是规则的**唯一清单**：规则文件不会自动生效，必须用 `from:` 引入；`rules/` 下没被引入的 md，`validate` 会给 `undeclared-rule-file` 警告。

**规则由 target + action 决定做什么：**

| target | 含义 | 常用字段 |
|---|---|---|
| `system` | 常驻指令。无 `when` 且正文静态 → 写进静态位置（Claude 的每包 rules 文件、Codex 的 `developer_instructions`），不走 hook；否则走动态评估 | — |
| `tool` | 某类工具调用前/后 | `tool:`（分类，如 `shell`、`file.edit`、`file.write`）或 `names:`（原生工具名）、`at: before\|after` |
| `mcp` | 某个 MCP 工具 | `mcp: "server/tool"`，支持 `server/*` |
| `skill` | 某个 skill 被使用 | `skill: <名字>`，同时覆盖隐式（模型自己用）和显式（用户点名） |
| `event` | 统一事件 | `event:` 取 `session.start` / `prompt.submitted` / `tool.pre` / `tool.post` / `tool.error` / `subagent.start` |
| `native` | 直接写入原生 hook，不经过规则引擎 | 每个 harness 一个含 `event` / `command` 及可选 `matcher` / `timeout` 的映射 |

| action | 模型看到什么 | 用户看到什么 |
|---|---|---|
| `inject`（默认） | 正文 | — |
| `warn` | `model_text`，默认是正文第一句 | 正文全文 |
| `block` | 拒绝理由（工具调用）或不进模型（提示词） | 拒绝理由 |

**常用控制字段：**

| 字段 | 说明 |
|---|---|
| `when` | 条件，**全部**满足才触发；取不到数据的条件算不满足。可按 `model`、`thinking_effort`、`harness.name` 与 `conditions` 匹配 |
| `repeat` | `every` / `turn` / `session`。`inject` 默认 `session`（同会话同内容只注入一次），`warn` 默认 `every` |
| `reinject_on_compact` | 上下文压缩后是否补发；`system` 默认 `true` |
| `fallback` | harness 做不到时：`degrade`（默认，尝试降级，可能最终跳过）、`skip`、`error`（拒绝安装） |
| `priority` | 数字小的排在前面，同一次调用命中多条时按它拼接 |
| `enabled` | `false` 停用但保留文件 |

**模板** 只有替换，没有逻辑：`{{vars.x}}`、`{{settings.x}}`、`{{model | default: "未知"}}`。工具输入输出、prompt 等**事件内容不能进模板**，只能在 `when.conditions` 里匹配。用到的运行时变量必须先在 `reads` 里声明，否则报 `reads-missing`。

**单文件包** 把包信息和**一条**规则合在一个 `<name>.style.md` 里，适合只有一条规则的场景；需要第二条时改成目录包。

完整字段与约束见 [规则定义](spec/STYLE-spec-rules.md) 与 [包结构](spec/STYLE-spec-package.md)。规则写作建议：写事实陈述而不是“你必须…”之类的命令式文字；一条规则只说一件事；挂得越窄越省 token。

### 4.4 原生 hook

需要执行自定义命令时使用 `target: native`。在前面建立的隔离沙箱里创建一个独立示例包：

```bash
mkdir native-notice
cat > native-notice/STYLE.yaml <<'YAML'
spec: style/v0.3
name: native-notice
version: 0.1.0
description: Stop 事件的原生命令示例
rules:
  - id: on-stop
    target: native
    codex:
      event: Stop
      command: "printf STYLE_NATIVE_NOTICE >&2"
      timeout: 10
    claude-code:
      event: Stop
      command: "printf STYLE_NATIVE_NOTICE >&2"
      timeout: 10
YAML
style validate native-notice
style plan native-notice --harness codex,claude-code
style install native-notice --harness codex,claude-code --route codex=codex-user --yes
style uninstall native-notice --yes
```

`plan` 显示 `native-hook`、事件和实际 timeout，命令在安装确认和 `trust` 预览中披露。上面的命令仅向 stderr 写标记；安装不会立刻执行它，真正触发由 harness 的 `Stop` 事件决定。Codex 插件路线仍需按第 6.3 节登记并启用插件，Claude 仍需工作区信任。

native 的事件名和 matcher 使用各 harness 的原生写法，不使用 STYLE 的 `event: session.start` 或 `tool: shell`。不写 `action`、`when`、`repeat` 等规则引擎字段；命令的输出、退出码和副作用由作者负责。native 不参与跨包去重、不进入 `style stats`，也不估算输出 token。普通包和已授权 `--link` 包只要变更 native 命令、事件、matcher、timeout 或条目顺序，都需重新 `trust`；`sync` 退出 3。

---

## 5. validate 与 plan：安装前先看

这两个命令都是**只读**的，不改任何配置。

### 5.1 validate

```bash
style validate team-style
```

```
valid
```

出错时逐条列出错误码、位置和原因，退出码 1。比如把包名写成 `Bad_Name`：

```
bad-name STYLE.yaml: name must match ^[a-z0-9][a-z0-9-]{0,63}$
```

脚本里用 `--json` 取得结构化结果：

```json
[
  {
    "code": "bad-name",
    "path": "STYLE.yaml",
    "severity": "error",
    "message": "name must match ^[a-z0-9][a-z0-9-]{0,63}$"
  }
]
```

不带路径时校验当前目录。可识别的输入是包目录或 `*.style.md` 文件；传别的类型会得到 `invalid-package: … expected a package directory or .style.md file`。

### 5.2 plan

```bash
style plan team-style --harness claude-code
```

（省略 `--harness` 或写 `auto` 时，对检测到的 harness 都做规划；也可写 `--harness codex,claude-code`。）

真实输出（节选）：

```
team-style 0.1.0

Harness: claude-code
Preferred route: claude-user (availability checked during install)
Package: team-style
Adapter: claude-code
- style: native (inject system)
  needs: agent
  -> static:claude_rules inject (native)
- model-info: native (inject system)
  needs: agent, model
  -> session.start inject (native); capabilities: add-context
  -> prompt.submitted inject (native); capabilities: add-context
- no-force-push: native (block tool)
  needs: agent, tool.input.command
  -> tool.pre block (native); capabilities: deny
- no-todo: native (warn tool)
  needs: agent, tool.added
  -> tool.post warn (native); capabilities: add-context, system-message
Token (estimated; calibration pending):
  native input limit: 10000 chars; overflow=file-preview; source=Claude Code hook input limit
  budget: per_rule 400; per_event 1200; always_on 600 tokens
  always-on: 48 tokens / 108 chars
  per-rule:
    style: 24 tokens / 35 chars (native)
    model-info: 24 tokens / 73 chars (native)
    no-force-push: 12 tokens / 13 chars (native)
    no-todo: 24 tokens / 29 chars (native)
  PostToolUse Edit: 24 tokens / 29 chars
  PostToolUse Write: 24 tokens / 29 chars
  PreToolUse Bash: 12 tokens / 13 chars
  SessionStart: 24 tokens / 73 chars
  UserPromptSubmit: 24 tokens / 73 chars
  session cost (requests=60 triggers=3): 343.2 always-on + 459.0 triggered = 802.2 units
  scenario only: 1.25x cache write + 0.1x later hits; assumes an eligible unchanged prefix within cache lifetime; not billing
```

怎么读：

- **每条规则一行状态 + 若干 `->` 投递位置。** `static:claude_rules` 表示写进每包 `rules/style-<名字>.md`，不走 hook；`session.start` / `prompt.submitted` / `tool.pre` / `tool.post` 是 hook 评估点。`model-info` 因为带运行时变量，同时挂在 `session.start` 和 `prompt.submitted`（模型切换后会补发一次）。
- **状态含义：**

  | 状态 | 意思 |
  |---|---|
  | `native` | 该 harness 原生支持，按声明投递 |
  | `degraded` / `partial` | 降级或只保留部分输出（如 `warn` 只剩一个通道） |
  | `skipped` | 这个 harness 做不到，规则被跳过 |
  | `never-trigger` | 条件依赖的数据在该 harness 取不到，永远不会触发 |
  | `native-hook` | `target: native`，直接写入原生命令，不经过规则引擎 |
  | `install-error` | `fallback: error` 的规则做不到，安装会被阻止 |

- **Token 为离线估算，误差尚未完成真实校准，没有百分比误差保证。** `always-on` 是常驻规则估算；`session cost` 用 `--assume-requests N`（默认 60）和 `--assume-triggers N`（默认 3）比较规则的相对成本。公式假设首次写缓存 1.25 倍、之后每次都以 0.1 倍命中，前缀符合模型门槛、内容不变且未过期；不包括基础上下文、工具与模型输出，也不是账单。默认值未被典型工作负载校准。真实会话 usage 与规则估算分列于 [FINAL-10 缓存与成本记录](review/M1-FINAL-10-CACHE-2026-10-16.md)。

跳过与拒绝的真实例子。下面这条规则要求 `tool.error` 事件，在 Codex 上没有这个事件，又写了 `fallback: error`：

```
err 1.0.0

Harness: codex
Preferred route: codex-plugin (availability checked during install)
Package: err
Adapter: codex
- on-err: install-error (inject event)
```

`plan` 退出码为 **4**（blocked），`install` 也会因此被拒绝。把 `fallback` 换成 `degrade` 或 `skip` 则变成 `skipped`，其他规则照常安装。再比如 `mode: replace` 的 system 规则，两家都没有替换槽位：

```
- a: skipped (inject system)
  needs: agent
  ! adapter has no static replace location
```

`plan` 的 `--json` 输出顶层字段是 `package`、`version`、`reports`；每个 harness 一份 report，含 `route`、`plan`、`tokens`、`session_cost` 等。

---

## 6. install：写入 Claude Code / Codex

### 6.1 安装前先备份（真实目录）

`install` 会修改你的 harness 配置。对真实目录操作前，备份即将被改的文件：

```bash
cp -p ~/.claude/settings.json ~/.claude/CLAUDE.md /safe/place/ 2>/dev/null
cp -Rp ~/.claude/rules /safe/place/ 2>/dev/null
cp -p ~/.codex/config.toml ~/.codex/hooks.json /safe/place/ 2>/dev/null
```

STYLE 只改自己负责的托管区，保留你自己的配置和其他工具的 hook；`uninstall` 会还原。备份是为了出意外时有退路。

### 6.2 安装

```bash
style install team-style --harness claude-code --set max_findings=5 --yes
```

真实输出末尾：

```
installed team-style 0.1.0
route: claude-code=claude-user
```

在安装前会先打印和 `plan` 相同的能力/降级报告。**终端里需要你确认报告；非交互环境（脚本、CI）必须加 `--yes`**，否则报错：

```
style install: installation requires capability confirmation; pass --yes in non-interactive mode
```

选项说明：

| 选项 | 说明 |
|---|---|
| `--harness h` | 目标 harness，可重复或用逗号：`--harness codex,claude-code`。省略时安装到检测到的 harness |
| `--route h=r` | 指定路线，如 `--route codex=codex-user`。可用路线：`codex-plugin`、`codex-user`、`claude-user`（以 `style adapters` 为准） |
| `--set k=v` | 覆盖包里声明的 `settings`。未知名称报 `unknown setting "nope"`；没有默认值又没传的设置会报 `requires --set` |
| `--link` | 链接安装，保留对源目录的引用，便于开发（见第 8 节） |
| `--allow-replace` | 授权 `mode: replace` 的规则。注意：两家 adapter 当前都没有替换槽位，该选项不会凭空创造能力 |
| `--fail-closed h` | 仅 Claude：让阻断类 PreToolUse hook 在 `style` 自身出错时也拒绝工具调用。要求检测到 Claude Code ≥ 2.1.295，否则报 `Claude Code version is required for fail-closed PreToolUse hooks (requires >= 2.1.295)`。Codex 不支持 |
| `--yes` | 跳过交互确认 |
| `--json` | 输出结构化结果：`package`、`version`、`changed`、`link`、`replace`、`routes`、`reports` |

**同一个包名不能重复安装：**

```
style install: package "team-style" is already installed; use sync/trust or uninstall before replacing it
```

### 6.3 选路线：Codex 建议用 `codex-user`

| 路线 | 写到哪里 | 说明 |
|---|---|---|
| `claude-user` | `$CLAUDE_CONFIG_DIR/settings.json` 的 hooks + `rules/style-<名字>.md` | Claude 的唯一路线；是否信任工作区由 Claude 自己决定，`doctor` 会提示 `route-trust-unverified`，这是预期的 |
| `codex-user` | `$CODEX_HOME/hooks.json` + `config.toml` | 随安装预置 hook 信任摘要；**推荐** |
| `codex-plugin` | 插件目录 `$STYLE_HOME/adapters/codex/plugin` | Codex 的默认首选，但插件的 marketplace 登记/启用 STYLE 不会自动完成，需要手动管理 |

所以安装 Codex 时通常显式写 `--route codex=codex-user`。实测：检测到 Codex 又不指定路线时，输出的是 `route: codex=codex-plugin`。

### 6.4 安装后写入了什么

Claude 的 `rules/style-team-style.md` 内容如下（`max_findings=5` 已代入）：

```
<!-- style-rules:v1 team-style -->
本仓库评审先给结论，每条问题带 file:line，最多 5 条。
<!-- /style-rules:v1 team-style -->
```

Claude 的 `settings.json` 里出现若干 hook，每条形如：

```json
{
  "type": "command",
  "command": "/home/you/.local/bin/style",
  "args": [
    "--style-home=/home/you/.style",
    "--harness-home=/home/you/.claude",
    "--harness-version=",
    "agenthooks", "run", "--provider=claude-code", "--owner=style"
  ],
  "timeout": 60
}
```

Claude 按编译结果订阅事件（本例：`PostModelSwitch`、`PostToolUse` 的 `Edit/MultiEdit/NotebookEdit/Write`、`PreToolUse` 的 `Bash`、`SessionStart`、`UserPromptSubmit`）。Codex 则固定订阅五类事件，工具范围为 `Bash`、`apply_patch` 和 MCP；静态文本进入 `config.toml` 的根级 `developer_instructions` 托管区，并写入 `hooks.state` 信任摘要。

升级旧安装不会立即改文件。首次成功的 `install`、`sync`、`trust` 或 `uninstall` 会把仍有效的旧 `CLAUDE.md` 托管块迁到 rules，并报告包名和新旧路径；卸载目标直接删除，不生成新文件。单纯搬迁无需额外 trust，其他包的未批准源码仍使用批准基线。`doctor` 对有效旧安装提示 `static-migration-pending`。同名外部文件、管理文件漂移、symlink 或损坏旧标记会使操作退出 1；先恢复批准版本或解决冲突，再重试。STYLE 保留旧块外的字节与权限。

**让改动生效：新开 Claude / Codex 会话。** Claude 2.1.300的FINAL-06已实测：当前进程新增Read matcher可直接生效；此观察不保证所有版本、事件或Codex也会热加载。Claude 首次在工作区使用时按它自己的流程确认信任，可在会话里用 `/hooks` 查看已加载的 hook。

---

## 7. 验证规则真的生效

### 7.1 在真实会话里验证

新开会话后依次观察：

1. **静态规则**：核对 Claude rules / Codex `developer_instructions` 的加载证据；模型复述只能作为辅助，不能代替请求或原生加载观察。
2. **动态规则**：本例的 `model-info` 应让模型知道当前模型名。
3. **阻断**：让它执行 `git push --force`，应看到被拒绝并带上规则里的理由。
4. **警告**：让它写入一行 `// TODO`，用户界面应看到警告全文。

**检查与排查的依据是日志和统计，不是文件是否存在。** 配置文件写对了不等于模型已经收到注入（见第 14 节）。

### 7.2 不开 harness，直接喂 payload

hook 就是一个读 stdin、写 stdout 的进程。可以手工喂入 JSON 验证规则逻辑。命令参数照抄 `settings.json` 里生成的 `args`：

```bash
H() { style --style-home="$STYLE_HOME" --harness-home="$CLAUDE_CONFIG_DIR" --harness-version= \
        agenthooks run --provider=claude-code --owner=style; }
```

**SessionStart：动态 system 注入模型名**

```bash
echo '{"session_id":"s1","cwd":"/work","model":"claude-sonnet-5-5","hook_event_name":"SessionStart","source":"startup"}' | H
```

```
{"hookSpecificOutput":{"additionalContext":"当前会话模型：claude-sonnet-5-5。\n","hookEventName":"SessionStart"}}
```

**强制推送被阻断：**

```bash
echo '{"session_id":"s1","cwd":"/work","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git push --force origin main"}}' | H
```

```
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"不要强制推送，请新建提交。"}}
```

**普通推送放行**（没有输出时 Claude 一侧是 `{}`；Codex 一侧是空输出）：

```
{}
```

**新增 TODO：警告同时给模型和用户**

```bash
echo '{"session_id":"s1","cwd":"/work","hook_event_name":"PostToolUse","tool_name":"Edit","tool_input":{"file_path":"a.go","old_string":"x","new_string":"x // TODO fix"},"tool_response":{}}' | H
```

```
{"hookSpecificOutput":{"additionalContext":"本仓库不在代码里留 TODO 注释，没做完的事列在回复里。","hookEventName":"PostToolUse"},"systemMessage":"本仓库不在代码里留 TODO 注释，没做完的事列在回复里。"}
```

**去重与补发。** 同一会话里，`SessionStart` 之后再发一条提示词，模型没变就不再注入；换了模型，正文变了，补发一次：

```bash
# 同模型：无输出
echo '{"session_id":"s2","cwd":"/work","model":"claude-sonnet-5-5","hook_event_name":"UserPromptSubmit","prompt":"hi"}' | H
# {}

# 模型变了：补发
echo '{"session_id":"s2","cwd":"/work","model":"claude-opus-5-5","hook_event_name":"UserPromptSubmit","prompt":"hi"}' | H
# {"hookSpecificOutput":{"additionalContext":"当前会话模型：claude-opus-5-5。\n","hookEventName":"UserPromptSubmit"}}
```

hook 的运行日志写到 **stderr**，每次规则命中一行，**不含 prompt 或正文，只有规则 ID、评估点、动作和结果**：

```
level=INFO msg="STYLE rule evaluated" provider=claude-code event=PreToolUse point=tool.pre entry=team-style/no-force-push@tool.pre action=block result=blocked
```

各事件的真实 payload 样例（含 Codex）在 [testdata/payloads/](../testdata/payloads/README.md)，可直接复制改字段。

---

## 8. 修改规则：copy、link、sync、trust

### 8.1 两种安装方式

| | 复制安装（默认） | 链接安装（`--link`） |
|---|---|---|
| 规则来源 | 安装时复制一份到 `$STYLE_HOME/packages/<name>/` | 引用你的源目录 |
| 改源码后 | **不影响**已安装内容；`sync` 报 `changed=false` | 首次需 `trust` 授权开发链接；之后 `sync` 校验并自动应用变化 |
| 适合 | 稳定使用、分发给他人的包 | 编写和调试规则 |
| `list` 显示 | `link=no` | `link=dev` |

要更新复制安装的包：`style uninstall <name> --yes` 后重新 `install`。

### 8.2 链接安装的开发循环

```bash
style install team-style --harness claude-code --link --set max_findings=5 --yes
style sync   # 还没编辑：首次开发链接授权
```

```
team-style (link=true): 0 changed files
approval required: style trust team-style
```

此时文件未变，`0 changed files` 是预期结果；首次开发链接授权仍使 `sync` 退出 **3**。如果先编辑 `prompts/style.md`，则会列出 `1 changed files`，`list` 显示 `hash_matches=false`。批准前先预览 diff（不带 `--yes` 时只预览、不应用，退出码 1）：

```bash
style trust team-style         # 预览 Source diff / Rendered diff + 新的 plan
style trust team-style --yes   # 批准并应用
```

```
trust complete (changed=true, expired sessions=0)
```

首次批准完成后，编辑 `team-style/prompts/style.md`，再运行 `style sync`：它会列出变化并直接更新 `rules/style-team-style.md`，退出码 0；无需每次重新 `trust`。再运行一次得到 `sync complete (changed=false, expired sessions=0)`。每次同步仍会校验规则；native 定义和顺序变化需重新 `trust`，replace 等策略变更也需显式授权。

M1-FINAL-03 用真实 CLI 重测：源码改动会正确出现在 `list`、changed files 和 Source diff 中；旧教程记录的漂移缺口未复现。首次未改源码的链接也要授权，应与真实文件变化区分。完整结果见 [link 生命周期证据](review/M1-FINAL-03-LINK-2026-10-16.json)。多个包同时需要批准时，`trust <name>` 只批准指定的那个包，其余未批准包保持之前批准的快照。

**为什么要批准？** `trust` 是对当前快照的显式确认；对开发链接，它还授权后续 `sync` 使用变化后的源码。`--allow-replace` 在这里同样只是授权检查。

### 8.3 只想同步状态

`style sync` 检查源码和已安装状态是否一致：一致时输出 `sync complete (changed=false, …)` 并退出 0；普通安装副本有变化或开发链接尚未首次授权时退出 3，由你用 `style trust` 决定是否应用；已授权开发链接的普通正文变化会直接同步，native 变化仍退出 3。`expired sessions=N` 是应用变更后被判为过期的会话状态数。

### 8.4 跨包等价规则

组合安装时，静态变量替换后的完整正文和全部执行语义相同的规则只保留一条，优先级、包安装序号、包内序号小的优先。安装和 `trust` 报告会标明 `deduplicated_into`，被折叠规则保留源级估算，但组合有效成本为零。单包 `plan` 仍独立计算。

正文相同而条件、动作、repeat 或其他执行语义不同的规则会保留；native 也全部保留。同一等价组内任意一个包声明多条等价规则时，整组保守保留并在组合报告说明。卸载保留方后，另一包的规则接替；因为规则身份改变，在已有会话中可能再注入一次。编译阶段被折叠的规则不会生成虚构的运行时 stats 记录。

---

## 9. uninstall

```bash
style uninstall team-style --yes      # 卸载一个包
style uninstall --all --yes           # 卸载全部
```

```
uninstalled team-style
route updated: claude-code=claude-user
route updated: codex=codex-user
```

规则：

- 也需要确认：非交互环境不加 `--yes` 报 `uninstall requires confirmation; pass --yes in non-interactive mode`。
- 卸载一个包会**重新编译剩余的包**并更新各 harness 路线；最后一个包卸载时，STYLE 在 harness 配置里的托管区被清掉。你自己写的配置、其他工具的 hook 保持不变。
- 卸载最后一个包会删除未漂移的 STYLE rules 文件、仅含旧 STYLE 块的 `CLAUDE.md`、`hooks.json` 和原本为空的 `config.toml`；用户 rules 和旧块外的正文保留，原有内容为 `{}` 的 `settings.json` 也保留。
- 卸载后 `style list` 显示 `No packages installed.`

---

## 10. 日常检查：list、doctor、adapters、stats

### 10.1 list

```bash
style list
```

```
team-style 0.1.0 seq=1 source=dir:/path/to/team-style link=no hash_matches=true
```

`seq` 是安装顺序（同优先级时决定拼接先后）；`hash_matches=false` 且列出 `changed:` 文件，说明已装内容与记录不一致。`--json` 可供脚本使用。

### 10.2 doctor

```bash
style doctor
```

真实输出（节选）：

```
healthy=true
team-style 0.1.0 seq=1 source=dir:… link=no hash_matches=true
codex: detected=true version= config=…/codex
  codex-plugin: available=true installed=false trusted=false trust=user-confirmation stable_key=true; available
  codex-user: available=true installed=true trusted=true trust=preseed stable_key=true; available
claude-code: detected=true version= config=…/claude
  claude-user: available=true installed=true trusted=false trust=workspace stable_key=true; available
effort codex: source=unknown raw="" tier=""; configuration fallback only; …
effort claude-code: source=environment raw="high" tier="high"; environment fallback only; …
Top rules (last 7 days; estimated tokens; calibration pending):
  claude-code team-style/model-info: 26 tokens, 2 emits, 1 body hashes
  claude-code team-style/no-todo: 24 tokens, 1 emits, 1 body hashes
  claude-code team-style/no-force-push: 12 tokens, 1 emits, 1 body hashes
state: expired=0 reset=false; legacy stats without estimate=0
effort-unknown: codex: effort is unavailable; conditional effort rules will not match
route-trust-unverified: claude-user: Claude Code workspace trust controls hook execution; external confirmation cannot be inferred from generated files
```

它回答：

- 安装根是否一致、配置/index/包有没有漂移、路线是否安装（`healthy` 为 false 时退出码为 1）。
- **哪些规则最费 token**（近 7 天，按估算值）。
- 提示项：`effort-unknown` 表示依赖推理强度的条件规则在 Codex 上不会匹配（Codex 只在配置里写了 `model_reasoning_effort` 时才有值）；`route-trust-unverified` 对 Claude 是预期的，STYLE 无法从文件推断工作区信任。

`style doctor --reset-state` 清空会话去重状态，会让已注入的规则在新会话里重新注入。排查“为什么没再注入”之前先别用它。

### 10.3 adapters

```bash
style adapters
```

列出每个 harness 的检测结果（版本、配置目录）、路线可用性与信任状态、adapter 输入上限（Codex 2500 token，近似；Claude 10000 字符）、静态槽位，以及每个评估点的能力（`add-context` / `system-message` / `deny`）和可用字段。**写条件规则前先看它**：例如 Codex 的 `tool.error` 能力全部不可用，无条件规则的 degrade/skip 为 skipped，error 则阻止安装；缺失条件数据还可能先报告 never-trigger。单个 harness 检测失败只是报告里的一项，不影响其他 harness。

### 10.4 stats

```bash
style stats
style stats --by session --since 24h
style stats --by rule --since 2026-10-01T00:00:00Z --json
```

默认按规则汇总最近 168 小时的留存运行时记录；`--by session` 将同一 provider/session 的主 agent 和子 agent 合计。`--since` 接受正 duration（如 `30m`、`24h`）或带时区的 RFC3339 时间，`7d` 不是有效 duration。

| 字段 | 含义 |
|---|---|
| `outputs` / `blocks` | 普通规则的输出次数 / 阻断次数 |
| `dedup` / `cap_drops` | 运行时重复抑制次数 / 容量丢弃次数 |
| `estimated_tokens` | output/blocked 中带有效估算和正文摘要的 token 估算之和 |
| `unestimated` | 缺估算或摘要的 output/blocked 次数 |

它只读已有日志，不重新估算历史、不清理状态，也不要求包仍安装。没有记录时输出空结果。静态常驻正文、native 输出和已在编译阶段折叠的规则不在记录中；这里的 token 是规则输出估算，不是会话账单或缓存费用。坏行等可跳过问题在 JSON 的 `quality.partial` 与计数中披露；I/O 等致命错误退出 1。日志可能已被 `sync` / `doctor` 清理，较长窗口只覆盖仍保留的部分。

---

## 11. 命令速查与退出码

```
style init [name] [--single]
style validate [path] [--json]
style plan <source> [--harness auto|codex,claude-code] [--assume-requests N] [--assume-triggers N] [--json]
style install <source> [--harness h] [--route h=r] [--set k=v] [--link] [--allow-replace] [--fail-closed h] [--yes] [--json]
style uninstall <name|--all> [--yes] [--json]
style list [--json]
style sync [--json]
style trust <name> [--allow-replace] [--yes] [--json]
style doctor [--reset-state] [--json]
style adapters [--json]
style stats [--by rule|session] [--since <duration|RFC3339>] [--json]
```

| 退出码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 校验失败、用法错误、冲突、需要确认但未批准、`doctor` 不健康 |
| 3 | 仅 `sync`：源码、native 定义或安装信任需由 `style trust` 批准 |
| 4 | 仅 `plan`/`install`：被阻止（`fallback: error` 做不到、token 预算触发错误） |

在脚本和 CI 里：

```bash
style validate ./pkg || exit 1                       # 校验必须通过
style plan ./pkg --harness claude-code; rc=$?        # 4 = 会被拒绝安装
style sync; [ $? -eq 3 ] && echo "需要人工 trust"
```

---

## 12. 目录与文件布局

```
$STYLE_HOME/                       默认 ~/.style
├── style.lock.yaml                已装包（版本、来源、文件哈希）与各 harness 路线状态
├── packages/<name>/               复制安装的包快照
├── settings/<name>.yaml           安装时的 settings 取值（权限 0600）
├── approved/<name>.json           已批准的快照（trust 的基线）
├── build/index.json               编译产物：hook 运行时只读这份
├── state/                         会话去重状态与统计（只存元数据）
└── cache/agenthooks/              hook 编解码缓存

$CLAUDE_CONFIG_DIR/                默认 ~/.claude
├── settings.json                  合并了 owner=style 的 hooks
└── rules/style-<name>.md           每个有效静态包一份，整文件受摘要保护

$CODEX_HOME/                       默认 ~/.codex
├── hooks.json                     STYLE 固定五类事件的 hook + 包声明的 native hook
└── config.toml                    developer_instructions 托管区 + hooks.state 信任摘要
```

`STYLE_HOME` 隔离的是包和状态；它**不能**隔离两家用户配置里的 `owner=style` 条目。要同时维护两套 STYLE 安装，先通过原实例卸载第一套。

统计和日志只保存规则 ID、评估点、动作、结果、正文哈希和估算 token，**不保存 prompt、工具内容或渲染后的全文**。

---

## 13. 常见场景

### 13.1 只对 Codex 或只对 Claude 生效

用 `when.harness`：

```yaml
reads:
  harness: [name]          # when.harness 用到的键必须先在 reads 里声明
rules:
  - id: codex-only-note
    target: event
    event: prompt.submitted
    when:
      harness: {name: codex}
    text: 评审回复保持简短。
```

没命中的 harness 在 `plan` 里显示为跳过。

### 13.2 按推理强度调整

```yaml
reads:
  thinking_effort: true
rules:
  - id: low-effort-guard
    target: system
    when:
      thinking_effort: "<=low"
    text: 低强度下先确认需求再动手。
```

注意 Codex 的强度来自配置里写的 `model_reasoning_effort`，会话中途用 `/model` 改动看不到；缺值时普通已知档位条件不匹配；M3 可显式使用 `when.thinking_effort: unknown`。用 `style doctor` 查看当前解析结果。

### 13.3 给特定 skill / MCP 追加说明

```yaml
- id: review-skill
  target: skill
  skill: code-review
  file: prompts/skill-code-review.md
- id: before-create-pr
  target: mcp
  mcp: github/create_pull_request
  at: before
  file: prompts/mcp-create-pr.md
  fallback: skip
```

完整可运行的例子是 [example/terse-reviewer](../example/terse-reviewer/STYLE.yaml)：包含静态 system、动态强度规则、skill、MCP、`shell_regex` 阻断、`warn` 等各类写法。

### 13.4 让 token 成本可控

- 先 `style plan --assume-requests 200` 看长会话成本。
- `inject` 保持默认 `repeat: session`；要写 `every` / `turn` 就加 `reason:` 说明理由（否则 lint `T007` 告警）。
- 单条规则默认上限 400 token，单事件 1200，常驻 600（`budget` 可改）；超限默认只告警，超过 harness 上限的 80% 直接报错。
- 阻断理由不要超过 40 token：每被拦一次，理由就进一次模型上下文。

### 13.5 在 CI 里检查规则包

```bash
style validate ./pkg --json
style plan ./pkg --harness codex,claude-code --json
```

仓库自己的 [CI](../.github/workflows/ci.yml) 还会在临时 HOME 里用桩 `codex`/`claude` 做完整端到端冒烟（[scripts/smoke.py](../scripts/smoke.py)），可以作为你自己流水线的参考。

---

## 14. 排错

| 现象 | 原因与办法 |
|---|---|
| `adapters` 里 `detected=false` | 可执行文件不在 `PATH`，且配置目录没有配置文件。设置 `CLAUDE_CONFIG_DIR` / `CODEX_HOME`，或让 harness 至少运行一次 |
| `installation requires capability confirmation; pass --yes` | 非交互环境，加 `--yes`（先读完 plan 输出） |
| `package "x" is already installed` | 对普通来源先 `update x` 预览，再加 `--yes`；开发链接按 `sync` / `trust` 流程 |
| `unknown setting "x"` / `requires --set x=value` | 设置名要与 `settings[].name` 一致；没有 `default` 的设置必须传值 |
| 安装成功但会话里没效果 | 1) 新开会话；2) Claude 里 `/hooks` 看有没有 STYLE hook、工作区信任是否已确认；3) 手工喂 payload（第 7.2 节）确认规则逻辑；4) 看 hook 的 stderr 日志是否有 `STYLE rule evaluated`；5) `doctor` 看路线和漂移 |
| 规则注入了一次后不再出现 | 这是默认去重（`repeat: session`）。需要重复就改 `repeat`，并写 `reason:`；上下文压缩后要补发用 `reinject_on_compact: true` |
| `plan` 退出码 4 | 有 `fallback: error` 的规则在某个 harness 上做不到。改成 `degrade`/`skip`，或用 `--harness` 只装另一家 |
| `fail-closed … requires >= 2.1.295` | 安装级 `--fail-closed` 仅 Claude；规则级 `fail: closed` 在两家可 deny 的直接评估点使用，见[失败策略](STYLE-FAIL-CLOSED.md) |
| hook 报找不到命令 | 安装后移动或删除了 `style` 二进制。把它放回原路径，或重装 |
| 配置目录里有“别人”的 STYLE 条目 | 另一个 `STYLE_HOME` 的安装也在写 `owner=style`。用原实例卸载后再继续 |
| `validate` 提示 `undeclared-rule-file` | `rules/` 下的文件没有被 `from:` 引入，不会生效 |
| `body-has-frontmatter` | `file:` 指向的正文文件开头不能是 `---`。要写规则字段就放进 `rules/` 用 `from:` |

每个错误码的完整含义在 [testdata/invalid/](../testdata/invalid/README.md)（每个文件名就是一个错误码，内含最小复现）。

---

## 15. 当前不支持的功能

以下边界适用于当前 M3；完整状态见[任务清单](STYLE-M3-TASKS.md)。

- 完整 adapter 仍只有 Claude Code 和 Codex，其他 harness 留待 M4。
- 两家均无 replace 槽位，`--allow-replace` 不会创造替换能力；degrade/skip/error 沿用既有语义。
- Codex 中途 `/model` 修改 effort 的可靠观测未纳入 M3；provider 表示配置请求，不能证明网关实际上游或账单主体。
- native 保留 event/matcher/command/timeout 四字段；async、onFailure、HTTP/MCP/prompt/agent 类型明确拒绝，见[native 指南](STYLE-NATIVE.md)。
- Windows 支持两家 user 路线与原生 exe；项目/插件路线和作者 POSIX native 不支持。自动化通过不等于 W1 的实际 harness 验收。
- 用户已延期 Claude 托管云端 C3、Windows 实际主机/harness W1 和 Anthropic 独立计数 API；不得用本地 fixture 或编码对照替代。官方两家各 0/40 另为非阻塞遗留。
- 原生插件登记仍需按报告命令执行；文件存在、登记 enabled、回调 observed 和完整正文投递分别核对。Claude rules 的 compact 行为不由新会话验收外推。

## 16. M3 工作流程

### 16.1 来源、更新和锁恢复

普通 copy 安装使用批准快照；更新先审预览，再确认：

```sh
style update my-style --json
style update my-style --yes --json
style install --frozen --yes --json
```

Git 来源必须显式 `git+https://`，支持公共 HTTPS、完整 ref 和子目录；安装记录固定 commit 和原始对象快照。frozen 仅按当前整锁与绑定离线恢复，不取得新 ref、不批准新源码。详细语法与失败恢复见[来源指南](STYLE-SOURCES.md)。

### 16.2 项目、插件与云端

```sh
style install ./my-style --project . --harness codex,claude-code --yes --json
style doctor --project . --json
style sync --project . --json
style uninstall --all --project . --yes --json
```

项目 home 按规范根隔离；项目与 user 同名冲突、复制 clone 与原生工作区 trust 均有显式门禁，见[项目指南](STYLE-PROJECTS.md)。项目参数须沿用到管理、probe、explain 和 stats 等诊断命令。

Claude 插件显式选 `--route claude-code=claude-plugin`，生成后 registration-required/退出1仍需原生登记；卸载先停用原生插件，见[插件指南](STYLE-CLAUDE-PLUGIN.md)。云端 export 交付已批准 bundle，不在远端自动取来源或编译，见[云端指南](STYLE-CLOUD.md)。

### 16.3 解释与实际回调

```sh
style explain --harness codex --event PreToolUse --payload payload.json --index /absolute/index.json --json
style probe --harness codex --timeout=60s --json
```

explain 默认空隔离状态，可显式提供 state/facts；它不读取本机隐藏配置，也不请求模型。probe 观察已安装 hook，须由原生会话触发，不能手工调用替代；observed 仍不单独证明正文进了模型请求。见[解释](STYLE-EXPLAIN.md)、[probe](STYLE-PROBE.md)及[M3 真实验收手册](STYLE-M3-MANUAL.md)。

### 16.4 运行时与计数

| 功能 | 使用方式与边界 |
|---|---|
| [refresh](STYLE-REFRESH.md) | `refresh: N` 按已提交回合补发；repeat、compact 和完整单次预算仍分别生效 |
| [事件围栏](STYLE-EVENT-CONTENT.md) | 显式 reads.event / event. 引用；截断和转义后作为不可信内容，不能当作可信指令 |
| [provider](STYLE-PROVIDER.md)、[unknown](STYLE-UNKNOWN.md) | 声明 reads；只用可观测配置请求/缺值，不猜测实际服务或思考行为 |
| [effort 映射](STYLE-EFFORT-MAPPING.md) | 当前 STYLE home 的 effort.yaml，sync 编译；保留 raw，用户别名标记 approx |
| [失败策略](STYLE-FAIL-CLOSED.md) | 规则 `fail: closed` 保护受支持直接事件；index 不可读或宿主终止有明确边界 |
| [exact](STYLE-COUNT.md) | count/plan/explain 显式 opt-in；本地具名编码和 Anthropic 最小消息计数分别披露，预算仍用估算 |

```sh
style count rules.txt --count=exact --count-provider=openai --encoding=o200k_base --json
```

本地编码内置词表，不需 key 或下载；固定实现/编码名、文本摘要与范围随结果返回。API 路线会发送文本且需独立 key，已按用户要求延期实际验收。精确编码数不能称为完整请求、账单或估算误差保证。
