# 周期补发规则

`refresh: N` 让同一条规则、同一份渲染后的模型正文，在首次命中后，隔满 N 个实际收到的提交回合再次发送。T40 的离线验收见 [M3-A9](review/M3-A9-2026-10-17.json)，具体契约见 [D4](review/M3-D4-REFRESH-2026-10-17.md)。

```yaml
spec: style/v0.3
name: review-reminder
version: 1.0.0
description: 长会话评审提醒
rules:
  - id: locations
    target: system
    repeat: session
    refresh: 3
    reason: 在长会话中保留评审约定
    text: 评审意见请给出文件位置。
```

这条 system 规则转为动态规则。启动时 q=0 首次发送，提交回合 1、2 不发，3 再发，6 再发。工具规则继续等到原来的工具匹配点才发送：若回合 3 没命中、直到 7 才命中，就在 7 发一次，下次最早是 10。不会补发错过的多份提醒。

`refresh` 也可写在 `defaults` 和 `from` 覆盖中。只接受 0–1,000,000 的整数，省略或显式 0 关闭，0 可以取消继承值。正数只与 `inject` 或 `warn`、`repeat: session` 搭配。warn 默认仍为 every，启用周期时要明确指定 session 或从 defaults 继承；block、every、turn、subagent.start 和 system replace 不兼容。native 显式写入该字段，包括 0，均被拒绝；native 不继承普通 defaults。

inject 开启周期却没有非空 reason 会得到 T007 成本告警。reason 不进入模型正文。每次预算、T001/T003 和运行时硬上限均按完整正文计算，不除以 N；`always_on` 沿现有挂载点口径统计。只有完整策略相同的跨包规则才会折叠。

## 回合与生命周期

时钟只数 STYLE 实际收到的 `prompt.submitted`。工具调用、skill expansion、模型切换、compact、resume 和子 agent 启动均不增加回合。它不代表模型响应次数或 API 请求数。Claude 的 UserPromptSubmit 还可能来自自主启动的回合。

有原生回合 ID 时，同一 ID 的 prompt 重放只计一次；迟到工具保留该 ID 的原始回合数，不借用较新的回合。缺 ID 时按 prompt 回调计数，工具沿用当前值，plan/explain 披露 `prompt-count-fallback`；此时无法识别重复回调或迟到工具。工具携带尚未登记的 ID 时，不能触发周期到期，仍允许普通首次命中。没有 prompt 回调的子作用域只支持首次发送。

时钟按实际 state 根、harness、session 和 agent 隔离，保留旧 `scope.json.prompt_count` 和 repeat turn 的原有语义。

| 事件或变更 | 行为 |
|---|---|
| startup / clear | 新生命周期从 q=0 开始，按原匹配条件首发 |
| compact，reinject_on_compact=true | q 不变，新 epoch 可立即补发，随后重新计算间隔 |
| compact，reinject_on_compact=false | 保留原游标，不强制发送 |
| resume / 同一 scope 的 fork | 保留时钟与游标，不绕过 sources |
| 新 session、子 agent、其他项目或用户范围 | 独立状态，不复制父作用域游标 |
| 正文 A→B→A | B 可以首发；A 保留自己的上次发送回合 |
| 旧安装首次启用 / 改到新 N | 同正文已有普通 session claim 时，从当前 q 建立基线，等待 N 个新观察回合 |
| 关闭后以相同 N 重开 | 复用原游标；关闭期间没有 refresh 时不计数，不补建历史 |

未命中、空输出、block 优先和 cap-drop 不消耗补发机会。cap-drop 恢复先前游标及本次新增的普通 claim；去掉 refresh 后仍遵循普通 session 去重。

## 安装、升级与诊断

当前支持 macOS/Linux 上的 `codex-user`、`codex-plugin`、`codex-project`、`claude-user` 和 `claude-project`。Windows 的锁实现留待 D11/T70。Claude 云端与插件路线仍暂缓；两家真实周期投递仍由 M3-B3/C5 单独验收。

system 需要可输出的 prompt.submitted 入口：双入口时挂 session.start 和 prompt，只有 prompt 时披露降级；缺 prompt 则按 fallback 跳过或拒绝。工具类不会静默退回静态文件。Claude 安装在有任何有效 refresh 时增加 UserPromptSubmit 时钟入口和全部 SessionStart 生命周期来源，即使没有 prompt 输出规则；纯时钟回调默认不输出正文。

**先升级配置中实际调用的 STYLE runner，再重编译安装。** 只有最终有效运行时条目含正数 refresh，整份 index 才升为 format 2；旧 runner 会拒绝整份格式，沿用原有失败策略，旧错误提示可能只建议 sync。没有有效 refresh 继续 format 1，旧 wire/golden 保持。新版 doctor 的 `refresh-index-2` 提醒检查 runner 升级，不能仅凭管理命令已升级推断原生 hook 调用了新版。

修改间隔是规则效果变化，照常使用 install/update/sync/trust 的审批路径。frozen 仅重现原锁批准的效果；lock 1/2/3 和项目声明格式保持。真实旧 CLI 的 sync/trust/update/frozen 对不兼容源码或批准结果拒绝且不写入；这不承诺旧程序不能执行单纯移除全部包的操作。

`style plan` 的可选 refresh 信息说明间隔、计数方式和限制。`style explain` 继续使用已有 payload/index/state-dir 参数，在私有根运行相同逻辑；条目新增安全的 interval/current/last/clock/reason，原因包括 initial、seeded、not-due、due、clock-unavailable 和 cap-release。不会把原生 ID 或正文放进新增 trace。命令用法见 [explain](STYLE-EXPLAIN.md)。

stats 字段和 result 集合保持：保留输出为 output，未到期为 dedup，容量丢弃为 cap-drop；每份输出按全文估算。output 仍是运行时输出尝试，实际模型投递需要原生证据。

## 状态与恢复边界

新增状态位于 session/agent 的 `refresh/<harness>.json`，独立 format 1，包含 q、原生 ID 的 SHA256 收据和正文策略游标。原有 session.json、scope.json 和 seen 格式保持。单份新增状态最多 4 MiB、收据与游标合计 40,000 项，严格解码并拒绝 symlink、非法类型或溢出；超限明确失败，不自动丢弃历史去重记录。

当前 harness 有有效 refresh 时，运行时通过 session 的永久 `refresh.lock` inode 获取 OS 锁，保护共享状态、计数、claim 和释放。进程退出自动释放锁；不删除所谓陈旧锁抢占。等待最多 20ms，同时服从更早的取消/deadline，失败沿用现有 codec 策略。无 refresh 的 harness 继续原路径。

读取含新增状态的 snapshot 时只读取得已有锁，再双重捕获；锁缺失、忙或身份漂移均失败，不创建源锁、不改源文件时间。新旧状态合计仍受 16 MiB / 10,000 文件限制，锁文件不复制。冷路径清理过期 session 时也须取得同一锁，跳过活动写入者。不要手工删除活动锁或状态以绕过错误。

claim 在 stdout 前落盘；进程在成功占位后崩溃，可能损失这次发送，沿用原有 repeat 的取舍。不承诺跨文件掉电原子，也不把持久 claim 当作原生投递确认。
