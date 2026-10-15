# STYLE CLI M1 测试清单

更新时间：2026-10-15

本文件记录测试矩阵和每项当前状态。测试通过必须有命令输出、fixture/golden 文件或真实环境记录作为证据。

## 状态约定

- `[x]` 已通过。
- `[ ]` 尚未实现或尚未执行。
- `[~]` 已执行但受环境影响，或只完成部分验证。
- `[!]` 发现失败或设计风险。
- `[?]` 需要真实 Codex/Claude 环境。

## 测试环境约定

agenthooks 测试从 `agenthooks/` 目录执行。为避免宿主环境中的 `CODEX_HOME`、`CODEX_SANDBOX` 等变量改变 provider 检测，标准命令使用：

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  go test ./...
```

静态检查：

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  go vet ./...
```

## 已完成的 agenthooks 基线

- [x] **B0.1 全量单元/集成测试（隔离环境）**：`go test ./...` 通过；覆盖 root、install、provider、e2e、transcript 等包。
- [x] **B0.2 静态检查（隔离环境）**：`go vet ./...` 通过。
- [x] **B0.3 Claude/Codex codec 定向测试**：解码、编码、round-trip 和 no-op 行为通过。
- [x] **B0.4 能力降级定向测试**：Codex 不支持 ask 时按策略降级；Claude deny 输出通过。
- [x] **B0.5 安装渲染定向测试**：Codex async/trust、定义哈希、幂等合并和外部条目保留通过。
- [~] **B0.6 继承宿主环境的全量测试**：`TestCopilotDetection` 会因外部 `CODEX_SANDBOX` 优先于测试设置的 `COPILOT_CLI` 而失败；清理变量后通过。按当前策略只记录，不修改 agenthooks。

## 顶层构建和模块

- [x] **B1.1 T10 双模块入口**：从仓库根目录执行 `make test-go`、`make vet-go` 通过；根依赖烟测可以调用本地 `agenthooks.New()`，agenthooks 全量测试及 vet 通过。`go list -m all` 显示根 module 和本地 agenthooks module。Go `./...` 不跨越嵌套 module，所以聚合入口明确分别执行两个 module 的命令；两边均使用临时 Go 缓存并清理 `CODEX_HOME`、`CODEX_SANDBOX`。证据：`Makefile`、`internal/agenthookscheck/agenthooks_test.go` 和 `docs/PROGRESS.md` T10/B1.1。

## 核心包解析

