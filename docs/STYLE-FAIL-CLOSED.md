# 规则故障时阻断

M3 支持普通规则 `fail: open|closed`，省略为 open。defaults 可以设置；native 规则不能使用这个字段。

```yaml
spec: style/v0.3
name: guarded-guidance
version: 1.0.0
description: 工具执行前的故障保护示例
rules:
  - id: shell
    target: tool
    tool: shell
    at: before
    fail: closed
    text: 执行前核对命令的影响范围。
```

inject、warn 和 block 都可启用，必须直接落到具有 deny 能力的 tool.pre、prompt.submitted 或 skill.explicit。system、after、不能阻断的事件/adapter 会拒绝安装；fallback 不能削弱该策略。`plan --json` 披露 fail、deny 能力和故障范围。

当有效 index 中某条 closed 规则属于本次原生回调的评估点，handler error、panic 或内部 timeout 会由 agenthooks 输出安全 deny/block。正常 target/when 未命中、缺条件数据、dedup、cap-drop 或空 inject/warn 不触发故障保护。空 block reason 属于错误。

保护按整个原生事件范围生效：同事件中其他规则失败也阻断；state 尚不可读时无法可靠计算 when/selector，因此它们不能缩小故障范围。Codex 显式 skill 和普通 prompt 共用原生回调，故障范围也共用。正常求值仍完整使用各条规则的条件。

index 缺失/损坏、原生 payload 解码失败、程序没启动、宿主杀进程或外部超时、hook 未获信任，不能从规则中恢复失败策略。Claude 原有 `install --fail-closed claude-code` 继续独立工作，PreToolUse 的 onFailure:block 需要 Claude >=2.1.295，内部失败退出 1；规则级保护不自动启用该安装开关。规则级被处理的故障输出合法 deny/block JSON，退出码沿原 codec。

只有 closed 产物使用 index 7 和严格锁 min_index_format:7。升级所有 STYLE runner；旧管理器会拒绝该锁。移除最后一条 closed 后，重新 update/sync 按剩余功能计算最低格式。frozen 使用批准源及严格锁；显式 open/省略保持旧运行时产物形状。云端 bundle 携带同一编译策略。

设计和验证见 [D8](review/M3-D8-FAIL-CLOSED-2026-10-18.md)、[A14](review/M3-A14-2026-10-18.json)。离线 fixture 不能替代 B3/C5 原生投递验收。
