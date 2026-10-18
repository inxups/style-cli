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

`probe_codex_context.py` 单独测量 resume/fork/compact 和 fresh/fork/嵌套子 agent 的 scope。compact 使用 `reinject_on_compact: false` 与 startup-only 探针；转发器只记录指定标记在请求各角色的位置/摘要，不把模型复述当投递证据。子 agent 的 harmless printf 分别用于 block/allow；没有远端操作。

```sh
PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_codex_context.py \
  --style /absolute/path/to/style --scratch "$(mktemp -d)" \
  --output /tmp/codex-context-result.json
```

若仅采集器断言失败，原始报告保留；`--verify-report <original.json> --output <new.json>` 只能对已有真实结果续验，不运行模型，并记录原报告摘要。真实运行未成功时不能用它把失败改为通过。

`probe_codex_overflow.py` 先对两家调用真实 STYLE 安装，验证 T003 拒绝且不写配置；然后直接用独立 Python hook 向真实 Codex 返回 12k/50k 字符的完整字段，绕开 STYLE runtime，不将 STYLE 的 cap-drop 当作原生处理。比较真实请求/rollout 中的预览与原生落盘文件，不能仅以首尾标记均出现判断完整投递。

```sh
PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_codex_overflow.py \
  --style /absolute/path/to/style --scratch "$(mktemp -d)" \
  --output /tmp/codex-overflow-result.json
```

此脚本对 Claude 只验证 STYLE 自身保护；Claude 的原生大字段仍需已认证的隔离会话，不得据此关闭该部分。

`calibrate_tokens.py` 使用固定 `token-calibration-40.json`（20 中文/20 英文），通过生产 STYLE CLI plan 取得估算；两家官方 count 接口分别对同结构基线和每条正文请求计数，差值用于比较。模型需显式指定。只读取 OPENAI_API_KEY / ANTHROPIC_API_KEY，忽略代理 base URL 和日常 Codex 令牌；缺凭据退出 2，报告保持 pending，不能当实际计数通过。不要把 key 粘到命令参数或报告里。

```sh
PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/calibrate_tokens.py \
  --style /absolute/path/to/style --openai-model gpt-6.1-sol \
  --anthropic-model claude-sonnet-5-5 --output /tmp/token-calibration.json
```

