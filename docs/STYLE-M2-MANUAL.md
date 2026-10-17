# STYLE M2 真实环境手册

更新时间：2026-10-17。提供 D4/D5 的 Claude 设计探针、Codex B1–B4 和 Claude C2/C3 的隔离验收步骤。T40/T50 离线实现已完成；手册/启动器准备通过，2026-10-17 用户报告 Claude 恢复并授权 agent 交互终端代跑；C1–C4 真实交互结果已通过并归档。

任务和状态以 [任务清单](STYLE-M2-TASKS.md)、[测试矩阵](STYLE-M2-TESTS.md) 为准。Claude Code 界面项已获用户授权由 agent 通过交互终端代跑；以下准备与本地协议结果不代替 M2-C1/C4。

## 1. 已完成的固定版本检查

- STYLE T30 实现提交 `74a473d`；隔离 Go 环境 `-trimpath -buildvcs=false` 构建 SHA256 为 `c35d85bd70c4bd575e5eaf401662870b45867b419995e2747a5917c57952ab8c`。这些设计探针直接检查 Claude，不安装 STYLE 包。
- 本机被固定的 Claude 可执行文件为 `/Users/inxups/.nvm/versions/node/v24.21.0/bin/claude`，`--version` 为 `2.1.301 (Claude Code)`。二进制摘要见 [协议报告](review/M2-D4-D5-PROTOCOL-2026-10-16.json)；没有声称它是公开最新版本。
- [探针脚本](../testdata/manual/probe_claude_m2_design.py) 的 protocol 模式复用 M1 的 `ClaudeProbe`，用真实 CLI 向 loopback 脚本端点发送请求，不做远端推理；usage 为协议 fixture，不用于 M2-CARRY-01。
- 四个新会话固定 `CLAUDE_CODE_SIMPLE_SYSTEM_PROMPT=0`，分别选择 default、既有用户 style、候选替换 style、保留编码指令的对照 style。

| 模式 | system 文本字符数 | user rules 全文 | 选中 style 全文 | 未选 style |
|---|---:|---|---|---|
| default | 26,512 | 一次 | 无 | 均无 |
| existing | 23,229 | 一次 | existing 一次 | 均无 |
| replace | 23,229 | 一次 | replace 一次 | 均无 |
| keep | 26,550 | 一次 | keep 一次 | 均无 |

以上来自实际原生请求的完整连续正文比较，不以模型回答或首尾标记代替。rules/CLAUDE.md/style 正文在该版本的 messages 中，style 正文没有出现在 system 字段；replace 模式仍保留与 default 相同的 20,615 字符系统片段（将 system 文本块以换行连接后比较）。因此当前证据支持继续评估 rules 迁移，output style 更适合作为局部行为样式；不能据此宣布它提供整个 system prompt 的替换位置。

最终短生命周期 headless 运行的 `InstructionsLoaded` 异步日志为空，但四份请求均包含 rules 全文；不将空日志判为规则未加载。此前短 prompt 对照曾记录 `memory_type=User`、`load_reason=session_start`，最终保留报告以可核对的完整请求为证。界面会话需要保持打开直到观察器写出记录。

官方资料复核日期为 2026-10-16，页面 URL/内容摘要见协议报告的 `official_references`：memory 说明用户级 rules，hooks 说明 `InstructionsLoaded` 的异步观察性质，output-styles 说明 `keep-coding-instructions` 只控制完整 prompt 中的编码指令部分。设计已于 2026-10-17 经用户批准，T40/T50 按任务清单依次推进，后续 C1–C4 已按本手册通过真实交互验收，见对应归档报告。

额度阻塞期间已整理 [D4/D5 具体设计](review/M2-D4-D5-DESIGN-2026-10-17.md)：rules 迁移时机/归属/冲突/往返、replace 回退和候选验收均已列明。用户批准先凭已有原生请求证据开发 T40；额度恢复后已补齐 C1–C4，当前结论以任务清单及真实归档为准。

## 2. Claude 界面验收与复现

