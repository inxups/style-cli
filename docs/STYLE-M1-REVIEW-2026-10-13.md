# STYLE M1 已完成工作回顾

初稿日期：2026-10-13；T20–T24、T30–T34 回顾更新：2026-10-14

结论：T10–T14、T20–T24 与 T30–T34 的实现维持清楚的分层，`agenthooks/` 负责 hook 协议和 provider 事实，STYLE core 负责包、规则语义、规划、预算、编译和运行时策略；这条边界应继续保留。三轮回顾均未发现 P0/P1；T10–T14 的 8 个 P2 问题，以及 T20–T24 回顾发现的 2 个 P2 边界问题，均已加回归测试并修复。M1a/M1b 尚未完成，按任务依赖继续推进。

## 修复跟进

每项只有在回归测试通过后标记 `[x]`。以下状态覆盖报告中的历史发现；原观察结果保留用于对照。

| 编号 | 状态 | 修复结果与证据 |
|---|---|---|
| R01 | [x] | YAML decoder 完整消费输入，拒绝第二文档或非法尾部；`core/spec/frontmatter_test.go`，`go test -count=1 ./core/spec` 通过 |
| R02 | [x] | 显式来源和 glob 结果必须为 `rules/**/*.md`，validate 复查；`core/spec/from_source_test.go`，`go test -count=1 ./core/spec` 通过 |
| R03 | [x] | 数字/scalar 校验统一拒绝 NaN/Inf，修正合法数字 defaults 的枚举误判；`core/spec/validate_numbers_test.go` 覆盖数字来源与有效值 JSON 编码，`go test -count=1 ./core/spec` 通过 |
| R04 | [x] | 原始嵌套 mapping 报 schema 错误，map 转换拒绝有损输入；`core/spec/validate_map_keys_test.go` 覆盖包、规则和覆盖字段，`go test -count=1 ./core/spec` 通过 |
| R05 | [x] | native `names` 选择器可参与 `tool.added` 事件适用性判断；adapter 决定实际提取，字段缺失不匹配；`core/spec/validate_condition_test.go`，`go test -count=1 ./core/spec` 通过 |
| R06 | [x] | skill 隐式事件按 `at` 和 block 默认时机派生，显式事件另行保留；`tool.output` 仅在 after 可用；`core/spec/validate_condition_test.go`，`go test -count=1 ./core/spec` 通过 |
| R07 | [x] | validate 与 Compile 共用 `core/when.ValidateModelGlob`，无效模式报 `bad-model-glob`；`core/spec/validate_model_glob_test.go`，`go test -count=1 ./core/spec ./core/when` 通过 |
| R08 | [x] | 单文件包在 `target: native` 时接受与目录规则相同的 `codex`/`claude-code` hook 字段；`core/spec/validate_native_single_file_test.go` 验证两家 provider、两种包格式及非法字段，`go test -count=1 ./core/spec` 通过 |

## 已完成范围

| 条目 | 提交 | 结果 |
|---|---|---|
| T10 | `b965681` | 根 module、独立 `agenthooks` module、workspace 和双 module 测试入口完成 |
| T11 | `b731f54` | 目录包、单文件包、frontmatter、`from:` 文件和 glob 解析完成 |
| T12 | `da1da7f` | 默认值、规则覆盖、`reads` 并集和停用规则合并完成 |
| T13 | `a91c497` | schema、路径、模板变量、条件、lint 和诊断完成 |
| 规范拆分 | `632d5fa` | 规范入口、包结构、规则、运行时、编译产物五篇文档完成 |
| T14 | `ff32b99` | `core/template`、`core/when`、spec 校验/lint 复用模板解析器完成 |
| T15 | 本次提交 | `core/warn.FirstSentence` 与中英文、多行、空行、无标点、CRLF 和动态模板渲染用例完成；A21/B1.6 已标记 `[x]` |

T14 的边界已经明确：`FieldReader` 是 core 与 adapter 之间的接口，实际 payload 字段提取、effort 来源和 runtime engine 接入留给后续 adapter/engine 任务。这些尚未完成的部分不应伪装成 M1a 或 M1b 通过。

## 验证结果

以下命令在清理 `CODEX_HOME`、`CODEX_SANDBOX` 并使用临时 Go 缓存的环境中通过：

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  GOPROXY=off GOSUMDB=off \
  go test -count=1 ./core/template ./core/when ./core/spec

GOPROXY=off GOSUMDB=off make test-go
GOPROXY=off GOSUMDB=off make vet-go

env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  GOPROXY=off GOSUMDB=off \
  go mod tidy -diff

git diff --check
```

T14 的定向测试覆盖模板默认值和转义、静态/动态分类、引号内分隔符、条件 AND/OR、模型 glob、effort 比较、agent 范围、缺失字段、provider 永不匹配、matcher 复用和 JSON 往返。原完整测试没有覆盖下面列出的包输入边界；本次新增回归测试已覆盖 R01–R08。修复后的定向测试、`make test-go`、`make vet-go`、`go mod tidy -diff`、gofmt 和 diff 检查均通过。

## 原回顾发现的 8 个边界问题

优先级均为 P2：会让无效包通过 validate、阻止有效规则通过校验，或让错误延迟到后续编译/运行阶段。

| 编号 | 复现输入 | 修复前观察 | 修正方向 |
|---|---|---|---|
| R01 | `STYLE.yaml` 的有效内容后追加第二个 YAML 文档，或追加一个未闭合的 YAML 文档 | `Load` 成功，只有第一份文档进入包；`Validate` 无诊断，尾部错误被忽略 | 使用 YAML decoder 检查完整输入；要求单一 mapping，并拒绝剩余文档或非法尾部 |
| R02 | `from: rules/test.txt`，或 `from: rules/*.txt` | `Load` 成功，`.txt` 被当成规则，默认 ID 为 `test.txt` | 对显式 `from:` 和 glob 结果统一强制 `rules/**/*.md`，并在 validate 阶段再次检查来源 |
| R03 | `priority: .nan`、`budget.per_rule: .inf` | validate 无诊断；合并结果无法 `json.Marshal`，分别报 `NaN` 和 `+Inf` | 所有数字字段统一检查 `finite`，覆盖包、defaults、规则、budget 和编译输出所用字段 |
| R04 | `vars: {1: value}` 或 `reads: {1: true}` | 非字符串键在 `asStringMap` 转换时被静默丢弃，validate 无诊断 | map 转换返回键错误；嵌套 YAML mapping 遇到非字符串键必须报告 schema 错误 |
| R05 | `target: tool`、`names: Edit`，条件使用 `tool.added` | 当前只看 canonical `tool` 字段，报 `condition-field-wrong-event`；支持的原生 Edit 选择器无法表达 | 让 native-name selector 参与能力/事件判断，由 adapter registry 提供事实；core 不硬编码具体 harness 行为 |
| R06 | `target: skill`、`at: before`，条件使用 `tool.output` | skill 校验固定加入多个事件，错误条件不报诊断 | 按 action/`at` 推导隐式 tool 事件，同时单独处理 `skill.explicit`；`tool.output` 只能在 post 事件可用 |
| R07 | `when.model: '['` | validate 无诊断，但 `core/when.Normalize`/Compile 后续报 glob syntax error | validate 与运行时复用同一 glob 语法检查，避免 validate 成功后 compile 失败 |
| R08 | 目录包的 native 规则可接受 `codex: {}`; 等价单文件包报 `unknown package field "codex"` | 单文件 schema 把 native 字段当未知包字段 | 单文件包按目录包相同的 native rule schema 拆分包字段和规则字段，并为两种格式共用校验 |

源码定位（基于 `ff32b99`）：

| 编号 | 主要位置 |
|---|---|
| R01 | [frontmatter.go](../core/spec/frontmatter.go#L14)：只解码第一份 YAML 文档 |
| R02 | [load.go](../core/spec/load.go#L71)：展开来源后未检查 Markdown 后缀 |
| R03 | [validate_values.go](../core/spec/validate_values.go#L38)、[validate_package.go](../core/spec/validate_package.go#L205)：只检查数字类型/正值 |
| R04 | [values.go](../core/spec/values.go#L44)：转换 mapping 时跳过非字符串键 |
| R05 | [validate_conditions.go](../core/spec/validate_conditions.go#L211)：忽略 `names` 选择器 |
| R06 | [validate_conditions.go](../core/spec/validate_conditions.go#L191)：skill 同时加入 pre/post 事件 |
| R07 | [validate_conditions.go](../core/spec/validate_conditions.go#L23)：model 只检查类型 |
| R08 | [validate_package.go](../core/spec/validate_package.go#L32) 与 [validate_rules.go](../core/spec/validate_rules.go#L59)：native 白名单不一致 |

探针的可重复输入和完整 JSON 结果在 [M1-2026-10-13-probe.go.txt](review/M1-2026-10-13-probe.go.txt) 和 [M1-2026-10-13-results.json](review/M1-2026-10-13-results.json)。从仓库根目录运行：

```sh
cp docs/review/M1-2026-10-13-probe.go.txt /tmp/style-cli-m1-review-probe.go
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  GOPROXY=off GOSUMDB=off \
  go run /tmp/style-cli-m1-review-probe.go
