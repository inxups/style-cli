# M3-D7 用户 effort 映射定稿

日期 2026-10-17；依赖 T43/A12 完成。按用户全部设计评审通过授权定稿并直接进入 T44。设计通过不替代 A13 与真实 B3/C5。

## 配置与优先级

用户在当前 STYLE 存储根（用户级或项目隔离 home）的 `effort.yaml` 配置，包不可声明或携带该文件。格式固定为单份严格 YAML；最多 64 KiB、每个已注册 harness 最多 128 条。缺文件表示内置映射。未知字段/重复键、未知 harness、非法 tier、归一化冲突、symlink/非普通文件、超限或读取错误阻止管理写入；不静默回退。

```yaml
format: 1
harnesses:
  codex:
    ultra: xhigh
    future-tier: high
  claude-code:
    custom: medium
```

键是 1–64 字符的 ASCII 标识符（字母/数字开头，后续字母/数字/下划线/点/横线），按去两侧空白和小写归一化，冲突拒绝。值是现有七档或 open；unknown 不作为 canonical 档位。显式用户项优先于内置 exact/alias，包括用户主动覆盖同名原生值；未命中的原值继续内置映射。每个用户结果 `thinking_effort_approx: true`，内置已知仍 false，未知仍 nil；raw 保持原文，用户映射不证明原生思考开关或模型实际行为。

## 快照、状态和兼容

配置由管理冷路径捕获并纳入并发变化保护；install/sync/update/uninstall 使用完整配置快照，计划/预览及 doctor 披露来源和规范化摘要。编译 index 增加可省略的 effort_mapping（仅有用户映射时），独立使用 format 6。严格锁保存相同的规范化 effort_mapping 和 min_index_format: 6；没有映射时省略，两种新特性并用取最高格式，unknown 单独仍 5。旧 runner/管理器分别拒绝新 index/锁，无新增配置时旧 JSON/YAML/golden 保持。

frozen 使用锁中已批准的映射快照，不读取现时 effort.yaml，不自动采纳新配置；重建后锁逐字节不变。正常 sync 采纳配置修改或删除，移除后恢复内置映射并清理对应锁元数据（如果还有 unknown，则最低格式 5）。云端导出带编译快照，不读取云端或本地 ambient 映射文件。

原生 raw 来源和优先级保持：Codex 会话首次读取配置，Claude event > 环境 > session。自定义映射只解释已有 raw，不制造缺失原值。规范化单 harness 映射的摘要及安全来源码（user/builtin/unmapped）按需保存在 state；映射变更或删除后用最近 raw 重算 canonical/approx，避免旧 tier 残留；缺 raw 时清除不能核验的 canonical。无用户映射且无旧映射状态时保持原 hot path/state 形状。

映射属于 adapter 的数据解释，core 仅校验通用映射数据结构及 canonical 词汇并承载快照；agenthooks 仍负责原生协议、事件 codec 和安装渲染。不得将本功能扩展为读取私有进程参数、提供不存在的 Claude off 事实或改变已有 Codex 中途 /model 的不可见边界。

## 诊断与验收

plan 和安装报告按需附加映射来源/摘要；doctor 同时报告编译快照与本地待同步差异；explain 使用 index 快照、绑定 facts 和显式 state，不读 ambient 映射，结果披露编译摘要及实际 user/builtin/unmapped 状态，原始输入不额外写入日志。

| 编号 | 状态 | 断言 |
|---|---|---|
| A13-01 | [ ] | 严格配置、完整内置行对照、用户覆盖、冲突/非法/超限/非普通文件、缺值和 raw/approx |
| A13-02 | [ ] | 两家 raw 来源不变、会话首次与动态读取、映射修改/删除重算、未知值清旧 tier、并发与来源摘要 |
| A13-03 | [ ] | plan/doctor/explain、离线与普通 codec 一致、六路线管理/frozen/失败恢复、云端快照 |
| A13-04 | [ ] | format 6/schema/严格构造/锁快照、真实旧程序零写入拒绝、旧 golden 与 CLI 回归 |

实现证据待 T44/A13 写入。真实 B3/C5 后续独立，用户延期的 C3/W1/Anthropic API 不在本设计中恢复为必过项目。
