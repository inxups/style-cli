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