2026-10-17 用户报告 Claude 已恢复，并明确选择由 agent 通过交互终端代跑。既有设计目录的 auth status 为 loggedIn=true、claude.ai/firstParty；全新交互会话成功回复 OK，rules 的原生 InstructionsLoaded 证据通过 [C1](review/M2-C1-CLAUDE-2026-10-17.json)。本次没有再出现 OAuth/额度错误；C4 已按以下新会话步骤通过，结局为回退方案生效，见 [C4 报告](review/M2-C4-CLAUDE-2026-10-17.json)。新安装验收目录仍未登录，与该既有认证目录分别处理。

已准备 `/private/tmp/style-m2-claude-ui-20261016`，`prepared.json` 明确 `acceptance=false`。启动器以 `env -i` 选择环境，保留真实 `HOME`、`USER`、`LOGNAME` 以访问 macOS 登录钥匙串；Claude 配置使用独立 `CLAUDE_CONFIG_DIR`，profile 配置使用临时 `ANTHROPIC_CONFIG_DIR`/`XDG_CONFIG_HOME`，工作目录、三份样式和用户级 rules 都在 scratch 中。该准备过程没有复制登录凭据或自动启动模型会话。

首次登录报告 `Couldn't save your login`。原启动器把 `HOME` 指向临时空目录，实测该环境中 `security default-keychain -d user` 退出 1（找不到默认钥匙串），保留真实 HOME 后退出 0。该只读修复检查本身不能证明钥匙串是否锁定或 OAuth 已保存成功；本轮另以 auth status 和真实请求确认既有隔离配置认证可用。五个现有脚本已修正，原脚本备份在 `auth-home-fix-before-20261016/`，已有设置与日志摘要保持相同；[修复证据](review/M2-PREP-01-AUTH-2026-10-17.json) 保留当时登录重试待确认的历史事实。以下步骤用于复现；本轮已完成，不要求用户重跑。新空配置需要登录时从第 1 步继续。

1. 需要登录时，在自己的终端运行：

   ```sh
   /private/tmp/style-m2-claude-ui-20261016/login.sh
   ```

   看到 `Login successful` 后再进入第 2 步。若仍有同样错误，打开“钥匙串访问”，确认 `login` 钥匙串已解锁，再运行同一登录脚本。若已解锁仍失败，保留终端错误并回报，复现运行应单列环境阻塞，不改写已完成运行的证据。

2. 启动默认样式的全新界面会话，接受这个临时目录的工作区信任后，发送 `只回复 OK，不调用工具。`：

   ```sh
   /private/tmp/style-m2-claude-ui-20261016/launch-default.sh
   ```

   等待一次响应后退出。D4/C1 的证据是 `instructions.jsonl` 中 `file_path` 对应 `claude/rules/style-m2-user-probe.md`、`memory_type=User`、`load_reason=session_start`。观察器仅保存白名单元数据，不保存文件正文、prompt、工具输入或凭据。没有记录时保留日志并排查信任/事件版本，不直接标通过。

3. 分别启动以下三个新会话，每次在 `/config` 查看当前 output style，发送同样的短消息后退出。依次记录显示的 style 名字及是否发生冲突、报错或额外确认：

   ```sh
   /private/tmp/style-m2-claude-ui-20261016/launch-existing.sh
   /private/tmp/style-m2-claude-ui-20261016/launch-replace.sh
   /private/tmp/style-m2-claude-ui-20261016/launch-keep.sh
   ```

   预设名称分别是 `M2Existing`、`M2Replace`、`M2Keep`。启动参数显式覆盖本次会话的选择；磁盘用户设置原来选择 `M2Existing`。这组检查测量已有样式与候选样式如何切换，不能把“样式已选中”当成“整个系统提示已替换”。不要 resume 前一个会话，避免 system prompt 快照影响比较。

4. 复现完成后记录三次显示的 style 名称、任何异常或工作区信任提示，并核对已知 scratch 的 `instructions.jsonl`、`prepared.json` 与分模式日志。归档 C1/C4 的白名单证据和 D4/D5 结论；本次结果已分别通过，设计无需重新批准。