```

修复后的 13 个场景观察结果见 [M1-2026-10-13-fixed-results.json](review/M1-2026-10-13-fixed-results.json)，原结果保留用于对照。R01/R02 已在 Load 阶段拒绝；R03/R04/R06/R07 输出校验错误；R05/R08 有效输入通过。探针会刻意继续合并无效包，因此 R03 仍记录 JSON 编码错误；使用者必须先确认 Validate 没有错误，合法有限数字的 JSON 编码另有回归测试。

探针本身不会因不合规行为失败；R01–R08 已由新增回归测试固定期望，修复前失败、修复后通过。

## 维护性和文档结论

- `core/spec` 已按 loader、frontmatter、文件安全、merge、package/rule/condition/template/reads/lint/value 校验拆分。`validate_rules.go` 在 2026-10-13 基线约 299 行，当前新增 target selector 检查；后续若继续增长，可再拆 schema 与语义，但现在不需要重写。
- 规范拆分保留了章节编号和正文顺序，最大单文件实际为 242 行；进度日志中曾写 243 行，是切片时保留的 EOF 空行造成的计数差异，本次记录以实际文件计数为准。
- 规范入口和 CLI 设计仍有“尚未实现”的历史状态文字。`STYLE_DOCS/STYLE-cli-design.md` 被忽略、未跟踪，新 clone 中规范到它的链接不可用；附录引用的 `style-gen/cross-harness.md`、`thinking-effort.md` 和 `style-review/solution-2026-10-12.md` 仍缺失。T24 已补上示例 `example/terse-reviewer/STYLE.yaml` 并修正其跟踪规范链接。由负责 plan/docs 的对话统一处理剩余状态文字和引用。
- `agenthooks/` 在 T10–T14 没有源代码改动；根 module 通过本地 `replace` 使用它，符合“hook 直接在 agenthooks、CLI 另起 module”的约定。

## 当前进展

T30–T34 已完成双 module 全量测试和运行时/状态 race 检查；A4–A9、A15、A17、A19、A20 已有离线证据，A10/A11 因实际 adapter 映射仍未完成而保持部分状态。A1、A2、A12–A14、A18、A22、adapter conformance 及 Codex/Claude 真实环境尚未验收。提交和验证结果见下方 T30–T34 记录及 `docs/PROGRESS.md`。

## Go 与 CLI 设计研究跟进（2026-10-13）

- [x] 已完成当前 module、STYLE core、CLI 基线和 `agenthooks` 边界的维护性评估。
- [x] 已形成模块化单体、application 用例层、薄 CLI、窄 adapter 接口、稳定输出契约和双 module 发布策略。
- [x] 已写入完整设计报告：[STYLE-GO-CLI-DESIGN-2026-10-13.md](STYLE-GO-CLI-DESIGN-2026-10-13.md)，覆盖依赖方向、CLI 生命周期、错误/输出、文件事务、测试和 P0–P3 实施顺序。
- [x] 本项只完成研究和文档登记，没有进入 T15，也没有修改 `agenthooks` 源码；后续按用户指令推进。

## T15 `warn` 首句提取跟进（2026-10-13）

- [x] 实现 `core/warn.FirstSentence`，严格按规则处理渲染正文的首个非空行和中英文句末标点。
- [x] `core/warn/sentence_test.go` 覆盖开头空行、多行、ASCII/中文标点、无句末标点、CRLF、空内容，以及先用 `core/template` 渲染动态正文再提取。
- [x] `docs/STYLE-M1-TASKS.md`、`docs/STYLE-M1-TESTS.md` 和 `docs/PROGRESS.md` 已同步；A21、B1.6 以定向/全量测试证据标记完成。
- 范围：此条只提供可复用的提取函数，不提前标记 compile、runtime engine 或完整 A1–A22 通过。

## T20–T24 完成后回顾（2026-10-14）

结论：规划、token analysis、adapter contracts、index compiler 和 schema/golden 的职责分界整体合理；`core` 不引入具体 provider，`agenthooks/` 不需要复制实现。两个 P2 边界问题已修复：事件级 `target: event` 挂到 tool 事件时原先编译出空 `match`，与 schema 的 selector 要求冲突；空/空白 tool、native-name、skill selector 及 malformed MCP selector 原先能通过 Validate。修复没有改变 hook 协议或 adapter 事实。

| 编号 | 状态 | 复现与影响 | 修复和证据 |
|---|---|---|---|
| RV-T2X-01 | [x] | `target: event` + `event: tool.pre` 是规范允许的全局规则，但 compiler 输出 `{}` match，正式 schema 拒绝该 index | 编译条目保留原 `target`；event-wide tool 条目不输出 match，schema 区分全局 event 与 routed selectors。`core/compile/golden_test.go:TestEventTargetAtToolPointCompilesWithoutMatch` 直接编译并验证 schema；golden 已更新 |
| RV-T2X-02 | [x] | `tool: ''`、空 `names`/`skill` 列表、空白 selector、没有或多于一个 `/` 的 MCP selector 原先通过 Validate，可能匹配不到事件或产生非法 index | Validate 和 Compile 检查非空 selector、MCP 单分隔符/非空组件；T24 schema 现在要求 match 与 target 类型一致、列表非空，并拒绝 MCP 组件里的 `/` 和首尾空白。`core/spec/validate_edge_test.go`、`core/compile/golden_test.go:TestNormalizedMatchRejectsInvalidSelectors` 及 schema 负向用例覆盖坏例和合法 MCP wildcard；两份规范已同步约束 |

验证命令和结果记录在 `docs/PROGRESS.md` 的 `RV-T2X-01/RV-T2X-02` 条目。T20–T24 commits 为 `19a3d89`、`8e144cc`、`372f923`、`9c8f5ad`、`bf0572f`；回顾修复及文档提交随后独立提交。双 module 全量测试、vet 和 diff 检查通过；本次未修改 module files，独立 tidy 检查因临时缓存缺少仅供依赖自身测试使用的模块、且网络被关闭而未通过（详见进度日志）。未修改 `agenthooks/`。

维护结论：继续采用当前模块化单体与窄 adapter ports，不需要为 T20–T24 重写架构。该轮回顾结束时，index 的 `target` 消费仍列为 T32 后续工作；T30–T34 已在下一节完成并复核，但这不代表 CLI 或真实 harness 验收完成。

## T30–T34 完成后回顾（2026-10-14）

结论：state、去重、运行时 engine 与 `agenthooks` codec/runner 的职责仍然分明。`core/engine` 消费 provider-neutral 的已解码输入与能力事实；`internal/runtime` 负责生命周期拼接；stdin、hook 事件解码、输出 codec 和 Claude fail-closed 机制落在 `agenthooks/`。本轮没有发现需要改动该分层的 P0/P1 问题。T33 对 `agenthooks/` 的改动分别补充了 Claude prompt block 的 `suppressOriginalPrompt` 和 Claude tool-pre fail-closed 退出码，均有 codec/runner/runtime 用例。

| 条目 | 状态 | 完成结果与证据 |
|---|---|---|
| T30 | [x] | session/scope 生命周期、来源、gen/epoch/prompt_count、原子私有状态文件和过期清理；`core/state/state_test.go` 覆盖 startup/resume/clear/compact、权限、损坏状态及清理 |
| T31 | [x] | turn/session/every、正文变化、compact epoch、block/subagent.start 例外和 `O_EXCL` 预留释放；`core/state/state_test.go` 与 `core/engine/engine_test.go:TestTwentyProcessesEmitOneRepeatSessionEntry` 覆盖 |
| T32 | [x] | schema 校验、matcher 预编译、Enrich、条件和模板、block 优先、warn/inject merge、上限裁剪、stats/log；`core/engine/`、`internal/runtime/` 有定向测试 |
| T33 | [x] | 输出 whitelist；缺失/损坏 index、state 失败默认空输出；Claude fail-closed 的拒绝和非零退出码；日志/stat 不带输入正文。证据：`internal/runtime/runtime_test.go`、`agenthooks/runner_test.go` |
| T34 | [x] | startup 与 prompt 双挂载，model/effort 变化后的动态渲染和按正文去重，compact 选择性重注入；`internal/runtime/runtime_test.go:TestRuntimeReevaluatesDynamicSystemWhenModelChanges` |

可复现验证：双 module 根测试和 `agenthooks` 测试均以 `-count=1` 通过；state/engine/runtime/schema 的 race 测试通过；双 module vet 通过。index-only 200 样本 p95 为 128.083µs，300 次 benchmark 为 33,567 ns/op。完整命令及运行结果保存在 `docs/PROGRESS.md` 对应条目。

边界：本轮 runtime 集成使用测试 `RuntimeEnricher` 验证 provider-neutral engine；Codex/Claude 原始 effort 映射、native model-switch 事件完整解码、子 agent context 以及实际订阅面仍由 T41/T42/T44/T45 落地。故 A10 与 A11 标为 `[~]`，M1a/M1b 不提前通过。继续现有架构并推进 T40，不需要重写 T30–T34。

## T40 完成后回顾（2026-10-14）

结论：安装层仍保持 `agenthooks/` 负责 provider 配置和协议事实、STYLE 通过 `Manifest.Owner` 声明归属的边界；不需要复制 hook 行为或重写安装架构。F1–F4 已完成离线实现和回归验证，真实 Codex 插件启用、信任和 shell 触发仍属于 M1b 环境验收。

| 条目 | 状态 | 完成结果与证据 |
|---|---|---|
| F1 | [x] | Codex canonical shell selector 现在编译为 `Bash`；`agenthooks/matcher_test.go` 固定编译和匹配行为；`agenthooks/e2e/codex_test.go:TestCodexCanonicalShellMatcher` 覆盖真实路线，默认跳过。 |
| F2 | [x] | 新增 `install.Uninstall`，JSON 按 `--owner` 删除 handler，TOML 按 owner marker 删除区域，独立生成文件只在内容仍匹配时删除；覆盖同组 sibling handler、外部配置、Kimi/Codex TOML 和插件文件清理。 |
| F3 | [x] | 所有生成命令加入 `--owner=<owner>`；STYLE 使用 `Owner: "style"`，默认消费者保持 `agenthooks`；合并/卸载不再按命令中是否包含 `agenthooks` 粗略识别。 |
| F4 | [x] | Codex plugin 路线生成 `.codex-plugin/plugin.json`、`hooks/hooks.json`、POSIX `bin/style`（0755）和 `CODEX_HOME/config.toml`；插件 trust key 使用 `<plugin_id>:hooks/hooks.json:<event>:<group>:<handler>`，hash 与渲染命令/timeout 一致。 |

离线验证：

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  GOPROXY=off GOSUMDB=off \
  go test -count=1 -C agenthooks ./install .

env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  GOPROXY=off GOSUMDB=off \
  go test -race -count=1 -C agenthooks ./install .

GOPROXY=off GOSUMDB=off make test-go
GOPROXY=off GOSUMDB=off make vet-go
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off \
  go test -count=1 -C agenthooks -run '^$' -tags e2e ./e2e
```

