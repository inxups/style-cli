# FINAL-06 Claude 隔离补验收

此页保留操作步骤；真实验收结论以关联JSON和原始完成记录为证。日常配置不变；`prepare_claude_acceptance.py` 生成私有 scratch、固定二进制、两个原包与一个可见性探针包。观察器与 STYLE hook 分开，保留每个 hook 的字段名、session/model/effort/source 以及指定环境变量；不保存 prompt、工具正文或凭据。

本次已准备 `/private/tmp/style-m1-claude-ui-c-20261016`，Claude 2.1.300，现已登录。FINAL-06 修复版 SHA256 为 `8d5fbf758cbe7920a9cc978f7715abcf3c00461f9d2289e6f90c2c92501db5d1`；旧程序、prepared.json、旧 launcher 均保留。全局 CLI 在验收期间变为2.1.301，隔离 launcher 已固定到从官方 npm 包核对 integrity 的2.1.300，日常安装不变。更新和边界实测见 [FINAL-06 证据](M1-FINAL-06-BOUNDARY-2026-10-16.json)，前一次更新仍见 [FINAL-12 binary 证据](M1-FINAL-12-CLAUDE-BINARY-2026-10-16.json)。重新准备的命令：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/prepare_claude_acceptance.py \
  --style /absolute/path/to/style --claude /absolute/path/to/claude --scratch "$(mktemp -d)"
```

1. 在终端单独登录隔离配置，然后启动；工作区信任提示由用户接受。不要把凭据粘贴到对话。

   ```sh
   env CLAUDE_CONFIG_DIR=/private/tmp/style-m1-claude-ui-c-20261016/claude claude auth login
   /private/tmp/style-m1-claude-ui-c-20261016/launch.sh
   ```

2. U-C5：记录启动界面是否看到 `M1_FINAL_CLAUDE_USER_SESSION_START`。发送 `STYLE_CLAUDE_BOUNDARY：依次用 Bash 执行 printf STYLE_CLAUDE_BOUNDARY_OK 和 exit 7 # STYLE_CLAUDE_BOUNDARY；不要重试失败命令。`，分别记录 PROMPT_SUBMITTED、TOOL_PRE、TOOL_POST、TOOL_ERROR 的 `M1_FINAL_CLAUDE_USER_...` 是否可见，以及模型是否收到对应 MODEL 标记。失败事件必须有真实失败的 tool result。再发送 `/style-boundary`，记录 SKILL_EXPLICIT 标记。界面缺失与模型上下文投递分开判断。
3. U9/U-C1：用该版本界面的思考开关关闭思考，记录具体操作和界面显示，再触发一次 Bash `printf STYLE_CLAUDE_BOUNDARY_THINK_OFF`。不要只用 `--effort low` 代替关闭思考。`hook-facts.jsonl` 将逐事件记录 `effort` 和 `CLAUDE_EFFORT`；若无关闭选项，记录实际限制。
4. U-C3：发送 `/clear`，再发一句简单提示和一次 Bash；记录清除前后界面会话信息，原生 session_id/model/source 由观察器交叉核对。
5. U-C2：保持会话运行，在另一终端执行新准备环境的 `install-hot.sh`，然后让当前 Claude 用 Read 读取 `probe-fixture.txt`。记录是否出现 HOT_READ_USER/HOT_READ_MODEL；没有时再新开会话对照。不要在首次运行中主动重载 hooks，否则无法区分热更新。本次原 Glob 探针没有实际工具调用，不能计通过；后来新增 Read matcher、当前进程实际 Read 调用和 USER 标记已交叉核对。技能探针放在用户配置 `skills/`，以匹配 `--setting-sources user`。
6. C9：边界核查完成且程序无需进一步修复后，用 `/private/tmp/style-m1-claude-ui-c-20261016/launch-c9.sh` 另开带独立 debug 的会话。先确认 PreToolUse/PostToolUse 的 STYLE 订阅存在，再要求**严格串行、逐次等待**调用 Bash 50 次，每次仅 `printf 'STYLE_M1B_C9\n'`；不合并、不用 shell 循环、不并行。保留全部 debug completion、带时间的 transcript tool id/result id。按原手册整体 p95 `<50ms` 判断，同时报告每类分布和所有慢样本。若中途更换二进制，须重新采集。

操作后回复已完成的步骤和各用户标记是否可见。agent 从 scratch 的 `hook-facts.jsonl`、`claude-debug.log`、transcript 及 STYLE state/stats 收集证据；原始日志先留在私有目录，入库前仅摘录必要字段。C9 可复算命令：

```sh
python3 testdata/manual/measure_claude_hooks.py \
  /private/tmp/style-m1-claude-ui-c-20261016/c9-final06-debug.log \
  --binary /private/tmp/style-m1-claude-ui-c-20261016/bin/style
```

准备程序会拒绝非空 scratch，避免覆盖历史证据。热更新安装和 `/clear` 只影响此探针环境。认证不可用时 FINAL-06 继续保持 `[?]`，不以准备成功代替真实通过。

2026-10-16 边界观察：六类事件的 USER 标记可见；clear 的启动标记虽已输出，用户未看到。clear 创建新 session，原生不含 model，state 保持 unknown；后续工具 hook 给 effort=medium。启动/提示/显式技能未提供 effort，工具前/后/失败提供 effort.level 和 CLAUDE_EFFORT。用户所见 Opus5.5 只有强度档位，无关闭选项，未采到 off 的原生表示。不能将 medium 当作 off。C9 已用本修复版重新采集，见下方追加证据。

本次C9已完成，见 [C9结果](M1-FINAL-06-C9-2026-10-16.json)：50次真实串行Bash，完整103样本整体p95=28ms，保留全部慢样本。原生全量debug日志留在私有目录，提交的HOOKS仅摘录全部匹配STYLE完成行；可用原measure_claude_hooks.py直接复算。

U9控制配置补测见 [THINKING](M1-FINAL-06-THINKING-2026-10-16.json)：真实请求仍adaptive、hook/state仍high，设置关闭不等于实际关闭。该版本/模型缺少可观测off路径，保留不产出none的既定回退；FINAL-06按此能力边界和最终C9关闭。
