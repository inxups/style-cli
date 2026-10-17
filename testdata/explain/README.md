# M3-A1 离线解释 fixture

`index.json` 与 `pre-tool.json` 是两家共用的固定输入；`codex.golden.json`、`claude-code.golden.json` 固定完整解释报告。预期原生输出为 `PreToolUse` 的 additionalContext `Use fixture context.`，stderr 为空、原生退出码为 0；报告不得包含 payload canary。

`TestM3A1FixedGoldenAndOrdinaryRuntimeWire` 同时逐字节核对 golden 与普通 runtime/codec 的 wire。fixture 不代表真实 harness 调用、模型投递或 probe 验收；其他空输出、失败策略、状态与编译说明边界由 M3A1 Go 用例覆盖。
