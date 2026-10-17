# style explain 使用说明

2026-10-17：M3-T10 已实现，M3-A1 离线验收通过，见 [验收报告](review/M3-A1-2026-10-17.json)。完整契约见 [D1 设计](review/M3-D1-DIAGNOSTICS-2026-10-17.md)。[probe](STYLE-PROBE.md) 已由 M3-T11 实现并通过独立的 A2 离线验收；本页示例不代表真实 harness 回调或模型投递验收。

## 固定输入重放

在仓库根目录使用已构建的 `style`：

```sh
style explain --harness codex --event PreToolUse \
  --payload testdata/explain/pre-tool.json \
  --index testdata/explain/index.json --json
style explain --harness claude-code --event PreToolUse \
  --payload testdata/explain/pre-tool.json \
  --index testdata/explain/index.json
```

这组 [fixture/golden](../testdata/explain/) 的原生 stdout 为 additionalContext `Use fixture context.`，stderr 为空、原生退出码为 0。JSON 的 `wire` 保存实际输出和摘要；文本模式将两条流转义后展示。合法空输出保留为空字符串。

## 解释自己保存的事件

```text
style explain --harness h --event NativeEvent --payload f.json
              [--index index.json] [--state-dir state-root] [--facts facts.json]
              [--provenance explain.json] [--fail-closed] [--json]
```

`--harness` 必须指定一家，`claude` 是 `claude-code` 的别名；`--event` 使用 `PreToolUse` 等原生事件名，并与 payload 的 `hook_event_name` 完全相同。payload 必须是一个 UTF-8 JSON 对象，并有非空的原生事件名和 `session_id`。不改写保存的 payload。

index 默认取 `$STYLE_HOME/build/index.json`，STYLE_HOME 未设时取 `~/.style/build/index.json`。导出的 index 可以显式指定；命令不检测本机 harness、扫描包源码、同步安装或执行 native 命令。

省略 `--state-dir` 时从空状态开始。显式指定时，只读捕获所选 session 的 `session.json` 和当前 agent 的 `scope.json`、`seen`，在私有临时根重放生命周期、repeat 和容量释放，源文件的 bytes/mode/mtime 保持。报告标为 `captured-nonatomic`；检测到复制期间的变化会失败。解释过去的事件应使用预先保存的事件前快照，当前已执行事件的 state 可能得到 repeat-seen。

## 补充事实

默认不读取 harness 配置、CLAUDE_EFFORT、进程祖先、transcript、Skill manifest、MCP discovery 或网络。需要这些已捕获事实时，用严格的 format=1 文件显式提供；摘要必须绑定 payload 的精确字节，harness 使用 canonical ID：

```json
{
  "format": 1,
  "harness": "claude-code",
  "payload_sha256": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
  "harness_version": "2.1.301",
  "claude_env_effort": "high"
}
```

全零摘要仅为格式示例，实际值可用 `shasum -a 256 f.json` 计算后加 `sha256:` 前缀。可选字段还有 `model_provider`（受限服务 ID；省略/空串为缺值，覆盖快照中的 provider，不读取外部来源）、`codex_config_effort`、`mcp: {server, tool}` 和成功 Claude Skill PostToolUse 的 `claude_skill_content`。跨 harness、未知字段、null、摘要不符或与已知 MCP 身份冲突会失败；空字符串和省略分别处理。effort 优先级与现有 runtime 相同，Codex 已解析 session 仍用缓存。

payload/index/facts/编译说明上限分别为 32/16/1/16 MiB，文件必须为普通文件；拒绝叶项 symlink、FIFO 等。快照 JSON 单文件上限 64 KiB，seen 只接受空占位文件、最多 10,000 个，复制总量最多 16 MiB。Skill 补充正文最多 65,536 字节。

## 阅读结果

`points` 列出本次评估点和候选数，`entries` 按 engine 稳定顺序说明 selector、sources、when、render、block、repeat 和最终 output。只执行一次真实匹配与 Claim；短路后的阶段保持 `not-evaluated`。条件只显示可用性、来源和结果，渲染只显示长度与摘要；最终 wire 可以包含本次选中的规则正文。

新安装重建会与 index 同事务生成 `build/explain.json`。摘要匹配时，它解释跨包 `compile-deduplicated` 及保留方，static 标为 `static-not-replayed`，native 标为 `native-not-executed`。缺少、损坏、过大或过期的说明只产生 warning，覆盖范围降为 `index-only`；命令不会为旧安装补写文件。

管理命令退出 0 表示重放完成，包括普通未命中、repeat-seen、cap-drop、空事件或规则 block。输入、求值、取消或清理失败退出 1；有效解析的 `--json` 请求仍输出单个 error 报告，用法错误只写 stderr。`wire.exit_code` 单列原生结果，fail-open 的 0 不使管理失败变成成功；`--fail-closed` 仅模拟现有 Claude 安装级策略。

所有模拟 state/stats/cache 位于本次私有临时根，成功和错误后清理；清理失败会报告该目录并退出 1。完整 wire 只输出给本次调用者，不保存到真实 hook.log 或 stats。fixture、离线重放与补充事实不证明已发生真实投递；M3-B1/C1 继续独立待验。

可附加 `--count=exact --count-provider=openai --encoding=o200k_base` 计最终模型文本，原生 wire 不变；完整范围和 Anthropic 的显式发送行为见[文本计数](STYLE-COUNT.md)。
