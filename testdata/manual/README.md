# M1b 手动探针

配置步骤和验收标准见 [手动验收手册](../../docs/STYLE-M1B-MANUAL.md)。本目录提供测试包、最小 skill fixture 和只读计时收集器；不会自动注册 hook、原生 skill 或 MCP。

`m1b-probes` 为第二个包，包含五条规则：

- `session-info`：输出模型/effort，允许 compact 后补发，也可用于 model switch 观察。
- `warn-shell`：仅匹配命令中的 `STYLE_M1B_WARN`，分别输出用户/模型标记。
- `block-prompt`：仅阻断含 `STYLE_M1B_BLOCK_PROMPT` 的 prompt，提示不含原 prompt。
- `parallel-once`：仅匹配命令中的 `STYLE_M1B_PARALLEL`，按 session 去重。
- `subagent-marker`：在 Codex `subagent.start` 无条件注入 `M1B_SUBAGENT_CONTEXT` 标记，用于 U7 子 agent 投递验证；Claude 上 plan 为 skipped。它会进入该安装下每个 Codex 子 agent，验证完可从已安装副本中删去。

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

`probe_codex_config.py` 使用临时 CODEX_HOME、临时 git 项目和本地 HTTP sink，运行真实 Codex 解析配置并记录请求的 reasoning.effort。sink 故意返回 400，进程非零是预期；脚本按每例断言判定通过，不宣称模型推理或 STYLE 注入投递。

```sh
PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_codex_config.py
```

`probe_codex_routes.py` 在空 scratch 目录内建立独立 CODEX_HOME、STYLE_HOME、Git 仓库与本地 bare remote，用真实 Codex 和现有自定义 Responses 服务逐条执行 B1–B9。它只读取日常 Codex 的模型/provider 配置，把所需连接配置保留在私有 scratch 中；日常安装不变。插件登记使用真实本地 marketplace 管理命令。请求经仅监听 loopback 的转发器，报告只保留模型、effort、STYLE 上下文/摘要、rollout、stats/state、原生 hook span 与 usage，凭据不写入报告。模型服务失败与准备流程失败均保留在结果中，脚本非零不能标记验收通过。

```sh
PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_codex_routes.py \
  --style /absolute/path/to/style --route codex-user \
  --scratch "$(mktemp -d)" --output /tmp/codex-user-result.json
# 在另一个空目录对 codex-plugin 独立运行同一整套检查。
# 双向切换另加 --switches-only，以原生 span 检查四类事件各一次及来源。
```

minimal/启动覆盖用历史实测支持 minimal 的 `gpt-6-sol`，其他分支沿用配置模型。未知原值要求 native request、STYLE unknown 与条件不命中相符；模型服务可能拒绝该值，结果单列 `model_turn_completed=false`，不宣称远端模型支持未知档位。项目层无法可靠确定信任时，STYLE 按既定边界跳过并由 doctor 解释；报告同时列出原生值与 STYLE 值。首次工作区 TUI 信任不由 exec 探针验收。
