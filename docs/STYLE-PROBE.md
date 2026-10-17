# style probe 使用说明

2026-10-17：M3-T11 观察已安装的 STYLE runtime hook，离线验收记录见 [M3-A2](review/M3-A2-2026-10-17.json)，契约见 [D1 §5–6](review/M3-D1-DIAGNOSTICS-2026-10-17.md#5-probe-的观察范围与关联)。当前路线为 `codex-user`、`codex-plugin`、`claude-user`。本轮 fixture 不代表 M3-B1/C1 的真实回调或模型投递验收。

## 启动观察

```sh
style probe --harness codex --timeout 60s --json
style probe --harness claude --timeout 30s
style probe --timeout 2m --json
```

省略 `--harness` 时选择当前 lock 中全部已安装且已注册的 harness，按既有报告顺序输出；显式参数只选一家，`claude` 是 `claude-code` 的别名。`auto`、列表、多次指定同一参数及位置参数会失败。timeout 是 Go duration，默认 `60s`，范围 `1s`–`10m`。

命令使用 `$STYLE_HOME`，未设置时使用 `~/.style`。它只读现有 lock、路线配置、STYLE 自身 runner 命令/订阅、信任绑定和可执行文件；不会检测版本、扫描包源码、运行作者 native 命令或发起模型请求。所有目标预检通过后才建立本次观察窗口。

看到 stderr 的 `probe ready` 后，在自己的 harness 会话中触发列出的已订阅原生事件。不要只运行 `--version` 或手工调用 hook。保持产生回调的 harness 进程存活，供控制器核对出生身份和 executable。观察器不延长 hook 的退出等待；短命进程或超过记录预算可能没有收据，可在窗口内再次触发实际订阅事件。

预检由既有 adapter/agenthooks 核对 STYLE 命令归属和原生配置；foreign/native 作者命令不会成为 runtime 订阅。只有静态规则或 native 命令、无 runtime hook、未启用插件、配置/信任漂移、旧 runtime 缺观察能力时均不能成功。请使用正常安装/注册流程处理这些情况后重新观察。

runtime 的文件身份和完整摘要在预检固定；native executable 从本次 PATH 解析并只读捕获。已安装命令必须与当前入口及锁定的路线绑定一致。PATH 包装脚本、不同的原生子程序、插件缓存根不符、进程读取权限不足或无法复核存活祖先时保持来源未核验。当前 OS 来源实现为 Darwin/Linux；Windows 的原生实现与验收仍归 D11/W1。

## 阅读结果

`--json` 的 stdout 只有一个最终 format=1 报告，stderr 只写有限的就绪/事件提示。文本输出对可变字段及残留路径转义。报告不含 raw payload、prompt、工具正文、规则正文或环境值，session/agent/turn 仅以 SHA256 表示。

| 目标状态 | 含义 |
|---|---|
| `waiting` | 预检通过，尚未取得合格回调；另一个目标预检失败时不会实际开始等待 |
| `callback-observed` | 本轮收据的事件/安装根/runtime/路线与 OS 祖先身份关联通过 |
| `timed-out` | 等待窗口结束，未取得合格回调 |
| `unverified` | 有候选回调，但进程或路线来源无法核验 |
| `installation-changed` | 等待期间 lock/配置/index 或 executable 绑定发生变化 |
| `unavailable` | 没有可观察的安装、订阅、可执行文件或路线 |

整体 `observed` 表示全部目标通过；`partial` 表示到期时部分目标通过；其余为 `timed-out`、`interrupted` 或 `error`。只有全部目标合格且清理成功时退出 0，其余退出 1，保留已观察目标。用法错误只写 stderr；其他可解析的 JSON 请求在失败时仍有最终报告。

`index_status=captured` 只表示捕获了 index，不能据此认定规则求值成功；缺失或损坏 index 仍可观察回调，并给出 `probe-index-unavailable`。报告始终以 `execution=not-assessed` 标明执行未验，规则命中、最终 wire 和去重可用保存的事件前输入进行 [explain](STYLE-EXPLAIN.md) 对照。

`callback-observed` 仅证明本机入口关联。进程关系和 run ID 不是防同用户伪造的认证，无法排除 harness 祖先下的手工调用。真实 M3-B1/C1 还需独立的原生来源/事件和上下文或请求投递证据；官方计数仍两家各 0/40、非阻塞待验。

## 租约、清理与恢复

本轮数据只新增在 `$STYLE_HOME/probes/`：

```text
probes/
  codex.lease.json
  claude-code.lease.json
  runs/<128-bit-run-id>/
    owner.json
    codex/00.json
    claude-code/00.json
```

目录为 0700，文件为 0600；固定租约用 `O_EXCL` 获取，每家最多 32 条收据、每条最多 4096 字节。租约最多 16 KiB，收据只接受严格白名单的单个 UTF-8 JSON 对象。symlink、FIFO、非预期文件类型、超限或坏记录不计成功。租约不存在时，普通 hook 只检查固定路径，不创建探测文件或后台任务。

OnAny 在解码后、STYLE 求值前提交不可变的安全元数据；后台观察任务最多用 5ms/六层祖先的预算读取实际 OS 身份和写小收据。完整 executable 哈希、存活 harness 核验和配置复核由管理进程负责。观察任务不等待 engine，也不由 hook 退出等待；失败或阻塞只会丢失本轮证据。

成功、超时、错误、SIGINT/SIGTERM 均先停用自己的租约，再清自己的 run。并发探测返回 `probe-busy`；多目标初始化失败会撤销本次已获取的租约，别人的 lease/receipt 保留。租约和 run 同时核对字节、文件身份及 owner；检测到替换会报告错误，避免删除新探测的数据。

SIGKILL/崩溃可能留下租约；到期后 runtime 不再记录。下一次 probe 只有确认租约有效、已到期且原 owner 已退出，或 PID 已被不同出生身份复用时才回收。owner 仍活着、权限不足、身份不明或归属不匹配时不回收。请先结束原探测进程、等待到期并重新运行同一 probe；仍失败时按 `cleanup.remaining` 检查权限和归属，保留他人的活动数据。

清理失败输出 `probe-cleanup-failed`、本轮残留路径并退出 1，已经观察到的 target 保留。迟到写入会重新核对租约/到期时间，且只能打开已存在的目录，不会重新创建旧 run 或下一轮的租约。

probe 不更改配置、信任、静态文件、包、lock/index/编译说明。正常原生事件仍执行原 engine，按既有行为更新 state/stats/log；这些事件更新不会回滚。A2 以普通 runner 的 stdout/stderr/原生退出码及 state/stats 对照单独验证这一边界。