已知边界：离线测试不能证明当前机器上的 Codex 版本会实际加载插件、要求一次信任并发送 `Bash` hook；该验证保留在 `M1b/CDEX-01`、`CDEX-10` 和真实 e2e。完整 CLI 生命周期的字节级恢复、路线切换和 trust/sync 仍由后续 T52–T55/A12–A14 完成。

## T41 完成后回顾（2026-10-15）

结论：F5–F6 已按现有分层完成离线实现；没有把 Codex 配置解析或 Claude effort 来源塞进 `core`，也没有另造一套 session store。`agenthooks` 只暴露受限的 Codex 启动事实并保留 Claude 原始 `effort.level`，`internal/adapter/effortmap` 和两个 provider 包负责映射与来源优先级，`core/state` 负责跨 hook 的会话持久化。

| 条目 | 状态 | 完成结果与证据 |
|---|---|---|
| F5 | [x] | `SessionInfo` 新增 `Effort`、`EffortRaw`、`EffortApprox`、`HarnessVersion`；Claude payload 的 `effort.level` 保留为 raw；`effortmap` 嵌入式 YAML 表支持七档/别名、未知 raw 保留和近似标记；Codex 按 `-c`、profile、受信项目、用户配置解析，Claude 按事件、`CLAUDE_EFFORT`、session 优先级解析。证据：`agenthooks/codec_test.go`、`agenthooks/detect_test.go`、`internal/adapter/effortmap/*_test.go`、`internal/adapter/codex/effort_test.go`、`internal/adapter/claude/effort_test.go`。 |
| F6 | [x] | session JSON 新增 tri-state `effort_approx` 和 `effort_resolved`；runtime 把 session effort/raw/approx/version 投影到 engine 变量；状态仍使用原子写、私有权限和既有 session/scope 路径。证据：`core/state/state_test.go`、`internal/runtime/runtime.go`。 |

已知边界：T41 提供 provider-neutral 的解析和持久化端口；将实际事件 Enrich、harness 版本 Detect、Codex/Claude 订阅面接入运行时分别留在 T44/T45。未知 effort 不会被降级成某个档位，`EffortApprox` 保持 nil；这保证条件缺失时不误触发。

离线验证：

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off \
  go test ./internal/adapter/... ./core/state ./core/engine ./internal/runtime ./agenthooks