- [x] **B1.2 T11 目录包、单文件包和规则来源**：`go test -count=1 ./core/spec` 覆盖目录包、单文件包、`STYLE.yaml`、规则 frontmatter、`file:`、`text:`、显式 `from:`、递归 `**` glob（零层及多层）、glob 相对路径排序、未声明规则不自动加载、单文件默认 ID、路径逃逸、符号链接、文件大小、UTF-8/NUL、正文 frontmatter 和 `file-into-rules`。完整根/agenthooks 测试和 vet 通过。证据：`core/spec/spec_test.go`、`docs/PROGRESS.md` T11/B1.2。
- [x] **B1.3 T12 规则字段合并**：覆盖标准默认、包 priority/budget、包级 defaults、规则 frontmatter、from 覆盖顺序；覆盖 `reads` 并集、`when` 整体替换、动态 target/action 默认、block repeat 忽略和停用过滤。根/agenthooks 全量测试与 vet 通过。重复 ID、`defaults` schema 白名单、禁止覆盖 `target`/`id` 在 T13 validate 中检查。证据：`core/spec/merge_test.go`、`docs/PROGRESS.md` T12/B1.3。
- [x] **B1.4 T13 validate**：`go test -count=1 ./core/spec ./internal/tokenestimate` 覆盖包/规则 schema、单文件规则清单限制、重复 ID、from 覆盖、未声明文件、模板变量和运行时 reads、条件字段事件可用性、`tool.added` 适用工具、负向可选字段告警、RE2、effort、合法动作组合、T005/T006/T007、诊断码和稳定顺序；离线 token 估算有独立公式测试。`make test-go`、`make vet-go`、`go mod tidy -diff` 和 `git diff --check` 通过。证据：`core/spec/validate*_test.go`、`internal/tokenestimate/estimate_test.go`、`docs/PROGRESS.md` T13/B1.4。
- [x] **B1.5 T14 模板和条件**：`go test -count=1 ./core/template ./core/when ./core/spec` 覆盖静态/动态分类、默认值、缺失/空字符串、转义、引号内 `|`/`}}`、无效模板、字段 AND/列表 OR、大小写不敏感模型 glob、`**/`、effort 档位和比较（排除 `open`）、主/子 agent 默认、provider 永不匹配、条件字段缺失时否定不匹配、FieldReader、matcher 数据独立及复用、JSON 往返保留空 selector 和 agent 范围。`make test-go`、`make vet-go`、`go mod tidy -diff`、gofmt 和 diff 检查通过。证据：`core/template/*_test.go`、`core/when/*_test.go`、`core/spec/validate_template_parser_test.go`、`docs/PROGRESS.md` T14/B1.5。实际 harness 字段提取、effort 来源和 engine 集成仍在后续任务，不提前标记 A10/A11 完成。
- [x] **B1.6 T15 `warn` 首句提取**：`go test -count=1 ./core/warn ./core/template` 覆盖中英文句末标点、英文标点的空白/行尾边界、首个非空行、前导空行、多行、无标点、CRLF、空正文及动态模板渲染后提取。完整根/agenthooks 测试和 vet 通过。证据：`core/warn/sentence_test.go`、`docs/PROGRESS.md` T15/B1.6。compile/engine 对静态与动态规则的调度接入仍未完成。
- [x] **B1.7 T20 `core/plan`**：隔离环境下 `go test -count=1 ./core/plan ./core/spec ./core/template ./core/when` 覆盖 Requirement 的事件/能力/数据需求，native/degraded/partial/skipped/never-trigger/install-error 状态，tool 替代时机、条件数据缺失、block→warn 且不走静态回退、warn 部分能力、skill 隐式+显式事件、fallback skip/error、动态/静态 system、replace 降级、静态槽位缺失、harness 过滤、可选模板值告警、文本与 JSON 报告不泄漏正文并保持确定性，以及 Load→Validate→Merge→Build 整合。`make test-go`、`make vet-go`、`go mod tidy -diff`、gofmt、diff 检查通过。证据：`core/plan/plan_test.go`、`docs/PROGRESS.md` T20/B1.7；A2 的 CLI golden 与 token 报告现由 B1.24 覆盖。
- [x] **B1.8 T21 `core/tokens`**：隔离环境下 `go test -count=1 ./core/tokens ./core/spec ./internal/tokenestimate` 覆盖估算公式与 Unicode rune 数、静态包/规则/settings/vars 值、64/128 字符运行时占位符、默认值/非法模板、warn 首句和 `model_text`、prompt block 不计模型输入、动态 system/static fallback、T001/T002/T004 严重程度与限值、adapter Layout 原生事件/工具聚合、block 与 inject/warn 分隔符及最坏情况、跨包 T003 token/字符 80% 边界、共享 T005/T006/T007、稳定 JSON 和正文不泄漏。双 module `make test-go`、`make vet-go`、`go mod tidy -diff`、gofmt 和 `git diff --check` 均通过；agenthooks 没有改动。证据：`core/tokens/*_test.go`、`core/spec/validate_lints_test.go`、`internal/tokenestimate/estimate_test.go`、`docs/PROGRESS.md` T21/B1.8。T003 的完整调用仍由后续 adapter/install 集成提供已安装包清单和 adapter 上限；CLI golden/A18 尚未完成。
- [x] **B1.9 T22 适配器一致性接口**：隔离环境下 `go test -count=1 ./internal/adapter ./core/plan ./core/tokens` 覆盖 provider 注册的稳定排序/重复拒绝/查找复制、检测结果规范化与错误传播、能力/数据来源/静态位置投影、route priority 和 Plan/Apply/Remove/Status 生命周期、输入上限单位与超限策略、compact signal 校验，以及 `core` 不导入 `agenthooks` 或 `internal/adapter` 的边界扫描。`go vet ./internal/adapter`、完整双 module `make test-go`/`make vet-go`、`go mod tidy -diff`、gofmt 和 `git diff --check` 通过；未修改 `agenthooks/`。证据：`internal/adapter/registry_test.go`、`internal/adapter/boundary_test.go`、`docs/PROGRESS.md` T22/B1.9。具体 provider adapter、effort 映射和 runtime Enrich 仍由后续任务接入。
- [x] **B1.10 T23 `core/compile`**：隔离环境下 `go test -count=1 ./core/compile ./core/plan ./core/tokens ./core/spec ./core/when ./core/template ./core/warn ./internal/tokenestimate` 覆盖静态 system/settings 渲染、动态 system 双评估点、model/effort/harness/conditions 归一化、tool block、skill 隐式/显式 match、warn 的显式 `model_text` 与动态首句处理、来源/正文 SHA256、settings 和 runtime 默认值、fractional priority、多包顺序不受输入顺序影响、index JSON 往返与稳定编码、无 HTML 转义、blocked/错包/不完整/过期 plan、来源缺失、非法评估点和静态槽位。双 module `make test-go`/`make vet-go`、`go mod tidy -diff`、gofmt 和 diff 检查通过；`agenthooks/` 未修改。证据：`core/compile/build_test.go`、`docs/PROGRESS.md` T23/B1.10。逐字节 example golden 与 JSON Schema 属于 T24/A3，运行时消费属于 T32，均未提前标记。
- [x] **B1.11 T24 index schema/golden**：`core/compile/golden_test.go` 从 `example/terse-reviewer/STYLE.yaml` 执行 Load→Validate→Merge→Plan→Compile，使用 Draft 2020-12 JSON Schema 验证 `testdata/golden/index.json`，并对编译输出做逐字节比较。负向用例覆盖缺少必需字段、未知评估点、session 缺少 `sources`、session 错带 `match`、prompt 错带 `sources`、routed skill entry 缺少 `match`、未知 entry 属性；正向边界覆盖负 priority、重复 `sources`，以及 event-wide tool entry 无 `match`。`make test-go`、`make vet-go`、`go mod tidy -diff`、gofmt 与 `git diff --check` 通过，均使用隔离 Go 缓存且 `GOPROXY=off GOSUMDB=off`；新增 schema 测试依赖 `github.com/google/jsonschema-go`。证据：`schema/index.schema.json`、`example/terse-reviewer/`、`testdata/golden/index.json` 和 `docs/PROGRESS.md` T24/B1.11。
- [x] **B1.12 T30 `core/state`**：`go test -count=1 ./core/state` 覆盖启动/恢复/清理/压缩来源、scope counters、原子写、严格 JSON、0600/0700 权限、unsafe ID、过期清理；全量 race 测试见 T30–T34 验证记录。证据：`core/state/state_test.go`。
- [x] **B1.13 T31 repeat 去重**：state 单测覆盖 session/turn/every、body hash 变化、block 与 `subagent.start` 例外、compact 重注入、释放与并发 claim；engine 子进程测试覆盖 20 个进程竞争且恰好一个输出。证据：`core/state/state_test.go`、`core/engine/engine_test.go:TestTwentyProcessesEmitOneRepeatSessionEntry`。
- [x] **B1.14 T32 runtime engine**：engine/runtime 测试覆盖 schema load、预编译 matcher、target/source/when、模板渲染、block 优先、inject/warn 顺序、cap 裁剪和 reservation release、metadata-only stats/log。证据：`core/engine/engine_test.go`、`internal/runtime/runtime_test.go`。
- [x] **B1.15 T33 白名单和 fail policy**：Codex/Claude fixture 精确 stdout、Claude prompt suppress whitelist、missing/invalid index 与 invalid state fail-open、Claude fail-closed exit code、敏感内容不进入 stdout/log/stats 均有测试。证据：`internal/runtime/runtime_test.go`、`agenthooks/runner_test.go`。
- [x] **B1.16 T34 动态 system**：startup/prompt 双评估、effort/model 更新后重渲染、相同正文去重、正文变化补发和 compact 选择性重注入通过 runtime/engine 集成测试。证据：`internal/runtime/runtime_test.go:TestRuntimeReevaluatesDynamicSystemWhenModelChanges`、`core/engine/engine_test.go:TestDynamicSystemReevaluatesAndDeduplicatesByRenderedBody`。
- [x] **B1.17 T40 安装层 F1–F4**：Codex canonical shell matcher 编译为 `Bash`；`install.Uninstall` 只删除匹配 `--owner` 的 JSON/TOML 内容并保留同组其他 handler；重复安装保持幂等；Codex 插件生成 `.codex-plugin/plugin.json`、`hooks/hooks.json`、可执行 `bin/style` 和 CODEX_HOME trust 配置，长格式插件 trust key 与 `DefinitionHash` 一致；owner 解析和命令行回读有覆盖。证据：`agenthooks/matcher_test.go`、`agenthooks/install/install_test.go`、`agenthooks/detect_test.go`、`agenthooks/e2e/codex_test.go`（真实运行需 `AGENTHOOKS_E2E=1`）。
- [x] **B1.18 T41 F5–F6**：`SessionInfo` 保留 effort/raw/approx/harness version，Claude `effort.level` 解码；映射表解析拒绝重复/未知档位，未知值保留 raw 且不伪造近似结果；Codex `-c`、profile、受信项目和用户 TOML 优先级及 `--ignore-user-config` 有 fixture；Claude 事件/环境/状态优先级有 fixture；会话状态原子持久化近似标记和一次性解析位。证据：`agenthooks/codec_test.go`、`agenthooks/detect_test.go`、`internal/adapter/effortmap/*_test.go`、`internal/adapter/codex/effort_test.go`、`internal/adapter/claude/effort_test.go`、`core/state/state_test.go`；真实 provider 的 adapter Enrich/版本检测留在 T44/T45。
- [x] **B1.19 T42 F7–F9**：Runner golden 验证 Codex `SubagentStart` payload 的 agent id/type 到决策 handler、`additionalContext` 编码和 legacy observer 兼容；Claude `UserPromptExpansion` 的 decode→handler→block/context 编码通过精确 JSON；codec 测试覆盖 fork source、prompt_id、effort、MCP `name/source`、`to_model` 更新及 `suppressOriginalPrompt` nested HSO；Codex skill parser 覆盖规范正则、顺序/去重、shell 读取变体和绝对命令路径。双 module `GOPROXY=off GOSUMDB=off make test-go`、`make vet-go`、gofmt 和 diff 检查通过。`skills.read`/显式 skill runtime Enrich 属于 U6/T44，真实 SubagentStart context 投递属于 U7/M1b。证据：`agenthooks/runner_test.go`、`agenthooks/codec_test.go`、`agenthooks/claude_skill_test.go`、`internal/adapter/codex/skill_test.go`、`agenthooks/provider/claudecode/claudecode_test.go`。
- [x] **B1.20 T43 F10 Claude exec 安装**：render 输出独立 executable 与 argv，不 shell 拼接；fixture 验证有空格的 binary 路径和附加参数原样保留、hook args 次序、timeout 向上取整/SessionStart 默认值、fail-closed 只对 blocking PreToolUse 输出 `--fail-closed`/`onFailure:block`。版本 fixture 接受 2.1.295/v2.1.295 及更新版本，拒绝 2.1.294、缺失/畸形/同版 prerelease；fail-open 与 telemetry 不要求版本。merge/uninstall 回归证明 args 中的 owner 会被替换/删除并保留外部 handler，且降级或版本缺失时仍可卸载。`GOPROXY=off GOSUMDB=off make test-go`、`make vet-go`、gofmt 和 diff 检查通过。证据：`agenthooks/install/install_test.go`、`docs/PROGRESS.md` T43/B1.20。
- [x] **B1.21 T44 Codex adapter**：固定 surface 覆盖 SessionStart、UserPromptSubmit、Pre/PostToolUse、SubagentStart，并验证原生 matcher；Codex plugin/user route 对 `config.toml` 与 hook 文件生成可审阅 changeset，Apply 拒绝过期快照，静态-only 更新/卸载和嵌套根目录均有回归；已有用户 `developer_instructions` 时不声明静态槽位；native 工具 blind spot 有测试。Enrich 覆盖配置 effort 一次解析、未知 effort raw、prompt/显式 skill、隐式 skill、MCP、嵌套 `tool.input.*` 模板/条件值、patch `tool.added`、tool output、turn/session/agent scope 及 SubagentStart additionalContext 精确 codec 输出。验证：`GOPROXY=off GOSUMDB=off make test-go`、`GOPROXY=off GOSUMDB=off make vet-go`、隔离缓存下 `go test -race -count=1 ./internal/adapter/codex ./internal/runtime ./agenthooks/install`、gofmt 与 `git diff --check` 通过。证据：`internal/adapter/codex/{route_test.go,runtime_test.go,effort_test.go,skill_test.go}`、`internal/runtime/runtime_test.go:TestCodexSubagentStartRuntimeDeliversContextForAgentType`、`agenthooks/install/install_test.go`、`docs/PROGRESS.md` T44/B1.21。真实 Codex context delivery、Claude adapter、CLI sync/trust 与完整 adapter conformance 仍待后续任务/M1b。
- [x] **B1.22 T45 Claude adapter**：动态 surface 按 native event+matcher 合并并稳定排序；用户级 `settings.json` 复用 owner 合并/卸载并保留外部 handler；`CLAUDE.md` 按 `style:<package>` 标记块增删、保留用户字节、拒绝 malformed marker；Detector 注入版本和配置目录；Facts 覆盖能力、数据源、静态位置和 workspace trust；Enrich 覆盖 event→`CLAUDE_EFFORT`→session effort、后续 event 覆盖已解析状态、prompt/explicit skill、Edit/Write `tool.added`、MCP、implicit skill、tool output、agent scope；runtime 接入 `UserPromptExpansion` decision，skill block 不输出 Claude prompt-only suppress 字段。验证：根 module 与 nested `agenthooks` 全量测试、`GOPROXY=off GOSUMDB=off make vet-go`、隔离缓存下 `go test -race -count=1 ./internal/adapter/claude ./internal/runtime ./agenthooks/install`、gofmt 和 `git diff --check` 通过。证据：`internal/adapter/claude/{adapter_test.go,route_test.go,runtime_test.go}`、`internal/runtime/runtime_test.go:TestClaudePromptExpansionRuntimeUsesAdapterAndRunnerHandler`、`agenthooks/install/install_test.go`、`docs/PROGRESS.md` T45/B1.22。真实 Claude workspace trust、compact、model switch 和性能仍待 C1–C9。

