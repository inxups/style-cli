# STYLE CLI M1 任务清单

更新时间：2026-10-15

本文件是 STYLE CLI 的持久化任务清单。每项任务完成后必须同时补充证据（命令、fixture、golden 文件或真实运行记录），不能只根据对话上下文标记完成。

## 状态约定

- `[x]` 已完成，且有可重复证据。
- `[ ]` 尚未开始。
- `[~]` 部分完成或被环境阻塞。
- `[!]` 已知问题或设计风险，尚未解决。
- `[?]` 等待真实环境验证。

## 当前基线

- [x] **T01 仓库和资料检查**：已检查 STYLE 规范、CLI 设计、20 张 SVG 流程图、示例 `STYLE.yaml` 和当前 Git 结构。规范入口为 `STYLE_DOCS/STYLE-spec.md`，正文按包结构、规则、运行时语义和编译产物拆分在 `STYLE_DOCS/STYLE-spec-*.md`。证据：`STYLE_DOCS/STYLE-spec*.md`、`STYLE_DOCS/STYLE-cli-design.md`、`STYLE_DOCS/svg/*.svg`。
- [x] **T02 agenthooks 职责边界**：已确认 `agenthooks/` 提供统一事件、provider codec、工具/MCP/skill 识别、能力矩阵、配置渲染和测试 fixture；STYLE 仍需实现规则包、编译、状态、预算、trust/sync 和 CLI。证据：`agenthooks/DESIGN.md` §3–7、`STYLE_DOCS/STYLE-cli-design.md` §2–8。
- [x] **T03 agenthooks 可重复基线**：清理会影响 provider 检测的外部环境变量后，`go test ./...` 和 `go vet ./...` 通过；完整结果见 `docs/STYLE-M1-TESTS.md` 和 `docs/PROGRESS.md`。
- [x] **T04 持久化执行约定**：新增根目录 `AGENTS.md`，规定任务/测试文档同步、独立验收后 Git 提交、提交报告和环境隔离。证据：`AGENTS.md`。

## 实现顺序

### 基础工程和规则包

- [x] **T10 顶层 Go module/workspace**：根 module `github.com/inxups/style-cli` 用本地 `replace` 依赖独立的 `agenthooks/` module；`go.work` 注册两个 module。Go 的根 `./...` 不跨越嵌套 module，统一入口 `make test-go` 分别执行两边 `go test ./...`，`make vet-go` 执行两边 vet。证据：`go.mod`、`go.work`、`Makefile`、`internal/agenthookscheck/agenthooks_test.go`；命令和结果见 `docs/PROGRESS.md` T10/B1.1。
- [x] **T11 `core/spec` 解析**：新增目录包和 `<name>.style.md` 单文件包解析；读取 `STYLE.yaml`、拆分规则 frontmatter/正文、读取 `file:` 和 `text:`，按声明展开 `from:` 文件及递归 glob（glob 命中按相对路径排序）。保留未合并字段供 T12 使用；拒绝正文 frontmatter、`file:` 指向 `rules/`、路径逃逸、符号链接、超限和无效文本。实现按模型、加载器、frontmatter、包内文件/路径及字段操作职责拆分在 `core/spec/types.go`、`load.go`、`frontmatter.go`、`package_files.go`、`values.go`；合并逻辑位于 `merge.go`。证据：`core/spec/spec_test.go`；命令和结果见 `docs/PROGRESS.md` T11/B1.2。
- [x] **T12 规则合并**：按规范默认值、包级字段/`defaults`、规则文件 frontmatter、`from:` 覆盖的优先级生成有效规则；`reads` 取并集，其他字段按顶层整体覆盖；过滤 `enabled: false`，保留原始声明供 T13 校验。证据：`core/spec/merge.go`、`core/spec/merge_test.go`；命令和结果见 `docs/PROGRESS.md` T12/B1.3。
- [x] **T13 `validate`**：实现 schema、路径/名字/ID 限制、重复 ID、未声明规则、模板变量、`reads`、正则、合法组合、T005/T006/T007 和规范错误码。校验实现按包、规则、条件、模板、reads、lint、文件扫描和值工具拆分在 `core/spec/validate_*.go`；token 离线估算复用 `internal/tokenestimate`。证据：`core/spec/validate*_test.go`、`internal/tokenestimate/estimate_test.go`；命令和结果见 `docs/PROGRESS.md` T13/B1.4。
- [x] **T14 模板和条件**：新增 `core/template` 的替换语法解析、静态/动态分类、默认值、转义和渲染；spec 校验和 lint 复用同一解析器。新增 `core/when` 的字段 AND/列表 OR、模型 glob、effort 比较展开、agent 默认范围、缺失数据不匹配和条件字段读取接口；编译后 matcher 可复用，JSON 往返保留空 selector 语义。原生 payload 字段读取由后续 adapter 接入 `FieldReader`。证据：`core/template/*_test.go`、`core/when/*_test.go`、`core/spec/validate_template_parser_test.go`；命令和结果见 `docs/PROGRESS.md` T14/B1.5。
- [x] **T15 `warn` 首句提取**：`core/warn.FirstSentence` 从渲染后的正文跳过开头空行，处理首个非空行；中文 `。！？` 直接终止，ASCII `.!?` 仅在后接空白或行尾时终止，无句末标点时返回该行，空正文返回空串。测试覆盖 CRLF、无标点、多行和先模板渲染再提取。静态编译/动态运行时调用时机由后续 compile/engine 任务接入。证据：`core/warn/sentence_test.go`；命令和结果见 `docs/PROGRESS.md` T15/B1.6。

