# M3-D1：explain / probe 诊断契约评审稿

日期：2026-10-17。状态：`[x]`，设计已获用户确认；M3-T10 explain 已实现并通过 [M3-A1](M3-A1-2026-10-17.json)，M3-T11 probe 也已实现并通过 [M3-A2](M3-A2-2026-10-17.json)。用户在 `87c878fa380d01a5f6b9e4355896ed2c86dc74c5` 评审稿的明确确认问题后回复“继续”，批准本稿的三项产品选择；实现和功能验收另行记录。调查基线为 clean `bf278633cd58661b1d60b7128bdff06b79f0d92b`；M3-T00/M3-A0 已完成。原评审提交只完成设计调查和文档检查；本次依据用户确认关闭 M3-D1，T10/T11 实现及 A1/A2/B1/C1 验收分别记录。

任务与状态见 [M3 任务](../STYLE-M3-TASKS.md)，测试见 [M3 矩阵](../STYLE-M3-TESTS.md)。CLI 摘要回写 [§4.1](../spec/STYLE-cli-design.md#41-m3-d1-诊断契约评审稿)，隔离语义见 [运行时 §8.9](../spec/STYLE-spec-runtime.md#89-m3-d1-诊断隔离评审稿)，已实现的冷路径说明文件见 [编译产物 §9.7](../spec/STYLE-spec-compiled.md#97-m3-d1-编译说明文件评审稿)。

## 1. 当前实现与设计依据

下列事实固定在上述 revision，不能把设计中的接口当成现有命令。

| 当前源码 | 调查事实 | D1 处理 |
|---|---|---|
| [CLI 分派](../../internal/app/entry.go) | 只有现有管理命令和独立 hook 分派，没有 explain/probe | 新增管理入口，隐藏 hook 入口保留 |
| [hook 入口](../../internal/app/hook_command.go) | 直接绑定已安装的 index、state、codec cache；不读取 lock/source | 离线重放显式传入隔离依赖，不调用该入口后再恢复真实文件 |
| [runtime](../../internal/runtime/runtime.go) | 解码后先更新 session/scope，再 enrich、engine、输出校验和 stats；index 错误保存到处理阶段 | 复用这条链；模拟写入私有临时根，管理失败与 hook fail-open 分开 |
| [engine](../../core/engine/process.go)、[when](../../core/when/match.go) | 过滤原因目前只有布尔结果；records 只有 output/blocked/dedup/cap-drop | 从同一求值过程产生可选 trace，不另写一个匹配器 |
| [state](../../core/state/state.go)、[占位](../../core/state/dedup.go) | startup 清 seen，clear/compact/prompt 改计数；Claim 真正创建文件；Read 不刷新 mtime | 显式复制输入，重放只操作副本；展示事件前后计数和真实抢占结果 |
| [index 类型](../../core/compile/types.go)、[编译去重](../../core/compile/dedup.go) | static 只有摘要；native 不进 index；Deduplicated 仅在内存 Result，折叠项已删除 | 新增与 index 摘要绑定的冷路径说明文件，旧安装缺文件仍可解释现有条目 |
| [Runner](../../agenthooks/agenthooks.go)、[codec](../../agenthooks/codec.go) | Run 保持原生 no-op/退出码；解码错误也可能退出 0；OnAny 只观察已解码事件 | 增加可选结构化运行结果观察，不能用 stderr 文本或退出 0 推断模拟成功 |
| [MCP 补充](../../agenthooks/mcpresolve.go)、[skill 补充](../../agenthooks/claude_skill.go) | 默认可能读外部配置/缓存；Claude Skill 的 PostToolUse 在解码时读取本地 manifest | 离线模式覆盖解码阶段的文件读取，不能只加 WithoutMCPResolution 就宣称无外部数据 |
| [Codex enrich](../../internal/adapter/codex/runtime.go)、[Claude enrich](../../internal/adapter/claude/runtime.go) | Codex 可读祖先进程/配置；Claude 可读 CLAUDE_EFFORT；两者都复用 session | 禁用环境回落，用明确输入的补充事实替代；不改变既有来源优先级 |
| [最后卸载](../../internal/app/uninstall.go) | 最后一包卸载移除 index 和 lock；保留包时重新编译 | 编译说明与 index 同事务更新/移除，不由只读命令补写 |

调查复核（只读，不启动 harness）：

```sh
git grep -n -E 'case "stats"|case "explain"|case "probe"' bf278633cd58661b1d60b7128bdff06b79f0d92b -- internal/app/entry.go
git grep -n -E 'State.Update|Engine.Process|r.record|WithoutBackfill|loadErr' bf278633cd58661b1d60b7128bdff06b79f0d92b -- internal/runtime/runtime.go
git grep -n -E 'backfillClaudeSkillOutput|readClaudeSkillContent|resolveMCPContext|OnAny' bf278633cd58661b1d60b7128bdff06b79f0d92b -- agenthooks
git grep -n -E 'Deduplicated|Format = 1|index.json' bf278633cd58661b1d60b7128bdff06b79f0d92b -- core/compile internal/app/uninstall.go
```

## 2. explain 输入契约

```text
style explain --harness h --event NativeEvent --payload f.json
              [--index index.json] [--state-dir state-root] [--facts facts.json]
              [--provenance explain.json] [--fail-closed] [--json]
```

| 输入 | 约定 |
|---|---|
| harness | 必填、恰好一家；按注册表解析 canonical ID，沿用 CLI 的 `claude` → `claude-code` 别名；不接受 auto、列表或重复值。注册但没有 runtime 接线的 harness 返回 `runtime-unavailable`，不触发 Detect |
| event | 必填、原生事件名，区分大小写，例如 `PreToolUse`；不是 STYLE 评估点 `tool.pre`。必须与 payload 的 `hook_event_name` 完全一致，不补写或覆盖原 payload |
| payload | 必填、UTF-8 JSON 对象且只有一个 JSON 值；`hook_event_name` 和 `session_id` 必须为非空字符串。保留原始字节作摘要，其他字段交给真实 codec；未知字段的兼容方式保持 codec 现状 |
| index | 默认 `$STYLE_HOME/build/index.json`，STYLE_HOME 未设时沿用 `~/.style`；显式路径相对调用目录。按 engine/schema 的 16 MiB 上限和 format=1 校验，只读取一次捕获的字节 |
| state-dir | 可选，指向 state 根而非单个 session。省略时从空状态开始，不自动读取真实 state。显式输入只复制 payload 所指 session 的 session.json、当前 agent 的 scope.json/seen；其他 session/agent/stats 不参与 |
| facts | 可选，严格的 format=1 补充事实文件，结构见下文；省略即无外部补充数据，不读取调用者环境、harness 配置、进程、transcript、skill 文件、MCP registry 或网络 |
| provenance | 默认查 index 同目录的 explain.json；可显式指定导出的说明文件。只辅助解释编译阶段，不决定规则是否执行 |
| fail-closed | 默认 false；只模拟所选 harness 已支持的安装级开关，当前只接受 Claude。不会自动继承另一个 STYLE_HOME 的 lock；报告明确显示采用的策略，不提前实现 D8 |
| json | stdout 输出单个 format=1 报告；管理层不直接把 hook 响应写成自己的顶层 stdout |

payload 上限为 32 MiB，读取到上限加一即拒绝，不能依赖 Runner 当前的截断读取来接受超限输入。index/payload/facts/provenance 必须为普通文件；显式路径的父目录先规范化为只读锚点，拒绝叶项或锚点内后代的 symlink，不读 FIFO、socket、设备。macOS /tmp 等既有父目录别名规范化不改变输入内容的授权范围；不会因为 payload.cwd 或 transcript_path 改变读盘范围。可选 provenance 的文件错误按 §4 降为 index-only，其余必需/显式输入错误退出 1。

能够被 codec 识别、但没有 STYLE 求值处理器的原生事件可以得到成功的空输出，报告 `points=[]` 和 `no-style-handler`；Claude 的 PostModelSwitch 按现有专用处理器只更新隔离 session。未知原生事件返回 `event-unsupported`，不能用 KindOther 的默认 no-op 假装已支持；事件识别表由 agenthooks 提供，STYLE 只补自己实际注册的 OnOther 入口。已注册 harness 在合法 index 中没有条目时仍是成功的空候选结果。

补充事实结构（全部可选数据必须属于所选 harness；未知字段、格式号或摘要冲突退出 1）：

| 字段 | 内容与来源 |
|---|---|
| format、harness、payload_sha256 | 必填；format=1、canonical harness、带 `sha256:` 前缀的原 payload 精确摘要 |
| harness_version | 可选，模拟已安装命令传入的版本；仍由 adapter 按现有事件/状态/安装事实顺序处理 |
| codex_config_effort | 仅 Codex，已捕获的有效 `model_reasoning_effort` 原值；作为配置解析结果注入，不再读取任何配置层。session 已 EffortResolved 时继续沿用现有缓存语义 |
| claude_env_effort | 仅 Claude，捕获的 CLAUDE_EFFORT 原值；优先级仍是事件 effort.level → 此值 → session，未知事件原值继续清除旧 tier |
| mcp | 可选 `{server, tool}`，补充原解析流程缺失的身份；只能填补缺失部分，不覆盖 payload/工具名已明确的身份，冲突为 `facts-conflict`。不接受 endpoint、命令或凭据 |
| claude_skill_content | 仅成功的 Claude Skill PostToolUse，可选字符串，不超过现有 65,536 字节限制；替代本地 manifest 补充，不改 Raw payload。其他事件上提供即报错 |

facts 总上限 1 MiB。空字符串与未提供分别保留，合法未知原值交给现有 effort 映射。报告把补充事实标为 `provided`，它们是重放输入，不作为原生采集已发生的证明。没有必要补充的数据保持 unavailable，条件仍按当前缺值语义求值；报告逐字段说明缺失与跳过的外部来源。

facts 示例（全零摘要是格式占位，实际必须换成 payload 的精确摘要）：

```json
{
  "format": 1,
  "harness": "claude-code",
  "payload_sha256": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
  "harness_version": "2.1.301",
  "claude_env_effort": "high"
}
```

## 3. 隔离重放与 trace

1. 先校验命令与输入，捕获 index/payload/facts/provenance；用私有临时目录创建 state 和 codec cache（目录 0700、文件 0600）。解码、事件识别和原生输出编码均由 agenthooks 完成。
2. 显式 state-dir 使用 core/state 共用的 ID 路径规则，拒绝 symlink/非普通文件和路径逃逸。session/scope JSON 各不超过 64 KiB，seen 只接受当前占位命名的空普通文件，最多 10,000 个；本次复制总字节上限 16 MiB，超限报错，不丢占位后继续。缺少所选 scope 可按不存在处理，报告 `seed_scope_exists=false`。
3. 复制前后检查源文件身份/大小/mtime 和目录清单；可检测的并发变化报 `state-snapshot-changed`。这不是跨文件原子快照，也不承诺输入等于历史事件发生前的状态。解释过去的事件应使用事先保存的事件前快照；对已执行事件的当前 state 重放可能显示 dedup。
4. 复用 runtime 的 StateUpdate → state.Update → Enrich → engine → ValidateFor → codec 路径。startup/clear/compact/prompt、子 agent、原生 turn ID 与 prompt_count 回落均在副本真实执行；重复占位和 cap-drop 释放也必须实际发生在副本。
5. agenthooks 的离线选项覆盖 decode 阶段的 Skill 补充、MCP config/list/cache/后台 warm、transcript/backfill、进程与环境回落；adapter 的外部依赖显式注入。用调用计数/拒绝式依赖验证零外部读取和零子进程，不能靠临时修改全局环境隔离并发请求。
6. engine 与 when 增加可选诊断结果，但普通 Process/Match 与诊断入口共用求值实现。trace 不执行第二次 Claim，不额外渲染一次，不为了诊断计算原本短路的条件；harness 字段按稳定顺序呈现。
7. runtime 记录过程只写临时根或内存，真实 log/stats/probe 观察器关闭。成功、错误、取消后都清理临时根；清理失败返回诊断和退出 1，路径只指向本次私有目录。

每个点记录候选数；每个实际检查的 entry 记录 rule/entry、point、order、action、needs、来源路径及以下阶段。未运行的阶段明确 `not-evaluated`，不能写成通过：

| 阶段 | trace 原因/结果 |
|---|---|
| selector / sources | `target-mismatch`、`source-mismatch`，或通过；实际比较仍用 engine 的 selector/source 逻辑 |
| when | 字段/conditions 下标、`missing-data` 或 `predicate-false`；AND/OR、否定条件的缺值语义不变。只报告可用性和来源，不复制原始字段值/正则输入 |
| render | 使用同一渲染结果，记录模型/用户文字摘要与长度；空输出为 `empty-render`，不是内部错误 |
| block | 命中 block 后所有其他候选为 `block-precedence`，不抢占其 repeat；各 block 的理由依现有顺序合并 |
| repeat | `repeat-seen`、成功占位或 `repeat-bypass`（block/every/subagent.start）；记录 gen/epoch/turn 来源和逻辑键摘要，不暴露临时绝对路径 |
| capacity / output | 同一原生调用的全部评估点共同排序、合并和容量判断；记录 `cap-drop` 及占位释放，或 `output` / `blocked`。估算/字符单位与现有上限不变 |

跨包编译折叠单列为 `compile-deduplicated` 并指向保留方，不能计为 runtime `dedup` 或 stats；这类规则没有虚构的 runtime entry。static 只展示 index 的 slot、摘要和估算，标为 `static-not-replayed`；native 只展示匹配说明文件中的 rule/event/timeout，标为 `native-not-executed`。本命令不执行 native，不读取真实静态文件来声称已加载，也不声称解释其他包在编译前已删除而没有记录的声明。

## 4. explain 报告与退出码

文本按输入/隔离范围、事实可用性、评估点、逐 entry 决定、编译说明、最终原生输出、诊断的顺序呈现。最终 stdout/stderr 用 JSON 字符串转义展示，空字节明确为 `""`，防止终端控制字符被执行；不会用占位文字替代真正的空 hook 输出。

JSON 顶层固定 `format=1`、`command="explain"`、`status`（ok/error）、`harness`、`event`、`inputs`、`state`、`facts`、`points`、`entries`、`compile_provenance`、`wire`、`diagnostics`。数组为空时输出 `[]`；相同捕获输入得到相同报告，不包含当前时间、耗时或随机临时路径。

- inputs：index/payload/facts（如有）的精确 SHA256；state：empty/snapshot、所选 scope 是否存在、输入清单摘要、事件前后 gen/epoch/prompt_count；snapshot 质量标为 `captured-nonatomic`。
- facts：字段名、available、source（payload/session/provided/unavailable）；points 与 entries 使用上文 trace，entries 按 engine 的稳定 order/entry/point 顺序呈现。
- compile_provenance：matched/unavailable/stale/invalid、说明文件摘要和安全的规则/折叠/静态/native 元数据。说明缺失或不匹配只产生 warning，执行仍按捕获的 index，覆盖范围明确为 `index-only`。
- wire：codec 实际返回的 `stdout`、`stderr` 字符串、`exit_code`、两条流的字节数和 SHA256；未到达编码阶段时为 null，不合成一个“预期响应”。结构化运行观察额外区分 decode/dispatch/encode/fail-policy，不能分析 logger 文本猜测。
- diagnostics：稳定 code/severity/message，可有 entry/field；不得包含原 payload、工具输入/输出、未选中正文、配置或凭据。最终 wire 是用户主动要求检查的内容，可以包含渲染后的规则正文；不自动保存到 hook.log、stats、probe 收据或仓库。

CLI 退出 0 表示重放完成；普通未命中、repeat、cap-drop、合法空事件或规则 block 都退出 0。用法、输入/index/state/facts 错误、未支持的事件/运行时、内部求值/编码错误、取消或清理失败退出 1。即使 fail-open 产生 `wire.exit_code=0`，管理命令仍退出 1；现有安装级 fail-closed 的原生结果单列在 wire 中。只因可选编译说明缺失或未提供外部事实而出现 warning 不改为失败。

有效解析的 `--json` 请求即使诊断失败也返回单个 error 报告；用法错误 stdout 为空，stderr 输出用法诊断。3/4 继续属于既有 sync/安装语义，不用于 explain。

合法空候选的 JSON 形状示例（输入摘要占位；空流摘要为实际 SHA256，不是验收结果）：

```json
{
  "format": 1,
  "command": "explain",
  "status": "ok",
  "harness": "codex",
  "event": "PreToolUse",
  "inputs": {
    "index_sha256": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
    "payload_sha256": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
  },
  "state": {
    "mode": "empty",
    "seed_scope_exists": false,
    "before": {"gen": 0, "epoch": 0, "prompt_count": 0},
    "after": {"gen": 0, "epoch": 0, "prompt_count": 0}
  },
  "facts": [],
  "points": [{"point": "tool.pre", "candidate_count": 0}],
  "entries": [],
  "compile_provenance": {"status": "unavailable", "coverage": "index-only"},
  "wire": {
    "stdout": "",
    "stderr": "",
    "exit_code": 0,
    "stdout_bytes": 0,
    "stderr_bytes": 0,
    "stdout_sha256": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "stderr_sha256": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
  },
  "diagnostics": [{"code": "provenance-unavailable", "severity": "warning", "message": "Compilation provenance is unavailable; index-only coverage."}]
}
```

## 5. probe 的观察范围与关联

```text
style probe [--harness h] [--timeout 60s] [--json]
```

probe 观察**已经安装的 STYLE runtime hook**。没有临时安装、注入测试规则、改订阅面、改信任或自动启动模型请求。用户在等待窗口内从自己的 harness 会话触发已订阅事件；命令输出就绪信息和该安装的可观察事件。只按实际 STYLE runtime 订阅判断，Codex 既有固定订阅仍可观察；静态/native-only 安装不会为了探测额外增加订阅。

- harness 省略时选择当前 lock 中已注册的全部已安装 harness，按既有报告顺序；提供时只接受一家，沿用 claude 别名。无安装、未知/runtime 不可用或没有 STYLE runtime 订阅时，给出 `probe-not-installed` / `runtime-unavailable` / `probe-no-runtime-hook`，不把 Detect 成功当完成。
- timeout 为正 Go duration，默认 60s，范围 1s–10m。各 target 同时开始等待，每家至少一个合格回调才整体成功；部分完成仍返回失败及各自状态。
- 预检只读 lock 和已安装路线的 STYLE 自身命令/订阅信息，由现有 route/agenthooks 归属识别核对；不扫描包源码，不检查原生作者命令是否能运行。固定 STYLE_HOME、route/config 根、lock 摘要、runtime 可执行文件摘要、原生 executable 身份和支持的进程来源检查方式。安装配置缺失/漂移在等待前报告 unavailable；等待中的漂移报告 installation-changed。
- Codex user/plugin 分别报告锁定的路线；当前一个 harness 只有一条锁定路线，本次不覆盖另一条路线。插件缓存或旧 runtime 不具备观察能力时明确不能成功，需要按正常安装/注册流程更新。

监听数据放在 `$STYLE_HOME/probes/`，不复用 state/stats。每个 harness 一个固定位置的短租约，`O_EXCL` 创建，目录 0700、文件 0600；内容含 format=1、随机 128-bit run ID、目标绑定、owner PID/进程出生身份、开始/到期时间及本次收据目录。根/后代 symlink 或非预期文件类型拒绝，不越出本次目录。并发占用返回 `probe-busy`，不抢占或删除别人的活动租约；多个 harness 的准备失败时撤销本次已创建的租约。

runtime 使用 agenthooks 的 OnAny 观察已经解码并通过正常 runner 入口的事件，在 STYLE 求值之前向观察器提供 provider/native event/session 等元数据。读取固定租约位置即可发现一次探测，不扫描目录；租约不存在时不创建文件，不启动线程/子进程。OnAny 不等待记录完成，只提交一份不可变的安全元数据；活动探测的观察任务可在最多 5ms/六层祖先的预算内捕获 OS 进程身份并写小收据，以免短命的 shell/hook 在控制器读取前退出。观察任务不与 engine 共享可变数据、不启动子进程；来不及记录即丢弃，不能延长 hook 退出等待。全量二进制哈希、来源验证和配置复核由等待中的 probe 管理进程完成，hook 不因此读取 lock 或包源码。提交失败/阻塞不能改变普通 hook 的输出、repeat、stats、deadline/fail 策略；实现须证明阻塞依赖被隔离，未记录只导致本次 probe 无证据。

合格收据必须同时满足：

1. 属于仍活动的 run ID，创建在租约窗口中；provider、STYLE_HOME、安装根和 runtime 可执行文件与本次绑定一致；旧收据、别家、别根、过期或重复收据不计数。adapter 必须能核对命令/路线来源，不能只把 lock 中的 route 标签抄给任何同名进程；无法区分 user/plugin 来源时仍为 origin-unverified。
2. native event 属于预检读取的 STYLE 自身订阅，payload 已由 agenthooks 成功解码且 session 非空。native 作者命令、安装文件存在、旧 stats 行、模型文字或仅执行 `--version` 都不算。
3. 管理进程通过 adapter 的进程来源检查核对 hook 与真实 harness 的关系，使用观察任务从 OS 捕获的 executable/父子关系/出生身份，以及控制器可复核的仍存活 harness 身份和预检文件身份，限制遍历层数与耗时；hash 在管理进程执行，不能只把租约给出的期望 hash 抄作实际结果。只保存 PID、可执行文件摘要和判定，不保存 argv/环境/路径中的私密参数。进程已退出而无足够来源事实、权限不足、平台不支持、来源缺失或二进制不符为 `origin-unverified` / `runtime-mismatch`，不能用任意 payload 自报 provider/PID 当作证明。

来源检查可以复用 agenthooks 已有 OS 读取能力，STYLE adapter 决定何种 harness 关系有效；不得把它变成启动 harness 或读取凭据的入口。进程关系和本地 run ID 只提供本机诊断关联，**不是防同用户伪造的认证**，也不能排除 harness 祖先进程下的手工调用。M3-B1/C1 因此还必须有独立的原生 hook 来源/事件证据，与该收据、时间和 session 关联；离线直接调用 handler 或伪造进程 fixture 只能关闭 A2。

收到合格回调可在 index 不可读时仍报告 `callback-observed`；它证明运行入口被调用，执行成功/最终投递另看 runtime 结果和 explain。probe 不拦截原有 hook，也不为了探测跳过普通 engine；真实事件本来会产生的 state/stats/log 更新继续存在，不能把它们归为 probe 的独立写入或承诺整棵安装树 mtime 不变。

## 6. probe 结果、清理与恢复

JSON 顶层为 `format=1`、`command="probe"`、`run_id`、`status`、`timeout`、`targets`、`cleanup`、`diagnostics`。targets 按报告顺序列 harness、route、callback_observed、origin、runtime/index 摘要（能够取得时）、event、session/agent/turn 的摘要和时刻；不保存 raw payload、prompt、工具正文、规则正文或全量环境。runtime 收据另有严格的 format=1 和大小上限（每 target 最多 32 条、每条 4 KiB），超限/坏行不计成功并给出质量诊断。

每个 target 状态为 waiting/callback-observed/timed-out/unverified/installation-changed/unavailable；整体状态为 observed/timed-out/partial/interrupted/error。文本同样区分“回调已观察”和“规则投递已验收”。`--json` 的 stdout 只有一个最终报告；就绪/触发提示写 stderr，不能输出无限进度行。

退出 0 要求全部 target 合格且本次租约/收据清理成功；超时、部分完成、来源无法验证、取消、配置/二进制绑定改变、I/O 或清理失败均退出 1，并保留已观察的 target。参数错误 stdout 为空；其他失败的 JSON 报告仍完整。callback-observed 不使 B1/C1、原生信任或官方计数自动通过。

| 结束方式 | 清理与恢复 |
|---|---|
| 成功/超时/正常错误/SIGINT/SIGTERM | 先停用本次租约，再清理自己的收据目录；只操作 run ID/文件身份相符的对象，不动其他探测、用户配置或安装文件 |
| cleanup 失败 | 报 `probe-cleanup-failed`、退出 1，给出本次残留路径和重跑恢复方式；已到期租约被 runtime 忽略，不能继续收集 |
| SIGKILL/崩溃 | 不保证进程立即清理；租约过期后不再记录。下一次 probe 核对同格式、到期、原租约进程已退出及身份后回收；无法确认时报告 busy/stale，需要先处理该条残留 |
| 租约过期/清理与并发回调竞争 | 写收据前后再次核对租约身份/到期；不得重新创建已删除目录或新的租约。退出清理要覆盖在途写入，不能把迟到回调算入下一次探测 |
| 等待中 install/sync/trust/uninstall 或可执行文件改变 | 绑定摘要变化即 `installation-changed`，撤销本次观察；不自动修复、迁移或重试安装 |

probe 不持久改变 hook 配置、lock/index/编译说明、包、信任或静态文件，因此没有“还原整份配置”步骤；这样不会覆盖等待期间其他程序合法写入的配置。A2 必须用 bytes/mode/mtime 对照证明这些边界，并用普通 hook 前后对照分离事件本身的 state/stats 变化。

成功报告形状示例（摘要/时间/run ID 为示意，不能用于关闭 B1）：

```json
{
  "format": 1,
  "command": "probe",
  "run_id": "00000000000000000000000000000000",
  "status": "observed",
  "timeout": "1m0s",
  "targets": [{
    "harness": "codex",
    "route": "codex-user",
    "status": "callback-observed",
    "callback_observed": true,
    "origin": "process-linked",
    "runtime_sha256": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
    "event": "SessionStart",
    "session_sha256": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
    "observed_at": "2026-10-17T00:00:00Z"
  }],
  "cleanup": {"complete": true, "remaining": []},
  "diagnostics": []
}
```

## 7. 分层与兼容

| 层 | 实现任务中的责任 |
|---|---|
| core/when、core/engine | 可选且确定性的 trace；保留 Match/Process 的既有行为，诊断与正常执行共用求值、排序、render、Claim 和 cap 逻辑；core 不导入 adapter 或 agenthooks |
| core/state | 现有 state 语义与 ID 编码；提供只读、有界且不跟随 symlink 的所选 scope 捕获支持；不把 probe 写进 session/stats |
| core/compile + app 冷编译 | 捕获实际 RulePlan/Deduplicated 生成 explain.json；与 index 的摘要/事务绑定，支持旧安装无说明文件；热路径不读取它 |
| internal/adapter | 构造隔离 runtime enricher、补充事实声明、支持的特殊事件和 probe 来源检查；名称合法性继续来自 T00 注册表，不在 core 新增两家名单 |
| internal/runtime + app | 编排重放、trace/report、租约/收据生命周期与 CLI；明确管理结果与原生结果，关闭重放时的真实观察/记录 |
| agenthooks | 继续独占原生事件表、decode/encode、MCP/Skill 协议补充、配置渲染/owner 合并/卸载；补充可选离线数据依赖和结构化运行结果观察。默认 Run API、输出和策略保持兼容，不复制 codec 到 STYLE |

新增的 agenthooks 离线依赖必须覆盖 codec 解码内部读盘；单纯在解码后观察不满足隔离要求。结构化运行结果只返回阶段/错误类别/原生退出码和实际流元数据，不返回未经处理的错误正文。探测观察使用现有 OnAny，不能伪造一个允许放行的 Decision；对没有观察器/租约的原有命令，旧 wire golden 和普通 stats 逐字节保持。

现有 index/lock/state format 均不升号；新增可选冷文件不使旧安装损坏。T10 只读操作不补写 explain.json、不 sync、不采信未批准源；T11 不要求额外 native 授权。D3 新安装范围/路线、D4–D8 新语义将在各自设计中扩展诊断，当前先忠实解释 M2/T00 行为；Windows 来源检查与真实执行按 D11/W1 另验，不能用本机 fixture 关闭。

## 8. 待执行的验收细化

下表保留定稿断言。2026-10-17，A1-01–A1-12 已按 [T10 验收报告](M3-A1-2026-10-17.json) 逐项通过，26 个新函数/145 个叶子用例、完整 test/vet/race 和兼容脚本均有新证据；A2-01–A2-12 已按 [T11 验收报告](M3-A2-2026-10-17.json) 逐项通过，28 个新函数/181 个叶子用例、三条路线的离线 OS 进程链、普通 runner 对照与完整兼容回归具备新证据。下一项 D2 设计，B1/C1 继续需要独立真实环境。

| 编号 | 验收断言 |
|---|---|
| A1-01 | CLI 参数、单 harness/别名、native event 与 payload 一致性、未知事件、未知/无 runtime 注册项、合法无条目/无 handler 事件及 PostModelSwitch 有确定输出/退出码 |
| A1-02 | JSON/UTF-8/尾随值/类型/必需字段/32 MiB payload、16 MiB index、坏 format/schema、symlink/FIFO/路径锚定都按边界拒绝，不执行外部程序 |
| A1-03 | 两家每个实际 runtime 事件走原 codec/adapter/engine；显式/隐式 skill、MCP、PermissionRequest、Claude error/expansion/model switch 与真实执行结果一致 |
| A1-04 | selector/sources/when 的 AND/OR、agent、model/effort/harness、缺失/空值、否定条件和原有 provider 未实现行为被忠实解释；短路阶段不伪造 pass |
| A1-05 | 空 state 与显式所选 snapshot 的 startup/clear/compact/resume/fork、原生/fallback turn、子 agent、session/turn/every repeat 和正文变化对照；不会修改源 bytes/mode/mtime |
| A1-06 | snapshot 坏 JSON/占位、超限、symlink、部分不存在和复制中变化明确处理；只取所选 session/agent，两个并发 explain 不共享环境/临时 state |
| A1-07 | 需要外部事实时只用绑定 facts；Codex 缓存、Claude 事件/env/session、未知 effort、Skill 补充与 MCP 冲突正确。用拒绝式依赖证明 decode 起的文件/进程/网络零调用 |
| A1-08 | block 优先、warn 双正文、空 render、单调用多点评估和 cap-drop/释放与普通 Process 的输出、状态、stats 一致；trace 不多 Claim 或多执行一次 |
| A1-09 | 匹配的编译说明解释跨包折叠/保留方与静态/native 边界；说明缺失/坏/过大/摘要漂移保持 index-only，不扫描新源、不执行 native、不虚构 stats |
| A1-10 | 说明文件的所有安装重建、sync、trust、保留包/最后卸载、失败回滚/并发读取，以及 M2 无说明安装兼容；index/lock/state/普通 hook golden 不变 |
| A1-11 | 文本/JSON 确定性、空 wire、实际 stdout/stderr 字节/hash/退出码；管理失败与 fail-open/fail-closed 分离，错误与报告不泄露 raw 输入或未选中正文，终端字符转义 |
| A1-12 | 成功/取消/内部错误/清理失败的隔离与恢复；相关完整 test/vet/race、双 module codec/协议/路线 conformance 和既有 smoke/迁移/replace 回归通过 |
| A2-01 | 默认全部已锁定目标、显式单家/别名、无安装/无 runtime hook、只生成文件/未加载插件、timeout 范围与多 target 部分完成均按契约报告 |
| A2-02 | 就绪后发生的 callback 按 run ID/时间/provider/root/route/executable 关联；旧收据、错误根、另一家、坏/空 session、仅版本命令和 native 作者命令不计成功 |
| A2-03 | 进程来源 fixture 覆盖真实祖先关系、手工直接调用、伪造 payload PID、权限缺失、不支持平台、hash 不符；这些 fixture 标为离线，不能冒充 B1/C1 |
| A2-04 | index 缺失/损坏仍能观察合法回调，执行失败另外报告；无回调或解码失败不因 fail-open=0 而成功，不以模型回复代替回调 |
| A2-05 | 单 target 同时探测返回 busy；多 target 初始化失败只撤销自己租约；别人的 lease/receipt 或合法并发配置变更不被覆盖 |
| A2-06 | 成功、超时、部分完成、SIGINT/SIGTERM、I/O 与 cleanup 失败均保留实际结果并清理/报告自己的残留，不留下活动探针 |
| A2-07 | SIGKILL/过期/崩溃后的下一次恢复、PID 重用/身份不明、迟到写入和租约替换竞争，不复活旧目录、不误删新探测 |
| A2-08 | receipt 上限、坏行/超限、正文/环境/凭据 canary、文本控制字符、JSON stdout 单报告与 stderr 就绪提示均符合白名单 |
| A2-09 | 等待期间锁/路线/配置绑定或 runtime 二进制改变报告 installation-changed；不自动 install/sync/trust，不批准或回退其他包 |
| A2-10 | 有/无租约的同一真实 runner fixture 输出、原生退出码、state/stats 与未加观察器对照一致；观察器失败/阻塞不改变 hook deadline/策略，昂贵来源检查只在管理进程，原有热路径性能门槛继续通过 |
| A2-11 | 原有配置/信任/静态文件/包/lock/index/说明文件 bytes/mode/mtime 保持；无跨包 owner 残留，scope 的正常事件更新单独对照 |
| A2-12 | CLI 帮助、稳定诊断/退出码、多 target 汇总及完整 test/vet/race/conformance/兼容脚本通过；不触发真实登录/模型/官方计数 |

B1/C1：固定真实 harness/runtime 二进制和安装根；从已安装路线产生原生回调，保存原生来源证据、probe 收据白名单及摘要；用同一 payload、index、事件前 state 和补充事实执行 explain，对照输出/去重/状态。需要人工采集原 payload 时留私有目录，归档只保留摘要和脱敏必要事实。Codex user/plugin 各自执行，Claude 当前 user 路线执行；旧/新会话、未触发与清理/卸载后的不再调用均需核对。投递给模型的证据继续使用原生上下文/请求观察，不以 probe 成功代替。

## 9. 评审与下一步

已确认的三项产品选择：原生 event + 默认空 state、显式 facts 的离线重放；与 index 绑定的可选编译说明文件及旧安装 index-only 回退；probe 采用已有 STYLE hook 的短时观察，不自动改配置或发送模型请求，来源不明不能算成功。

本稿已回写相关规范，文档/源码调查的可重复检查证据见 [D1 文档报告](M3-D1-DOCS-2026-10-17.json) 和 PROGRESS.md。用户已在明确评审问题后回复“继续”，据此关闭 M3-D1。批准检查见 [定稿报告](M3-D1-APPROVED-2026-10-17.json)；独立提交本次定稿后，按顺序执行 M3-T10/M3-A1、M3-T11/M3-A2。D1 定稿提交当时未执行功能测试；其后 T10/A1 已独立完成，操作说明见 [style explain](../STYLE-EXPLAIN.md)。T11/A2 也已独立完成，操作说明见 [style probe](../STYLE-PROBE.md)；B1/C1、Windows 及官方计数保持待验。下一项为 D2 来源与锁定契约设计，D2 及后续新契约没有提前批准。