- [x] **B1.23 T50 `style init`**：`go test -count=1 ./internal/app -run 'TestInit|TestRunInit'` 覆盖目录包/单文件包骨架可被 `spec.Load`、`Validate`、`Merge` 接受，当前空目录初始化，名称归一化，`--help` 成功且不写文件，已存在/非空目标拒绝并保留原字节，非法名称拒绝，命令成功输出稳定；`go run ./cmd/style init --help` 可执行且退出 0。双 module `make test-go`、`make vet-go`、gofmt 和 `git diff --check` 通过。证据：`internal/app/init_test.go`、`internal/app/init.go`、`docs/PROGRESS.md` T50/B1.23。

- [x] **B1.24 T51 `style validate` / `style plan`**：隔离缓存下 `go test -count=1 ./internal/app` 覆盖有效/无效包稳定文本诊断、JSON 诊断码与退出码、参数错误、显式/多 harness 选择、默认/显式 session 假设、Codex/Claude 原生 token bucket 聚合；`TestExamplePlanMatchesProviderGoldens` 对示例包 Codex/Claude stdout 逐字节比较，`TestProviderLayoutUsesNativeAggregateBuckets` 固定 MCP 聚合及 canonical/native tool 映射。`go run ./cmd/style validate example/terse-reviewer` 返回 `valid`，两个 `style plan` 命令返回 0。双 module `make test-go`、`make vet-go`、gofmt 和 `git diff --check` 通过。证据：`internal/app/command_test.go`、`plan_golden_test.go`、`plan_layout_test.go`、`testdata/golden/plan-{codex,claude-code}.txt`、`docs/PROGRESS.md` T51/B1.24。
## M1a：STYLE 离线验收