### 规划、预算和编译

- [x] **T20 `core/plan`**：`RequirementFor` 从合并规则提取事件、能力、条件/模板数据需求；`Build` 查询纯事实 Adapter 接口，按规范执行 static/dynamic system、tool/MCP/skill/event 路由、替代事件、warn 部分输出、block→warn、静态回退、fallback skip/error 和盲区处理；输出稳定结构及不含正文的文本报告。Adapter 不依赖具体 harness，`core/plan` 不导入 `agenthooks`。证据：`core/plan/*_test.go` 覆盖能力/需求、降级、数据源缺失、skill 双触发点、harness 选择、system 动静态和 Load→Validate→Merge→Build 集成；B1.7 和 `docs/PROGRESS.md` T20。
- [x] **T21 `core/tokens`**：实现 Unicode rune 字符计数与离线 token 估算；静态变量和动态占位符渲染；按 adapter Layout 聚合原生事件/工具的最坏成本；实现 `per_rule`、`per_event`、`always_on`、跨已安装包/适配器单位的 T003，以及 T001–T007 报告。`core/spec.Validate` 产出的 T001/T005/T006/T007 lint 由 token 报告统一复用；T002/T004 依赖具体 plan。block 与 inject/warn 的互斥最坏情况和输出分隔符分别计算。证据：`core/tokens/*_test.go`、`internal/tokenestimate/estimate_test.go`、`core/spec/validate_lints_test.go`；B1.8 和 `docs/PROGRESS.md` T21/B1.8。
- [x] **T22 适配器一致性接口**：新增 `internal/adapter` 的窄接口和显式注册表：复用 `agenthooks.Provider`/`EventKind`，定义检测与安装、能力事实、运行时数据来源、静态位置、路线生命周期、trust、changeset、输入上限/单位/超限行为和 compact 信号；注册表做确定性 provider/route 校验与查找，且可投影为 `core/plan.Adapter`。`core` 不导入 provider 或具体 adapter 实现，具体 Codex/Claude adapter 留给 T44/T45。证据：`internal/adapter/*.go`、`internal/adapter/*_test.go`、B1.9 和 `docs/PROGRESS.md` T22/B1.9。
- [x] **T23 `core/compile`**：新增纯编译器，把已合并包、安装值和选定 plan 编译为确定的 index 与静态托管正文；记录静态条目、规范化 event/match/when、来源和正文哈希、顺序、runtime body parts、session 默认值及 placement 估算。拒绝 blocked/不匹配 plan、缺失来源字节、未知评估点和静态槽位；输出按包序、规则 priority 和包内序号稳定排序。规范同步了小数 priority 顺序与动态 warn 首句标记。证据：`core/compile/*_test.go`、`STYLE_DOCS/STYLE-spec-compiled.md` §9；B1.10 与 `docs/PROGRESS.md` T23/B1.10。
- [x] **T24 index schema/golden**：新增 `example/terse-reviewer/` 示例包、Draft 2020-12 `schema/index.schema.json` 和 `testdata/golden/index.json`；编译测试通过真实 JSON Schema 验证器检查 golden 与必需/禁止字段边界，并逐字节比较输出。回顾补充了 `target` 约束和全局 event/tool selector 区分；验证和证据见 B1.11、A3、`docs/PROGRESS.md` T24/B1.11 与 RV-T2X-01。

### 运行时