GOPROXY=off GOSUMDB=off make test-go
GOPROXY=off GOSUMDB=off make vet-go
git diff --check
```

以上命令均通过；真实 Claude/Codex effort 来源和版本值仍需 M1b 环境记录，不能由离线 fixture 代替。

## T42 完成后回顾（2026-10-15）

结论：F7–F9 按现有边界落地，无需改变 `agenthooks/` 与 STYLE adapter/core 的分工。hook event、原始 payload 和 provider codec 保留在 `agenthooks/`；Codex 显式 skill 只在 `internal/adapter/codex` 提供轻量解析；具体规则评估、state/scope 投影和 engine Enrich 仍由 T44 adapter 完成。本轮没有发现需要架构调整的 P0/P1。

| 条目 | 状态 | 完成结果与证据 |
|---|---|---|
| F7 | [x] | 新增 `SubagentStartDecision.WithContext` 与 `OnSubagentStartDecision`，保留既有观察型 `OnSubagentStart` API；Codex 能力矩阵和 HSO 编码仅开放 `additionalContext`。Runner 测试从 native payload 验证 agent id/type、handler 决策、精确输出及旧观察回调仍运行。Claude Code 不宣称支持该能力。 |
| F8 | [x] | `SkillActivationOf` shell 解析增加 `head`/`tail`/`bat` 与安全的 `cd … && cat SKILL.md` 形式，并保留绝对路径可执行文件兼容；Codex `ExplicitSkills` 依规范正则按首次出现顺序提取并去重，数字可作为首字符。云端 `skills.read` 仍待 U6，显式 skill 评估接线在 T44。 |
| F9 | [x] | Claude codec/native views 覆盖 prompt id→turn、effort raw、MCP server name/source 与嵌套未知字段、fork source、UserPromptExpansion、PostModelSwitch/to_model；prompt block 的 `suppressOriginalPrompt` 放入 `hookSpecificOutput`。Codec、typed view、Runner 和 runtime golden 有覆盖。 |

复核中发现并在统一测试前修正了四项实现/验证不一致：旧的 suppress 输出 golden 仍使用顶层字段；F7 注释/spec 将 Claude 错列为支持方；skill 名首字符测试与规范允许数字的正则冲突；F8 解析器改动会漏掉 `/bin/cat` 等绝对命令。以上均已补充回归或同步文档。

验证：`GOPROXY=off GOSUMDB=off make test-go` 与 `GOPROXY=off GOSUMDB=off make vet-go` 通过；Makefile 为两个 module 分别清理 `CODEX_HOME`、`CODEX_SANDBOX`，并使用 `/tmp/style-cli-gopath`、`/tmp/style-cli-gomodcache`、`/tmp/style-cli-gocache`。gofmt 和 `git diff --check` 通过。

边界：离线测试证明 Codex payload 经 Runner 能编码 additionalContext，但不能证明实际 Codex 会把内容投递给子 agent；该结论仍由 U7/M1b 验证。`skills.read` 尚未进入固定订阅面，显式 `$name` 到 runtime evaluation point 的接线与 scope.agent_id projection 留在 T44；因此 A11 保持 `[~]`，M1a/M1b 不提前标记通过。

## RV-T41-01 模块依赖修正（2026-10-15）

回顾 T41 后发现根 module 的 `internal/adapter/codex/effort.go` 直接导入 `github.com/pelletier/go-toml/v2`，但 `go.mod` 仍将该依赖标为 indirect。已将其移入直接依赖；`go mod tidy -diff` 与 nested `agenthooks` 的 tidy diff 均通过。该修正不改变运行时行为，只让模块声明与源码导入保持一致。

## T43 完成后回顾（2026-10-15）

结论：F10 已在既有 `agenthooks/install` renderer 内完成，没有为 Claude 另造安装路径。Claude handler 以 exec 形式分别写出 `command` 与 `args`，保留传入 argv 和空格；owner 识别同时兼容旧 shell command 与新的 args 数组，保证升级后 merge/uninstall 可识别已有条目。卸载绕过新安装的版本门槛，避免 Claude 降级或检测不到版本时遗留已安装 hook。未发现需要改变分层的架构问题。

fail-closed 的范围收窄为 `Manifest.Fail == FailClosed`、`Blocking` 且 `KindToolPre` 三项同时成立：只有这条 hook 增加 `--fail-closed` 和 `onFailure: "block"`。`Target.HarnessVersion` 缺失、不可解析或低于 2.1.295 时拒绝生成该配置；fail-open 和 telemetry 不要求版本。timeout 继续复用既有秒单位转换和默认值。

回归覆盖：`TestRenderClaudePlugin` 验证 exact command/args、timeout、onFailure 与 telemetry 例外；`TestClaudeExecArgsAndFailClosedVersionGate` 覆盖带空格的 executable/argv、最低版本与解析边界、fail-open 和非阻断 hook；`TestRenderClaudeTimeoutRoundingAndDefault` 覆盖超时向上取整及 SessionStart 默认值；merge/uninstall 测试覆盖 owner 位于 args 时替换/删除、保留外部 handler，及缺失/降级版本下仍能卸载。

验证：隔离环境下安装包定向测试、`GOPROXY=off GOSUMDB=off make test-go`、`make vet-go`、gofmt、两 module 的 `go mod tidy -diff` 和 `git diff --check` 通过。T43/B1.20 的离线 renderer 与 owner 生命周期证据完整；Claude 的真实版本检测将由 T45 接入，当前 CLI 尚未实现把 `--fail-closed` 接至 `internal/runtime.Load` 的命令解析，该 argv→runtime 路径由后续 CLI 工作覆盖。真实 Claude 对 `command`/`args` 与 `onFailure` 的接受仍由 M1b 实机验证，不提前标记通过。

## T44 Codex adapter 完成后回顾（2026-10-15）

结论：T44 沿用 T22 定义的 adapter port，并把 Codex 专属事实、路线和 payload enrichment 放在 `internal/adapter/codex`；hook protocol、provider codec、manifest renderer 和 owner merge/uninstall 继续由 `agenthooks/` 承担。没有发现需要改变分层的 P0/P1 问题。T30–T34（T3X）此前已完成，本轮在 Codex adapter 接入后再次执行双 module 全量测试，并额外做了相关包 race 检查。

| 检查项 | 状态 | 结论与证据 |
|---|---|---|
| RV-T44-01 固定订阅面与事实 | [x] | `Surface` 固定五类 hook；工具 matcher 只订阅 Codex 可观察的 Bash、apply_patch 与 MCP。native names 与 canonical tool selector 都参与 blind-spot 判断；用户已占用 `developer_instructions` 时不报告静态槽位；隐式 skill 的 `skill.name` 数据源在 tool hook 上有明确声明。证据：`internal/adapter/codex/route_test.go:TestCodexSurfaceAndManifestStayFixed`、`TestCodexFactsDetectUnavailableStaticSlotAndUnobservableTools`。 |
| RV-T44-02 路线变更与静态区 | [x] | Plugin/user route 通过 `PlanFiles` 生成含 Before 快照的 changeset，再用共享 `ApplyFiles` 检查 stale plan；静态 instructions 与 hook/trust TOML 同路径合并。静态-only 更新/移除不依赖 hook 文件是否变化，嵌套目录按最具体根路径归属，外部 TOML 保持原字节；未安装或尚未确认的 plugin route 不报告已信任。证据：`internal/adapter/codex/route_test.go` 的静态-only、残留清理、幂等、stale、nested-root 和 trust 状态用例；底层安装计划见 `agenthooks/install/install_test.go`。 |
| RV-T44-03 runtime Enrich | [x] | Codex session/model/effort 一次性状态解析、prompt 和 `$skill` 显式点、agenthooks 隐式 skill、MCP identity、patch added lines、tool output、嵌套输入条件和模板值均有测试。Codex `SubagentStart` callback 将 engine context 送入专用 decision codec，并写入 child scope state。证据：`internal/adapter/codex/runtime_test.go`、`internal/runtime/runtime_test.go:TestCodexSubagentStartRuntimeDeliversContextForAgentType`。 |

复核过程中补齐了七处边界：静态内容单独变化或卸载时仍要更新 `config.toml`；plugin 与 CODEX_HOME 路径嵌套时必须选择最窄根目录；未订阅的 native tool 名必须被识别为 blind spot；隐式 skill 字段必须在 planning facts 中声明；复合 JSON 输入路径也要同时可用于条件和模板；SubagentStart 不能停留在解析层，必须接入 runtime decision handler；未确认的 plugin route 不能伪报 trusted。各处均由上述可重复测试覆盖。

验证：`GOPROXY=off GOSUMDB=off make test-go` 和 `make vet-go` 通过，覆盖根 module 与 `agenthooks` nested module；隔离 Go 缓存下 `go test -race -count=1 ./internal/adapter/codex ./internal/runtime ./agenthooks/install` 通过；gofmt、`git diff --check` 通过。T44、B1.21 和本节 review 状态已同步。

边界：离线 Runner/fixture 验证不等同真实 Codex 子 agent context 投递（U7/M1b）。Codex `skills.read` 工具未纳入固定订阅面；A10 仍等 Claude adapter/真实 hook 输入，A11 仍等真实 Codex 验收。T45、CLI lifecycle/A1–A2/A12–A14/A18/A22 和完整 adapter conformance 未完成，M1a/M1b 不标记通过。

## T45 Claude adapter 完成后回顾（2026-10-15）

结论：T45 继续使用 `internal/adapter` 的窄接口和 `agenthooks/install` 的共享 owner 生命周期，没有为 Claude 复制 hook codec 或配置合并。Claude adapter 负责动态订阅面、原生字段到 engine point 的映射、用户级路线和 `CLAUDE.md` 标记块；workspace trust 只在 `TrustInfo` 中声明为外部信任要求，`Status` 不从配置文件存在性推断已信任。

| 检查项 | 状态 | 结论与证据 |
|---|---|---|
| RV-T45-01 surface/facts | [x] | 相同 native event+matcher 合并、点位去重和稳定排序；PostModelSwitch 作为原生观察事件保留；能力矩阵、数据源、静态位置和 native matcher blind-spot 通过 `internal/adapter/claude/adapter_test.go`。 |
| RV-T45-02 route/static | [x] | user-level `settings.json` 复用 `PlanFiles`/`PlanUninstallFiles`/`ApplyFiles`，保留外部 handler；`CLAUDE.md` 以 `style:<package>` 标记块增删，malformed marker 拒绝，静态-only 变更和幂等由 `route_test.go` 覆盖。 |
| RV-T45-03 runtime/output | [x] | event→`CLAUDE_EFFORT`→session effort、后续事件覆盖、prompt/explicit skill、Edit/Write added text、MCP、implicit skill、tool output 和 agent scope 有 fixture；`UserPromptExpansion` 已接入 runtime，skill block 不再输出只适用于普通 prompt 的 `suppressOriginalPrompt`。证据：`internal/adapter/claude/runtime_test.go`、`internal/runtime/runtime_test.go:TestClaudePromptExpansionRuntimeUsesAdapterAndRunnerHandler`、`core/engine/process.go`。 |

本轮复核修正了一处输出边界：原先 engine 将所有 prompt 类点位都设置 Claude 的 `suppressOriginalPrompt`，导致 `skill.explicit` 经过 UserPromptExpansion codec 时会产生不支持的字段；现在只对 `prompt.submitted` 设置该字段。共享 renderer 另补充了 direct `NativeEvent`、原生正则 matcher 和 `UserPromptExpansion` 映射。

验证：根 module `go test -count=1 ./...`、nested `agenthooks` `go test -count=1 ./...`、`GOPROXY=off GOSUMDB=off make vet-go`、定向 `go test -race -count=1 ./internal/adapter/claude ./internal/runtime ./agenthooks/install`、gofmt 与 `git diff --check` 均通过。真实 Claude workspace trust、compact、model switch、性能和 CLI lifecycle 仍属于 C1–C9/T50–T55，M1a/M1b 不提前标记完成。

## T50 `style init` 完成后回顾（2026-10-15）

结论：`style init` 保持在 `internal/app` 编排、`cmd/style` 薄入口的边界内，没有把包生成逻辑放入命令解析，也没有复制 `agenthooks` 行为。目录包和单文件包共用名称校验与安全写入路径；当前空目录初始化只接受空目录，已有用户文件不会被覆盖。

| 检查项 | 状态 | 结论与证据 |
|---|---|---|
| RV-T50-01 骨架合法性 | [x] | 目录包和单文件包生成后都通过 `spec.Load`、`spec.Validate`、`spec.Merge`；证据：`internal/app/init_test.go`。 |
| RV-T50-02 冲突和回滚 | [x] | 已存在目标、非空当前目录和非法名称拒绝；写入失败删除本次创建的文件/目录并保留已有字节；证据：`internal/app/init.go`、`init_test.go`。 |
| RV-T50-03 CLI 边界 | [x] | `cmd/style` 只调用 `internal/app.Run`，`style init` 的 help、成功/失败退出码和成功文本稳定；help 不产生文件副作用。证据：`internal/app/entry.go`、`init_command.go`、`init_test.go`。 |

验证：隔离 Go 缓存下 T50 定向测试、双 module 全量测试、`make vet-go`、gofmt 和 `git diff --check` 均通过。T51 的 validate/plan、安装生命周期和离线验收仍未提前标记完成。

## T51 `style validate` / `style plan` 完成后回顾（2026-10-15）

结论：CLI 入口继续保持薄层，validate/plan 的编排集中在 `internal/app`，复用 `core/spec`、`core/plan`、`core/tokens` 和 provider adapter facts。Codex/Claude token bucket 由 provider layout 映射；没有把 hook codec 或安装行为复制到 CLI。T51 完成离线 validate/plan 与 A2 golden，不代表 M1 lifecycle 已完成。

| 检查项 | 状态 | 结论与证据 |
|---|---|---|
| RV-T51-01 稳定诊断与命令契约 | [x] | 文本/JSON 诊断、参数错误、help 和 exit code 有命令级测试；包级错误码独立于人类消息。证据：`internal/app/command_test.go`。 |
| RV-T51-02 多 provider 规划与成本 | [x] | 显式或 auto harness 选择顺序确定；报告含 route、输入上限、package budget、逐规则/原生事件估算与 session 假设；token bucket 测试覆盖 MCP 聚合及 canonical/native tool 对应。证据：`internal/app/plan.go`、`plan_layout_test.go`。 |
| RV-T51-03 示例 golden | [x] | Codex 与 Claude Code 显式 harness 的完整 plan stdout 与版本控制内 golden 逐字节比较；golden 每次由当前 CLI 生成并由测试固定。证据：`internal/app/plan_golden_test.go`、`testdata/golden/plan-{codex,claude-code}.txt`。 |

验证：隔离 Go 缓存下 `go test -count=1 ./internal/app ./cmd/style`、根 module 与 nested `agenthooks` 全量测试、`make vet-go`、gofmt 和 `git diff --check` 通过；CLI 示例 validate 与两个显式 harness plan 命令可执行。T52–T55 的 install/uninstall/list/doctor/sync/trust 尚未开始，A1/A12/A13/A18/A22 与 adapter conformance 未完成，M1a/M1b 不标记通过。

## T52 `style install` 完成后回顾（2026-10-15）

结论：安装继续由 `internal/app` 编排，provider 路线只通过 `internal/adapter.Route` 生成 changeset；实际 hook 合并、owner 隔离和文件写入仍复用 `agenthooks/install`。包、锁文件、编译 index、settings 和 provider 配置在同一组可回滚变更中更新，确认和能力检查先于任何写入。

| 检查项 | 状态 | 结论与证据 |
|---|---|---|
| RV-T52-01 安装前确认和输入 | [x] | 非交互安装必须带 `--yes`；settings 无默认值时要求 `--set`，未知 setting 和不满足 `requires.runtime` 均在写入前拒绝。证据：`internal/app/install_test.go`。 |
| RV-T52-02 冲突和 replace | [x] | 同版本同内容重复安装幂等；已锁定包、外部存储和 replace 未获 `--allow-replace` 均拒绝。证据：`internal/app/install.go`、`install_test.go`。 |
| RV-T52-03 事务和 owner 边界 | [x] | package swap 与 provider changeset 均先校验 Before 快照；agenthooks 多文件写入在取消/后续失败时逆序恢复，已有文件权限保留，provider 路线通过现有 owner-aware renderer。证据：`agenthooks/install/plan_files.go`、`install_test.go`。 |

验证：隔离环境下 `go test -count=1 ./internal/app`、`go test -count=1 -C agenthooks ./install`、根/嵌套 module `make test-go`、`make vet-go`、gofmt 和 `git diff --check` 通过；示例包的隔离 `style install --yes --json` 返回 0。T53–T55 仍未完成，A1/A12/A13/A18 和 adapter conformance 继续保持未完成，M1a/M1b 不提前标记通过。

## T53 `style uninstall` 完成后回顾（2026-10-15）

结论：卸载逻辑由 `internal/app` 根据 lock 重建剩余包的编译结果，再委托 `adapter.Route` 生成路线变化，hook owner 合并/删除仍走 `agenthooks/install`。最后一个包卸载时一并清掉 STYLE lock、index、本地包和设置；link 包源文件保留。lock 新增保存 provider `config_dir`，使卸载与路线切换使用安装时配置根路径，避免依赖当前进程环境。

| 检查项 | 状态 | 结果与证据 |
|---|---|---|
| RV-T53-01 单包/全部与确认 | [x] | `uninstall <name> --yes` 与 `uninstall --all --yes --json` 有命令级覆盖；未确认时不执行。证据：`internal/app/uninstall_test.go`。 |
| RV-T53-02 部分卸载和最后卸载 | [x] | 删除选中包后从剩余 lock 读取包，重新 plan/token/compile、更新 route/index/lock；移除最后一个包时清理 route/index/lock/settings 与本地存储。link 源不删除。证据：`TestUninstallRequiresConfirmationAndKeepsRemainingPackage`、`TestRunUninstallAllPreservesLinkedSource`。 |
| RV-T53-03 配置字节和事务 | [x] | TOML managed region 不再吞入分隔换行；无末尾换行的外部文件通过生成边界标记隔离，移除 owner 后恢复原始字节。package removal 先 rename 到可回滚备份，再应用 provider/state changeset。证据：`TestManagedTOMLRoundTripRestoresExternalBytes`、`TestUninstallRemovesOnlyStyleOwnerAndRestoresLastRoute`。 |

验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./internal/app -run 'TestUninstall|TestRunUninstall'`、同环境 `go -C agenthooks test -count=1 ./install`、双 module `make test-go`/`make vet-go`、gofmt 与 `git diff --check` 均通过。A13 的 install idempotence、uninstall 外部配置保留和最后路线清理现有离线证据；安装时切换 route 后无残留仍需独立回归，故保留 `[~]`。T54/T55、A1/A12/A13 补项/A18 和 adapter conformance 仍未完成，M1a/M1b 不提前标记通过。