- [x] **B1.25 T52 `style install`**：`go test -count=1 ./internal/app -run 'TestInstall|TestRunInstall'` 覆盖 `--yes` 确认、settings 必填值和 0600 持久化、requires.runtime、replace/`--allow-replace` 边界、重复安装幂等、包/锁/index/适配器路线写入；`go test -count=1 -C agenthooks ./install` 覆盖多文件 changeset 的 stale 检查、原子写入和取消时回滚。示例包隔离 `STYLE_HOME` 的 `go run ./cmd/style install ... --yes --json` 返回 0。双 module `make test-go`、`make vet-go`、gofmt 和 `git diff --check` 通过。证据：`internal/app/install_test.go`、`agenthooks/install/install_test.go`、`docs/PROGRESS.md` T52/B1.25。
- [x] **B1.26 T53 `style uninstall`**：`go test -count=1 ./internal/app -run 'TestUninstall|TestRunUninstall'` 覆盖确认门槛、单包卸载、部分卸载后保留剩余包并重编译 index/route、`--all --json`、link 源保留、最后一包移除和外部 Codex TOML 字节恢复；`go test -count=1 -C agenthooks ./install` 的 `TestManagedTOMLRoundTripRestoresExternalBytes` 覆盖已有文本以无换行、单换行和双换行结束时逐字节往返，且保留外部内容和其他 owner。双 module `make test-go`、`make vet-go`、gofmt 和 `git diff --check` 通过。证据：`internal/app/uninstall_test.go`、`agenthooks/install/install_test.go`、`docs/PROGRESS.md` T53/B1.26。

