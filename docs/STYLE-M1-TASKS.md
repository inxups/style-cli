# STYLE CLI M1 任务清单

更新时间：2026-10-13

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
- [ ] **T15 `warn` 首句提取**：覆盖中英文标点、空行、多行、无句末标点和动态正文。

### 规划、预算和编译

- [ ] **T20 `core/plan`**：把规则转换为能力需求，查询适配器，执行降级阶梯、盲区判断和 plan 报告。
- [ ] **T21 `core/tokens`**：实现字符/token 估算、`per_rule`、`per_event`、`always_on` 和 T001–T007。
- [ ] **T22 适配器一致性接口**：定义 provider 注册、能力、数据来源、静态位置、路线、上限和 compact 信号接口。
- [ ] **T23 `core/compile`**：生成稳定的 `build/index.json`，包含静态条目、评估点、顺序、匹配条件、正文片段和估算值。
- [ ] **T24 index schema/golden**：为示例包建立 index schema 和逐字节 golden 输出。

### 运行时

- [ ] **T30 `core/state`**：实现 session、scope、gen、epoch、prompt_count、会话来源和过期状态清理。
- [ ] **T31 repeat 去重**：实现 `every`、`turn`、`session`、正文哈希、compact 重注入和 `O_EXCL` 并发占位。
- [ ] **T32 runtime engine**：实现 stdin 解码、Enrich、match/when、模板渲染、block 优先、inject/warn 合并、上限裁剪和 stats/log。
- [ ] **T33 输出白名单和 fail-open**：只输出 STYLE 允许的字段；index、模板、状态或编码失败时默认空输出并记录日志。
- [ ] **T34 动态 system**：实现 `session.start` 与 `prompt.submitted` 双挂载，以及模型/强度改变后的重新评估。

### agenthooks 和两个适配器

- [ ] **T40 F1–F4 安装层改动**：修 Codex shell matcher、补卸载、改 STYLE owner 标记、补 Codex 插件路线和信任键。
- [ ] **T41 F5–F6 运行时数据/状态改动**：补 effort、原始 effort、近似标记、harness 版本和 STYLE 会话状态接口。
- [ ] **T42 F7–F9 事件/codec 改动**：补 Codex 子 agent context；补 skill 识别和显式 `$name`；核对并补 Claude prompt/effort/MCP/model-switch 字段和输出字段。
- [ ] **T43 F10 Claude exec 安装**：支持 command/args、timeout 和可选 fail-closed 配置。
- [ ] **T44 Codex adapter**：实现固定订阅面、插件/用户级路线、`developer_instructions` 静态托管区、配置 effort 读取和 MCP/skill Enrich。
- [ ] **T45 Claude adapter**：实现动态订阅面、用户级 settings、`CLAUDE.md` 静态托管区、环境 effort 和工作区信任。

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

## 回顾跟进项（2026-10-13）

- [x] **REVIEW-001 已完成项回顾**：复核 T10–T14 的职责边界、测试证据、源码和规范，固化 13 个探针场景及结果；报告见 `docs/STYLE-M1-REVIEW-2026-10-13.md`，证据见 `docs/review/M1-2026-10-13-*`。回顾完成后按用户要求停止功能推进。

历史条目 T11/T13 的已通过状态保留；以下是回顾中新增、尚未修正的边界问题，修复后分别补充回归证据：

- [x] **RV-T11-01/R01**：`STYLE.yaml` 追加第二个 YAML 文档或非法尾部时被静默忽略。 已修复；证据：`core/spec/frontmatter_test.go`：拒绝第二个文档、空第二文档、非法尾部和非 mapping；接受单文档结束标记与尾部注释。`go test -count=1 ./core/spec` 通过。
- [x] **RV-T11-02/R02**：`from:` 显式路径和 glob 接受 `rules/**/*.txt` 等非 Markdown 来源。 已修复；证据：显式路径与 glob 结果统一限制为 `rules/**/*.md`，validate 复查来源；`core/spec/from_source_test.go` 覆盖 `.txt`、混合 glob、rules 外路径及绕过 Load 的校验。`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-01/R03**：NaN/Inf 数字通过校验，导致合并值无法 JSON 编码。 已修复；统一数字与 scalar 检查拒绝 NaN/Inf；覆盖包、defaults、规则、from 覆盖、budget 和 vars，合法数字 defaults 与 JSON 编码通过。证据：`core/spec/validate_numbers_test.go`；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-02/R04**：嵌套 YAML 的非字符串键被转换时静默丢弃。 已修复；递归检查原始 mapping，拒绝非字符串键；asStringMap 拒绝有损转换。证据：`core/spec/validate_map_keys_test.go` 覆盖包/规则/覆盖字段及合法字符串键；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-03/R05**：native tool name selector 无法使 `tool.added` 条件通过事件可用性校验。 已修复；native `names` selector 参与 `tool.added` 可用性判断，具体提取由 adapter 决定，缺失值仍不匹配。证据：`core/spec/validate_condition_test.go` 覆盖 `Edit` 单值及名字列表；`go test -count=1 ./core/spec` 通过。
- [x] **RV-T13-04/R06**：skill 的 `at` 不参与隐式 tool 事件可用性判断。 已修复；skill 隐式事件按 `at` 派生，并尊重 block 默认 before；`skill.explicit` 仍独立可用。证据：`core/spec/validate_condition_test.go` 覆盖 before/after `tool.output` 与显式 prompt；`go test -count=1 ./core/spec` 通过。
- [!] **RV-T13-05/R07**：非法 `when.model` glob 通过静态 validate，后续 compile 才失败。
- [!] **RV-T13-06/R08**：单文件 native schema 与目录包 schema 不一致。

详细复现、影响和修正方向见 `docs/STYLE-M1-REVIEW-2026-10-13.md`。

## 后续 backlog

- [ ] **M2**：`target: native`、`style stats`、跨包去重、Claude `~/.claude/rules/` 静态位置和 replace 评估。
- [ ] **M3**：`explain`、`probe`、`update`、git 来源、`--frozen`、项目级路线、刷新注入、事件内容围栏、provider、规则级 fail-closed、用户 effort 映射、Windows。
- [ ] **M4**：Gemini、OpenCode、Cursor、Copilot、Kimi 等额外 harness 适配器。
