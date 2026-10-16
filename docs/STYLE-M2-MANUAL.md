# STYLE M2 真实环境手册

更新时间：2026-10-17。当前先提供 D4/D5 的 Claude 设计探针；M2-PREP 仍为部分完成。Codex B1–B4、Claude C2/C3 的最终安装验收步骤在 D4/D5 定稿及 T40/T50 闭合后补齐。

任务和状态以 [任务清单](STYLE-M2-TASKS.md)、[测试矩阵](STYLE-M2-TESTS.md) 为准。Claude Code 界面项由用户执行；以下准备与本地协议结果不代替 M2-C1/C4。

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

官方资料复核日期为 2026-10-16，页面 URL/内容摘要见协议报告的 `official_references`：memory 说明用户级 rules，hooks 说明 `InstructionsLoaded` 的异步观察性质，output-styles 说明 `keep-coding-instructions` 只控制完整 prompt 中的编码指令部分。设计结论仍需按任务清单评审，T40/T50 尚未开始。

## 2. 用户执行的 Claude 界面步骤

已准备 `/private/tmp/style-m2-claude-ui-20261016`，`prepared.json` 明确 `acceptance=false`。启动器以 `env -i` 选择环境，保留真实 `HOME`、`USER`、`LOGNAME` 以访问 macOS 登录钥匙串；Claude 配置使用独立 `CLAUDE_CONFIG_DIR`，profile 配置使用临时 `ANTHROPIC_CONFIG_DIR`/`XDG_CONFIG_HOME`，工作目录、三份样式和用户级 rules 都在 scratch 中。没有复制登录凭据或自动启动模型会话。

首次登录报告 `Couldn't save your login`。原启动器把 `HOME` 指向临时空目录，实测该环境中 `security default-keychain -d user` 退出 1（找不到默认钥匙串），保留真实 HOME 后退出 0。这不能证明钥匙串是否锁定或 OAuth 已保存成功。五个现有脚本已修正，原脚本备份在 `auth-home-fix-before-20261016/`，已有设置与日志摘要保持相同；[修复证据](review/M2-PREP-01-AUTH-2026-10-17.json) 单列登录重试待确认。请重新执行下面第 1 步。

1. 需要登录时，在自己的终端运行：

   ```sh
   /private/tmp/style-m2-claude-ui-20261016/login.sh
   ```

   看到 `Login successful` 后再进入第 2 步。若仍有同样错误，打开“钥匙串访问”，确认 `login` 钥匙串已解锁，再运行同一登录脚本。若已解锁仍失败，保留终端错误并回报，此时 C1/C4 继续记环境阻塞。

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

4. 完成后在当前对话回复“已跑完”，并附上三次显示的 style 名称、任何异常或工作区信任提示。agent 会读取这个已知 scratch 的 `instructions.jsonl`、`prepared.json` 与分模式 debug 日志，整理 C1/C4 证据和 D4/D5 设计结论，再提交用户评审。

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

probe 的退出 0 表示脚本执行结束，实际结果检查 JSON 的 `protocol_status`、各 phase 的 exit/request 和正文匹配；界面验收始终单列。当前 M2-D4/M2-D5 保持 `[~]`，M2-C1/M2-C4 因登录失败记 `[~]`，等待修正后的真实登录与界面重试。