已核对官方 [OpenAI counting tokens](https://developers.openai.com/api/docs/guides/token-counting) 和 [Claude token counting](https://platform.claude.com/docs/en/build-with-claude/token-counting)。计数接口的结果按其语义记录，不代表实际会话账单；计数模型若不可用，保留失败，另起有明确模型的新报告。

### M3-CARRY-01：Claude 官方校准

2026-10-18 用户选择本轮只测 Claude。[M3 采集器](../../docs/review/M3-CARRY-01-CALIBRATE-2026-10-18.py)复用上述固定语料、同结构请求与生产 `plan` 估算，读取仓库外官方 settings 中的 `env.ANTHROPIC_API_KEY` 和 `model`。先核对已验二进制及全部构建输入，使用独立 HOME/STYLE/Codex/Claude 目录和版本 fixture；后者不代表真实 hook 投递。固定官方 HTTPS、系统 CA 校验、关闭代理/重定向，无自动重试；私有逐请求日志实时保存，不含 key 或原始 headers。

```sh
env -u OPENAI_API_KEY PYTHONDONTWRITEBYTECODE=1 python3 \
  docs/review/M3-CARRY-01-CALIBRATE-2026-10-18.py \
  --anthropic-settings /Users/inxups/.claude-style-official/settings.json \
  --output /tmp/new-claude-calibration.json
```

每次使用新输出路径以保留旧证据。共 46 次官方计数：空基线、40 条单规则及空基线/中英文首尾 5 次复测；不调用生成接口。继承的双供应商采集器在 OpenAI 未测时退出 2、`passed=false`；应核对 Anthropic `completed=40`、`available_provider_checks_passed=true` 和五项复测，不能将整体未完成误判为 Claude 失败或把整体标为通过。此次结果见[脱敏报告](../../docs/review/M3-CARRY-01-ANTHROPIC-2026-10-18.json)与[误差说明](../../docs/STYLE-COUNT.md#固定语料估算校准)。[离线核对工具](../../docs/review/M3-CARRY-01-VERIFY-2026-10-18.py)重算逐条/汇总结果、检查文档与历史保全，无 API 请求。

```sh
PYTHONDONTWRITEBYTECODE=1 python3 \
  docs/review/M3-CARRY-01-VERIFY-2026-10-18.py /tmp/claude-calibration-check.json
```


### M3-CARRY-01：OpenAI 自定义 API 配置

2026-10-18 用户要求开始配置可自定义请求链接的 OpenAI API。独立模板位于 `/Users/inxups/.style-openai/settings.json`，目录权限 0700、文件权限 0600。填写 `request_url`（完整请求 URL，包含接口路径）、`api_key` 和 `model`；密钥仅保存在该仓库外文件，不发送到聊天或写入命令参数。创建时三个值为空，用户随后已填写；准备阶段尚未调用服务。

用户随后指定用原生 codex-cli、隔离日常环境、直接 Responses。当前 `request_url` 填的是 HTTPS 根地址，独立入口将其规范为 `/v1` base URL，由原生 Codex 请求 Responses；也接受带 `/v1` 的 base URL 或完整 `/v1/responses` 地址。已成功完成固定回复连接验证，见[原生报告](../../docs/review/M3-OPENAI-CODEX-2026-10-18.json)与[可复跑采集器](../../docs/review/M3-OPENAI-CODEX-2026-10-18.py)。独立入口为 `codex-style-openai`，源码见[启动器](../../docs/review/M3-OPENAI-CODEX-LAUNCHER-2026-10-18.py)，每次读取私有 JSON 后直接执行本机 Codex；没有 HTTP 转发器。

```sh
codex-style-openai
```

配置和会话位于独立 `/Users/inxups/.style-openai/codex`，HOME/XDG/tmp 同样隔离；key 只注入专用进程环境，不写入生成的 TOML。当前验收配置为 low、read-only，关闭 shell 工具/网页搜索/重试；日常 Codex、Claude 和已有官方 Claude profile 原样。修改私有 JSON 的地址、key 或模型后，下次启动生效。

初次仅验证自定义服务的原生对话连接；现有官方校准采集器仍固定官方地址，不读取此模板，不将自定义结果自动登记为官方结果。[准备报告](../../docs/review/M3-OPENAI-CUSTOM-SETUP-2026-10-18.json)保留未请求时的历史快照。

### M3-CARRY-01：Codex 原生误差对照

用户后续要求开始固定 40 条测试。[结果](../../docs/review/M3-CARRY-01-CODEX-2026-10-18.json)为 39 条计数、1 条服务策略拒绝（`en-concurrency`）；6 项复测稳定。参见[计数口径和指标](../../docs/STYLE-COUNT.md#codex-自定义-responses-原生对照)。只有原生 Codex 接触远端 Responses，没有 HTTP 转发器；配置与原始日志均保存在私有目录。

[采集器](../../docs/review/M3-CARRY-01-CODEX-CALIBRATE-2026-10-18.py)读取仓库外 settings，验证已验二进制和 299 个构建输入，并以生产 `plan` 取得估算。每次使用全新输出目录：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 \
  docs/review/M3-CARRY-01-CODEX-CALIBRATE-2026-10-18.py \
  --output-dir /tmp/NEW-native-calibration
```

若原组在服务拒绝后中断，[续跑工具](../../docs/review/M3-CARRY-01-CODEX-CONTINUE-2026-10-18.py)复用原组的同一个冻结采集函数、配置和目录，归档原报告，记录拒绝项后继续剩余样本；不会修改或重试被拒绝的正文。当前续跑入口针对已验的根地址配置，每组只续跑一次。实际执行命令：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 \
  docs/review/M3-CARRY-01-CODEX-CONTINUE-2026-10-18.py \
  --directory /tmp/style-m3-carry-codex-measured-20261018
```

原生 `turn.completed` 可能汇总多个请求，不能直接当规则计数；首 response 的 `token_usage_record.usage` 和 `token_count` 必须一致。有效样本以空基线相减，缓存不扣除；CLI 加载规则时的首尾空白归一化与所有初始消息均由原生 rollout 验证。只测首请求输入增量，不以输出、整轮用量或账单替代。传输仅在零已记录响应时允许有上限的明确补跑，所有失败单列。续跑退出 0 表示全部语料已尝试且有效数据核对通过；报告的完整 `passed` 仍因拒绝项而为 false。完整校准仍未通过，官方 OpenAI 仍为 0/40，生产系数不改。

[离线复核](../../docs/review/M3-CARRY-01-CODEX-VERIFY-2026-10-18.py)读取原私有证据，独立重算有效样本、三组指标、原生计数/上下文/复测，检查失败保留和文档；不调用模型或构建 Go：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 \
  docs/review/M3-CARRY-01-CODEX-VERIFY-2026-10-18.py \
  /tmp/NEW-native-calibration-check.json
```

### 无 API key 的补充对照（M2-ALT-01）

`compare_tokens_offline.py` 对同一 40 条语料读取生产 CLI 估算，分别比较具名本地编码 `o200k_base` / `cl100k_base` 的纯正文计数及固定前缀的本地文本增量。它不假设模型映射，不包含请求封装/工具/隐藏指令，不代表 Claude 官方分词。`completed` 仅说明对照完整；没有准确性通过阈值。生产估算系数保持不变，官方校准继续 pending。

可选依赖仅安装到临时 venv，不加入产品或普通 CI 依赖。首次加载编码表需下载公共数据；预热后可离线运行，编码表摘要写入报告。

```sh
token_ref_dir=$(mktemp -d /tmp/style-token-reference.XXXXXXXX)
python3 -m venv "$token_ref_dir/venv"
"$token_ref_dir/venv/bin/python" -m pip install --index-url https://pypi.org/simple tiktoken==0.12.0
env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache \
  go build -trimpath -buildvcs=false -o "$token_ref_dir/style" ./cmd/style
PYTHONDONTWRITEBYTECODE=1 TIKTOKEN_CACHE_DIR="$token_ref_dir/encoding-cache" \
  "$token_ref_dir/venv/bin/python" testdata/manual/compare_tokens_offline.py \
  --style "$token_ref_dir/style" --output "$token_ref_dir/local-token-reference.json"
```

`summarize_claude_acceptance_usage.py` 只读取已通过的白名单报告所引用、摘要完全匹配的原生 transcript，输出会话 usage。保留每个响应身份的最后快照，不重复累加流式快照或缓存 TTL 子项；不导出正文。需原私有证据仍在本机。

```sh
PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/summarize_claude_acceptance_usage.py \
  --acceptance-report docs/review/M2-C1-CLAUDE-2026-10-17.json \
  --acceptance-report docs/review/M2-C2-CLAUDE-2026-10-17.json \
  --acceptance-report docs/review/M2-C4-CLAUDE-2026-10-17.json \
  --output /tmp/style-m2-subscription-usage.json
```

2026-10-17 的离线对照与六个已验收订阅会话结果见 [M2-ALT-01](../../docs/review/M2-ALT-01-2026-10-17.md)。会话之间工具、指令、模型/缓存状态不同，不能作受控 A/B 或逐规则成本差分。此补充证据不替代官方计数，M2-CARRY-01 保持待验。

`summarize_session_usage.py` 只读汇总真实报告和 retained Claude transcript/stats。Codex resume 使用最后会话累计 usage/stats，Claude 按响应身份保留最后 usage，避免重复相加；规则估算与整个会话 usage 分列。复现命令、缓存条件、覆盖范围及默认公式假设见 [FINAL-10](../../docs/review/M1-FINAL-10-CACHE-2026-10-16.md)。

`probe_claude_overflow.py` 绕开 STYLE，驱动真实 Claude2.1.300 CLI 和真实 Bash；Messages 服务仅绑定127.0.0.1并返回脚本内容。它捕获原生实际请求，检查完整正文或原生预览文件的精确字节，不用首尾标记猜测完整投递，也不读取/转发日常凭据。stub 的 usage 是协议占位，不是官方计数。复现：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_claude_overflow.py \
  --claude /absolute/path/to/claude-2.1.300 \
  --scratch "$(mktemp -d)" --output /tmp/claude-native-overflow.json
```

八组原生传输结果见 [FINAL-08 Claude](../../docs/review/M1-FINAL-08-CLAUDE-2026-10-16.json)。`probe_claude_failure.py` 使用同一本地协议支撑核对真实error字段、tool.error投递及stats；M1公开条件仍不开放tool.error上的tool.output。完整请求仅留私有scratch，提交报告排除合成usage和认证信息。

Claude真实认证补测使用 `probe_claude_context.py`（resume/fork/关闭补发compact/原生Agent scope）及 `probe_claude_thinking.py`（控制配置与原生effort）。两者的认证仅由原生CLI从用户已登录的M1隔离配置消费，程序不读取或复制凭据；用户settings/hooks被空setting-sources排除，运行自己的显式settings和独立STYLE_HOME。执行要求已登录的原生目标版本，失败仍留原报告。

```sh
PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_claude_context.py \
  --style /absolute/path/to/style --claude /absolute/path/to/claude-2.1.300 \
  --auth-config /absolute/path/to/isolated-auth-config \
  --scratch "$(mktemp -d)" --output /tmp/claude-context.json
```

原生请求由已核对canonical hash的api-request-blob/shape、assistant/user记录及keep/tail重放。fork旧system别名必须有已验证源transcript匹配，否则拒绝；采集的只是marker路径/参数/哈希，不复制提示词或思考正文。结果见 [FINAL-07](../../docs/review/M1-FINAL-07-CLAUDE-2026-10-16.json)；压缩必须另核对native boundary、epoch及输出次数，不能将补发当历史保留。

## M2 验收入口

当前步骤和阻塞统一见 [M2 手册](../../docs/STYLE-M2-MANUAL.md)。`probe_codex_m2.py` 检查 B1–B4 的两条真实路线；`probe_claude_m2_design.py` 保留 D4/D5 的协议/界面探针；`prepare_claude_m2_acceptance.py` 在新私有目录准备 C2/C3 的 native、每包 rules、跨包去重与卸载恢复。

Claude 准备程序只执行 `--version` 和 STYLE 安装，生成 `acceptance=false` 的 prepared.json；不登录或启动模型。2026-10-17 用户报告恢复并授权 agent 交互终端代跑，C1–C4 已按手册通过；最终安装验收复用已认证隔离配置并另建 STYLE_HOME/工作目录，不读取或复制凭据。原 PREP-03 未登录目录保留，新的准备程序仍只负责准备。三个新增测试验证最小环境/系统身份、凭据排除、原生日志白名单和恢复校验；统一运行 `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py'`，当前共 53 项。
