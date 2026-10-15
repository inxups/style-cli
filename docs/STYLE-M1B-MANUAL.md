# M1b 手动配置与真实环境验收

日期：2026-10-15。配置由用户执行。M1a 已完成；本文的准备验证不代表 M1b 通过。验收编号以 [测试矩阵](STYLE-M1-TESTS.md) 为准。

## 1. 构建与配置根

在新的终端执行。二进制放在固定位置，因为生成的 hook 保存它的绝对路径，不能使用 `go run` 的临时程序。下面默认使用现有 Codex/Claude 用户配置根；如使用自定义根，先修改相应变量。

```sh
cd /Users/inxups/project/style-cli
export STYLE_REPO="$PWD"
export STYLE_BIN="$HOME/.local/lib/style-m1b/style"
export STYLE_HOME="$HOME/.style-m1b"
export CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
export CLAUDE_CONFIG_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"

mkdir -p "$(dirname "$STYLE_BIN")"
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  go build -o "$STYLE_BIN" ./cmd/style

codex --version
claude --version
"$STYLE_BIN" adapters --json
"$STYLE_BIN" validate "$STYLE_REPO/example/terse-reviewer"
"$STYLE_BIN" validate "$STYLE_REPO/testdata/manual/m1b-probes"
```

确认 `adapters` 中的 `detected`、版本和 `config_dir` 正确。本机现已确认 Codex CLI `0.159.3`、Claude Code `2.1.300`。检测到命令不代表 hook 安装有效，仍需核对配置中的可执行路径和实际投递。

STYLE_HOME 隔离包和状态，不能隔离两家用户配置中的 `owner=style`。若已有另一套 STYLE 安装，先记录其 STYLE_HOME 并通过原实例卸载，再开始本次验收。

## 2. 保存原始配置

在首次安装前执行一次，不要覆盖已有备份。关闭待测的 Codex/Claude 会话，避免卸载和配置切换期间继续触发 hook。

```sh
export M1B_BACKUP="$STYLE_HOME/manual-backup"
mkdir -m 700 -p "$STYLE_HOME"
if [ -e "$M1B_BACKUP" ]; then
  printf 'Backup already exists; keep it: %s\n' "$M1B_BACKUP"
else
  mkdir -m 700 "$M1B_BACKUP"
  mkdir -m 700 "$M1B_BACKUP/codex" "$M1B_BACKUP/claude"
  for name in config.toml hooks.json; do
    if [ -e "$CODEX_HOME/$name" ]; then
      cp -p "$CODEX_HOME/$name" "$M1B_BACKUP/codex/$name"
    else
      touch "$M1B_BACKUP/codex/$name.absent"
    fi
  done
  for name in settings.json CLAUDE.md; do
    if [ -e "$CLAUDE_CONFIG_DIR/$name" ]; then
      cp -p "$CLAUDE_CONFIG_DIR/$name" "$M1B_BACKUP/claude/$name"
    else
      touch "$M1B_BACKUP/claude/$name.absent"
    fi
  done
fi
```

如果备份目录已存在，停止此节，保留已有备份。`.agenthooks/style/` 是安装器的恢复记录，验收期间保留。

## 3. Codex：先验证用户级路线

```sh
"$STYLE_BIN" plan "$STYLE_REPO/example/terse-reviewer" --harness codex
"$STYLE_BIN" install "$STYLE_REPO/example/terse-reviewer" \
  --harness codex --route codex=codex-user \
  --set max_findings=10 --yes
"$STYLE_BIN" list
"$STYLE_BIN" doctor --json
```

`--yes` 确认 STYLE 的能力/降级报告；先阅读 plan。plan 展示首选路线，实际安装路线以 install 输出和 lock 为准。

当前路线 ID 为 `codex-plugin`、`codex-user`、`claude-user`，以 `style adapters --json` 为准。CLI 不接受 `--preseed-trust`；Codex 两条路线随正常安装生成信任摘要，插件仍需手动注册并记录真实信任结果。`adapters` 信任种类 `user-confirmation`/`preseed` 在锁文件中分别保存为 `user`/`preseeded`，不是已接受 UI 信任的证据。

预期配置文件：