- [x] **T30 `core/state`**：新增持久 session/scope state，维护 gen、epoch、prompt_count、model/effort/harness version；startup/clear/compact/resume 按来源更新计数，支持显式过期清理。JSON 原子写、严格读、私有权限和路径安全有回归覆盖。证据：`core/state/state.go`、`core/state/state_test.go`、`docs/PROGRESS.md` T30–T34。
- [x] **T31 repeat 去重**：实现 `every`、`turn`、`session`、渲染正文哈希、compact 重注入及 `O_EXCL` 占位/释放；block 与 `subagent.start` 不去重。单测验证 turn、generation、epoch、正文变化与并发进程唯一输出。证据：`core/state/dedup.go`、`core/state/state_test.go`、`core/engine/engine_test.go`。
- [x] **T32 runtime engine**：新增 schema 校验和预编译 index、point/target/source/when 匹配、模板值渲染、block 优先、inject/warn 合并、输入上限裁剪、失败占位释放及 metadata-only stats/log；stdin 解码和 provider codec 继续由 `agenthooks.Runner` 负责。证据：`core/engine/`、`internal/runtime/`、双 module 测试和 `docs/PROGRESS.md`。
- [x] **T33 输出白名单和 fail-open**：输出由 engine whitelist 与 agenthooks provider codec 双重约束；默认 index/state/evaluation 错误 fail-open，Claude 可选 fail-closed 并返回失败码；日志/stat 不含 prompt 或工具正文。证据：`core/engine/engine_test.go`、`internal/runtime/runtime_test.go`、`agenthooks/runner_test.go`。
- [x] **T34 动态 system**：system 条目可在 `session.start` 与 `prompt.submitted` 双挂载；模型/effort 改变时重渲染，并仅在正文变化时补发；compact 仅重注入选择加入的条目。运行时数据提取由测试 adapter 验证，真实 provider 映射仍在 T41/T42/T44/T45。证据：`internal/runtime/runtime_test.go:TestRuntimeReevaluatesDynamicSystemWhenModelChanges`、`core/engine/engine_test.go:TestDynamicSystemReevaluatesAndDeduplicatesByRenderedBody`。

### agenthooks 和两个适配器

- [x] **T40 F1–F4 安装层改动**：修 Codex shell matcher、补卸载、改 STYLE owner 标记、补 Codex 插件路线和信任键。证据：`agenthooks/matcher_test.go`、`agenthooks/install/install_test.go`、`agenthooks/e2e/codex_test.go`；离线安装/卸载、owner 隔离、插件权限、长 trust key 和 DefinitionHash 定向测试通过，真实 Codex 运行仍留在 M1b。
- [x] **T41 F5–F6 运行时数据/状态改动**：`SessionInfo`/`core/state` 保存 effort、原始值、近似标记、harness 版本及一次性解析状态；新增 `internal/adapter/effortmap` 和 Codex/Claude 映射表，Codex 按 `-c`→profile→受信项目→用户配置解析 `model_reasoning_effort`，Claude 按事件→`CLAUDE_EFFORT`→会话状态解析。证据：`agenthooks/codec_test.go`、`agenthooks/detect_test.go`、`internal/adapter/effortmap/*_test.go`、`internal/adapter/codex/effort_test.go`、`internal/adapter/claude/effort_test.go`、`core/state/state_test.go`；完整验证见 `docs/PROGRESS.md` T41/B1.18。
- [x] **T42 F7–F9 事件/codec 改动**：Codex `SubagentStartDecision.WithContext` 走新增决策回调，同时保留旧观察回调；skill shell 识别扩展到 `head`、`tail`、`bat`、`cd … && cat SKILL.md`，显式 `$name` 解析按规范保持顺序并去重；Claude prompt id/effort/MCP、`UserPromptExpansion`、`PostModelSwitch` 和 suppress 输出字段补齐。`skills.read` 及显式 skill 到 engine 的 adapter Enrich 留 T44/U6。证据：`agenthooks/runner_test.go`、`agenthooks/codec_test.go`、`agenthooks/claude_skill_test.go`、`internal/adapter/codex/skill_test.go`、`agenthooks/provider/claudecode/claudecode_test.go`；完整验证见 `docs/PROGRESS.md` T42/B1.19。
- [x] **T43 F10 Claude exec 安装**：生成原生 `command` + `args`，原样保留 executable/argv 中的空格，延续秒级 timeout 与默认值；只在 blocking `tool.pre` 且 Manifest FailClosed 时添加 `--fail-closed` 和 `onFailure: block`，并要求已知 Claude Code ≥ 2.1.295。owner 合并/卸载同时识别旧 command 字符串和 exec args。证据：`agenthooks/install/install_test.go` 的 exec 参数、版本门槛、fail-open/telemetry、owner merge/uninstall 回归；验证见 B1.20 和 `docs/PROGRESS.md` T43/B1.20。
- [x] **T44 Codex adapter**：实现固定 hook 订阅面、Codex 插件/用户级路线、`developer_instructions` 静态托管区与配置 effort、MCP/skill/tool/subagent runtime Enrich；复用 `agenthooks/install` 的 hook codec、manifest renderer 和 owner 生命周期。回顾发现的静态-only 变更/卸载、嵌套路线根目录、native tool blind spot、复合 `tool.input.*` 值和 SubagentStart runtime 接线问题均有回归测试。证据：`internal/adapter/codex/*_test.go`、`internal/runtime/runtime_test.go:TestCodexSubagentStartRuntimeDeliversContextForAgentType`、`agenthooks/install/install_test.go`；双 module 全量测试、vet 与 adapter/runtime/install race 测试见 B1.21 和 `docs/PROGRESS.md`。
- [x] **T45 Claude adapter**：实现动态订阅面、用户级 settings、`CLAUDE.md` 静态托管区、环境 effort 和工作区信任；动态 surface 合并原生事件/匹配器，PostModelSwitch 只更新状态，UserPromptExpansion 已接入 runtime decision，路线状态不伪报 workspace trust。证据：`internal/adapter/claude/*_test.go`、`internal/runtime/runtime_test.go:TestClaudePromptExpansionRuntimeUsesAdapterAndRunnerHandler`、`agenthooks/install/install_test.go`；验证见 B1.22 和 `docs/PROGRESS.md`。