### 包格式和 validate

- [ ] **A1** 示例包 validate 退出 0；所有 invalid fixture 对应错误码退出 1：`dup-id`、`from-and-inline`、`inline-missing-id`、`file-and-text`、`file-into-rules`、`body-has-frontmatter`、`rule-file-pkg-field`、`empty-body`、`override-target`、`defaults-field`、`undeclared-var`、`reads-missing`、`event-var-in-template`、`sensitive-reserved`、`condition-field-wrong-event`、`bad-regex`、`bad-effort-expr`、`action-target`、`block-after`、`replace-non-system`、`bad-name`、`bad-id`、`path-escape`、`symlink`、`file-too-large`、`bad-encoding`、`marker-in-body`。
- [x] **A2** 示例包 `style plan` 对 Codex/Claude Code 使用显式 harness 生成的 stdout 与 `testdata/golden/plan-codex.txt`、`plan-claude-code.txt` 逐字一致；golden 包含每条规则状态、降级/能力、路线、原生输入上限、包预算、逐规则/逐事件 token 和会话成本。证据：`internal/app/plan_golden_test.go`、`internal/app/plan_layout_test.go`、`testdata/golden/plan-{codex,claude-code}.txt`。
- [x] **A3** 示例包生成的 `index.json` 与 golden 逐字一致，并通过 Draft 2020-12 schema。证据：B1.11。
- [x] **A21** `warn` 首句提取覆盖中英文、无标点、多行和开头空行；新增 ASCII 句末条件、CRLF 和动态模板值用例。证据：`core/warn/sentence_test.go`、B1.6。
- [x] **A22** 非交互安装没有 `--yes` 退出 1；replace 没有 `--allow-replace` 被拒绝。证据：`internal/app/install_test.go:TestInstallRequiresReplaceConfirmationAndCommandYes`。