- `$CODEX_HOME/hooks.json`：合并 STYLE 固定事件订阅。
- `$CODEX_HOME/config.toml`：STYLE 静态 `developer_instructions` 区与 `hooks.state` 区。若已有用户自有 `developer_instructions`，以实际 plan 的动态降级为准，记录 B8 的适用情况。
- `$STYLE_HOME/build/index.json`：编译规则；`$STYLE_HOME/state`：会话去重和统计。

在独立验收工作目录运行 `codex`，按正常流程处理信任提示，记录提示内容及次数。用户级路线预置 hook 定义摘要；文件存在、doctor 状态或摘要均不能证明模型已经收到注入。

第一包的动态文字以 `Current session: model …, effort …` 开头。安装第 5 节探针后，可发送：`请报告你从 STYLE 注入上下文看到的 M1B_SESSION、model 和 effort；看不到就明确说看不到。`

## 4. Claude Code：用户级配置

2026-10-15 的旧临时程序路径问题已由用户重新安装解除。C1–C9 已按记录范围真实通过；C9 首轮整体 p95=53ms 失败，a4426a3 优化后独立复测整体 p95=46ms，102 条样本完整保留。PreToolUse 单类 p95 仍为 53ms、resume=798ms；通过口径为本手册 §5.2 既定的整批样本 p95，不宣称各事件单独均达标。保留启动 model/effort unknown 的边界。证据见 `review/M1B-CLAUDE-2026-10-15.json` 的 C9.timing_real_retest。

复现 C3：先按第 1 节重建固定二进制，再执行下面安装命令，保留原安装前备份。重新打开 Claude 会话，确认 `/hooks` 新增 PostModelSwitch（M1B-FIX-06 后本示例为 6 类事件、12 条 hook，其中 PostToolUse 增加 Skill）。用 `/model` 切到另一个当前可用的模型，随后分两条提示引用最新 `Current session:`。第一条应收到新模型正文，第二条应去重；同时核对 state/stats。运行中自动加载订阅变更仍未验证，故采用新会话。

```sh
"$STYLE_BIN" plan "$STYLE_REPO/example/terse-reviewer" --harness claude-code
"$STYLE_BIN" install "$STYLE_REPO/example/terse-reviewer" \
  --harness claude-code --route claude-code=claude-user \
  --set max_findings=10 --yes
"$STYLE_BIN" doctor --json
```

预期合并 `$CLAUDE_CONFIG_DIR/settings.json` 的 hooks，在 `$CLAUDE_CONFIG_DIR/CLAUDE.md` 添加 STYLE 静态区。生成的 hook 使用 `type: command`、固定可执行文件绝对路径和独立 `args`，其中应包含绑定安装状态目录的 `--style-home=…`；锁文件应新增 `claude-code: route: claude-user`。用真实 Claude 确认该版本接受配置。

进入验收工作目录运行 `claude`，审阅工作区信任，使用 `/hooks` 检查已加载的 STYLE hook。新开会话后验证实际投递。默认策略为 fail-open。需要单独验收 fail-closed 时，可在上述 install 增加 `--fail-closed claude-code`；本项目要求检测到版本至少 `2.1.295`，并生成 PreToolUse 的 `onFailure: block`。记录真实版本与行为。

## 5. 第二包与可识别探针

先完成第一包的新会话观察，再按实际已安装的 harness 执行下面命令；只测一家时将 `--harness` 改为该家名称。

```sh
"$STYLE_BIN" plan "$STYLE_REPO/testdata/manual/m1b-probes" \
  --harness codex,claude-code
"$STYLE_BIN" install "$STYLE_REPO/testdata/manual/m1b-probes" \
  --harness codex,claude-code --yes
"$STYLE_BIN" sync
"$STYLE_BIN" doctor --json
```

第二包使用已有路线，不会替你注册原生 skill 或 GitHub MCP。新开会话后检查这些唯一标记：

| 操作 | 预期观察 | 对应条目 |
| --- | --- | --- |
| 新会话、首条消息 | `M1B_SESSION model=… effort=…` 与实际选择一致 | B2、C1 |
| 同会话 `/compact` 后再发两条消息 | `m1b-probes/session-info` 每次 compact 只补发一次 | B3、C2 |
| Claude `/model` 切换后发消息 | 新模型的 `M1B_SESSION`，不重复补发同一内容 | C3 |
| 让模型运行 `printf 'STYLE_M1B_WARN\n'` | 用户提示 `M1B_WARN_USER`，模型上下文 `M1B_WARN_MODEL` | C5 |
| 发送含 `STYLE_M1B_BLOCK_PROMPT` 的消息 | `M1B_PROMPT_BLOCKED`；Claude 的阻断结果不回带原 prompt | C6 |
| 新会话中要求并行运行两个含 `STYLE_M1B_PARALLEL` 的 printf 命令 | 同 session 的 `M1B_PARALLEL_ONCE` 只输出一次 | C7 |
| 关闭再 resume 原会话 | 注入次数和历史回放分别记录 | B12、C9 |