### CLI 和安装生命周期

- [ ] **T50 `style init`**：生成目录包和单文件包骨架。
- [ ] **T51 `style validate` / `style plan`**：输出稳定错误、降级、能力和 token 报告。
- [ ] **T52 `style install`**：收集 settings，检查 requires、冲突、replace、能力披露，原子写包/锁/index/适配器配置。
- [ ] **T53 `style uninstall`**：只删除 STYLE owner 内容，保留其他条目，卸载后配置字节级恢复。
- [ ] **T54 `style list` / `style doctor` / `style adapters`**：报告安装、路线、信任、index、状态和适配器能力。
- [ ] **T55 `style sync` / `style trust`**：哈希变更检测、差异展示、link 包行为、同一份内存字节编译和原子更新。

## 里程碑验收

- [ ] **M1a 离线验收**：STYLE 验收项 A1–A22 全部通过；adapter conformance 解析、往返、安装、报告和输出白名单通过。
- [?] **M1b Codex 真实环境**：B1–B12 全部完成，记录 Codex 版本、路线、信任行为、compact/resume、skill 和性能。
- [?] **M1b Claude Code 真实环境**：C1–C9 全部完成，记录 Claude Code 版本、工作区信任、compact/model switch、skill、warn 和性能。

## 回顾跟进项（2026-10-13 起）

- [x] **REVIEW-001 已完成项回顾**：复核 T10–T14 的职责边界、测试证据、源码和规范，固化 13 个探针场景及结果；报告见 `docs/STYLE-M1-REVIEW-2026-10-13.md`，证据见 `docs/review/M1-2026-10-13-*`。回顾完成后按用户要求停止功能推进。
- [x] **REVIEW-002 T42 完成后回顾**：复核 F7–F9 的 API 兼容性、Claude/Codex 能力、codec 输出和待接线边界；报告见 `docs/STYLE-M1-REVIEW-2026-10-13.md` 的“T42 完成后回顾”。

历史条目 T11/T13 的已通过状态保留；以下是回顾中新增的边界问题，现已全部修复并补充回归证据：