## T54 检查命令完成确认（2026-10-15）

- [x] **RV-T54-01 list/adapters**：list 从 lock 枚举安装序号、原始来源、单文件与 link=dev，并比较实际存储字节；adapters 对全部注册 provider 和全部 route 调用 Detect/Available/Status，展示能力、数据源、静态槽位、上限、信任方式及未知外部信任。证据：`internal/app/inspect_test.go`。
- [x] **RV-T54-02 doctor 完整性与维护**：通过 engine/schema 校验 index，再从锁定包/settings 重编译比较内容，并检查生成路线和静态指令；检测登录 shell stdout、显示 effort 来源和可判断范围、提示 link/replace/fail_closed；清理 7 天未更新会话或 `--reset-state`，拒绝 symlink 状态根。证据：`internal/app/doctor_test.go`。
- [x] **RV-T54-03 历史成本与隐私**：runtime 只追加估算 token、正文 SHA256 和规范 point 等元数据；doctor 按最近 7 天实际记录聚合 top3，提示同正文多次输出。旧日志缺少成本时单独计数，不借当前 index 伪造历史；不保存正文或 shell 输出。证据：`core/state/stats_test.go`、`internal/runtime/runtime_test.go`、`TestDoctorCostsUseHistoricalMetadataAndSevenDayWindow`。

验证：隔离缓存且清理 CODEX 环境变量后，app 定向/全量测试、双 module `make test-go`/`make vet-go`、`go test -race -count=1 ./internal/app ./internal/runtime ./core/state ./core/engine`、CLI list/adapters JSON smoke、gofmt 与 `git diff --check` 通过。T55、A1/A12/A13 补项/A18、adapter conformance 与真实 M1b 继续未完成。

## A13/T52 路线生命周期补项完成确认（2026-10-15）

- [x] **RV-A13-01 重复安装和切换**：同包重复安装仍检查指定 route/harness、已有包哈希与 settings；同路线无变化时幂等，换路线不增加 package seq。新增包也可同时切换路线。Codex plugin/user 双向命令回归后，旧插件文件、hook owner 和 trust key 无残留。
- [x] **RV-A13-02 配置和策略边界**：hook 配置组合仍实现于 `agenthooks/install.PlanFilesAfter`，在内存中按先移除再安装生成一份 changeset，保留原 Before 快照并由 ApplyFiles 原子验证。外部 TOML 最后卸载后逐字节恢复，外部 JSON 键与其他 owner 保留；已有 fail_closed 不被后续安装关闭。
- [x] **RV-A13-03 路径和信任稳定性**：默认 Codex 插件目录跟随显式/环境 STYLE_HOME；卸载和 doctor 同样绑定该根。安装路线沿用 lock 配置根，新增目录在 symlink 祖先下的 trust key 在目录创建前后相同。未知 route/非法 fail-closed 参数不再因重复安装被忽略。

证据：`internal/app/install_routes_test.go`、`agenthooks/install/plan_after_test.go`。隔离 Go 环境下 app 定向/全量、双 module 全量测试/vet、`go test -race -count=1 ./internal/app ./internal/adapter/codex ./agenthooks/install` 和 diff 检查通过。A13 由 `[~]` 更新为 `[x]`，T55/A1/A12/A18、adapter conformance 与真实 M1b 仍未完成。

## T55 / A12 完成确认（2026-10-15）

- [x] **RV-T55-01 审批与相同字节**：`spec.LoadSnapshot` 复制输入并在 validate 时仅使用内存文件清单。Install/已安装包/sync/trust 共用该入口；确认回调再改源码或修改 preview 不改变待提交字节，后续 sync 仍能检出漂移。证据：`core/spec/snapshot_test.go`、`internal/app/sync_test.go:TestTrustCompilesExactlyTheConfirmedSnapshot`。
- [x] **RV-T55-02 差异和开发包**：审批基线包含原始文件和已编译静态正文/动态模板表示，以 0600 存于 STYLE_HOME/approved，lock 保存其摘要。trust 展示真实源码差异和编译差异；没有 `--yes` 时只读预览。link 首次 trust 后可自动同步；普通包同时改动时按名字逐一审批，其他包继续使用旧基线。单文件原始名称和安装名不同也能正确比较，seq 保留。证据：`sync_test.go`、`sync_command_test.go`、`trust_policy_test.go`。
- [x] **RV-T55-03 事务与策略**：审批的哈希/index/基线/settings/route/static 一起交给现有 agenthooks ApplyFiles。T003 全局预算、requires、schema、新 setting、replace 显式确认/唯一性先于写入；stale lock/route、损坏/缺失的已锚定基线、取消均有无部分写入回归。Claude 重建订阅面时保留外部配置和 fail_closed；sync 冷路径清理旧会话，uninstall 清理基线。证据：`trust_policy_test.go`、`sync_claude_test.go`、`sync_command_test.go`。