并行验收必须确认 harness 实际并行派发了工具调用，串行执行不能算 C7 通过。第二包安装、sync、卸载时记录 Codex 是否重复要求信任。Claude 的动态订阅随规则变化，新增订阅后按实际 UI 重新加载或新开会话。

C7 已真实通过：第一次把命令分两条消息发送，区间不重叠，仅作为串行观察保留；用户 `/clear` 后，在一条消息中请求两个独立 Bash 调用，实际运行区间重叠，同 session/gen 的规则仅一次 output、一次 dedup。复现时将并行请求整段作为一条消息发送，先确认当前 session/gen 没有触发过该规则；不要把两条命令分别发给模型。证据见 Claude JSON 的 C7。

C5 已真实通过：用户确认 `PreToolUse:Bash says: M1B_WARN_USER. This is the manual hook warning probe.` 在工具 UI 内容中可见，模型引用 `M1B_WARN_MODEL`，实际日志和正文摘要一致。提示不要求弹窗或特定警告样式；本次模型在工具结果后复述，不能仅凭复述时机推断 hook 执行时间。本次只确认 PreToolUse，其他事件的用户提示仍需各自验证。

以下检查使用 `terse-reviewer`，记录工具原始名称、输入及 hook 结果：

- **B4/C4**：先确保两家原生环境已识别 `code-review` skill，再分别测试显式 `$code-review` / `/code-review` 与隐式读取；Codex 另测 cat/sed/head 读取真实 SKILL.md。比较同一会话的去重记录。STYLE 的 `target: skill` 只附加规则，不安装 skill。
- **B5/C6**：在独立 scratch Git 仓库与本地 bare remote 中，要求模型执行 `git push --force` 和 `git push --force-with-lease`。前者应被 STYLE 拒绝，后者应实际进入工具执行；不要用一次 Git 自身失败代替 hook 阻断证据。
- **B6**：要求 Codex 通过 apply_patch 添加 TODO，再删除 TODO；只新增时出现 no-todo-comments 警告。
- **B7**：准备可用的 GitHub MCP 和测试仓库，在实际 `create_pull_request` 前观察注入；没有真实调用则保持待验证。
- **B9**：修改 `$STYLE_HOME/packages/terse-reviewer/prompts/session-info.md` 的文字，旧 index 应继续生效；`"$STYLE_BIN" sync` 应退出 3；`"$STYLE_BIN" trust terse-reviewer` 只展示 diff，`"$STYLE_BIN" trust terse-reviewer --yes` 才应用。之后用新会话观察新文字。此流程使用普通安装，不加 `--link`。
- **B11/C9**：保留 harness 端每次 hook 的耗时样本、样本数与 p95 算法；分别核验 Codex PreToolUse p95 <150ms、Claude exec hook p95 <50ms。`doctor` 检查登录 shell；A20 的进程性能结果不能代替此项。

模型回答只是观察线索。结合 harness 的 hook/会话记录、模型可见上下文和 `$STYLE_HOME/state` 内 `stats.jsonl` 的规则输出增量判断；统计只证明 STYLE 发出结果，不能单独证明模型收到了结果。回传所需片段即可。

C6 已真实通过：Claude 的两次 prompt 阻断仅记录固定理由，`promptKept=false`；force 命令被 hook 拒绝，lease 命令进入 Git 并成功，临时本地 remote 与工作仓库 HEAD 不变。允许 lease 是当前验收契约，模型建议禁止它不构成规则变更。若回传所在的 Codex/Claude 会话也安装了探针，完整 prompt 触发串会再次拦住回传；回传时省略该串，并由原 Claude transcript 核对精确结果，不能把省略后的文字当成原始输出。

### 5.1 Claude C4：最小原生 skill