日志位于私有 scratch；不要把整个目录加入 Git。归档只写必要的白名单摘录、摘要、会话 ID 和用户界面观察。需要重跑时使用新空目录，不覆盖历史证据。

## 3. 可重复准备与测试

从仓库根执行，版本路径换成待验收的固定 CLI：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_claude_m2_design.py \
  --claude /Users/inxups/.nvm/versions/node/v24.21.0/bin/claude \
  --scratch "$(mktemp -d /tmp/style-m2-claude-protocol.XXXXXX)" \
  --output /tmp/style-m2-claude-protocol.json

PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_claude_m2_design.py \
  --prepare-ui --claude /Users/inxups/.nvm/versions/node/v24.21.0/bin/claude \
  --scratch "$(mktemp -d /tmp/style-m2-claude-ui.XXXXXX)"

PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s testdata/manual -p 'test_*.py'
```

`--prepare-ui` 只运行 `claude --version`，不登录或创建模型会话。七项探针测试覆盖非空目录拒绝、凭据/Codex 检测环境不继承、带空格和引号路径的 shell 语法、0700/0600 权限、三样式 fixture、观察白名单、标记命中不足以通过完整正文检查，以及实际执行五个生成脚本时保留系统身份/隔离配置、仅重写启动器不覆盖既有设置与观察记录。

probe 的退出 0 表示脚本执行结束，实际结果检查 JSON 的 `protocol_status`、各 phase 的 exit/request 和正文匹配；界面验收始终单列。当前 M2-C1–C4 已通过真实交互会话；D4 结局为符合预期，D5 结合 C4 界面与完整请求对照以回退方案生效关闭。

## 4. Codex B1–B4 独立验收

本部分依赖的 native/stats/跨包去重实现均已完成，可以在 Claude 额度阻塞时独立执行。由 agent 执行，不关闭任何 Claude 条目。[probe_codex_m2.py](../testdata/manual/probe_codex_m2.py) 复用既有 `NativeProbe` 的隔离 CODEX_HOME、STYLE_HOME、工作目录和只监听 loopback 的转发器，沿用日常已配置的 Responses 服务进行真实推理；连接配置仅保留在私有 scratch，不写进报告。

固定当前 STYLE 源码构建；两条路线各使用新空目录：

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  go build -trimpath -buildvcs=false -o /tmp/style-m2-codex ./cmd/style

PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_codex_m2.py \
  --style /tmp/style-m2-codex --route codex-user \
  --scratch "$(mktemp -d /tmp/style-m2-codex-user.XXXXXX)" \
  --output /tmp/style-m2-codex-user.json

PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_codex_m2.py \
  --style /tmp/style-m2-codex --route codex-plugin \
  --scratch "$(mktemp -d /tmp/style-m2-codex-plugin.XXXXXX)" \
  --output /tmp/style-m2-codex-plugin.json
```

- B1：原生 SessionStart 和匹配 Bash 的 PreToolUse 各运行一次，Read matcher 不运行；白名单观察、OTel span、rollout 和请求中的完整正文分别核对。卸载后再启动真实会话，观察文件不新增、没有 hook span 或正文，配置字节与权限还原。
- B2：同一 SessionStart/PreToolUse 上 native 与 STYLE handler 各一次，来自所选 user/plugin 路线；原生命令不进入 STYLE stats。
- B3：生产 `style stats --by rule/session` 的每行及合计与真实 runtime 记录逐项核对，STYLE 的两次 output 对应两份 rollout 全文，native 的两次注入单列，不计为 STYLE 规则输出。
- B4：两包保留在 index，等价规则只有保留方入口；组合报告标折叠关系，真实请求和 rollout 正文均只有一份，stats 只有保留方一次 output；卸载后配置字节与权限还原。

