# 显式匹配缺值

M3-T43 为 `when.model`、`when.provider`、`when.thinking_effort` 和 `when.harness.permission_mode` 增加 `unknown` 选择器，匹配规范化值缺失或空串。各字段仍须声明对应 reads。

```yaml
reads: {model: true, thinking_effort: true}
rules:
  - id: missing-model
    target: event
    event: tool.pre
    when: {model: unknown}
    text: Model information is unavailable.
  - id: high-or-missing
    target: event
    event: tool.pre
    when: {thinking_effort: [high, unknown]}
    text: Apply this guidance for high effort or unavailable effort.
```

同字段列表为 OR，不同字段为 AND。普通模型 glob 不匹配缺值；保留字 unknown 也不匹配非空的模型名或 permission_mode 字面量 unknown。模型选择器沿用大小写不敏感，effort 沿用去空白/大小写归一化；provider 和 permission_mode 保持精确匹配。

未知原始 effort 保留在 `thinking_effort_raw`，canonical tier 为空时可匹配 unknown。会话已有的 model/effort 值仍是已知，当前事件未提供该字段不等于缓存缺失。provider 的缺值与重新解析遵循[服务来源说明](STYLE-PROVIDER.md)。unknown 条件不会保证模板有值，模板继续使用原有 default 或空串行为。

`>=`/`<=` 比较不含 unknown/open，`>=unknown` 和 `<=unknown` 拒绝。agent、thinking_effort_approx、harness.name 和 conditions 没有新增缺值语义；conditions 的 unknown 仍是普通字面量，缺字段的 not_contains 仍不满足。未用新选择器的规则保持原有缺值不满足。

plan 保留显式匹配缺值的规则，显示可能缺失的诊断；needs 仍完整。explain 对成功的缺值条件显示 `available: false`、`result: pass`、`source: unavailable`，不伪造可用数据，不记录输入正文。

这些规则生成 index 5，并按完整编译结果写入 `min_index_format: 5`。普通用户锁至少为严格 format 2，项目/Claude 插件锁仍为 3。旧 runner 拒绝 index，旧管理器拒绝锁，避免 sync 静默降级语义；doctor 提醒升级所有 runner。install/sync/update/uninstall 重新计算标记，移除最后一个 unknown 后清除。frozen 只按已批准锁恢复，不自动升级缺标记的锁。没有 unknown 的普通产物不增加标记。

契约与旧版本探针见 [D6](review/M3-D6-PROVIDER-UNKNOWN-2026-10-17.md)，本轮离线验收见 [A12](review/M3-A12-2026-10-17.json)。实际本地 B3/C5 投递另行验收。