若没有可用的 code-review，使用仓库中的 `testdata/manual/claude-skills/code-review/SKILL.md`。它只回传标记和 hook 附加指令，不执行实际 review，也不包含 STYLE 要注入的 findings 数值。官方 [skills 文档](https://code.claude.com/docs/en/skills) 支持项目级 `.claude/skills/<name>/SKILL.md`、模型调用和 `/name` 显式调用；项目同名 skill 会替代 bundled 命令。原生是否加载仍以当前真实版本为准。

下面命令由用户执行，在独立验收工作目录放置 fixture；保留现有用户级配置。`cp -n` 避免覆盖已有文件，`cmp` 不通过时先核对现存内容。若个人或企业级另有同名 skill，其优先级可能更高，必须确认实际加载的是此 fixture。

首轮失败记录：fixture 已正确复制，但真实会话 cwd 为 `/Users/inxups`，加载了进行实际 review 的 fork skill；同时旧安装没有隐式 Skill matcher。复测前按 §1/§4 重建安装；下面子 shell 会打印并校验目录，再启动新的 Claude，避免在已有 home 会话继续测试。

```sh
export M1B_C4_WORKSPACE="$HOME/.style-m1b/manual-workspace/claude-c4"
mkdir -p "$M1B_C4_WORKSPACE/.claude/skills/code-review"
cp -n /Users/inxups/project/style-cli/testdata/manual/claude-skills/code-review/SKILL.md \
  "$M1B_C4_WORKSPACE/.claude/skills/code-review/SKILL.md"
cmp /Users/inxups/project/style-cli/testdata/manual/claude-skills/code-review/SKILL.md \
  "$M1B_C4_WORKSPACE/.claude/skills/code-review/SKILL.md" && (
  cd "$M1B_C4_WORKSPACE" || exit 1
  printf 'C4_WORKSPACE=%s\n' "$PWD"
  test "$PWD" = "$M1B_C4_WORKSPACE" || exit 1
  CLAUDE_CONFIG_DIR="$HOME/.claude" command claude
)
```

接受此独立工作目录的正常信任提示，先确认打印目录正确，且 `/skills` 中 code-review 描述含 STYLE M1b hook acceptance。在第一个新会话依次发送：

1. 普通提示：`请用 Skill 工具调用一次 code-review，仅完成这个验收 skill 的要求。` 确认界面实际有 Skill 调用，回传 M1B_SKILL_LOADED 及原样引用的 hook 上下文。如果仍运行实际审查或 fork/background，先停止后续步骤并回传，不将该次当成 fixture 验收。
2. 单独发送 `/code-review`，回传结果。实际 UserPromptExpansion 必须发生；同 session 的 review-skill 应去重，模型仍能引用旧上下文不表示重复注入。

另开新会话，按显式 `/code-review` → 普通提示要求 Skill 调用的顺序复测。两个入口各作为首次调用时应输出 `review-skill` 一次，另一个入口复用同 session 去重。若 harness 避免重复调用 Skill，则记录实际行为，不用模型复述代替缺失的调用。结合 state/stats 检查 `terse-reviewer/review-skill` 的 output/dedup；本轮两种顺序已真实通过，证据见 Claude JSON 的 C4.real_retest。第二次回答可以复述旧上下文，也可以说明本次没有新增指令，是否重复投递以实际日志核对。

### 5.2 Claude C9：resume 与真实耗时

先保存待恢复 session 的 `session.json`、`scope.json`、`stats.jsonl` 行数和 `seen` 文件名。用户在同一工作目录用 `claude --resume <session-id> --debug hooks --debug-file <临时目录>/hooks.log` 恢复，不用新会话代替。发送一条无工具提示，引用最新 `Current session:`。核对实际 `SessionStart:resume` 中性输出、gen/epoch 和 seen 不变、prompt_count 随提示递增、后续仅 dedup；模型复述历史文字不能单独证明没有重新注入。

继续在带 debug 的会话中，以一条消息要求依次调用 Bash 50 次，每次只执行 `printf 'STYLE_M1B_C9\n'`，等返回后再调用下一次，不并行、不合并、不使用 shell 循环。用 timestamped transcript 的实际 tool id/result id、assistant message 和时间检查次数、错误、重叠；无 timestamp 的历史快照行不重复计数。最低采样量 100 是本收集流程的检查门槛，不表示设计 U5 的 1000 次 Codex 基准已完成。

从仓库根复算真实 debug 文件：

```sh
python3 testdata/manual/measure_claude_hooks.py /path/to/hooks.log \
  --binary "$HOME/.local/lib/style-m1b/style"
```

收集器只读，退出 0 表示 JSON 生成成功；`status=within-budget` 才表示该样本达到默认门槛。完整保留匹配的 STYLE command-hook 完成计时，不剔除预热/慢样本，不混入 Claude 内部 `hooks module` 计时；整体和各事件均按 `sorted[ceil(0.95*n)-1]` 计算。这里是 harness 报告的 command-hook 耗时，未独立测到 Go 进程起止，不能混用 A20 或合成输入进程测量。

2026-10-15 首轮结果：50 次 Bash 全成功、0 hook 错误，实际 49 个 assistant message，其中一个含两次重叠调用，故不宣称全串行。103 条完整样本整体 p95=53ms，PreToolUse=53ms、PostToolUse=29ms；C9 性能未通过。排除重叠区间的单独敏感性分析仍超预算，不能据此替换完整结果。原样 completion 片段与全部调用元数据见 `review/M1B-CLAUDE-C9-HOOKS-2026-10-15.txt`、`review/M1B-CLAUDE-C9-TIMING-2026-10-15.json`；对前者运行相同脚本可复算。不放宽阈值，先定位计时边界和 PreToolUse 开销；本次没有修改真实配置或 Go 实现。

随后 M1B-PERF-01A 已去掉 index 加载的重复编码/校验，保留完整 schema/语义校验及状态持久化。离线进程诊断改善约 1.5ms，不能证明真实 PreToolUse 的额外开销已消除。用户需按第 1 节重建固定二进制；订阅和规则没变，无需重新安装。恢复原会话时给 `--debug-file` 一个新的文件路径，再重复本节 50 次调用并检查实际重叠情况；新日志单独统计、保留全部匹配样本，目标仍为 p95 <50ms。原 103 条失败证据保留，不能混入新采样后计算一个通过结果。

用户已完成上述复测：新日志 102 条 completion（50 PreToolUse、50 PostToolUse、1 resume、1 prompt），50 个 Bash 调用/结果全部成功且无重叠。整体 median=27ms/p95=46ms/max=798ms，按现有收集器和整批样本门槛通过 C9；PreToolUse 单类 p95=53ms、PostToolUse=27ms，均保留原值，不把整体通过扩展为单类保证。原样日志片段 `review/M1B-CLAUDE-C9-RETEST-HOOKS-2026-10-15.txt` 可用同一命令复算；完整证据见对应 RETEST-TIMING JSON。此前失败仍为历史记录。

## 6. Codex 插件路线与手动注册

在用户级路线的投递、卸载往返检查完成后进行 B10。先关闭 Codex 会话，再让 STYLE 切换配置：

```sh
"$STYLE_BIN" install "$STYLE_REPO/example/terse-reviewer" \
  --harness codex --route codex=codex-plugin \
  --set max_findings=10 --yes
```

插件位于 `$STYLE_HOME/adapters/codex/plugin`。STYLE 当前生成插件目录和信任区，Codex 的插件注册由下面步骤完成。若已有名为 `style` 的 marketplace，先核对来源，不能覆盖另一个 marketplace。

```sh
mkdir -p "$STYLE_HOME/.agents/plugins"
cat > "$STYLE_HOME/.agents/plugins/marketplace.json" <<'JSON'
{
  "name": "style",
  "interface": {"displayName": "STYLE M1b"},
  "plugins": [{
    "name": "style-runtime",
    "source": {"source": "local", "path": "./adapters/codex/plugin"},
    "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
    "category": "Productivity"
  }]
}
JSON

codex plugin marketplace add "$STYLE_HOME" --json
codex plugin add style-runtime@style --json
```

以上语法已用本机 `0.159.3` 在临时 CODEX_HOME 实际注册和移除通过。其它版本先看 `codex plugin --help`；没有上述子命令时保留原始输出，本条暂不通过。不要使用未经本机支持确认的 `--plugin-dir`。

新开 Codex，审阅并记录实际插件/hook 信任行为，重复核心注入与 block 检查。注册命令返回的 `installedPath` 是 Codex 缓存副本；检查其 `hooks/hooks.json` 与 `bin/style`。插件通过 `PLUGIN_ROOT` 找到 launcher，launcher 再调用固定的 STYLE_BIN，运行时读取同一个编译 index。

**切回用户级路线前先取消插件注册**，因为删除生成目录不会删除 Codex 已缓存的副本：

```sh
codex plugin remove style-runtime@style --json
codex plugin marketplace remove style
"$STYLE_BIN" install "$STYLE_REPO/example/terse-reviewer" \
  --harness codex --route codex=codex-user \
  --set max_findings=10 --yes
"$STYLE_BIN" doctor --json
```

新会话确认只执行用户级 hook。插件注册/取消会由 Codex 自身修改配置，字节往返检查应区分 STYLE 的变更与 Codex marketplace/plugin 管理变更，记录差异。

## 7. 卸载与原文比较

在第一次用户级验收结束、开始插件注册之前执行一次；最后结束时再执行一次。先关闭两家会话。若当前是已注册的插件路线，先执行上一节的插件移除命令。

```sh
env STYLE_HOME="${STYLE_HOME:-$HOME/.style-m1b}" CLAUDE_CONFIG_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}" "${STYLE_BIN:-$HOME/.local/lib/style-m1b/style}" uninstall m1b-probes --yes
env STYLE_HOME="${STYLE_HOME:-$HOME/.style-m1b}" CLAUDE_CONFIG_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}" "${STYLE_BIN:-$HOME/.local/lib/style-m1b/style}" uninstall terse-reviewer --yes
env STYLE_HOME="${STYLE_HOME:-$HOME/.style-m1b}" "${STYLE_BIN:-$HOME/.local/lib/style-m1b/style}" list

for name in config.toml hooks.json; do
  if [ -f "$M1B_BACKUP/codex/$name.absent" ]; then
    test ! -e "$CODEX_HOME/$name" || printf 'unexpected file: %s\n' "$CODEX_HOME/$name"
  else
    cmp "$M1B_BACKUP/codex/$name" "$CODEX_HOME/$name"
  fi
done
for name in settings.json CLAUDE.md; do
  if [ -f "$M1B_BACKUP/claude/$name.absent" ]; then
    test ! -e "$CLAUDE_CONFIG_DIR/$name" || printf 'unexpected file: %s\n' "$CLAUDE_CONFIG_DIR/$name"
  else
    cmp "$M1B_BACKUP/claude/$name" "$CLAUDE_CONFIG_DIR/$name"
  fi
done
```

两包都卸载后才检查最后一包清理。原文不同则记录差异，检查期间是否有用户或 harness 外部修改；不要直接用备份覆盖，避免丢失新变更。继续插件路线前，重新执行第 3、4、5 节需要的安装命令。

卸载命令每条整行执行，保留开头的 `env` 和完整命令。单独输入未 export 的 `STYLE_HOME=…`，不能保证后续进程收到该值；未传入时 CLI 默认读取 `~/.style`，不等于本次验收的 `~/.style-m1b`。真实 C8 首轮在命令拆行后报无匹配包，包和 Claude hook 仍存在；单行 env 复测已成功卸载，settings 字节比较通过，CLAUDE.md 不存在。有效恢复基线来自最近一次安装的 owner 恢复记录，保存了后来新增的模型/界面偏好；初始 absent 备份不能覆盖这些后增设置。详细证据见 Claude JSON 的 C8。

## 8. 返回结果

每项记录：编号、CLI 版本、路线、工作区/会话标识、操作、预期、实际、证据片段和耗时数据。先返回两家 `--version`、install 输出、`doctor --json` 及新会话首条注入观察，就可以按结果继续排查。B1–B12、C1–C9 在真实证据返回前保持 `[?]`。

准备验证（2026-10-15）：隔离 HOME/CODEX_HOME/CLAUDE_CONFIG_DIR/STYLE_HOME，真实构建 STYLE CLI；两包 validate、两家 plan/install、sync、卸载后外部配置原文字节往返，以及 Codex 本地 marketplace 注册/安装/移除、路线切换均通过。探针经真实 STYLE 进程确认两家 session/warn/prompt block 与串行重复调用去重；并非真实并行工具验收。Claude 仅使用 `2.1.315` 版本探针做检测，未运行真实 Claude；未启动模型会话、未修改用户真实配置。

官方参考（核对日期 2026-10-15）：[Codex hooks](https://developers.openai.com/codex/hooks)、[插件本地 marketplace](https://developers.openai.com/plugins/build/plugins)、[Claude hooks](https://code.claude.com/docs/en/hooks)。CLI 子命令以用户已安装版本的 help 为准。
