# M1b 手动探针

配置步骤和验收标准见 [手动验收手册](../../docs/STYLE-M1B-MANUAL.md)。本目录提供测试包、最小 skill fixture 和只读计时收集器；不会自动注册 hook、原生 skill 或 MCP。

`m1b-probes` 为第二个包，包含四条规则：

- `session-info`：输出模型/effort，允许 compact 后补发，也可用于 model switch 观察。
- `warn-shell`：仅匹配命令中的 `STYLE_M1B_WARN`，分别输出用户/模型标记。
- `block-prompt`：仅阻断含 `STYLE_M1B_BLOCK_PROMPT` 的 prompt，提示不含原 prompt。
- `parallel-once`：仅匹配命令中的 `STYLE_M1B_PARALLEL`，按 session 去重。

从仓库根复现离线准备检查：

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off \
  go build -o /tmp/style-m1b-probe ./cmd/style
/tmp/style-m1b-probe validate testdata/manual/m1b-probes
/tmp/style-m1b-probe plan testdata/manual/m1b-probes --harness codex,claude-code
```

上述检查不能替代真实 harness 的配置接受、上下文投递、并行调用或性能验收。

`measure_claude_hooks.py` 读取 Claude 2.1.300 debug 日志中的 STYLE command-hook 完成记录，保留所有匹配样本，按 nearest-rank 算法计算整体及各事件 p95。它不执行日志内容、不修改配置，也不把 harness 计时当成纯进程计时。脚本退出 0 仅表示生成报告；是否达标须检查 JSON 的 `status`。默认至少 100 条匹配记录、p95 严格小于 50ms；不会丢弃预热或慢样本。

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s testdata/manual -p 'test_measure_claude_hooks.py'
python3 testdata/manual/measure_claude_hooks.py \
  docs/review/M1B-CLAUDE-C9-HOOKS-2026-10-15.txt \
  --binary /Users/inxups/.local/lib/style-m1b/style
```

上述留存样本的预期状态是 `over-budget`，103 条、整体 p95=53ms；它是失败证据，不是通过断言。采集新样本时换成实际 debug 文件及固定二进制路径；日志格式改变产生未解析记录时先核对格式。

`measure_codex_hooks.py` 读取 Codex 0.159.3 通过 OTLP/HTTP JSON 导出的 `codex.hooks.command` span（每行一个导出请求），按 `hook.source`（codex-user 为 `user`，插件为 `plugin`）和 command handler 过滤，保留全部匹配样本。门槛事件默认 PreToolUse，至少 100 条、p95 严格小于 150ms；同时报告各事件分布和门槛事件的时间重叠数。span 由 harness 计时，包含进程启动和 I/O，不等于纯 Go 进程时间。采集时只给单次运行加 `-c` 配置，不写入 config.toml：

```sh
codex exec -c 'otel.trace_exporter={ otlp-http = { endpoint = "http://127.0.0.1:4318/v1/traces", protocol = "json" } }' ...
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s testdata/manual -p 'test_measure_codex_hooks.py'
python3 testdata/manual/measure_codex_hooks.py /path/to/otlp.jsonl --source user
```
