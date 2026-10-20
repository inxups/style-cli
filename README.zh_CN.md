# STYLE

[English](README.md) | 简体中文

STYLE 是一个命令行工具，把编码风格规则和原生 hook 打包成「STYLE」，统一安装到 Codex、Claude Code 等编码 agent（harness）中。

## 快速介绍

STYLE 用一份规范的配置，让 harness 快速变成「你的」harness：按你的习惯写代码、在你在意的时机提醒你、用你自己的脚本把关。

开箱即用的 harness 并不了解你的偏好，而各家的规则文件、hook 事件和配置位置又各不相同：想让 Codex 和 Claude Code 遵守同一套约定，往往要分别研究、手工维护多份配置，换台机器或换个项目又得重来一遍。

STYLE 把这些差异收拢到一个 `STYLE.yaml` 里：

- **一次声明**：用统一的格式写下规则（什么时候、对什么工具、说什么）和原生 hook（在哪个事件运行哪个命令）
- **自动适配**：STYLE 把它编译成每个 harness 的原生配置，并在安装前用 `plan` 展示每条规则如何落地、占用多少 token
- **全程托管**：审批、安装、同步、更新、卸载都由 STYLE 负责，配置改了 `sync` 一下即可生效，不再手动改各处文件
- **随处复用**：同一份配置可以放在本地目录或 Git 仓库里，装到用户范围或单个项目，在多台机器、多个项目之间保持一致

无论是想把个人习惯带到每个 agent 里，还是想让团队共享同一套编码约定，都可以从一份 `STYLE.yaml` 开始。

## 快速开始

### 环境要求

- macOS（通过 Homebrew 安装），或 Go 1.25 及以上（从源码构建）
- 至少一个已安装的 harness：Codex 或 Claude Code

### 安装

macOS 使用 Homebrew：

```bash
brew install inxups/tap/style-cli
```

该 formula 安装 `style` 可执行文件，与 homebrew-core 中的 `style` 冲突，两者不能同时安装。

从源码构建：

```bash
git clone https://github.com/inxups/style-cli.git
cd style-cli
go build -o style ./cmd/style
```

### 运行

创建一个 STYLE，检查并预览它在各 harness 上的效果：

```bash
style init demo
style validate demo
style plan demo
```

`style init demo` 生成 `demo/STYLE.yaml`、`demo/prompts/default.md` 和 `demo/rules/review.md`；`validate` 输出 `valid`；`plan` 按检测到的每个 harness 列出规则的落地方式和 token 估算，例如：

```text
Harness: claude-code
Preferred route: claude-user (availability checked during install)
Package: demo
Adapter: claude-code
- style: native (inject system)
  ...
```

确认无误后安装。STYLE 会先打印预览，在 stderr 上询问 `[y/N]`，只有输入 `y` 或 `yes` 才会写入：

```bash
style install demo
```

## 主要功能

- **原生 hook 管理**：在 `native_hooks` 中声明 harness 原生命令，STYLE 负责披露、审批和安装，执行仍由 harness 完成
- **开发链接**：`style link` 把本地目录链接为开发包，修改后用 `sync` 生效
- **诊断工具**：`list`、`doctor`（含 `--live` 实时回调检查）、`stats`、`explain`
- **机器可读输出**：除 `init` 外的命令都支持 `--json`，输出统一的 JSON 信封

## 使用示例

### 编写 STYLE

目录包可以同时包含普通规则（`rules`）和原生 hook（`native_hooks`），两者的 `id` 不能重复：

```yaml
spec: style/v0.3
name: example
version: 1.0.0
description: Example package

rules:
  - id: shell-reminder
    target: tool
    tool: shell
    at: before
    text: 执行命令前检查风险。

native_hooks:
  - id: command-check
    description: 使用本地脚本检查命令。
    fallback: skip
    claude-code:
      event: PreToolUse
      matcher: Bash
      command: /absolute/path/check-command.sh
      timeout: 10
```

- hook 支持 `id`、`description`、`enabled`、`priority`、`fallback` 以及各 harness 的映射；每个映射只接受 `event`、`matcher`、`command`、`timeout`
- hook 默认启用，优先级 50（或包的优先级），`fallback: skip`；`fallback: error` 表示映射无法安装时阻止安装；缺少某个 harness 的映射时总是跳过
- 普通规则的 `defaults` 不作用于 hook；hook 也不进入 STYLE 的规则引擎、模板、去重、运行统计和 token 预算
- 单文件 `.style.md` 包（`style init NAME --single`）只支持普通规则

### 从 Git 安装

Git 来源使用 go-getter 格式，`//路径` 和 `?ref=` 都是可选的（默认为仓库根目录和远端 HEAD）：

```bash
style install 'git+https://host/org/repo.git//package/path?ref=refs/tags/v1'
```

ref 须写成完整形式：`refs/heads/…`、`refs/tags/…` 或完整的 commit OID。已安装的 Git 包可以用 `style update NAME --ref REF` 切换 ref。

### 安装到项目

全局选项必须写在命令之前：