- [x] **RV-T11-01/R01**：`STYLE.yaml` 追加第二个 YAML 文档或非法尾部时被静默忽略。 已修复；证据：`core/spec/frontmatter_test.go`：拒绝第二个文档、空第二文档、非法尾部和非 mapping；接受单文档结束标记与尾部注释。`go test -count=1 ./core/spec` 通过。
- [x] **RV-T11-02/R02**：`from:` 显式路径和 glob 接受 `rules/**/*.txt` 等非 Markdown 来源。 已修复；证据：显式路径与 glob 结果统一限制为 `rules/**/*.md`，validate 复查来源；`core/spec/from_source_test.go` 覆盖 `.txt`、混合 glob、rules 外路径及绕过 Load 的校验。`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-01/R03**：NaN/Inf 数字通过校验，导致合并值无法 JSON 编码。 已修复；统一数字与 scalar 检查拒绝 NaN/Inf；覆盖包、defaults、规则、from 覆盖、budget 和 vars，合法数字 defaults 与 JSON 编码通过。证据：`core/spec/validate_numbers_test.go`；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-02/R04**：嵌套 YAML 的非字符串键被转换时静默丢弃。 已修复；递归检查原始 mapping，拒绝非字符串键；asStringMap 拒绝有损转换。证据：`core/spec/validate_map_keys_test.go` 覆盖包/规则/覆盖字段及合法字符串键；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-03/R05**：native tool name selector 无法使 `tool.added` 条件通过事件可用性校验。 已修复；native `names` selector 参与 `tool.added` 可用性判断，具体提取由 adapter 决定，缺失值仍不匹配。证据：`core/spec/validate_condition_test.go` 覆盖 `Edit` 单值及名字列表；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-04/R06**：skill 的 `at` 不参与隐式 tool 事件可用性判断。 已修复；skill 隐式事件按 `at` 派生，并尊重 block 默认 before；`skill.explicit` 仍独立可用。证据：`core/spec/validate_condition_test.go` 覆盖 before/after `tool.output` 与显式 prompt；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-05/R07**：非法 `when.model` glob 通过静态 validate，后续 compile 才失败。 已修复；validate 与 Compile 共用 `core/when.ValidateModelGlob`，非法模式稳定返回 `bad-model-glob`。证据：`core/spec/validate_model_glob_test.go`；`go test -count=1 ./core/spec ./core/when` 通过。
- [x] **RV-T13-06/R08**：单文件 native schema 与目录包 schema 不一致。 已修复；单文件包的 provider native hook 字段与目录规则共用 native schema 判定。证据：`core/spec/validate_native_single_file_test.go` 以相同内容分别验证目录包和单文件包；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T2X-01**：event-wide `target: event` 挂在 `tool.pre` 时产生空 `match`，与 T24 schema 冲突。已修复；index 保留 `target`，schema 允许 event-wide tool 条目不带 match、要求 routed tool/MCP/skill 条目带 match。证据：`core/compile/golden_test.go:TestEventTargetAtToolPointCompilesWithoutMatch` 编译该规则并以 Draft 2020-12 schema 验证输出；`testdata/golden/index.json` 更新。
- [x] **RV-T2X-02**：空/空白 tool、names、skill selectors 及 malformed MCP selector 可通过 Validate。已修复；Validate、Compile 和 index schema 共同检查非空 selector、target 与 match 类型一致，以及 MCP 单个 `/` 和非空组件。证据：`core/spec/validate_edge_test.go`、`core/compile/golden_test.go:TestNormalizedMatchRejectsInvalidSelectors` 与 schema 负向用例覆盖拒绝和 wildcard 接受；两篇 STYLE 规范同步格式约束。
- [x] **RV-T3X-01**：回顾 T30–T34 的 state、repeat、engine/runtime、输出策略和动态 system 集成；未发现需要改变分层的缺陷。20 个独立进程竞争同一 session key 仅一个输出，state 错误 fail-open 和 safe logging 另有回归用例。真实 Codex/Claude 字段映射及 native model-switch 事件仍留在 T41/T42/T44/T45，相关 A10/A11 保持部分状态。结论与验证见 `docs/STYLE-M1-REVIEW-2026-10-13.md` 的“T30–T34 完成后回顾”和 `docs/PROGRESS.md`。
- [x] **RV-T41-01**：回顾发现 T41 的根 module 直接导入 `github.com/pelletier/go-toml/v2`，但 `go.mod` 仍标为 indirect，导致 `go mod tidy -diff` 报告差异。已将其移入直接依赖并通过双 module tidy diff；证据见 `docs/STYLE-M1-REVIEW-2026-10-13.md` 和 `docs/PROGRESS.md`。

T20–T24 与 T30–T34 回顾结论和剩余范围见 `docs/STYLE-M1-REVIEW-2026-10-13.md`；各项处理和验证结果见 `docs/PROGRESS.md`。M1a/M1b 仍未完成。

详细复现、影响和修正方向见 `docs/STYLE-M1-REVIEW-2026-10-13.md`。

## 后续 backlog

- [ ] **M2**：`target: native`、`style stats`、跨包去重、Claude `~/.claude/rules/` 静态位置和 replace 评估。
- [ ] **M3**：`explain`、`probe`、`update`、git 来源、`--frozen`、项目级路线、刷新注入、事件内容围栏、provider、规则级 fail-closed、用户 effort 映射、Windows。
- [ ] **M4**：Gemini、OpenCode、Cursor、Copilot、Kimi 等额外 harness 适配器。