兼容已有无审批基线的锁文件：内容未变时 sync 可建立基线；已有改动时 trust 明示旧字节不可用，展示完整当前源码和编译表示，不伪造旧内容。已带摘要的基线缺失或被改写则报错。需要保留其他未审批改动时必须有可验证旧基线，否则要求先恢复对应锁定字节。

验证：清理 CODEX_HOME/CODEX_SANDBOX、使用 `/tmp/style-cli-{gopath,gomodcache,gocache}` 与 GOPROXY=off/GOSUMDB=off；app/spec 定向和全量、双 module `make test-go`/`make vet-go`、`go test -race -count=1 ./internal/app ./core/spec`、gofmt 和 `git diff --check` 通过。T55、A12、B1.28 确认 `[x]`；M1a 的 A1/A18/adapter conformance 与真实 M1b 仍待完成。

## A1 完成确认（2026-10-15）

- [x] **RV-A1-01 validate 负向矩阵**：新增 `testdata/invalid/` 中 27 个按错误码命名的 YAML recipe 和运行说明；测试核对完整清单，将 recipe 物化为真实包后调用 CLI，文本/JSON 均要求退出 1 和相应错误码。示例包要求退出 0；symlink、256 KiB + 1 和非法编码真实生成，不跳过。证据：`internal/app/validate_acceptance_test.go:TestA1ValidateExampleAndEveryInvalidFixture`。

验证：隔离 Go 缓存、清理 CODEX 环境变量后，A1 定向及 `go test -count=1 ./internal/app` 通过，gofmt / `git diff --check` 通过。A1 标记 `[x]`；A18、adapter conformance 和真实 M1b 仍未完成。

## A18 完成确认（2026-10-15）

- [x] **RV-A18-01 golden 与诊断**：9 组固定字符串覆盖空文本、ASCII、取整、数字、CJK、kana/Hangul、混合标点、emoji/组合字符与空白，匹配独立 golden 数字；T001–T007 各有版本控制内包 fixture。T001/T002/T004–T007 由实际 plan 流程触发；T003 使用真实 Codex 默认上限，两个 1002-token 包合计 2005 大于 2000 阈值。第二包安装必须退出 4，完整存储/provider 字节和 lock 序号保持原样。证据：`internal/tokenestimate/golden_test.go`、`internal/app/token_acceptance_test.go`、`testdata/tokens/`。

验证：清理 CODEX 环境、隔离 Go 缓存的 `go test -count=1 ./internal/app ./internal/tokenestimate ./core/tokens`、gofmt 与 diff 检查通过，A18 `[x]`。adapter conformance 与 M1b 仍未完成；M1b 配置按用户最新约定由用户手动执行。

## Adapter conformance / A4 / A11 完成确认（2026-10-15）

- [x] **RV-CONFORMANCE-01 原生输入和输出**：新增共用 suite，46 个 payload 覆盖两家已映射事件、未知事件、PostModelSwitch、patch/Edit、MCP、隐式/显式 skill 和子 agent。真实 agenthooks codec → adapter → state → engine → codec 的统一字段和 stdout 分别匹配 golden，每个输入另验证空 index 的原生 no-op 方言。白名单检查实际输出及存储 golden。
- [x] **RV-CONFORMANCE-02 effort 和 agent scope**：25 组独立 fixture 覆盖全部嵌入 effort exact/alias 及规范未知值，验证 raw/approx、持久状态和模板值；新增表行未补 fixture 会失败。两家默认 main、仅 agent_type 的主 agent、显式 any 和 session 去重均经真实输入验证；Codex subagent.start 连续两次输出。Claude observe-only 边界保留。
- [x] **RV-CONFORMANCE-03 PermissionRequest 评估点**：统一 suite 复现 Codex 把 PermissionRequest 投影为 permission.request，导致 tool.pre 条目无法匹配。修正为 tool.pre，并以两家的 deny_permission payload/golden 验证阻断。固定订阅面未改变，原生 codec 和 owner 生命周期仍由 agenthooks 负责。

证据：`internal/adapter/conformance/`、`testdata/payloads/README.md`。清理 CODEX 环境变量，临时 Go 缓存、GOPROXY=off/GOSUMDB=off 下，双 module `make test-go`/`make vet-go`、`go test -race -count=1 ./internal/adapter/conformance ./internal/adapter/codex`、gofmt 和 diff 检查通过。A4/A11 和 conformance 的解析/effort/往返/报告/白名单确认 `[x]`；安装一致性、进程性能和真实 M1b 未完成。

## 安装 conformance / A13 完成确认补项（2026-10-15）

- [x] **RV-CONFORMANCE-04 配置逐字节往返**：此前 A13 仅验证 JSON 键/其他 owner 保留，证据不足以证明规范要求的逐字节还原。新增统一 suite 真实复现 Codex user hooks.json 和 Claude settings.json 重排版、Claude 无末尾换行的 CLAUDE.md 多换行。现三条路线 × 三种末尾换行的 Plan→Apply→Status→Remove 证明原始文件字节与权限完整恢复、重复安装/卸载幂等、没有新增文件残留；CLI 双向换路线也增加最终 JSON 字节断言。
- [x] **RV-CONFORMANCE-05 恢复边界**：在现有 agenthooks/install 内保存 `.agenthooks/<owner>/<config-path>.restore.json`，原始字节加已应用 SHA256，0600 并与配置一起原子计划/提交/验证 Before。摘要相符才还原旧字节；外部编辑时保留当前修改，重装后也不回滚它；旧安装无记录沿用语义 owner 清理。覆盖空对象、空 hooks、CRLF、Unicode 转义、更新、stale/损坏记录、文件已删除和多 owner 隔离。卸载删除记录。
- [x] **RV-CONFORMANCE-06 Claude 静态分隔符**：无末尾换行时插入可识别生成注释；最后一个 block 移除时连同生成换行清理，多 owner 任意卸载顺序及重复合并均恢复原文。保留原有空白文件字节。

证据：`agenthooks/install/json_restore_test.go`、`internal/adapter/conformance/routes_test.go`、`internal/adapter/claude/static_roundtrip_test.go`、`internal/app/install_routes_test.go`。清理 CODEX 环境变量、临时 Go 缓存、GOPROXY=off/GOSUMDB=off；agenthooks install 与 app/adapter 定向全量、双 module `make test-go`/`make vet-go`、`go test -race -count=1 ./internal/adapter/conformance ./internal/adapter/claude ./internal/app ./agenthooks/install`、gofmt 和 diff 检查通过。安装 conformance 与 A13 确认 `[x]`；进程级性能和真实 M1b 仍待完成。

## CLI hook 进程入口完成确认（2026-10-15）

- [x] **RV-CLI-HOOK-01 入口遗漏**：实际二进制验收发现此前生成的 `style agenthooks run` 没有 CLI 分派；已有套件直接调用 runtime，无法证明安装后可执行。现分派先于 DefaultService，复用真实 Runner/codec，只读取 index/state；provider、子命令与 fail-closed 参数错误不污染 stdout。
- [x] **RV-CLI-HOOK-02 安装归属和生命周期**：Codex plugin/user 与 Claude user 命令持久携带 STYLE_HOME、配置根和 harness 版本，含空格路径和父进程错误 STYLE_HOME 均通过。Runtime 关闭 agenthooks 合成 backfill，由 STYLE 维护实际事件计数；codec 辅助状态放在 STYLE_HOME/cache/agenthooks，metadata 日志使用真实评估 point。
- [x] **RV-CLI-HOOK-03 验收可观察性**：真实编译二进制通过 46 个 stdout golden、损坏 index 的两家 fail-open 和 Claude fail-closed；源码/lock/settings 故意损坏仍能处理 hook。in-process suite 捕获 Runner 和 runtime 日志并拒绝 ERROR。原 PermissionRequest Bash 解析 fixture 意外请求了该事件不支持的 inject，现改用未知工具做解析/no-op 检查；独立 deny_permission fixture 保留 Bash block 断言，未放宽 engine 能力检查。PermissionRequest 不属于当前 M1 固定订阅面，不能据离线探针宣称真实订阅支持。

证据：`internal/app/hook_command_test.go`、`internal/adapter/conformance/process_test.go`。清理 CODEX_HOME/CODEX_SANDBOX、临时 Go 缓存、GOPROXY=off/GOSUMDB=off；定向进程测试、双 module `make test-go`/`make vet-go`、`go test -race -count=1 ./internal/app ./internal/runtime ./internal/adapter/conformance`、gofmt 和 diff 检查通过。A20 进程性能尚未核验；真实 M1b 仍待用户手动配置后验证。

## A20 / M1a 完成确认（2026-10-15）

- [x] **RV-A20-01 进程级性能**：旧 A20 仅测 engine.Process，不能证明 CLI 加载和进程成本。新增 opt-in 进程测试，真实编译 CLI，每家预热 10 次后采样 200 次并逐次核对 stdout；Codex median=21.031958ms、p95=22.852ms、max=28.647875ms，Claude Code median=21.424125ms、p95=22.97575ms、max=27.726041ms。index 加载/解析/state/engine/编码/I/O 计入；文件和已解析 session 预热，不包含构建、shell、harness 启动或首次 effort 配置解析。宿主负载影响结果，测试须独立执行，普通测试不强制计时。
- [x] **M1a 离线验收**：A1–A22 与 adapter conformance 离线部分均有可重复证据，且 CLI 进程入口已独立验证。真实 harness 接受配置、信任、模型实际收到上下文及 harness 端耗时，仍保留 M1b `[?]`，不以本机进程数据代替。