### engine、state、codec 和输出

- [x] **A4** Codex/Claude `pre_tool_use.json` fixture 的 hook stdout 与精确 golden 一致。证据：`internal/runtime/runtime_test.go:TestRunMatchesCodexAndClaudeFixturesToExactBlockResponses`。
- [x] **A5** inject/warn 依 order 合并，block 优先且只输出拒绝。证据：`core/engine/engine_test.go:TestInjectAndWarnEntriesMergeInOrder`、`TestBlockTakesPrecedenceAndDoesNotClaimOtherEntries`。
- [x] **A6** session/turn/every、正文哈希变化、block 不去重、warn 每次发送均有覆盖。证据：`core/state/state_test.go`、`core/engine/engine_test.go:TestWarnRepeatEverySendsEveryMatchingInvocation`。
- [x] **A7** 20 个并发独立进程对同一去重键恰好一个输出。证据：`core/engine/engine_test.go:TestTwentyProcessesEmitOneRepeatSessionEntry`。
- [x] **A8** startup/resume/clear/compact 来源和 gen/epoch/prompt_count 行为正确。证据：`core/state/state_test.go:TestUpdateMaintainsSessionAndScopeLifecycle`、`TestStartupResetsScopeAndReservations`。
- [x] **A9** model/effort 变化触发动态 system 重评估；相同渲染正文去重、变化正文补发。证据：`internal/runtime/runtime_test.go:TestRuntimeReevaluatesDynamicSystemWhenModelChanges`、`core/engine/engine_test.go:TestDynamicSystemReevaluatesAndDeduplicatesByRenderedBody`。
- [x] **A10** core effort 条件比较、Codex/Claude 映射表、未知值、来源优先级和近似标记均有离线 fixture；T44/T45 已接通 Codex/Claude event/session 的状态解析，真实 hook 行为仍由 M1b C1–C3 验收。证据：`internal/adapter/codex/runtime_test.go`、`internal/adapter/claude/runtime_test.go`、`internal/adapter/{codex,claude}/effort_test.go`。
- [~] **A11** core main/subagent 默认、显式 any 和 subagent 不去重有测试；T44 已将 Codex agent id/type 投影到 scope/condition 并经 runtime 编码 `additionalContext`。真实 Codex context 投递仍待 U7/M1b。
- [x] **A15** engine 输出字段 whitelist，Claude prompt block 带 `suppressOriginalPrompt`，并按 Claude 方言放在 `hookSpecificOutput`；Codex 不接受该 Claude 专用字段。证据：`core/engine/engine_test.go:TestClaudePromptBlockSetsOnlyAllowedSuppressField`、`internal/runtime/runtime_test.go:TestClaudePromptBlockSuppressesOriginalPrompt`、`agenthooks/runner_test.go`。
- [x] **A17** 缺失/损坏 index 与损坏 state 默认空输出并记录安全错误码；Claude fail-closed 返回失败码。证据：`internal/runtime/runtime_test.go:TestMissingOrInvalidIndexUsesCodecFailPolicy`、`TestStateFailureUsesFailOpenAndLogsOnlySafeCode`。
- [x] **A19** 超出输入上限时从 order 大的一端裁剪并释放被裁条目的占位。证据：`core/engine/engine_test.go:TestCapDropsHighestOrderAndReleasesItsReservation`。
- [x] **A20** index-only engine `Process` 的 200 次样本 p95 为 128.083µs，低于 50ms；独立 benchmark 为 33,567 ns/op（300 次），不含 shell 启动。证据：`docs/PROGRESS.md` T30–T34 验证记录。

