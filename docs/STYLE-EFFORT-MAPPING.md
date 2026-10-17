# 用户 effort 映射

在当前 STYLE 存储根写 `effort.yaml`。用户级通常为 `~/.style/effort.yaml`；项目级放在该项目独立的 STYLE home，可从安装 JSON 的 scope.home 查看。包内同名文件不作为用户配置。

```yaml
format: 1
harnesses:
  codex:
    ultra: xhigh
    future-tier: high
  claude-code:
    custom: medium
```

运行 `style sync`（项目级加 `--project <项目根>`）后，配置进入 index 和锁快照。install/update 也会采纳当前配置；单纯改文件不会立即改变运行中的规则。缺文件使用内置映射，删除后再 sync 恢复内置解释。

同一原值的用户条目优先于内置 exact/alias；未列出的值继续内置映射。原值键按去空白/小写归一化，只允许 1–64 字符 ASCII 标识符，字母或数字开头，后续可用 `_`、`.`、`-`。同名冲突拒绝；值限于 none/minimal/low/medium/high/xhigh/max/open，unknown 不能作为映射目标。每个 harness 最多 128 条，文件最多 64 KiB，未知字段、未注册 harness、非法值、多份 YAML、symlink 等会阻止管理写入。

用户结果的 `thinking_effort_approx` 为 true，内置已知值仍为 false，未知值为缺失。`thinking_effort_raw` 保持原文；映射只是用户对已有原值的解释，不能证明实际思考开关、模型行为或原生未提供的值。规则中的 when 仍使用 canonical 档位；用 `when.thinking_effort_approx: true` 区分用户映射，用 `when.thinking_effort: unknown` 匹配无法映射的值。

Codex 的原生配置读取顺序和会话缓存保持，Claude 仍按 event > 环境 > session 取 raw。映射更改或删除后会用最近 raw 重算 canonical/approx，避免旧档位残留；没有 raw 就不制造档位。state 按需记录映射摘要和 user/builtin/unmapped 来源，日志不额外记录原始配置。

plan 和管理报告显示当前配置摘要；doctor 报编译快照及待同步差异，并按快照解释诊断 raw。explain 只使用所选 index 快照、绑定 facts 和显式 state；inputs 显示摘要，canonical/approx 的 fact 来源为 mapping:user/builtin/unmapped，raw 的来源仍指原生事实。不会读取 ambient effort.yaml。

映射产物使用 index 6，严格锁含 effort_mapping 和 min_index_format: 6。旧运行器和管理器分别拒绝 index 和锁，doctor 提醒升级 runner。frozen 使用锁内已批准映射，现时文件缺失、变更或无效也不会被采纳；云端导出同样携带编译快照。删除映射后 sync 会清理锁元数据，若还有 unknown 则保留最低格式 5。已有映射 state 由当前 runner 下一次事件重算并移除专属来源字段；回退旧 runner 前须先用当前 runner 完成该迁移或显式重置相应 state。

定稿和离线证据见 [D7](review/M3-D7-EFFORT-MAPPING-2026-10-17.md)与 [A13](review/M3-A13-2026-10-17.json)。真实 B3/C5 投递独立验收。