验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off STYLE_PERF=1 STYLE_PERF_OUTPUT=/tmp/style-cli-a20-2026-10-15.json go test -count=1 ./internal/adapter/conformance -run '^TestHookProcessLatency$' -v` 独立运行通过。聚合证据：`docs/review/M1-A20-2026-10-15.json`。此前同实现的双 module 全量测试/vet 和 app/runtime/conformance race 已通过，本提交仅新增 opt-in 性能测试与证据文档。

## M1b 手动配置准备完成确认（2026-10-15）

- [x] **M1B-PREP**：已提供 `docs/STYLE-M1B-MANUAL.md` 和第二包 `testdata/manual/m1b-probes/STYLE.yaml`。配置、备份、三条路线、Codex 插件手动注册/取消、warn/prompt block/compact/model switch/去重观察、字节比较与结果返回均有具体步骤。
- [x] **准备验证**：真实构建 STYLE CLI；临时 HOME 与三家配置/状态根内完成两包 validate/plan/install/sync/uninstall，原有四个配置文件逐字节恢复。两家 STYLE 进程输出 session/warn/block 与串行去重标记通过。Codex CLI 0.159.3 实际接受本地 marketplace JSON，插件 add/remove、marketplace remove 及切回 user 路线通过。
- [?] **真实 M1b**：Claude 只用固定版本输出模拟检测，没有运行真实 Claude；Codex 未启动模型会话。串行去重不等于实际并行工具验证，插件命令成功不等于上下文投递。真实配置由用户执行，B1–B12、C1–C9 均未提前完成，M1 整体仍未完成。

复现入口：`testdata/manual/README.md` 的构建/validate/plan 命令，以及手册内的安装、插件注册/取消、卸载和 cmp 命令；将所有配置根指向临时目录即可复核准备流程。shell 代码块语法、相对链接与 `git diff --check` 在提交前检查。

## M1b Codex 首轮真实验收（2026-10-15）

- [x] **CDEX-05/B5**：用户配置的 codex-plugin 路线已在真实工作会话完成 force push 拦截和 force-with-lease 放行；仅使用临时 Git 仓库和本地 bare remote。真实工具错误和 no-force-push/blocked stats 对应，放行命令退出 0。
- [~] **CDEX-02/B2**：待测模型按用户选择的 gpt-6-sol/low 记录。新会话 SessionStart 输出、后续 prompt 去重和另一会话 prompt 注入有真实 stats，截图回答与 state 一致。工作对话的 gpt-6.1-sol/low 工具测试独立记录，不混作用户模型读数；minimal/high 与其余模型场景未验收。
- [!] **M1B-EXAMPLE-01**：写测试文档的 Python heredoc 被误拦，因 no-force-push 正则匹配了被引用的示例命令。属于示例规则精度问题，需补引用文本边界；本轮记录证据，未修复。

证据：`docs/review/M1B-CODEX-2026-10-15.json`，包含命令、结果、状态行、会话范围和复现方法。没有更改真实配置或 Go 实现。M1b 尚未完成，Claude 暂不验收。

## M1b 问题修复确认（2026-10-15）

- [x] **M1B-FIX-01 Codex 卸载残留**：两条路线在原配置不存在时均复现卸载后仅剩 `"\n"`。新文件直接使用 agenthooks 的 render 输出，其 end marker 后的生成换行在删除托管区时留下；现令 Codex 渲染与已有合并逻辑一致，不在 end marker 后生成额外字节。未用 TrimSpace 删除用户原文。
- [x] **M1B-FIX-01 验证**：新增 `internal/app/uninstall_test.go:TestUninstallCodexConfigWithoutGeneratedNewlineResidue`，两条路线 × absent/原有换行/混合空白/用户内容共 8 组，包含重复安装和双向路线切换；不存在的文件恢复不存在，原有字节及权限保留。隔离 Go 缓存、清理 CODEX 环境后的双 module 全量测试/vet、gofmt、diff 检查通过。

此修复防止新安装往返生成残留。历史上已产生的单换行文件与用户原有单换行文件无法仅凭当前字节区分，清理应依据安装前备份；本轮不修改用户真实配置，真实 B8 仍待验收。

- [x] **M1B-FIX-02 设计文档完成确认**：路线示意统一为 `codex-plugin`、`codex-user`、`claude-user`；从安装命令表删除未实现的信任选项，明确 CLI 不接受 `--preseed-trust`。记录当前两条 Codex 路线均生成摘要、plugin 仍需手动登记且不据生成文件声称真实信任。锁文件中的 `user`/`preseeded` 与报告的 `user-confirmation`/`preseed` 区别已写清，移除不存在的锁字段；添加实现源文件链接和当前里程碑状态。更新主设计文件及手册，主设计文件单独纳入版本控制。

M1B-FIX-02 复核：使用临时 Go 缓存、清理 CODEX 环境构建 `go build -o /tmp/style-cli-m1b-doc-check ./cmd/style`；把 HOME、STYLE_HOME、CODEX_HOME、CLAUDE_CONFIG_DIR 指向临时目录。运行该二进制的 `install --help`、`adapters --json`、`install <example> --harness codex --preseed-trust codex --yes`（预期退出 1/unknown option 且无写入）；随后按手册分别安装 codex-plugin/codex-user，核对安装报告、锁文件、trusted_hash，并各自卸载。全部符合文档，最终 config.toml 不存在；新增源链接和 diff 检查通过。未更改 CLI 选项或真实配置，M1b 其余验收仍未完成。

- [x] **M1B-FIX-03 未知 effort 残留旧档位**：`state.Update` 对 effort 三个字段各自“非空才覆盖”，所以 raw 从 `high` 变成表外值时档位仍是 `high`，规则误触发、模板显示过期值。规范 §6 的“从不用空值覆盖”指事件没带值，不适用于“带了值但映射不到档位”。修复把非空 raw 当作完整读数，档位/近似一起替换；只带档位不带 raw 的更新保持原语义。Codex 每会话只解析一次，实际受影响的是 Claude 的逐事件 `effort.level`。
- [x] **M1B-FIX-04 Claude 环境变量短路**：`StateUpdate` 在 `previous.EffortResolved` 且事件无 `effort.level` 时提前返回，这是 Codex“每会话解析一次配置”的语义误用到 Claude。规范 §6 对 Claude 的顺序是事件 → `$CLAUDE_EFFORT` → 状态，没有一次性约定。修复后每个 Claude 事件都按此顺序解析；只多一次 `os.Getenv`，不读文件，A20 热路径不受影响。剩余风险：若 `CLAUDE_EFFORT` 是请求值而 `effort.level` 是降级后的生效值，两者不一致时动态正文可能在两值间切换，待 U-C1/C1 真实环境确认。

## Codex B6 完成确认（2026-10-15）

- [x] **CDEX-06/B6**：真实 codex-plugin 的 apply_patch 新增 TODO 后，工作模型收到 `Do not leave TODO comments in code.`，stats 记录 no-todo-comments/tool.post/warn/output；第二次 apply_patch 只删除该行，工具完成且没有同规则警告或 stats，临时文件逐字节恢复。观察期间的两条新 prompt dedup 不计为 TODO 输出。

证据和复现步骤：`docs/review/M1B-CODEX-2026-10-15.json` 的 B6。范围是本工作会话 gpt-6.1-sol/low，不替代用户 gpt-6-sol 的其他模型/effort 场景。未改动真实配置或 Go 实现，M1b 整体未完成。

## Claude 安装基线检查（2026-10-15）

- [x] **M1B-CLAUDE-SETUP-01**：初次预检发现 10 条 STYLE hook 指向已不存在的 go-run 程序。用户重新安装后，当前 command 均为固定且可执行的二进制，args 绑定状态目录，锁文件新增 claude-user；真实 SessionStart 已输出。原阻塞已解除，失败预检作为历史证据保留。

证据及复现：`docs/review/M1B-CLAUDE-2026-10-15.json`。手册已更新实际进展；配置由用户修改，未覆盖原始备份。

## Claude C1 完成确认（2026-10-15）

- [x] **CLAUDE-01/C1**：新会话 SessionStart 输出已到达模型，用户回传的 claude-sonnet-5-5/unknown 正文与真实 stats 摘要一致。用户界面选择 high，Bash 读到 CLAUDE_EFFORT=high；首次工具后的下一条提示补发 high，正文摘要再次一致，state 记录 high/raw high/approx false。符合设计 c1 明确允许的延后补发分支，无需为启动阶段的 unknown 修改实现。

证据和复现步骤见 Claude JSON 的 C1。范围为 Claude Code 2.1.300、claude-user、claude-sonnet-5-5/high。没有捕获各 hook 进程环境或原始 effort payload，因此保留 U-C1 的精确可用性问题；不把 Bash 环境读数等同于所有 hook 的环境。本项无 Go/配置变更，C2–C9 仍独立验收。

## Claude C2 完成确认（2026-10-15）

- [x] **CLAUDE-02/C2**：同一会话 compact 后，scope epoch=1 且对应 seen 标记存在；真实 SessionStart 对 session-info 补发一次。用户随后分两条消息引用的 high 正文均与输出摘要一致，实际两条 UserPromptSubmit 均为 dedup；观察区间没有额外 output。

证据、观察区间及复现步骤见 Claude JSON 的 C2。判定依赖真实 stats、epoch/seen 和模型可见回传，未把模型重复引用当成 hook 重复注入。本项无 Go/配置变更，C3–C9 继续独立验收。

## Claude C3 问题与修复完成确认（2026-10-15）

- [!] **CLAUDE-03/C3 真实验收**：用户 UI 从 Sonnet 切到 Haiku 5.5，随后两次回答仍引用 Sonnet。真实 state 未更新、两条 prompt 去重旧 hash；实际安装中没有 PostModelSwitch。C3 未通过，待用户重装复测。
- [x] **M1B-FIX-05 实现修复**：安装订阅只遍历 index 注入点，遗漏模型依赖的观察事件。现按条目 needs 添加 PostModelSwitch，并同步等待状态更新以免下一提示竞态；观察事件返回中性 JSON，不增加规则注入。没有 model 依赖时不安装，最后一个依赖移除后随 trust/recompile 清理。
- [x] **M1B-FIX-05 验证**：新回归在修复前明确失败（观察事件缺失、三类模型依赖均缺订阅）；修复后真实安装 argv→CLI 入口→状态更新→下一提示新模型→后续去重通过，静态/无关工具及移除依赖的对照通过。双 module 全量 test/vet、app 定向 race、gofmt 和 diff 检查通过。

证据、失败时间和复现步骤见 Claude JSON 的 C3；测试位于 `internal/app/claude_model_switch_test.go`。修复属于 CLI 的安装订阅推导，继续复用现有 agenthooks renderer/codec 与 Claude runtime，不重复实现 hook。真实配置未由 agent 修改；以上为修复时的失败记录，后续真实复测见下。

## Claude C3 真实复测完成确认（2026-10-15）

- [x] **CLAUDE-03/C3**：用户重装后实际配置含 6 类事件、11 条 STYLE hook，PostModelSwitch 为同步观察。Haiku→Opus、Opus→Sonnet 后，下一条提示分别输出新模型正文一次，后续提示各为 dedup；用户原样引用与真实 stats 摘要及 state 一致，两个正文的 seen 标记存在。

证据及复现见 Claude JSON 的 `C3.real_retest`，原失败记录保留。本次初始 SessionStart 的 model/effort 均不可用，后续模型由切换刷新，effort 仍为 unknown；只确认 c3 的新模型单次补发，不扩大为所有启动或强度读数场景。本项无 Go/真实配置变更，C4–C9 继续独立验收。

## Claude C4 验收准备（2026-10-15）

- [x] **CLAUDE-04-PREP**：用户没有或不确定是否有 code-review；新增最小原生 fixture 及独立工作目录的手动配置步骤。fixture 仅回传加载标记和 hook 指令，不预填 STYLE findings 数值，避免把 skill 自带文字误认为 hook 投递。手册规定两次新会话交换 Skill/显式入口顺序，分别确认首次输出和共享去重。

验证限于 fixture 内容、临时目录复制/字节比较和手册 shell 语法；没有修改真实 Claude 配置或安装用户 skill。C4 真实加载、两个入口及共享去重仍为 `[?]`，不据准备完成标记验收通过。

## Claude C4 问题与修复完成确认（2026-10-15）

- [!] **CLAUDE-04/C4 真实验收**：用户确实调用 Skill(code-review)，但会话 cwd 是 home，fixture 在另一验收目录，实际执行 fork review。真实配置也漏了 Skill matcher：隐式调用没有 review-skill stats，显式 UserPromptExpansion 输出一次，尚不能证明模型可见或共享去重。
- [x] **M1B-FIX-06 实现修复**：CLI 安装推导原来只处理原生名、canonical 和 MCP selector，忽略 Match.Skill；现按编译条目的实际工具评估点添加 Skill。继续复用 agenthooks matcher 渲染、skill 识别和 runtime 过滤。
- [x] **M1B-FIX-06 验证**：真实安装 argv 回归的 before/after × 两种入口顺序四组在修复前均缺订阅而失败；修复后首次投递、跨入口去重及移除 skill 后保留 Bash 通过。双 module test/vet、app 定向 race 与真实二进制隔离安装/两种顺序/卸载通过。手册补启动目录校验和 fixture 标记检查。

证据与复现见 Claude JSON 的 C4，回归位于 `internal/app/claude_skill_install_test.go`。以上保留首轮失败和修复时的记录，真实复测结论见下；不把 fork skill 的实际审查结果当成验收通过。

## Claude C4 真实复测完成确认（2026-10-15）

- [x] **CLAUDE-04/C4**：两个真实新会话均在验收目录、加载最小 fixture。隐式 Skill 首先调用时输出 findings 上限原文，随后显式入口 dedup；交换顺序后显式入口输出同一原文，随后实际 Skill 调用 dedup。每个 session 仅一次输出，使用同一 rule/body seen 标记；用户首次引用与正文摘要一致。

证据、实际工具调用、时间及复现见 Claude JSON 的 `C4.real_retest`。反向顺序的第二次回答再次复述旧文字，但实际 hook 为 dedup，不构成重复注入。本项确认默认 after、前台项目 skill 和 session 去重，不扩展为 fork 或 before 的真实投递。无 Go/真实配置变更，C5–C9 待验收。

## Claude C5 完成确认（2026-10-15）

- [x] **CLAUDE-05/C5**：真实 Bash 调用执行成功，用户明确确认 M1B_WARN_USER 在工具 UI 内容中可见，模型引用未出现在测试提示中的 M1B_WARN_MODEL。实际 PreToolUse warn/output 的正文摘要一致，记录时间早于工具结果。

证据、工具 ID、时间和复现见 Claude JSON 的 C5。用户最初称“没有警告”，澄清后确认工具内容中可见，符合 c5 的界面提示要求。模型称在结果后收到上下文，不能据此判定 hook 延后执行或确定模型内部接收时机；U-C5 仅关闭 PreToolUse 显示这一分支。无 Go/真实配置变更，C6–C9 待验收，M1 未完成。

## Claude C6 完成确认（2026-10-15）

- [x] **CLAUDE-06/C6**：两次真实 prompt 阻断的 hook 消息只有固定理由，Claude 记录 preventContinuation=true、promptKept=false。真实 force push 收到 PreToolUse 的明确拒绝理由，模型正确引用；lease 对照进入 Git 并成功返回 Everything up-to-date，符合当前设计。测试只连接本地 bare remote，前后 HEAD 一致。

证据、实际工具 ID、两次阻断记录及复现见 Claude JSON 的 C6。用户因本工作 Codex 也安装了探针而删减回传标记；以原 Claude 记录核验精确输出，不重建未保留的原 prompt。模型建议禁止 lease 不属于本项验收契约；M1B-EXAMPLE-01 的正则误拦问题继续独立保留。无 Go/真实配置变更，C7–C9 待验收，M1 未完成。

## Claude C7 完成确认（2026-10-15）

- [x] **CLAUDE-07/C7**：首次分消息执行的串行结果未用于通过验收。用户 /clear 后单条消息实际发起两个不同 Bash 工具调用，同属一个 assistant message，两个 start/end 区间均为 [1792072394,1792072397]。规则只有一次 output、一次 dedup，正文摘要一致，gen=1 的 session seen 标记存在；模型引用实际注入标记。

证据、首次观察和并行复测见 Claude JSON 的 C7。确认范围是两个真实工具并行时单次注入，不据此声称 hook 进程精确同时执行或通过压力测试。本项无 Go/真实配置变更，C8–C9 待验收，M1 未完成。

## Claude C8 首轮记录（2026-10-15）

- [~] **CLAUDE-08/C8**：用户已清理插件和 marketplace 登记；卸载命令与环境赋值拆行后均报无匹配包。只读确认默认 home 无包、验收 home 两包和 12 条 Claude hook 仍在，卸载尚未发生。恢复基线和当前配置的 applied 摘要一致，保存了后增模型/界面设置；待单行 env 命令复测及字节比较。

证据见 Claude JSON 的 C8。没有采集失败进程环境，因此命令拆行导致默认 home 回退是与现象一致的解释，不补造运行数据。手册已改用完整单行 env 命令；本项未通过，不当作配置恢复失败，也不删除真实设置。

## Claude C8 完成确认（2026-10-15）

- [x] **CLAUDE-08/C8**：用户以单行 env 命令成功卸载两包，cmp 通过；只读确认 settings.json 与核验过的本次安装原始字节完全一致（197 字节及 SHA256 一致），CLAUDE.md 和 owner 恢复记录均不存在，包列表为空且 lock/index 已移除。模型/界面偏好保留，未用过时的 absent 备份删除它们。

首轮记录仅为历史状态，最终通过依据和复现见 Claude JSON 的 C8.real_retest。本次没有外部 hook 条目，不扩大为该场景的真实验收或 Codex B8；外部条目保留仍有离线回归。无 Go/真实配置代操作，C9 待用户重装后继续，M1 未完成。