### 安装、信任和 token

- [ ] **A12** 普通包改动由 sync 阻塞并由 trust 确认；link 包行为符合设计；trust 基于同一份内存字节。
- [~] **A13** install 重复幂等、install/uninstall 外部配置保留和最后路由清理已有证据；install 换路线无残留仍需命令级回归，完成 T54/A13 补项后再标记 `[x]`。
- [x] **A14** Codex 静态托管区位置正确；用户自定义 developer instructions 时降为动态；Claude `CLAUDE.md` 标记块正确增删，并保留外部 settings/用户字节。证据：`internal/adapter/codex/route_test.go`、`internal/adapter/claude/route_test.go`。
- [x] **A16** core 不直接 import `adapter/codex` 或 `adapter/claude`。证据：`internal/adapter/boundary_test.go`。
- [ ] **A18** token estimator 与 golden 一致；T001–T007 各有触发 fixture；T003 超限安装不产生变更。

## 完成后回顾发现

B1.2 和 B1.4 的历史通过状态表示原有测试套件在声明范围内通过，不覆盖所有输入边界。回顾探针新增以下边界项，现已全部修复并补齐回归覆盖：

- [x] **RV-T11-01/R01**：多 YAML 文档和非法尾部被忽略。 已修复；证据：`core/spec/frontmatter_test.go`：拒绝第二个文档、空第二文档、非法尾部和非 mapping；接受单文档结束标记与尾部注释。`go test -count=1 ./core/spec` 通过。
- [x] **RV-T11-02/R02**：非 Markdown `from:` 被加载。 已修复；证据：显式路径与 glob 结果统一限制为 `rules/**/*.md`，validate 复查来源；`core/spec/from_source_test.go` 覆盖 `.txt`、混合 glob、rules 外路径及绕过 Load 的校验。`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-01/R03**：NaN/Inf 通过 validate，合并值无法 JSON 编码。 已修复；统一数字与 scalar 检查拒绝 NaN/Inf；覆盖包、defaults、规则、from 覆盖、budget 和 vars，合法数字 defaults 与 JSON 编码通过。证据：`core/spec/validate_numbers_test.go`；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-02/R04**：非字符串嵌套键被静默丢弃。 已修复；递归检查原始 mapping，拒绝非字符串键；asStringMap 拒绝有损转换。证据：`core/spec/validate_map_keys_test.go` 覆盖包/规则/覆盖字段及合法字符串键；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-03/R05**：native name selector 与 `tool.added` 的可用性判断不一致。 已修复；native `names` selector 参与 `tool.added` 可用性判断，具体提取由 adapter 决定，缺失值仍不匹配。证据：`core/spec/validate_condition_test.go` 覆盖 `Edit` 单值及名字列表；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-04/R06**：skill `at` 未参与隐式事件判断。 已修复；skill 隐式事件按 `at` 派生，并尊重 block 默认 before；`skill.explicit` 仍独立可用。证据：`core/spec/validate_condition_test.go` 覆盖 before/after `tool.output` 与显式 prompt；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-05/R07**：非法 model glob 延迟到 compile 才失败。 已修复；validate 与 Compile 共用 `core/when.ValidateModelGlob`，非法模式稳定返回 `bad-model-glob`。证据：`core/spec/validate_model_glob_test.go`；`go test -count=1 ./core/spec ./core/when` 通过。
- [x] **RV-T13-06/R08**：单文件 native schema 与目录包不一致。 已修复；单文件包的 provider native hook 字段与目录规则共用 native schema 判定。证据：`core/spec/validate_native_single_file_test.go` 以相同内容分别验证目录包和单文件包；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T2X-01**：事件级 `target: event` 挂到 `tool.pre` 时，compiler 原先生成空 `match`，导致有效规则生成的 index 被自己的 schema 拒绝。证据：`core/compile/golden_test.go:TestEventTargetAtToolPointCompilesWithoutMatch` 端到端编译并验证 schema；golden 包含每个条目的 `target`。
- [x] **RV-T2X-02**：空/空白 tool、native names、skill selector 及格式错误 MCP selector 原先通过 Validate。证据：`core/spec/validate_edge_test.go` 覆盖错误输入和合法 MCP wildcard；`core/compile/golden_test.go:TestNormalizedMatchRejectsInvalidSelectors` 覆盖 compile 防御性拒绝；schema 负向用例覆盖 target/match 不一致、空 selector 列表和 MCP slash/空白约束；定向 spec/compile 测试通过。
- [x] **RV-T41-01**：Go TOML parser 在 `internal/adapter/codex` 被直接导入，但根 `go.mod` 仍将其声明为 indirect。依赖层级已修正；`GOPROXY=off GOSUMDB=off go mod tidy -diff` 和 nested `agenthooks` tidy diff 通过。
- 完整回顾和处理结论：`docs/STYLE-M1-REVIEW-2026-10-13.md` 的“T20–T24 完成后回顾”；全量复验和提交记录见 `docs/PROGRESS.md` 的 `RV-T2X-01/RV-T2X-02`。

复现源文件和结果见 `docs/review/M1-2026-10-13-probe.go.txt`、`docs/review/M1-2026-10-13-results.json`；修复后的探针结果见 `docs/review/M1-2026-10-13-fixed-results.json`。R01–R08 已完成，M1a 仍需其余离线验收条目。

## M1b：Codex 真实环境

- [?] **CDEX-01/B1** 全新安装、信任一次、第二包安装/卸载/sync 后不重复信任。
- [?] **CDEX-02/B2** 模型、low/minimal/high effort 条件和上下文内容正确。
- [?] **CDEX-03/B3** compact 后 session-info 只补发一次，静态 developer instructions 可见性已记录。
- [?] **CDEX-04/B4** `cat`、`sed`、`head` 等读取 SKILL.md 和 `$code-review` 显式 skill 去重正确。
- [?] **CDEX-05/B5** force push 被拦，force-with-lease 放行。
- [?] **CDEX-06/B6** apply_patch 新增 TODO 触发 warn，删除 TODO 不误报。
- [?] **CDEX-07/B7** GitHub create_pull_request MCP 调用前注入。
- [?] **CDEX-08/B8** 静态规则在 config.toml 开头，卸载后其他字节不变。
- [?] **CDEX-09/B9** md 改动在 sync/trust 前不生效，trust 后生效。
- [?] **CDEX-10/B10** 插件和用户级路线都通过，切换路线无残留和重复。
- [?] **CDEX-11/B11** 固定订阅面 PreToolUse p95 小于 150ms，doctor 登录 shell 检查通过。
- [?] **CDEX-12/B12** resume 的 session.start 注入和历史保留行为已记录。

## M1b：Claude Code 真实环境

- [?] **CLAUDE-01/C1** 新会话 SessionStart、模型和 effort 行为正确。
- [?] **CLAUDE-02/C2** compact 后可重注入规则只出现一次。
- [?] **CLAUDE-03/C3** model switch 后下一条 prompt 补发新模型内容。
- [?] **CLAUDE-04/C4** 隐式 Skill 和显式 `/code-review` 各触发一次并共享去重。
- [?] **CLAUDE-05/C5** PreToolUse warn 的用户提示和模型上下文均符合实际能力。
- [?] **CLAUDE-06/C6** force push 被拒；prompt block 不携带原始 prompt。
- [?] **CLAUDE-07/C7** 并行工具调用同一 session 规则只输出一次。
- [?] **CLAUDE-08/C8** 卸载后 settings.json 和 CLAUDE.md 字节级恢复。
- [?] **CLAUDE-09/C9** exec hook p95 小于 50ms；resume 不重复 session.start。

## Adapter conformance

- [ ] 解析：每个事件的统一 event、model、cwd、turn、agent、tool.added、MCP 和 skill 正确。
- [ ] effort：每个 `effort.yaml` 映射值、原始值、未知值 fixture 正确。
- [ ] 往返：规则 engine 输出各 provider golden；no-op 使用正确方言。
- [ ] 安装：Plan → Apply → Status → Remove 保留外部内容、无 STYLE 残留、幂等、换路线清理。
- [ ] 报告：同一包在每个适配器上的 plan/token golden 稳定。
- [ ] 输出白名单：所有 golden 不含禁止字段。
- [?] e2e：真实 CLI 注入到达模型，block 真正生效。