脚本不修改日常安装；插件使用隔离 marketplace 登记，卸载前由原生命令移除登记。`passed` 和四个 `checks` 都为 true 才可标通过。失败报告保留错误类型和已收集事实，原始日志仍在 scratch；不将 exec 对哈希/插件信任的接受等同于首次 TUI 工作区信任提示。三项新增采集器测试覆盖全文/角色/重复/失败拒绝、stats 行与总量/折叠行核对、原生观察器白名单，全部采集器共 50 项。

2026-10-17 实际结果：固定 codex-cli 0.159.3、T30 STYLE SHA256 `c35d85bd…ab8c`，codex-user/plugin 两路线各运行三次真实模型会话，B1–B4 全部通过；每路线共存场景两份 STYLE 输出与 26 估算 token，native 两份注入单列。完整证据分别为 [B1](review/M2-B1-CODEX-2026-10-17.json)、[B2](review/M2-B2-CODEX-2026-10-17.json)、[B3](review/M2-B3-CODEX-2026-10-17.json)、[B4](review/M2-B4-CODEX-2026-10-17.json)。原始日志和连接配置保留在 `/private/tmp/style-m2-codex-user-20261017`、`/private/tmp/style-m2-codex-plugin-20261017`，不入库。Claude C1–C4 后续已通过真实交互；最终远端 Ubuntu/macOS CI 也已在 `2aded84` 通过，官方计数遗留状态仍单列。

## 5. Claude C2/C3 最终安装验收

2026-10-17 C2 实际采用已认证的 `/private/tmp/style-m2-claude-ui-20261016/claude`，没有读取/复制凭据。将 PREP-03 固定 STYLE/observer 和等价包复制到新证据目录 `/private/tmp/style-m2-claude-live-20261017.5fzqcx0b`，STYLE_HOME/工作目录独立，安装前保存既有六份用户文件基线；原待登录 PREP-03 目录保留。实际命令为该目录 `style.sh install <包> --harness claude-code --yes --json`、`launch.sh --ax-screen-reader --model sonnet --permission-mode manual --tools Bash,Read --session-id <新 ID>`；真实新工作区信任提示已确认。安装/卸载后两个真实会话及逐字节恢复通过 [C2](review/M2-C2-CLAUDE-2026-10-17.json)；每包规则加载、完整正文、去重和原生附件顺序通过 [C3](review/M2-C3-CLAUDE-2026-10-17.json)。下列新配置登录步骤仍是独立复现方案；无需为了本次验收再次登录。

本轮已由用户授权的 agent 通过交互终端完成。以下保留新配置的复现步骤。准备程序只执行 STYLE install 与固定 Claude 的 `--version`，不登录、不启动模型会话。它创建独立配置/工作目录，保留真实 HOME/USER/LOGNAME 以便 macOS 钥匙串工作，并清除调用者的凭据与检测变量；需要登录时由用户运行隔离的 login.sh。不会复制日常凭据。

准备新的空目录（已有证据目录会被拒绝）：

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  go build -trimpath -buildvcs=false -o /tmp/style-m2-final ./cmd/style

PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/prepare_claude_m2_acceptance.py \
  --scratch "$(mktemp -d /private/tmp/style-m2-claude-acceptance.XXXXXX)" \
  --style /tmp/style-m2-final \
  --claude /Users/inxups/.nvm/versions/node/v24.21.0/bin/claude