```bash
style --project . install SOURCE_A SOURCE_B --set SOURCE_A_NAME.KEY=VALUE
```

只有在项目首次安装时才能一次传入多个来源（最多 64 个），它们作为一个事务审核和安装。只有一个来源时用 `--set KEY=VALUE`；多个来源时每项设置都要带包名，即 `--set PACKAGE.KEY=VALUE`。

### 本地开发

```bash
style link ./my-package
style sync
```

链接后第一次 `sync` 需要审批；之后修改普通规则只需 `sync` 即可生效，修改可执行的 hook 声明则需要重新审批（只改 `description` 不需要）。`sync` 会逐个包展示来源差异、渲染差异和原生差异并询问，拒绝其中一个会停止 `sync` 并以 `1` 退出；`sync NAME` 只审批指定的包。`style uninstall NAME` 移除链接。

### 诊断

```bash
style list
style doctor
style doctor --live --timeout 60s
style explain --harness claude-code --payload payload.json
```

`doctor --live` 先做静态检查，再等待每个选中 harness 的回调；stderr 提示就绪后，在 harness 会话中触发一个已订阅的事件即可。`explain` 读取单个 payload，必须指定唯一的 harness，事件默认取 payload 中的 `hook_event_name`。

### 在脚本和 CI 中使用

变更命令在没有输入、输入结束或回答不是 `y`/`yes` 时都会取消且不写入，因此脚本需要显式应答：

```bash
echo y | style install SOURCE --json
```

使用 `--json` 时，预览和提示写到 stderr，stdout 只输出最终报告。报告统一为一个信封：

```json
{"format": 2, "command": "list", "status": "ok", "scope": {"kind": "user", "home": "…"},
 "diagnostics": [], "result": {"packages": []}}
```

`status` 与退出码对应（`ok`、`error`、`blocked`）。退出码含义：

| 退出码 | 含义 |
|---|---|
| `0` | 成功 |
| `1` | 请求被拒绝或执行失败，包括缺少 `--set`、未检测到 harness、包未安装、预览未确认（`confirmation-declined`） |
| `2` | 命令行无法解析：未知命令、选项、harness 或 route，参数缺失、格式错误或冲突（含错误的 Git 地址和 `--ref`）；此时 stdout 不输出任何内容 |
| `4` | 被策略或预算阻止 |

hook 调用永远不会以 `2` 退出，因为 harness 会把它当作阻止。

## 配置说明

| 名称 | 说明 | 默认值 |
|---|---|---|
| `STYLE_HOME` | 用户范围的 STYLE 主目录，也是项目绑定所在的主目录 | `~/.style` |
| `--style-home DIR` | 全局选项，替代 `STYLE_HOME` | 无 |
| `--project DIR` | 全局选项，把 plan、安装和诊断命令限定到指定项目 | 无（用户范围） |
| `--harness` | 选择 harness，可重复、可用逗号分隔，大小写不敏感，`claude` 是 `claude-code` 的别名 | `auto`：`plan`、`install` 选择检测到的 harness，`doctor --live` 选择所有已安装的 |
| `CODEX_HOME` | Codex 的配置目录，影响 harness 检测 | Codex 自身默认值 |
| `CLAUDE_CONFIG_DIR` | Claude Code 的配置目录，影响 harness 检测 | Claude Code 自身默认值 |

`style --help` 列出所有命令；`style help COMMAND` 或 `COMMAND --help` 还会显示高级选项和项目选项。布尔选项不带值（写 `--json`，不写 `--json=true`）。

## 项目结构

| 目录 | 说明 |
|---|---|
| `cmd/style` | CLI 入口 |
| `core` | 包规范解析、编译、规划、模板、token 计算等核心逻辑 |
| `internal/app` | 命令实现与安装、同步等生命周期管理 |
| `internal/adapter` | 各 harness 的适配器（`codex`、`claude`） |
| `internal/source` | 本地与 Git 来源的解析和缓存 |
| `agenthooks` | 内置的 harness hook 编解码与路由库（独立 Go module） |
| `schema` | 索引 JSON Schema |
| `scripts` | 冒烟测试与验收检查脚本 |
| `testdata` | 测试 fixture 与 golden 文件 |
| `third_party` | 第三方许可说明 |

## 开发与测试

运行测试和静态检查（会使用临时 Go 缓存，并清理 `CODEX_HOME`、`CODEX_SANDBOX` 以免影响 harness 检测）：

```bash
make test-go
make vet-go
```

构建后对二进制做冒烟测试：

```bash
go build -o /tmp/style ./cmd/style
python3 scripts/smoke.py /tmp/style
```

CI 配置见 [.github/workflows/ci.yml](.github/workflows/ci.yml)，覆盖 Linux、macOS 和 Windows。

## 常见问题

### `brew install` 提示与 `style` 冲突

homebrew-core 中有一个同名的 `style`，两者都安装 `style` 可执行文件，需要先卸载其中一个。

## 许可证

本项目使用 [MIT](LICENSE) 许可证。内置的第三方依赖许可见 [third_party/README.md](third_party/README.md)。