```

程序输出 scratch 与启动器位置；以下以该路径为 `<scratch>`，不要原样输入尖括号占位符。`prepared.json` 的 `acceptance=false` 表示只准备完毕。固定包含两个等价静态声明，但只应有 `rules/style-m2-claude-first.md`；另有 SessionStart、匹配 Bash 和匹配 Read 的三条 native。用户自有 `CLAUDE.md`、`rules/user.md`、output style 及 InstructionsLoaded 观察器在安装前已保存为 baseline.json。源包、二进制、观察器及导入文件均留在私有 scratch，日志只收白名单加载/事件事实，不收提示词或工具输入。

1. 记录 `prepared.json` 的 STYLE/Claude SHA256 和版本；如需登录，额度恢复后运行 `<scratch>/login.sh`，确认登录保存成功再继续。若仍有钥匙串错误，按 §2 处理，保留环境阻塞状态。
2. 运行 `<scratch>/launch.sh`，在此隔离目录按 Claude 自己的流程确认工作区信任。发送：`用 Bash 只运行一次 printf STYLE_M2_TOOL_OK，禁止 Read 和其他工具，然后只回复 DONE。` 保持界面打开，直到异步观察器写出日志。不要以模型复述当投递证据。
3. **C2**：`native-observations.jsonl` 应只有该真实 session 的 start/SessionStart、shell/PreToolUse 各一条，tool_name 为 Bash；Read fixture 为零。结合 Claude 原生 `/hooks` 或调试日志确认处理器执行；需要核对上下文时保留该会话原生请求中的完整固定正文，不只检查标记。不得通过手工喂 observer 输入声称真实触发。
4. **C3**：`instructions.jsonl` 应有该 session 的 `file_path=<scratch>/claude/rules/style-m2-claude-first.md` 和加载原因，或保留含完整静态正文的原生请求；核对文件完整内容与 prepared.json 的正文摘要、折叠方文件不存在。记录用户规则/其他 memory 的实际加载顺序，不能只从文件名推断优先级。异步日志暂空不代表加载失败，先等待；仍缺证据则记 `[~]`。
5. 退出 Claude，在独立终端运行 `<scratch>/style.sh uninstall --all --yes --json`，然后 `<scratch>/verify-restored.sh`。后者逐字节/权限核对四份外部基线、两份 STYLE rules 不存在、lock/index 删除；校验失败保留全部现场。
6. 再运行 `<scratch>/launch.sh` 开一个全新会话，发送相同 Bash 指令。该新 session 不应新增任何 native fixture 行，也不应再观察到 STYLE rules 加载。外部 InstructionsLoaded 观察器仍在，这是为核对卸载后状态而保留的用户条目。

回传时只提供版本/摘要、真实 session ID、白名单观察行、实际命令与退出码、卸载校验 JSON 和必要的加载/顺序结论。原始 debug/请求与登录数据留在私有 scratch；不要把凭据、完整用户提示或连接配置提交到仓库。C1/C4 的真实结果来自 §2 的独立设计探针，不能由本节离线安装或 A8 回退通过代替。

准备验证：[M2-PREP-03](review/M2-PREP-03-CLAUDE-2026-10-17.json)。三个新增测试实际执行生成启动器/观察器并验证系统身份、凭据排除、非空目录保护和恢复校验拒绝漂移；实际 STYLE CLI 的独立干跑验证两包/一份静态、3 个 native、doctor 健康及四份外部基线还原。干跑没有执行真实模型会话，不关闭 C2/C3。原始未登录准备目录见该报告的 `ready_scratch`；本次复用认证的真实目录、六份用户文件基线和结果见本节开头及 C2/C3 报告。


## 6. 本轮真实结果与证据边界

Claude 2.1.301 的 C1–C4 已通过。C1/D4 确认用户级 rules 发现，C2 核对真实原生 hook/matcher 和卸载后零触发，C3 核对生产每包规则/去重/加载顺序/六文件还原，C4/D5 核对样式切换并以回退方案生效关闭。六个独立交互会话均实际调用 first-party 模型，原始终端/会话在私有目录保留；四份白名单报告见测试矩阵。

附加 `replay_requests` 未通过本次 native API blob 的哈希校验，未绕过校验，也未用作 C2/C3 的请求正文证据。C2/C3 按矩阵使用 `hook_success`、完整 fixture 的 `hook_additional_context`、`InstructionsLoaded`、原生 `instructions` 附件和卸载恢复。新 rules 的 compact 行为未在本轮声明。官方 token 计数需要独立 API key，订阅认证恢复不关闭该遗留；远端 CI 已由 M2-CI-01 在 Ubuntu/macOS 全部通过，M2 总文档收尾仍单列。
