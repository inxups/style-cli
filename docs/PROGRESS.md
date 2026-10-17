# STYLE CLI 工作进度

更新时间：2026-10-16

本文件采用追加式记录。每条记录包含日期、任务/测试编号、命令或依据、结果和下一步。

## 状态约定

- `[x]` 已完成且有可重复证据。
- `[ ]` 尚未开始。
- `[~]` 部分完成或环境阻塞。
- `[!]` 已知问题或设计风险。
- `[?]` 等待真实环境验证。

## 记录

### 2026-10-12 — DOC-001 `[x]`

- 任务：建立持久化任务、测试和进度文档。
- 依据：STYLE 规范、CLI 设计、20 张 SVG 流程图、`agenthooks/DESIGN.md`。
- 结果：已建立 `STYLE-M1-TASKS.md`、`STYLE-M1-TESTS.md` 和本文件。
- 下一步：执行并记录 agenthooks 隔离环境基线测试。

### 2026-10-12 — T01/T02 `[x]`

- 任务：检查仓库结构并确认 STYLE 与 agenthooks 的职责边界。
- 结果：`agenthooks/` 是当前仓库跟踪的 Go module；提供统一事件、codec、能力矩阵、安装渲染和 fixture。STYLE CLI 尚未实现规则包、plan、compile、state、token、trust/sync 和 CLI 生命周期。
- 证据：`STYLE_DOCS/STYLE-spec.md`、`STYLE_DOCS/STYLE-cli-design.md`、`agenthooks/DESIGN.md`。
- 下一步：保持 core 与 provider adapter 分层，按 `STYLE-M1-TASKS.md` 顺序实现。

### 2026-10-12 — B0.1 `[x]`

- 命令：

  ```sh
  env -u CODEX_HOME -u CODEX_SANDBOX \
    GOPATH=/tmp/style-cli-gopath \
    GOMODCACHE=/tmp/style-cli-gomodcache \
    GOCACHE=/tmp/style-cli-gocache \
    go test ./...
  ```

- 目录：`agenthooks/`
- 结果：root、agenthookstest、e2e、install、provider、transcript 等包全部通过。
- 下一步：执行 vet 和定向测试。

### 2026-10-12 — B0.2 `[x]`

- 命令：

  ```sh
  env -u CODEX_HOME -u CODEX_SANDBOX \
    GOPATH=/tmp/style-cli-gopath \
    GOMODCACHE=/tmp/style-cli-gomodcache \
    GOCACHE=/tmp/style-cli-gocache \
    go vet ./...
  ```

- 目录：`agenthooks/`
- 结果：通过，无 vet 报告。
- 下一步：执行 Claude/Codex codec、能力和安装定向测试。

### 2026-10-12 — B0.3/B0.4/B0.5 `[x]`

- 命令：

  ```sh
  env -u CODEX_HOME -u CODEX_SANDBOX \
    GOPATH=/tmp/style-cli-gopath \
    GOMODCACHE=/tmp/style-cli-gomodcache \
    GOCACHE=/tmp/style-cli-gocache \
    go test . -run 'TestRunClaudeDeny|TestAskDegradationOnCodex|TestRoundTripNoOpAndRawFidelity|TestCopilotDetection' -count=1

  env -u CODEX_HOME -u CODEX_SANDBOX \
    GOPATH=/tmp/style-cli-gopath \
    GOMODCACHE=/tmp/style-cli-gomodcache \
    GOCACHE=/tmp/style-cli-gocache \
    go test ./install -run 'TestRenderCodexAsyncAndTrust|TestInstallIdempotentAndMergePreservesForeignEntries|TestDefinitionHashCodexAlgorithm' -count=1
  ```

- 结果：两个定向测试命令均通过。
- 下一步：保留环境问题记录，不修改 agenthooks；开始 STYLE 顶层 module 和规则包实现前的测试 fixture 设计。

### 2026-10-12 — B0.6 `[~]`

- 命令：继承宿主环境直接执行 `go test ./...`。
- 结果：`TestCopilotDetection` 失败。宿主环境的 `CODEX_SANDBOX` 先于测试设置的 `COPILOT_CLI` 被 `detectFromEnv` 识别为 Codex；清理 `CODEX_HOME`、`CODEX_SANDBOX` 后同一套测试通过。
- 处理：按当前测试策略只记录，不修改 agenthooks 测试或依赖。
- 下一步：所有后续基线测试使用隔离环境命令。

### 2026-10-12 — T04/DOC-002 `[x]`

- 命令：`git diff --check`；`git status --short --branch`。
- 结果：新增根目录 `AGENTS.md`，明确任务文档、测试证据、独立验收、Git 提交和提交后报告规则；检查通过。
- 证据：`AGENTS.md`；本条目对应提交记录见 Git 历史。
- 下一步：开始 T10 顶层 Go module/workspace；完成后同步任务、测试和进度文档并单独提交。

### 2026-10-13 — T10/B1.1 `[x]`

- 任务：建立独立的 STYLE CLI 根 module 和本地 agenthooks 依赖，并提供双模块测试入口。
- 用户约定：hook 方面直接在 `agenthooks/` 演进，CLI 单独开发；不复制 hook 实现，不合并两个 module，不重写 agenthooks 的 import 路径。
- 实现：根 `go.mod` 声明 `github.com/inxups/style-cli`，通过 `replace` 使用 `./agenthooks`；`go.work` 注册两个 module。`Makefile` 的 `test-go`、`vet-go` 分别在两个 module 执行检查；根依赖烟测通过 `agenthooks.New()` 验证本地依赖可编译。
- 依据：Go 的 `./...` 不跨越嵌套 module；只有 workspace 不会自动扩大根命令的测试范围，因此聚合入口必须显式执行两边。
- 命令（目录：仓库根；Makefile 内配置临时 `GOPATH`、`GOMODCACHE`、`GOCACHE`，清理 `CODEX_HOME`、`CODEX_SANDBOX`）：

  ```sh
  make test-go
  make vet-go

  env -u CODEX_HOME -u CODEX_SANDBOX \
    GOPATH=/tmp/style-cli-gopath \
    GOMODCACHE=/tmp/style-cli-gomodcache \
    GOCACHE=/tmp/style-cli-gocache \
    go list -m all

  git diff --check
  git status --short --branch
  ```

- 结果：根依赖烟测、agenthooks 全量单元/集成测试、两边 vet 通过；模块图包含本地的 `github.com/inxups/style-cli` 与 `github.com/speakeasy-api/agenthooks`。agenthooks 代码和依赖文件没有改动。
- 下一步：T11 `core/spec` 解析目录包、单文件包、frontmatter、`from:` 文件和 glob。

### 2026-10-13 — T11/B1.2 `[x]`

- 任务：实现 STYLE 目录包和单文件包的 `core/spec` 解析。
- 实现：解析 `STYLE.yaml` 和单文件包 frontmatter，保留原始 manifest/规则字段与 `from:` 覆盖字段供 T12 合并；提取规则 frontmatter 与正文；支持规则 `file:`、`text:`、显式 `from:` 文件和含 `**` 的递归 glob。`from:` 匹配按相对路径排序，未声明文件不会自动成为规则。检查文件大小、UTF-8/NUL、符号链接和包内相对路径；拒绝正文 frontmatter 及 `file:` 指向 `rules/`。
- 命令（仓库根；Go 缓存隔离并清理 `CODEX_HOME`、`CODEX_SANDBOX`）：

  ```sh
  env -u CODEX_HOME -u CODEX_SANDBOX \
    GOPATH=/tmp/style-cli-gopath \
    GOMODCACHE=/tmp/style-cli-gomodcache \
    GOCACHE=/tmp/style-cli-gocache \
    GOPROXY=off GOSUMDB=off \
    go test -count=1 ./core/spec

  make test-go
  make vet-go
  git diff --check
  ```

- 结果：解析器测试、根 module 全量测试、agenthooks 全量测试及两边 vet 均通过。测试用临时包覆盖 `STYLE.yaml`、单文件 frontmatter、显式和 glob `from:`、深层 glob、排序、`file:`、`text:`、默认 ID、路径逃逸和两种 md 角色限制。agenthooks 源码未改动。
- 下一步：T12 规则优先级合并、`enabled: false` 和重复 ID 检查。

### 2026-10-13 — T12/B1.3 `[x]`

- 任务：完成规则字段合并、默认值、停用过滤和 ID 冲突检查；按维护性要求先拆分 `core/spec` 实现文件。
- 实现：`types.go` 定义 parsed/resolved 模型；`load.go` 加载目录和单文件包；`frontmatter.go` 解析 YAML/frontmatter；`package_files.go` 实现安全文件访问和 glob；`values.go` 提供字段复制和 reads 并集；`merge.go` 负责有效字段。合并顺序为规范默认 → 包级 priority/budget → `defaults` → 规则 frontmatter → `from:` 覆盖；除 `reads` 并集外均为顶层替换。所有规则的 `reads` 声明并入包级，保留声明顺序并去重。实现动作、target 相关默认和 `enabled: false` 过滤，原始包字段保留不变。重复 ID、defaults 白名单、from 禁止覆盖 `target`/`id` 留给后续 T13 校验。
- 命令（仓库根；Go 缓存隔离并清理 `CODEX_HOME`、`CODEX_SANDBOX`）：

  ```sh
  env -u CODEX_HOME -u CODEX_SANDBOX \
    GOPATH=/tmp/style-cli-gopath \
    GOMODCACHE=/tmp/style-cli-gomodcache \
    GOCACHE=/tmp/style-cli-gocache \
    GOPROXY=off GOSUMDB=off \
    go test -count=1 ./core/spec

  make test-go
  make vet-go
  git diff --check
  ```

- 结果：合并测试和模块全量测试/静态检查通过。测试覆盖字段优先级、完整 `when` 替换、`reads` 并集和次序、默认值、block 的 repeat 行为和 disabled 过滤。
- 下一步：T13 schema 与 validate 错误码。

### 2026-10-13 — T13/B1.4 `[x]`

- 任务：完成 `core/spec` 的包校验、规则校验和规范诊断，并按职责拆分校验实现，避免回到单个过大的 spec 文件。
- 实现：新增 `Validate(*Package) []Diagnostic` 及稳定的错误码/来源/严重级别；包、规则、条件、模板、reads、lint、未声明规则扫描和公共值操作分别位于 `core/spec/validate_package.go`、`validate_rules.go`、`validate_conditions.go`、`validate_template.go`、`validate_reads.go`、`validate_lints.go`、`validate_files.go`、`validate_values.go`。覆盖 metadata/defaults、名称/版本、规则 ID、from 覆盖、单文件规则清单、未声明文件、运行时 reads、模板变量、正则/effort、条件字段适用事件、`tool.added` 文件工具限制、`negated-optional-field`、`T005`、`T006` 和 `T007`。离线 token 估算抽到 `internal/tokenestimate`，供后续 plan/compile 复用。
- 命令（仓库根；Go 缓存隔离并清理 `CODEX_HOME`、`CODEX_SANDBOX`）：

  ```sh
  env -u CODEX_HOME -u CODEX_SANDBOX \
    GOPATH=/tmp/style-cli-gopath \
    GOMODCACHE=/tmp/style-cli-gomodcache \
    GOCACHE=/tmp/style-cli-gocache \
    GOPROXY=off GOSUMDB=off \
    go test -count=1 ./core/spec ./internal/tokenestimate

  make test-go
  make vet-go
  env -u CODEX_HOME -u CODEX_SANDBOX \
    GOPATH=/tmp/style-cli-gopath \
    GOMODCACHE=/tmp/style-cli-gomodcache \
    GOCACHE=/tmp/style-cli-gocache \
    GOPROXY=off GOSUMDB=off \
    go mod tidy -diff
  git diff --check
  ```

- 结果：核心校验、token 估算、根 module 和 agenthooks module 全部通过；vet、依赖整理和 diff 检查通过。CLI 的 A1 仍等待 T51 接入 `style validate`，不在本条提前标记。
- 下一步：T14 模板/条件运行时语义，随后进入 T15。

### 2026-10-13 — STYLE 规范按主题拆分

- 变更：保留 `STYLE_DOCS/STYLE-spec.md` 为入口、范围说明和决定总览；新增包结构、规则、运行时语义、编译产物四篇章节文档。编号、章节顺序及原正文均保留；CLI 设计文档仍通过原规范入口链接。
- 验证：拆分时按原文标题边界切片，并在写回前检查各章节字符串拼接与原文完全相同；章节编号顺序和规范文档间导航目标检查通过。规范最大单文件从 827 行降至 243 行。

### 2026-10-13 — T14/B1.5 `[x]`

- 任务：实现独立的模板解析/渲染与条件归一化/匹配，并提交当前 T14 工作后继续回顾已完成项。
- 实现：`core/template` 提供 Parse/ParseExpression、不可变 Parts、静态/动态分类、Render/RenderStatic、默认值与 `\{{` 转义；引号内 `|`、`}}` 保留为默认文本。`core/spec` 的模板校验和 token lint 复用该解析器，移除未使用的旧表达式包装函数。
- 条件：`core/when` 提供 Normalize/Compile/Match、字段 AND/列表 OR、模型 glob 和 `**/`、effort 七档比较及独立 `open`、agent 默认范围、provider 永不匹配、缺失字段不匹配、FieldReader/FieldMap/FieldFunc。matcher 复制输入规则并预编译正则；JSON 往返保留显式空 selector 以及归一化后的 agent 范围。
- 提交前修正：新增测试先复现空 model/effort 在 JSON 往返后变为无条件规则，以及带 `agent_id`、类型名为 `main` 的子 agent 误命中主 agent 规则；修正后同一测试通过。主 agent 始终按 `agent_id` 是否缺失判定。
- 命令（仓库根；Go 缓存隔离并清理 `CODEX_HOME`、`CODEX_SANDBOX`）：

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
  gofmt -l core/template core/when core/spec/validate_template.go core/spec/validate_lints.go core/spec/validate_template_parser_test.go
  git diff --check
  git status --short --branch
  ```

- 结果：T14 定向测试、根 module/agenthooks module 全量测试和 vet 通过；依赖整理无差异，gofmt 和 diff 检查通过。agenthooks 源码及依赖未改动。原生 payload 字段提取和 runtime engine 接入留在 adapter/engine 任务，A10/A11 与 M1a/M1b 不提前标记。
- 证据：`core/template/template_test.go`、`template_edges_test.go`；`core/when/when_test.go`、`json_test.go`、`match_test.go`；`core/spec/validate_template_parser_test.go`。
- 下一步：完成已实现 T10–T14 的回顾结论和待修正条目；按用户要求停止功能推进，不进入 T15。

### 2026-10-13 — REVIEW-001 `[x]`（发现项 `[!]`）

- 任务：完成 T10–T14 的回顾，检查架构边界、测试证据、规范拆分和输入边界；按用户要求在回顾后停止功能推进。
- 结论：模块边界和规则合并逻辑保留；没有发现 P0/P1，但发现 R01–R08 八个 P2 问题，集中在 YAML 完整消费、来源后缀、非有限数字、非字符串键、native selector 事件判断、skill `at`、model glob 静态检查和单文件 native schema。
- 复现：`docs/review/M1-2026-10-13-probe.go.txt` 是观察探针源文件，`docs/review/M1-2026-10-13-results.json` 是在提交 `ff32b99` 工作树上生成的结果。探针观察到 R01/R02/R04/R06/R07 的 validate 无诊断，R03 的 JSON 编码分别报 `NaN`/`+Inf`，R05 报错误的 `condition-field-wrong-event`，R08 目录包通过而单文件包报 `unknown package field "codex"`。
- 验证：T14 定向测试、`make test-go`、`make vet-go`、`go mod tidy -diff` 和 `git diff --check` 均通过；这说明已有覆盖范围稳定，不代表上述无效输入已被拒绝。
- 复现命令：从根目录将探针复制到 `/tmp/style-cli-m1-review-probe.go`，使用 T14 同一隔离环境执行 `go run /tmp/style-cli-m1-review-probe.go`；完整命令见报告。重复运行输出与存档结果逐字节一致；报告内源码链接及 13 个观察/8 个发现编号检查通过。
- 文档：新增 `docs/STYLE-M1-REVIEW-2026-10-13.md`，并在任务清单、测试矩阵加入 `[!]` 跟进项；历史 T11/T13 `[x]` 状态保留。规范拆分后的最大实际文件为 242 行，先前进度中的 243 行是 EOF 空行计数差异。

### 2026-10-13 — T21/B1.8 `[x]`

- 任务：完成 `core/tokens` 的离线字符/token 估算、预算分析和 T001–T007 报告，供 `plan`、后续 `compile` 和 CLI 重用。
- 实现：`internal/tokenestimate` 统一公式、Unicode rune 数、安装期静态值和动态占位符；`core/spec.Validate` 的 T001 按最终模型输入估算 warn 首句/`model_text`，遵循包预算与规则 `max_tokens` 优先级及 `budget.on`。`core/tokens` 在 `plan.Report` 上按规则、always-on 和 adapter Layout 的原生事件/工具分组计算估算；block 与 inject/warn 输出互斥时取最坏值，按运行时分隔符计费；跨已安装包 T003 支持 token/字符单位并严格按超过 80% 报错。T005–T007 复用共享 spec 诊断，报告不包含正文。
- 定向命令：

  ```sh
  env -u CODEX_HOME -u CODEX_SANDBOX \
    GOPATH=/tmp/style-cli-gopath \
    GOMODCACHE=/tmp/style-cli-gomodcache \
    GOCACHE=/tmp/style-cli-gocache \
    GOPROXY=off GOSUMDB=off \
    go test -count=1 ./core/tokens ./core/spec ./internal/tokenestimate
  ```

- 全量命令：`GOPROXY=off GOSUMDB=off make test-go`、`GOPROXY=off GOSUMDB=off make vet-go`、隔离环境 `go mod tidy -diff`、`gofmt -l core/tokens core/spec/validate_lints.go core/spec/validate_lints_test.go internal/tokenestimate/estimate.go internal/tokenestimate/estimate_test.go`、`git diff --check`。
- 结果：T21 定向测试、根 module 与 agenthooks module 全量测试和 vet 均通过；`go mod tidy -diff` 无输出；gofmt 与 diff 检查通过。新增测试覆盖字符/占位符、预算边界、warn 与 block 计费、native hook grouping、跨包 80% 边界、lint 传播、报告稳定性和正文不泄漏。`agenthooks/` 未修改。
- 下一步：T22 定义适配器一致性接口；T003 的已安装包清单和 provider 上限将在后续 install/adapter 集成接入。按用户要求，review 文档等 T2X 完成后统一更新。
- 状态：T14 已提交 `ff32b99`；T15、M1a、M1b 和 CLI/engine/adapter 集成仍未完成。下一步应先处理 R01–R08，再由用户决定后续规划。

### 2026-10-13 — RV-T11-01/R01 `[x]`

- 修复：使用 YAML decoder 要求单一 mapping 文档，第二次解码必须到 EOF；第二文档、空文档及非法尾部统一返回 `invalid-yaml`，保留来源路径。
- 回归：`core/spec/frontmatter_test.go` 新增完整文档测试与目录包入口测试。修复前第二文档/非法尾部与 Load 测试失败，修复后通过；结束标记和尾部注释保持合法。
- 验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./core/spec`；`git diff --check`；均通过。
- 状态：任务、测试矩阵和回顾报告同步标记 R01 完成；继续 R02，不进入 T15。

### 2026-10-13 — RV-T11-02/R02 `[x]`

- 修复：显式 from 与 glob 命中结果使用同一来源检查，只接受 rules/ 下的 .md 文件；非 Markdown 与目录外来源返回 `invalid-schema`。validate 再次检查来源，避免手工构造 Package 绕过约束。
- 回归：`core/spec/from_source_test.go`；修复前四个加载场景失败，修复后通过。现有递归 glob、排序和路径安全测试继续通过。
- 验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./core/spec`；`git diff --check`；均通过。
- 状态：任务、测试矩阵和回顾报告同步标记 R02 完成；继续 R03。

### 2026-10-13 — RV-T13-01/R03 `[x]`

- 修复：数字类型检查统一拒绝非有限值，vars scalar 使用相同检查。数字 defaults 原先被字符串枚举检查误拒绝，现只对枚举字段检查字符串取值。
- 回归：`core/spec/validate_numbers_test.go` 覆盖 .nan/.inf/-.inf 在 package priority、defaults、规则、from 覆盖、budget 三字段和 vars 的行为；修复前复现漏检，修复后全部拒绝。合法有限数字和 defaults 通过 validate，合并字段可 JSON 编码。
- 验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./core/spec`；`git diff --check`；均通过。
- 状态：任务、测试矩阵和回顾报告同步标记 R03 完成；继续 R04。

### 2026-10-13 — RV-T13-02/R04 `[x]`

- 修复：验证 manifest/metadata、规则与 from 覆盖中的所有嵌套 mapping；非字符串键报 `invalid-schema`。asStringMap 遇到非字符串键返回 nil，停止静默保留部分字段。
- 回归：`core/spec/validate_map_keys_test.go` 覆盖 vars、reads、budget、defaults、conditions、native 嵌套 data 和 from 覆盖；合法字符串键保持不变。将新增测试放入 `fc9243a` 的临时 archive，确认 vars/reads/budget/defaults/native/转换场景失败；修复后通过。
- 验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./core/spec`；`git diff --check`；均通过。
- 状态：任务、测试矩阵和回顾报告同步标记 R04 完成；继续 R05。

### 2026-10-13 — RV-T13-03/R05 `[x]`

- 修复：target tool 的 native `names` 选择器可用于 tool.added 可用性判定，但仅限规则实际挂载的 pre/post 事件。core 不复制 harness 名称或行为表；具体字段提取由 adapter 决定，字段缺失按 runtime matcher 语义不匹配。
- 回归：`core/spec/validate_condition_test.go` 新增原生 `Edit` 单名和名字列表两例；修复前均出现 `condition-field-wrong-event`，修复后通过。原有 shell/file.write/mcp 正负例通过。
- 验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./core/spec`；`git diff --check`；均通过。
- 状态：任务、测试矩阵和回顾报告同步标记 R05 完成；继续 R06。

### 2026-10-13 — RV-T13-04/R06 `[x]`

- 修复：skill 规则根据有效 `at` 推导隐式 `tool.pre`/`tool.post`，未指定时沿用 action 默认（block=before，其余=after）；`skill.explicit` 独立可用，不再因此错误放宽隐式工具字段。
- 回归：`core/spec/validate_condition_test.go` 验证 skill before + tool.output 被拒绝、after + tool.output 接受，以及 before + prompt 可由显式触发点使用。修复前第一例未报错，修复后通过。
- 验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./core/spec`；`git diff --check`；均通过。
- 状态：任务、测试矩阵和回顾报告同步标记 R06 完成；继续 R07。

### 2026-10-13 — RV-T13-05/R07 `[x]`

- 修复：`core/when.ValidateModelGlob` 暴露 model glob 语法检查并被 Compile 复用；spec validate 对单值/列表逐项调用同一实现，无效模式输出稳定 `bad-model-glob` 诊断。
- 回归：`core/spec/validate_model_glob_test.go` 覆盖未闭合 `[` 的多种位置和合法普通/`**/` glob；修复前非法模式都无诊断，修复后符合预期。既有 when compile glob 测试通过。
- 验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./core/spec ./core/when`；`git diff --check`；均通过。
- 状态：任务、测试矩阵和回顾报告同步标记 R07 完成；继续 R08。

### 2026-10-13 — RV-T13-06/R08 `[x]`

- 修复：单文件包顶层 schema 现在仅在 `target: native` 时把 `codex`/`claude-code` 识别为 native rule 字段，与目录包规则校验一致；其他未知字段仍报错。
- 回归：`core/spec/validate_native_single_file_test.go` 使用相同 native rule 字段验证目录包及单文件包。修复前单文件报 `unknown package field "codex"`，修复后两者均无错误。
- 验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./core/spec`；`git diff --check`；均通过。
- 状态：R01–R08 已全部修复；任务、测试矩阵和回顾报告均标为完成。开始最终全套验证，完成后等待用户指令，不进入 T15。

### 2026-10-13 — R01–R08 最终验证与暂停

- 完成：八项修复均有回归测试；review 的修复跟进表、任务清单、测试矩阵均为 `[x]`。R08 的共享 native 字段判断和测试覆盖 Codex、Claude、两者共存、未知字段、非 native provider 字段以及错误 hook 类型，目录包与单文件包结果一致。
- 最终验证：同一隔离环境运行 `go test -count=1 ./core/spec ./core/when ./core/template ./internal/tokenestimate`；`GOPROXY=off GOSUMDB=off make test-go`；`GOPROXY=off GOSUMDB=off make vet-go`；`go mod tidy -diff`；gofmt 和 `git diff --check`。全部通过。
- 探针对照：按回顾报告中的复制和 `go run` 命令重新运行 13 个场景，输出保存为 `docs/review/M1-2026-10-13-fixed-results.json`。保留原 results 作为修复前记录；R03 无效包被校验拒绝，探针刻意继续 Merge 导致的 JSON 错误也保留说明。
- 边界：本次没有改 agenthooks 源码或依赖，不提前完成 adapter/engine/CLI、M1a/M1b。按用户要求，完成修复后停止并等待指令。

### 2026-10-13 — GO-CLI-DESIGN-001 `[x]`

- 任务：评估 Go module、STYLE core、CLI 设计和 `agenthooks` 边界，形成高维护性设计建议。
- 结论：采用模块化单体；根 module 保留 CLI 和 STYLE core，`agenthooks/` 保持独立 module；CLI 只处理参数和渲染，`internal/app` 编排用例，core 保持 harness 无关，adapter 按调用方拆成窄接口。
- 文档：新增 `docs/STYLE-GO-CLI-DESIGN-2026-10-13.md`，记录当前差距、目标目录、依赖方向、module 发布策略、输出/错误契约、事务、测试矩阵和 P0–P3 顺序。
- Review：`docs/STYLE-M1-REVIEW-2026-10-13.md` 已追加本项跟进并标记 `[x]`。
- 外部参考：登记 Go 官方模块与代码评审文档、Cobra 与 GitHub CLI 项目入口；本次执行环境 DNS/TLS 访问失败，未声称已实时核验网页。评估以仓库源码和已有设计为基础。
- 范围：只完成设计评估和文档记录；不开始 T15、不修改 `agenthooks` 源码，等待用户下一步指令。

### 2026-10-13 — T15/B1.6 `[x]`

- 实现：新增 `core/warn.FirstSentence`，跳过开头空行并仅检查第一个非空行；`。！？` 直接截断，ASCII `.!?` 仅在后接 Unicode 空白或行尾时截断；无标点取整行，空正文返回空串。调用方传入渲染后的文本，因此动态模板值与静态正文走同一逻辑。
- 回归：`core/warn/sentence_test.go` 覆盖中英文标点、ASCII decimal period、空白/行尾边界、前导空行、多行、无标点、CRLF、空内容及 `core/template` 动态渲染后提取。
- 验证：隔离环境下 `go test -count=1 ./core/warn ./core/template`、`GOPROXY=off GOSUMDB=off make test-go`、`GOPROXY=off GOSUMDB=off make vet-go`、`go mod tidy -diff`、gofmt 和 `git diff --check` 均通过；代码、测试和状态文档按 T15 独立提交。
- 文档：T15、A21、B1.6 已在任务清单/测试矩阵标记 `[x]`，review 报告新增 T15 完成项；静态编译与动态运行时的接线仍属于后续 compile/engine 任务。

### 2026-10-13 — T20/B1.7 `[x]`

- 实现：新增 `core/plan`，将合并规则转换为 adapter-independent `Requirement`，提取事件、能力、条件字段和模板运行时数据需求；`Build` 通过窄 `Adapter` 事实接口生成确定的 `Report`。核心不导入 `agenthooks` 或 provider 包。
- 策略：实现 system 静态位置与动态双评估点、tool/MCP/skill/event 路由、备用 before/after、静态注入回退、warn 部分能力、block→warn、fallback `skip`/`error`、条件数据全缺失的 never-trigger、adapter blind spot、native M2 跳过和 harness selector。`mode: replace` 无静态位置时按规范跳过或阻止安装。
- 报告：结构化 placement 只保留事件、动作、状态、能力、降级来源、缺失数据和警告；人类可读/JSON 报告不写入规则正文或内部静态前缀，序列化结果稳定。静态 fallback 的前缀留在内存 placement，供后续 compile 消费。
- 回归：`core/plan/plan_test.go` 覆盖能力/需求、静态与动态 system、fallback 阶梯、skill 隐式+显式评估点、条件数据替代、空静态槽位、replace、harness 过滤、可选模板变量、fallback:error 和 Load→Validate→Merge→Build 集成。
- 验证：隔离环境 `go test -count=1 ./core/plan ./core/spec ./core/template ./core/when`、`GOPROXY=off GOSUMDB=off make test-go`、`GOPROXY=off GOSUMDB=off make vet-go`、`go mod tidy -diff`、gofmt 和 `git diff --check` 均通过；`agenthooks/` 无源代码修改。
- 状态：T20、B1.7 已在任务清单和测试矩阵标记 `[x]`。按用户要求，本次不更新 review 文档；待 T2X 阶段完成后统一回顾 T20 系列。A2 CLI golden、token 预算、compile/engine 和 M1a 仍未完成。

### 2026-10-14 — T22/B1.9 `[x]`

- 实现：新增 `internal/adapter` 适配器端口和显式 `Registry`。`ProviderID`/compact event 直接复用 `agenthooks`；检测、能力、数据来源、盲区、静态位置、路线、trust、changeset、输入上限及超限行为均按使用方拆成窄接口或明确 DTO。`Definition` 将这些端口组合起来，注册时校验 provider/route 唯一性、上限单位和 compact 元数据，并按稳定 provider 顺序查找；`BuildPlanAdapter` 将丰富的 provider facts 投影为 `core/plan.Adapter`。
- 边界：`core` 只依赖 `core/plan`、`core/spec` 和 `core/tokens` 的抽象，不导入 `agenthooks` 或 `internal/adapter`；T22 没有复制 hook codec、事件解析或安装渲染。具体 Codex/Claude 实现、effort 映射和 runtime Enrich 留给 T41–T45。
- 回归：`internal/adapter/registry_test.go` 覆盖注册确定性、查找复制、定义校验、检测、路由生命周期和规划事实投影；`boundary_test.go` 扫描 core imports，防止 provider 依赖倒灌。
- 验证：隔离环境下 `go test -count=1 ./internal/adapter ./core/plan ./core/tokens`、`go vet ./internal/adapter`、`GOPROXY=off GOSUMDB=off make test-go`、`GOPROXY=off GOSUMDB=off make vet-go`、`go mod tidy -diff`、gofmt 和 `git diff --check` 均通过；`agenthooks/` 无修改。任务清单 T22 与测试矩阵 B1.9 已标记 `[x]`；按用户要求暂不更新 review 文档，待 T2X 完成后统一回顾。

### 2026-10-14 — T23/B1.10 `[x]`

- 实现：新增 `core/compile`，`Build` 消费已校验/合并的包、安装顺序、settings、lock identity 和按 harness 选定的 plan，输出纯内存 `Index` 及静态托管正文；`Encode` 生成缩进、稳定键序和尾换行的 JSON。编译阶段不重新跑 fallback 策略、不读磁盘，source hash 使用 `spec.Load` 已读取的内存文件字节。
- 编译内容：生成静态条目与 event entries，归一化 match/when，写入 package/rule/order、来源和正文 SHA256、repeat/session sources/reeval/needs、body 静态文本或运行时片段及 placement 估算；`warn` 支持显式 `model_text`，隐式动态正文在运行时渲染后提取首句。为保持 index 格式完整，拒绝缺失静态值、来源字节、非法 plan、评估点和静态 slot；静态列表保留重复 rule 的独立记录且确定排序。
- 规范：更新 `STYLE_DOCS/STYLE-spec-compiled.md`，明确 `order.priority` 可为有限小数，并定义 `model_body_first_sentence` 的动态处理时机。
- 回归：`core/compile/build_test.go` 覆盖静态和动态内容、安装 settings、默认值、skill/tool 选择器、条件规范化、warn、hash、fractional priority、多包确定性、JSON 往返、HTML 字符及输入拒绝边界。
- 验证：隔离环境下定向 `go test -count=1 ./core/compile ./core/plan ./core/tokens ./core/spec ./core/when ./core/template ./core/warn ./internal/tokenestimate`、双 module `GOPROXY=off GOSUMDB=off make test-go` 和 `make vet-go`、`go mod tidy -diff`、gofmt、空白检查全部通过；未修改 `agenthooks/`。
- 状态：任务清单 T23、测试矩阵 B1.10 已标为 `[x]`。A3 的 example schema/golden 继续留在 T24；runtime 接线留给 T32；按用户要求 review 文档不在 T23 单独改动，待 T2X 完成后统一回顾。

### 2026-10-14 — T24/B1.11 `[x]`

- 实现：新增 `example/terse-reviewer/` 可重复的 STYLE 示例包、`schema/index.schema.json`（Draft 2020-12）和 `testdata/golden/index.json`。编译测试从包加载、校验、合并、规划并生成 index，直接调用 `github.com/google/jsonschema-go` 验证正式 schema，再逐字节比较 golden；schema 将 session 来源、事件 selector、warn model body、body 编码、哈希、estimate 和未知字段约束固化。
- 回归：无效文档用例覆盖必需字段、未知事件、session 缺来源/错带 match、prompt 错带来源、tool 缺 selector、未知 entry 字段；有效边界覆盖负 priority、重复 sources。编译器现在拒绝格式错误或大写的 package SHA256 lock，避免生成不符合 schema 的 index。
- 文档：修正已跟踪 spec 文档中示例包相对链接；CLI design 文件在仓库 `.gitignore` 下保持非跟踪状态，不纳入本次提交。
- 验证：隔离环境 `go test -count=1 ./core/compile`、`GOPROXY=off GOSUMDB=off make test-go`、`GOPROXY=off GOSUMDB=off make vet-go`、`go mod tidy -diff`、gofmt 和 `git diff --check` 全部通过；没有修改 `agenthooks/`。
- 状态：任务清单 T24、测试矩阵 B1.11 和离线验收 A3 标记 `[x]`。按用户要求，本条仍不单独写回顾文档；T2X 完成后统一审查 T20–T24。

### 2026-10-14 — RV-T2X-01/RV-T2X-02 `[x]`

- 回顾范围：T20 `core/plan`、T21 `core/tokens`、T22 adapter contracts、T23 `core/compile`、T24 index schema/golden。总体分层合理；未发现 P0/P1，也没有理由重写 core/adapter 边界或修改 `agenthooks/`。M1a/M1b、runtime 与真实 harness 验收仍未完成。
- RV-T2X-01 修复：合法的 `target: event` + `event: tool.pre` 原先编译为没有字段的空 `match`，与 schema 要求冲突。Entry 现在保留原 `target`；schema 按 target 区分带 selector 的 routed tool/MCP/skill entries 与覆盖整类工具的 event entries。更新 golden 与规范。`core/compile/golden_test.go:TestEventTargetAtToolPointCompilesWithoutMatch` 端到端生成 JSON 并通过 Draft 2020-12 schema 验证。
- RV-T2X-02 修复：Validate 原先接受空 tool、native-name、skill selectors，及无 slash/多 slash/空组件/首尾空白 MCP selector。现在 Validate 拒绝空列表、空白值和不符合单 `server/tool` 形状的 MCP selector；Compile 同步保留防御性检查，index schema 强制 target-match 对应、非空列表、selector trim 和 MCP 组件不含额外 `/`。`core/spec/validate_edge_test.go` 覆盖无效输入和合法 wildcard；`core/compile/golden_test.go:TestNormalizedMatchRejectsInvalidSelectors` 及 schema 负向用例覆盖拒绝路径；STYLE 规范已同步。
- 定向验证：隔离缓存环境下 `go test -count=1 ./core/spec ./core/compile ./core/plan ./core/tokens ./core/when ./core/template ./core/warn ./internal/tokenestimate ./internal/adapter` 通过。
- 全量验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off make test-go` 与同环境 `make vet-go` 通过，覆盖根 module 和 `agenthooks` nested module；`gofmt` 和 `git diff --check` 通过。此次没有修改 `go.mod`/`go.sum`；单独运行 `go mod tidy -diff` 未通过，因为临时 module cache 缺少 `gopkg.in/check.v1` 与 `github.com/google/go-cmp` 且 GOPROXY 关闭，既有 T24 记录中的 tidy check 曾通过。
- 文档：T20–T24 结论和两个已完成修复已写入 `docs/STYLE-M1-REVIEW-2026-10-13.md`；任务清单和测试矩阵增加 RV-T2X-01/02 `[x]` 及可重复回归证据。此项代码、测试与状态文档一起独立提交，提交 hash 由交付回复记录。
- 状态：按用户要求完成回顾后暂停；不继续 T30 或其他 M1 功能。

### 2026-10-14 — T30–T34/B1.12–B1.16 `[x]`

- 实现：`core/state` 持久化 session/scope、来源、gen/epoch/prompt_count 和 model/effort/version；使用 0700 目录、0600 文件、严格 JSON、临时文件加 rename 原子写、过期清理和安全 ID 路径组件。`core/state.Claim` 实现 `every`、`turn`、`session`、正文 SHA256 前缀、compact epoch、block/subagent.start 豁免与 `O_EXCL` 预留/释放。
- 引擎：`core/engine` 在加载前校验 index schema，限制 index 到 16 MiB，strict decode 并预编译条件 matcher；逐点匹配 target/source/when，渲染动态模板，执行 block 优先、inject/warn 合并、输入上限裁剪和占位释放。`internal/runtime` 将 agenthooks typed event 交给 provider-neutral `RuntimeEnricher` 与 engine，并只写入有界元数据 stats 和安全错误码日志。
- agenthooks 边界：新增 Claude prompt block 的 `suppressOriginalPrompt` codec 字段及 Claude tool-pre fail-closed 可配置退出码；runtime 不复制 stdin/event codec。其他 provider 行为不变，相关输出仅由允许的字段组成。
- 动态 system：`session.start` 与 `prompt.submitted` 双挂载；根据持久 model/effort 状态渲染，正文未变时去重、改变时补发；compact 只让 `reinject_on_compact` 条目重发。测试使用 fake `RuntimeEnricher`，实际 Codex/Claude 字段映射留在 T41/T42/T44/T45。
- 回归：state 测试覆盖 lifecycle、startup reset、resume/clear/compact、私有权限、严格解码、清理、重复键、正文变化和并发 claims；engine 测试覆盖 schema/index、target/condition/source、block、warn/inject 顺序、cap release、动态 system、白名单及 20 个独立进程恰好一个重复输出；runtime 测试覆盖 Codex/Claude fixture 精确 stdout、敏感输入不进入输出/log/stats、损坏 index/state fail-open 与 Claude fail-closed、model/effort/compact 重评估。
- 验证（均隔离临时 Go 缓存、移除 `CODEX_HOME`/`CODEX_SANDBOX`，关闭网络依赖获取）：
  - `env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./...` 通过。
  - `env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -C agenthooks -count=1 ./...` 通过。
  - `env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -race -count=1 ./core/state ./core/engine ./internal/runtime ./schema` 通过；新增 state-error 回归用例随后定向 `go test -count=1 ./internal/runtime` 通过。
  - `GOPROXY=off GOSUMDB=off make vet-go` 通过；gofmt 与 `git diff --check` 待本次文档修改后最终复核。
  - `go test -count=1 -v ./core/engine -run '^TestEngineProcessLatencyP95Below50ms$'`：200 样本 p95 `128.083µs`，低于 50ms；`go test ./core/engine -run '^$' -bench '^BenchmarkEngineProcessIndexOnly$' -benchtime=300x -count=1`：`33,567 ns/op`、`81,402 B/op`、`166 allocs/op`。
- 状态：T30–T34、A4–A9/A15/A17/A19/A20、A16 标记完成；A10/A11 记为部分完成，等待具体 provider adapter；任务清单、测试矩阵和回顾文档同步更新。M1a/M1b 未完成，继续 T40。

### 2026-10-14 — T40/B1.17 `[x]`

- 实现：在 `agenthooks/` 内完成 F1–F4。Codex canonical shell matcher 现在生成原生 `Bash`；`Manifest.Owner` 默认兼容 `agenthooks`，STYLE 可使用 `style`，所有生成命令带 `--owner=<owner>`。JSON 合并/卸载按精确 owner 删除 handler，保留同组 sibling、其他 owner 和无标记外部配置；Kimi/Codex TOML 使用 owner marker，新增 `install.Uninstall` 清理受管内容和未修改的独立生成文件。
- Codex plugin：新增 `Target.ConfigDir`、`Identity.PluginID` 和插件渲染路线，生成 `.codex-plugin/plugin.json`、`hooks/hooks.json`、POSIX 可执行 `bin/style`，信任预置写入 CODEX_HOME `config.toml`，插件键为 `<plugin_id>:hooks/hooks.json:<event>:<group>:<handler>`，`DefinitionHash` 以实际命令和 timeout 计算。
- 回归：`agenthooks/install/install_test.go` 覆盖插件布局、0755 启动器、长 trust key/hash、owner 隔离、同组 handler、JSON/TOML 卸载、外部配置和重复安装幂等；`agenthooks/matcher_test.go` 覆盖 Codex `Bash`；`agenthooks/detect_test.go` 覆盖 owner argv；`agenthooks/e2e/codex_test.go:TestCodexCanonicalShellMatcher` 加入真实 Codex opt-in 场景。
- 验证（清理 `CODEX_HOME`/`CODEX_SANDBOX`，临时 Go 缓存，关闭网络依赖获取）：
  - `go test -count=1 -C agenthooks ./install .` 通过。
  - `go test -race -count=1 -C agenthooks ./install .` 通过。
  - `go test -count=1 -C agenthooks ./e2e` 通过（真实环境测试默认跳过）；`-tags e2e -run '^$'` 编译通过。
  - `GOPROXY=off GOSUMDB=off make test-go` 与 `make vet-go` 通过。
  - `git diff --check`、gofmt 通过。
- 状态：任务清单 T40、测试矩阵 B1.17 和回顾文档 T40 段落已标记 `[x]`。A13、A14 及 M1b 真实 Codex 插件/信任/路线切换仍未提前完成；下一项按依赖顺序为 T41。

## T41 / B1.18（2026-10-15）

- 状态：T41 F5–F6、测试矩阵 B1.18 和回顾文档 T41 段落已标记 `[x]`。`SessionInfo` 新增 effort/raw/approx/harness version；Claude codec 保留 `effort.level` 原值；`internal/adapter/effortmap` 提供嵌入式 YAML 映射、七档/别名和未知值语义；Codex adapter 按 `-c` → profile → 受信项目 → 用户配置解析 `model_reasoning_effort`，Claude adapter 按事件 → `CLAUDE_EFFORT` → session 状态解析；state/session JSON 持久化 `effort_approx` 和 `effort_resolved`，runtime 投影 effort/raw/approx/version 变量。
- 定向验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test ./internal/adapter/... ./core/state ./core/engine ./internal/runtime ./agenthooks` 通过。
- 全量验证：`GOPROXY=off GOSUMDB=off make test-go`、`GOPROXY=off GOSUMDB=off make vet-go`、`git diff --check` 通过。新增 fixture 覆盖 Claude event/env/session 优先级、Codex 配置优先级与不可信项目、未知 effort raw 保留、近似值 tri-state 和一次性解析。
- 边界：真实 provider hook 的 Enrich 接线和版本检测仍由 T44/T45 完成；因此 A10 维持 `[~]`，M1a/M1b 不提前完成。

### 2026-10-15 — T42/B1.19 `[x]`

- F7：在 `agenthooks/` 增加 `SubagentStartDecision.WithContext` 和兼容性独立的 `OnSubagentStartDecision`，保留原 `OnSubagentStart` 观察回调；仅 Codex `subagent.start` 宣告 `add-context` 并编码为 `hookSpecificOutput.additionalContext`。Runner 回归从 Codex payload 验证 agent id/type 到决策 handler、精确 JSON 输出，且旧 observer 仍运行。
- F8：`SkillActivationOf` shell recognizer 支持 `head`、`tail`、`bat`、`cd … && cat SKILL.md`，保留 `/bin/cat`、`/usr/bin/head` 等 basename 兼容并覆盖行数参数；新增 Codex `ExplicitSkills`，遵循 `$name` 正则、首次顺序和去重。`skills.read` 等待 U6；skill evaluation point 和 state scope 的运行时接线由 T44 完成。
- F9：Claude native codec/view 增加 MCP server `name/source`、UserPromptExpansion 和 PostModelSwitch；prompt_id、effort、fork source 与 `to_model` 投影有 fixture；prompt block 的 `suppressOriginalPrompt` golden/runtime 输出更新到 `hookSpecificOutput`。
- 回归：`agenthooks/runner_test.go` 新增 Codex 子 agent context/旧 observer 和 Claude prompt expansion 的 Runner 级精确输出测试；`agenthooks/codec_test.go`、`agenthooks/provider/claudecode/claudecode_test.go`、`agenthooks/claude_skill_test.go`、`internal/adapter/codex/skill_test.go` 覆盖字段、解析和方言边界。
- 验证：`GOPROXY=off GOSUMDB=off make test-go` 和 `GOPROXY=off GOSUMDB=off make vet-go` 通过，覆盖根 module 和 `agenthooks` nested module。环境清理 `CODEX_HOME`/`CODEX_SANDBOX`，使用 Makefile 配置的临时 Go 缓存；gofmt、`git diff --check` 通过。
- 文档：任务清单 T42、测试矩阵 B1.19 和 review 文档 T42 段落均已标记 `[x]`。A11 保持 `[~]`：实际 engine Enrich/scope 投影留 T44，Codex 原生 delivery 留 U7/M1b。M1a/M1b 未完成。

### 2026-10-15 — RV-T41-01 `[x]`

- 回顾修正：`internal/adapter/codex/effort.go` 直接使用 `github.com/pelletier/go-toml/v2`，根 `go.mod` 已从 indirect 调整为 direct dependency。
- 验证：`GOPROXY=off GOSUMDB=off go mod tidy -diff` 与 `go -C agenthooks mod tidy -diff` 通过；未改变运行时代码或测试行为。

### 2026-10-15 — T43/B1.20 `[x]`

- 实现：Claude Code renderer 使用 exec schema，`Manifest.Command[0]` 写入 `command`，其余可执行参数与 `hookArgs` 按原顺序写入 `args`，不经过 shell join；timeout 继续使用既有 seconds 转换。`Target.HarnessVersion` 用于 fail-closed 版本门槛。
- Fail-closed：只有 blocking `KindToolPre` 且 Manifest `FailClosed` 时输出 `onFailure: "block"` 并在 argv 加 `--fail-closed`；未知/低于 2.1.295/格式错误版本会拒绝该配置。fail-open、telemetry 与非阻断 pre-tool hook 不触发版本要求。
- Owner 兼容：JSON owner 扫描现在同时识别旧 `command` 文本及 exec `args` 中的 `--owner=...`；重复安装和卸载测试证明能替换/删除 STYLE handler 并保留外部条目。Claude 降级或当前版本缺失时，卸载仍按 owner 清理，不被新安装的版本门槛阻塞。
- 验证：`go -C agenthooks test ./install -count=1`、`GOPROXY=off GOSUMDB=off make test-go`、`GOPROXY=off GOSUMDB=off make vet-go`、gofmt 与 `git diff --check` 通过。测试使用临时 Go 缓存，并清理 `CODEX_HOME`、`CODEX_SANDBOX`。
- 文档：任务清单 T43、测试矩阵 B1.20、review 文档 T43 段落均标记 `[x]`，`agenthooks/DESIGN.md` 同步 exec 与 fail-closed 规则。T45 仍需把真实检测到的 Claude 版本接入 `Target.HarnessVersion`；CLI 当前尚无 `--fail-closed` 到 `internal/runtime.Load` 的命令解析接线，留后续 CLI 任务做端到端验证；实际 Claude 环境验收保留 M1b，M1a/M1b 未标记完成。

### 2026-10-15 — T44/B1.21 `[x]`

- 实现：新增 Codex adapter 定义和固定 hook surface（SessionStart、UserPromptSubmit、Pre/PostToolUse、SubagentStart），提供 plugin 与 user routes、Codex facts、`developer_instructions` root static slot 和配置 effort/runtime enrichment。Codex route 在 `agenthooks/install` 共用安装层新增的 `PlanFiles`、`PlanUninstallFiles`、`ApplyFiles` 上组合 hook/trust 配置及 static blocks，不另造 hook renderer 或 owner 生命周期。
- runtime：按信任判定解析 Codex launch `-c`、profile、trusted project 和 user config effort，缓存 session facts；把 prompt、显式 `$skill`、agenthooks 识别的隐式 skill、MCP identity、nested tool input/output 与 apply_patch added lines 映射为 engine point/fields/template values。`Runtime.Register` 接入 `SubagentStartDecision`，保留 agent id/type 的子 agent scope，并通过 Codex HSO codec 发 context。
- 回顾修正：测试发现并修正 static-only plan/remove 丢失、嵌套路线根目录选错、native selector blind-spot 判断遗漏、隐式 skill 数据源声明遗漏、复合 JSON 输入模板值遗漏、未确认 plugin route 误报 trusted 和 SubagentStart runtime callback 缺失。定向测试覆盖这些边界，且 `ApplyFiles` 在写入前核对全部 Before 快照以拒绝 stale changeset。
- 验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./...`、同环境 `go test -C agenthooks -count=1 ./...`、`GOPROXY=off GOSUMDB=off make vet-go` 和 `go test -race -count=1 ./internal/adapter/codex ./internal/runtime ./agenthooks/install` 均通过；gofmt 与 `git diff --check` 通过。
- 状态：任务清单 T44、测试矩阵 B1.21 和回顾文档 T44 段落均标记 `[x]`。A10/A11 因 Claude adapter/真实 Codex 投递保留 `[~]`；T45、CLI 生命周期和 M1a/M1b 验收仍未完成。

### 2026-10-15 — T45/B1.22 `[x]`

- 实现：新增 Claude adapter 的动态 surface、Facts/Detector/Definition、用户级 settings route 和 `CLAUDE.md` 标记块托管。surface 按 native event+matcher 合并并稳定排序，PostModelSwitch 作为原生观察事件保留；settings 复用 `agenthooks/install` 的 exec renderer、owner 合并、卸载和 stale snapshot 检查。静态块保留外部和用户字节，malformed marker 拒绝，workspace trust 只报告为 `TrustWorkspace`，不从文件存在性伪报已信任。
- runtime：新增 Claude Enricher，按 event→`CLAUDE_EFFORT`→session 解析 effort，支持后续事件覆盖已解析状态；映射 prompt、UserPromptExpansion 显式 skill、Edit/Write `tool.added`、MCP、隐式 Skill、tool output、nested input 和 agent scope。Runtime 注册 `OnPromptExpansion`；engine 仅在普通 `prompt.submitted` block 时设置 Claude `suppressOriginalPrompt`，避免显式 skill 使用不支持的字段。
- 证据：`internal/adapter/claude/{adapter_test.go,route_test.go,runtime_test.go}`、`internal/runtime/runtime_test.go:TestClaudePromptExpansionRuntimeUsesAdapterAndRunnerHandler`、`agenthooks/install/install_test.go`。
- 验证：隔离环境下根 module `go test -count=1 ./...`、nested `agenthooks` `go test -count=1 ./...`、`GOPROXY=off GOSUMDB=off make vet-go`、`go test -race -count=1 ./internal/adapter/claude ./internal/runtime ./agenthooks/install`、gofmt 和 `git diff --check` 通过。
- 状态：任务清单 T45、测试矩阵 B1.22 和 review 文档 T45 段落标记 `[x]`；A10、A14 的离线部分完成，A11 保持 `[~]`，A12/A13/A18/A22、CLI T50–T55、adapter conformance 和真实 Codex/Claude CDEX/CLAUDE 场景仍未完成。

### 2026-10-15 — T50/B1.23 `[x]`

- 实现：新增 `internal/app` 的 `style init` 用例和薄入口，支持目录包、`--single` 单文件包、当前空目录初始化；生成内容直接经过 `spec.Load`、`Validate`、`Merge` 校验。名称限制为小写字母/数字/连字符，未传名称时从目录名归一化，已存在目标、非空当前目录和非法名称拒绝，写入失败回滚已创建内容，不覆盖用户文件；`--help` 成功退出且不改文件。
- 回归：`internal/app/init_test.go` 覆盖两种骨架、默认名称及目录名归一化、冲突/非空目录、非法名称、字节保留、稳定成功输出和 help 无副作用；`go run ./cmd/style init --help` 可独立执行。
- 验证：隔离 Go 缓存下 `go test -count=1 ./internal/app -run 'TestInit|TestRunInit'`、`GOPROXY=off GOSUMDB=off make test-go`、`GOPROXY=off GOSUMDB=off make vet-go`、gofmt 和 `git diff --check` 通过。
- 状态：任务清单 T50、测试矩阵 B1.23 和本回顾文档 T50 段落标记 `[x]`；T51–T55、A1/A2/A12/A13/A18/A22 和 M1a/M1b 仍未完成。

### 2026-10-15 — T51/B1.24/A2 `[x]`

- 实现：新增 `style validate` 与 `style plan` application/command 流程。validate 返回稳定诊断码、路径/规则/provider、severity 和消息；plan 支持显式/auto harness、多 provider 固定顺序、session 请求/触发假设，并汇总 provider route、native input limit、package budget、plan 降级/能力、逐规则和逐 native event token 估算及 cache-aware session cost。Codex/Claude bucket layout 映射 canonical 与 native tool selector，MCP 使用共享 native bucket。
- 回归：命令测试固定稳定文本、JSON 诊断、退出码、help 和参数错误；plan layout 测试固定 MCP 聚合、canonical/native selector 展开与 trigger 位置舍入；示例包对 Codex/Claude Code plan stdout 使用显式 harness 与逐字节 golden。
- 验证：隔离环境下 `go test -count=1 ./internal/app ./cmd/style`、根 module 与 nested `agenthooks` 的 `make test-go`、`make vet-go`、示例 `style validate` 及两个显式 harness `style plan` 命令、gofmt、`git diff --check` 通过。Go 缓存位于 `/tmp/style-cli-{gopath,gomodcache,gocache}`，`CODEX_HOME`/`CODEX_SANDBOX` 未设置。
- 文档：任务清单 T51、测试矩阵 B1.24/A2 与 review 的 T51 检查项已标记 `[x]`；修正 B1.7 对 A2 未完成的过时描述。证据：`internal/app/{validate.go,plan.go,command.go,command_test.go,plan_golden_test.go,plan_layout_test.go}`、`testdata/golden/plan-{codex,claude-code}.txt`、`docs/STYLE-M1-REVIEW-2026-10-13.md` T51。
- 状态：T52–T55、A1/A12/A13/A18/A22、adapter conformance 和 M1a/M1b 仍未完成，不提前标记。

### 2026-10-15 — T52/B1.25/A22 `[x]`

- 实现：新增 `style install` 生命周期编排。安装先读取并快照包字节，校验 `requires.runtime`、settings、replace 和已安装包冲突，再对所有已安装包按 provider 重新 plan/token/compile；生成锁文件、index、settings 和 adapter route changeset 后统一应用。`--yes` 是非交互确认门槛，`--link` 保留源路径和哈希，锁文件记录 route、surface、rendered、trust 和 fail-closed 状态。
- 事务：包存储使用 staging directory/file + backup rename；agenthooks `ApplyFiles` 在所有 Before 快照通过后逐文件原子替换，后续错误或 context cancellation 逆序回滚，保留已有文件权限。安装路线继续调用 adapter `Route.Plan`，不复制 agenthooks owner merge/uninstall 行为。
- 回归：`internal/app/install_test.go` 覆盖示例包安装、重复幂等、`--yes`、settings 必填及 0600、runtime 版本要求、replace 确认；`agenthooks/install/install_test.go` 覆盖多文件取消回滚。`TestInstallRequiresReplaceConfirmationAndCommandYes` 提供 A22 的命令级退出证据。
- 验证（清理 `CODEX_HOME`/`CODEX_SANDBOX`，使用临时 Go 缓存，关闭网络依赖获取）：
  - `env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./internal/app` 通过。
  - 同环境 `go test -count=1 -C agenthooks ./install` 通过。
  - 示例 `STYLE_HOME=$(mktemp -d) ... go run ./cmd/style install example/terse-reviewer --yes --json` 返回 0，并写入 package、lock、index 和 Codex/Claude route 文件。
  - `GOPROXY=off GOSUMDB=off make test-go`、`make vet-go`、gofmt 和 `git diff --check` 通过。
- 文档：任务清单 T52、测试矩阵 B1.25/A22 和 review 文档 T52 段落均已标记 `[x]`。T53–T55、A1/A12/A13/A18、adapter conformance 和 M1a/M1b 仍未完成。

### 2026-10-15 — T53/B1.26 `[x]`

- 实现：新增 `style uninstall <name|--all> --yes [--json]`。卸载从 lock 选择包，剩余包重新读取、plan/token/compile 并刷新 index、lock 和 provider route；卸载最后一个包时调用 route owner removal，并删除 STYLE lock/index/settings/本地包。link 包只移除 lock 和 STYLE 配置，不触碰源包。
- 持久路线：lock 的 harness 项保存安装时 `config_dir`；卸载从该路径重绑路线。install 换路线时也对旧路线使用已检测到的 installation facts，避免走另一个配置根目录。
- 配置往返：TOML managed region 不再把它自己的终止换行混入用户 suffix；无末尾换行的现有 TOML 会加一条生成的 separator 注释，卸载时一起移除。测试逐字节验证无换行、单换行、双换行外部内容。
- 回归：`internal/app/uninstall_test.go` 覆盖单包卸载、确认、保留剩余包、`--all --json`、link 源保留、最后一包的外部 config 精确恢复和状态文件清理；`agenthooks/install/install_test.go` 覆盖 TOML 字节往返和其他 owner 区保留。
- 验证（清理 `CODEX_HOME`/`CODEX_SANDBOX`，Go 缓存位于 `/tmp/style-cli-{gopath,gomodcache,gocache}`）：`go test -count=1 ./internal/app -run 'TestUninstall|TestRunUninstall'`、`go -C agenthooks test -count=1 ./install`、双 module `make test-go`/`make vet-go`、gofmt 和 `git diff --check` 通过。
- 文档：任务清单 T53、测试矩阵 B1.26 和 review 文档 T53 段落标记 `[x]`；A13 仍 `[~]`，缺 install route switch 无残留的独立命令级回归。T54/T55、A1/A12/A13 补项/A18、adapter conformance 与 M1a/M1b 未完成。

### 2026-10-15 — T54/B1.27 `[x]`

- 新增 list/doctor/adapters 文本和稳定 JSON 命令：安装来源/序号/link/hash、全部 provider/route 的检测/安装/信任状态、能力/数据源/静态位置/原生上限。
- doctor 冷路径校验 engine/schema 和锁定包/settings 重编译结果，比较渲染 route/static；损坏、过期或源变化均报错。使用 lock 的配置根检查实际安装，同时保留当前检测根；外部插件/workspace trust 明确未验证。
- doctor 使用 provider 的已有 effort resolver，披露当前诊断无法取得 launch/event 值和项目信任的边界；登录 shell 运行 `true`，只报告 stdout 字节数。显示 replace/fail_closed/link 风险；复用 state.Cleanup 的 7 日期限，并支持 reset，拒绝 symlink 状态根。
- 统计增加有界 metadata（实际规范 point、估算 token、渲染模型正文 SHA256），不记录正文；最近 7 日 top3 按历史数据计算，忽略 dedup/过期/未来记录，旧日志单独计数，同 hash 至少 3 次输出时提示 repeat。
- 证据：`internal/app/{inspect_test.go,doctor_test.go}`、`core/state/stats_test.go`、`internal/runtime/runtime_test.go`。Go 统一清理 CODEX_HOME/CODEX_SANDBOX，使用 `/tmp/style-cli-{gopath,gomodcache,gocache}`，设置 GOPROXY=off/GOSUMDB=off。
- 验证：`go test -count=1 ./internal/app ./core/state ./internal/runtime`、双 module `make test-go`/`make vet-go`、`go test -race -count=1 ./internal/app ./internal/runtime ./core/state ./core/engine`、CLI `list --json`/`adapters --json` smoke、gofmt 与 `git diff --check` 通过。
- 文档：T54、B1.27 与 review 的 RV-T54-01–03 标记 `[x]`。T55、A1/A12/A13 补项/A18、adapter conformance 和真实 M1b 尚未完成。

### 2026-10-15 — A13/T52 路线生命周期补项 `[x]`

- 复现并修复：Install 对同包相同内容直接返回，导致 --route 被忽略；新旧路线独立计划同一个 config.toml 时产生冲突。现在重复安装继续处理 provider/route 选择，保留包 seq，先校验实际安装内容。
- hook 配置变化实现于现有 agenthooks：新增 PlanFilesAfter，以前一步变化作为内存读视图，得到最终合并结果，保留原 Before 供事务校验；Codex adapter 组合静态区与 hook 区，app 只调用 TransitionPlanner，不复制 hook merge。
- 默认插件目录绑定请求选择的 STYLE_HOME，检查/卸载沿用该根；安装沿用 lock 保存的配置目录。新增 Codex home 在 symlink 祖先下的 trust key 提前规范化，避免初次安装后 fingerprint 漂移。后续安装保留 fail_closed，非法 flag/未知路线不静默接受。
- 证据：TestRunInstallSwitchesRoutesWithoutResidueAndKeepsExternalConfig（双向切换/幂等/最后卸载/外部 owner）、TestInstallNewPackageCanSwitchExistingRoute、TestReinstallRejectsUnknownRouteAndDetectsStoredDrift、TestInstallRetainsExistingFailClosedPolicy；agenthooks TestPlanFilesAfterComposesSharedConfigWithoutWrites（无写入规划、stale 拒绝、原子组合、外部字节）、TestCodexTrustKeysStayStableWhenNewHomeHasSymlinkedAncestor。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX、临时 Go 缓存与 GOPROXY=off/GOSUMDB=off 下，app 定向和全量、双 module make test-go/make vet-go、app/Codex adapter/agenthooks install race 测试、gofmt 和 git diff --check 通过。
- 任务/测试矩阵/review 已标记本补项完成，A13 `[x]`；T55、A1/A12/A18、adapter conformance、真实 M1b 未完成。

### 2026-10-15 — T55 / A12 / B1.28 `[x]`

- 新增 sync/trust 文本与 JSON 命令。普通包改动及 link 首次 trust 让 sync 退出 3，保持 index/lock/config；trust 展示源码和编译后差异，无确认只读，--yes 或确认回调后提交同一内存快照。已批准 link 后续同步不阻塞。
- spec 增加 LoadSnapshot，复用目录/单文件解析和 glob，复制源字节，validate 的未声明规则扫描不再读取磁盘。Install、已安装包和新命令都使用该入口；单文件安装名与原来源名不同的哈希比较一并修复。
- STYLE_HOME/approved/<name>.json 私有文件保存已批准原文与编译表示，lock 锚定摘要；后续安装/卸载/同步刷新其当前应用表示，卸载删除选中基线。多个普通包同时改动可逐包审批，未选中者继续从上次审批字节编译，避免互相阻塞或隐式信任。
- 事务继续复用 agenthooks ApplyFiles；保留安装 seq、config_dir 和 fail_closed。schema/requires/settings/replace 新确认与全局唯一性、全部安装包 T003 在写入前检查；stale lock/route、损坏基线和取消保持无部分写入。sync 冷路径清理 7 天旧会话。
- 证据：core/spec/snapshot_test.go；internal/app/sync_test.go（普通/link/同内存）、sync_command_test.go（exit0/1/3/4、JSON/help、不同文件名/单文件、资产/expiry）、sync_claude_test.go（订阅面/static/外部配置/fail_closed）、trust_policy_test.go（逐包审批、政策/预算/事务）。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX，使用临时 GOPATH/GOMODCACHE/GOCACHE，GOPROXY=off/GOSUMDB=off 下 `go test -count=1 ./core/spec ./internal/app`、双 module `make test-go`/`make vet-go`、`go test -race -count=1 ./internal/app ./core/spec`、gofmt 和 `git diff --check` 通过。任务、测试矩阵与 review 完成确认已同步；M1a A1/A18/adapter conformance、A11 真实投递与 M1b 尚未完成。

### 2026-10-15 — A1 `[x]`

- 新增 27 个版本控制内 invalid recipe，按 A1 全部错误码命名；测试显式比对文件清单，分别以文本和 JSON 模式调用 validate，要求退出 1、error severity 和预期 code。示例包命令要求退出 0。特殊文件在临时目录真实生成，避免提交大文件/平台相关 symlink。
- 证据：`testdata/invalid/README.md`、`testdata/invalid/*.yaml`、`internal/app/validate_acceptance_test.go`。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX，临时 Go 缓存，GOPROXY=off/GOSUMDB=off；`go test -count=1 ./internal/app -run TestA1ValidateExampleAndEveryInvalidFixture -v`（27/27）、`go test -count=1 ./internal/app`、gofmt 和 `git diff --check` 通过。测试矩阵和 review 已同步 `[x]`。

### 2026-10-15 — A18 `[x]`；M1b 手动配置约定

- 增加 estimator 的 9 组独立 golden 数字与 `testdata/tokens/T001`–`T007` 包 fixture。实际 Plan 验证六种包内诊断；T003 两包各 1002 token，第一包可安装，合计 2005 超过真实 Codex 2500 上限的 80%，第二包 CLI install 退出 4，所有既有 packages/lock/index/approval/provider 字节和 package seq 不变。
- 证据：`internal/tokenestimate/{golden_test.go,testdata/counts.json}`、`internal/app/token_acceptance_test.go`、`testdata/tokens/README.md`。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX、临时 Go 缓存、GOPROXY=off/GOSUMDB=off 下 `go test -count=1 ./internal/app ./internal/tokenestimate ./core/tokens` 通过，gofmt 和 diff 检查通过。A18 测试矩阵与 review 完成确认同步 `[x]`。
- 用户要求 M1b 时直接提供 Codex/Claude Code 配置过程，由用户手动配置；后续不得自动写入真实配置。当前所有安装测试仅操作临时目录。

### 2026-10-15 — RV-CONFORMANCE-01–03 / A4 / A11 `[x]`

- 新增共用真实适配器 conformance suite；46 个 payload 的统一 event/fields 和 stdout 分别固定 golden，每个输入用空 index 再验证 no-op。覆盖全原生事件、未知事件、PostModelSwitch、patch/Edit 新增行、MCP、skill、子 agent。25 个独立 effort fixture 覆盖嵌入表每行和未知/空值，验证 runtime 持久状态与模板值。两家 agent 默认范围/any、Codex subagent.start 不去重补齐 A11 离线证据。
- suite 实际发现 Codex PermissionRequest 使用 permission.request 评估点，现投影为 tool.pre；两家 deny_permission 真实 payload 确认 block 输出。固定订阅面没有变化。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX，使用 `/tmp/style-cli-{gopath,gomodcache,gocache}`，GOPROXY=off/GOSUMDB=off；双 module `make test-go`/`make vet-go`、`go test -race -count=1 ./internal/adapter/conformance ./internal/adapter/codex`、gofmt 与 diff 检查通过。任务/测试/review 已同步完成确认；conformance 安装生命周期、进程性能和真实 M1b 待验收。

### 2026-10-15 — RV-CONFORMANCE-04–06 / A13 配置往返补项 `[x]`

- 共用路线生命周期测试复现了旧 A13 证据遗漏的 JSON 字节排版变化和 Claude 无换行文档多换行。现在三条路线 × 无/单/双末尾换行均验证只读 Plan、Apply/Status、重复安装/卸载、原始字节和 0640 权限恢复及无新增文件残留；app 换路线回归新增最终 hooks.json 字节相等断言。
- 现有 agenthooks/install 增加 JSON 原文恢复记录（0600、owner 隔离、Applied SHA256），与 config 一起进入原子 changeset 和 stale 检查。外部编辑优先，重装更新基线，旧安装无记录保留语义清理；删除配置不会被旧记录复活。Claude 静态区使用可清理的生成分隔符保留无末尾换行及多 owner 顺序。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX、临时 GOPATH/GOMODCACHE/GOCACHE、GOPROXY=off/GOSUMDB=off 下，agenthooks install / app / adapter 定向与全量、双 module make test-go/make vet-go、conformance/Claude/app/agenthooks install race 测试、gofmt 和 diff 检查通过。任务、测试、review 与 agenthooks DESIGN 同步。M1a 仍需进程级性能核验，M1b 由用户手动配置。

### 2026-10-15 — RV-CLI-HOOK-01–03 / T52 进程入口补项 `[x]`

- 实际编译二进制发现 `style agenthooks run` 入口遗漏，前序测试只调用 runtime。新增管理流程之前的分派，解析 provider/安装事实并加载 index/state，仍使用 agenthooks Runner；不读取包、lock 或 settings。
- 三条路线持久携带 STYLE_HOME、harness 根和版本；runtime 关闭合成 backfill、隔离 codec 辅助状态，日志 point 与实际评估点一致。生成命令在含空格路径和错误父进程 STYLE_HOME 下通过。
- 全部 46 个 wire golden 由真实二进制通过，另测损坏 index 的 fail-open 与 Claude fail-closed。suite 捕获 runtime/Runner ERROR，修正原 PermissionRequest 解析 fixture 意外要求不支持 inject 的问题；独立 deny fixture 仍验证 block，未改变订阅面或放宽能力校验。
- 验证：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off go test -count=1 ./internal/adapter/conformance -run 'TestBuiltCLI|TestGeneratedRoutes'` 通过；相同隔离环境的双 module `make test-go`、`make vet-go`、`go test -race -count=1 ./internal/app ./internal/runtime ./internal/adapter/conformance`、gofmt 和 `git diff --check` 通过。
- 任务/测试矩阵/review 已确认完成。进程级性能继续单独验收，真实 Codex/Claude 配置按用户要求由用户手动执行，M1b 不提前标记。

### 2026-10-15 — A20 / M1a `[x]`

- 进程测试独立构建并运行 CLI，预热 10 次、每家采样 200 次，全部输出匹配真实 golden。Codex median 21.031958ms / p95 22.852ms / max 28.647875ms；Claude Code median 21.424125ms / p95 22.97575ms / max 27.726041ms，均通过 p95 <50ms。
- 包含 exec 开始到退出及 index/decode/state/engine/encode/I/O；文件和已解析 session 预热，排除构建、shell/harness 启动和首次 effort 解析。并非真实 harness 性能。旧 engine-only 数字保留为历史证据，测试矩阵更正验收范围。
- 复现：`env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off STYLE_PERF=1 STYLE_PERF_OUTPUT=/tmp/style-cli-a20-2026-10-15.json go test -count=1 ./internal/adapter/conformance -run '^TestHookProcessLatency$' -v`；通过。结果入库 `docs/review/M1-A20-2026-10-15.json`，gofmt 与 diff 检查通过。
- 任务/测试矩阵/review 确认 A20、M1a 完成；真实 Codex B1–B12 与 Claude C1–C9 保留 `[?]`，配置由用户手动执行。M1 整体未完成，暂不进入下一阶段规划。

### 2026-10-15 — M1B-PREP `[x]`；等待用户手动配置与真实结果

- 新增 `docs/STYLE-M1B-MANUAL.md`：固定二进制构建、用户配置备份、Codex user/plugin 与 Claude user 路线、第二包探针、全部 M1b 项的观察要求、卸载字节比较和结果返回。配置由用户执行，本轮未修改真实 Codex/Claude 配置，也未启动模型会话。
- 新增 `testdata/manual/m1b-probes/STYLE.yaml`，用 session/model/effort、warn 的用户/模型双标记、prompt block 和定向 session 去重标记补齐示例包不易辨认的观察点。探针不安装原生 skill 或 MCP；实际并行与真实上下文投递仍需 harness 证据。
- 核对官方 Codex hooks/插件本地 marketplace、Claude hooks 文档及本机 Codex CLI 0.159.3 help。该 CLI 使用 `plugin add`、`plugin remove`，无 `--plugin-dir`。STYLE 仅生成插件文件，手册明确补充 marketplace 注册、缓存副本和切回 user 前取消插件注册的顺序，避免误认为生成目录即可启用。
- 使用隔离的 HOME/CODEX_HOME/CLAUDE_CONFIG_DIR/STYLE_HOME，在 `/tmp/style-cli-m1b-manual/` 构建并运行真实 STYLE CLI。两包 validate、两家 plan/install、sync、选定卸载和外部四个配置文件原文字节恢复通过；两家 wire session/warn/block 与串行重复调用去重通过。Codex CLI 实际本地 marketplace add、plugin add/remove、marketplace remove、切回用户级和卸载均退出 0。Claude 只使用 `2.1.315` 固定版本检测脚本，未验证真实 Claude 接受配置。
- 可重复入口：`testdata/manual/README.md` 的隔离 Go build、validate/plan 命令；手册内的安装/插件注册/卸载/cmp 命令可将四个环境根指向临时目录复核。本轮临时执行记录为 `setup-wrg7xz9y`、`probes-y8hhubwq`，不以临时路径作为唯一证据。
- 验证：Go 构建清理 CODEX_HOME/CODEX_SANDBOX，使用 `/tmp/style-cli-{gopath,gomodcache,gocache}` 和 GOPROXY=off/GOSUMDB=off；两包 CLI validate 与探针的两家 plan 退出 0；10 个 shell 代码块 `zsh -n`、相对链接和 `git diff --check` 通过。本提交只含文档与手动 fixture，无 Go 实现改动。
- 任务/测试矩阵/review 已确认准备工作完成。B1–B12、C1–C9、真实 conformance e2e 保留 `[?]`；M1 尚未完成，等待用户配置和结果后继续验收。

### 2026-10-15 — CDEX-05/B5 `[x]`；CDEX-02/B2 `[~]`

- 用户提供插件 Hook stats 截图（SessionStart/UserPromptSubmit 均 Completed，Failed=0），并明确待测模型 gpt-6-sol。后续截图回答为 GPT-6 Sol/低推理强度；只读 STYLE state 确认 gpt-6-sol/low，SessionStart 输出和 prompt 去重。模型自述仅作交叉观察，截图到具体 session 无 UI ID，不据此扩展验收范围。
- 本工作对话为另一 gpt-6.1-sol/low 会话，实际收到 session-info、low-effort-guard；工具测试与用户待测会话分开记录。之前口头混用两个会话的读数，已更正。
- 临时 Git 仓库仅连接同目录本地 bare remote，先正常初始化并 push main。实际 Codex shell tool 的 force push 被 PreToolUse 返回 Do not force push，有 no-force-push/blocked stats；force-with-lease 正常执行，退出 0，Everything up-to-date。B5 完成，不以 Git 自身报错代替 hook 阻断。
- 另发现 M1B-EXAMPLE-01：写文档的 Python heredoc 仅引用测试命令也被规则阻断，误拦文本引用；作为示例正则精度问题记录，未修复。
- 本项专测真实环境，保留其 CODEX_HOME 等安装环境；未运行 Go 测试或更改 provider 配置。命令、结果、元数据和复现方法入库 `docs/review/M1B-CODEX-2026-10-15.json`，任务、测试、review 同步。B2 minimal/high、其余 Codex 项及 Claude 仍待验收。
- 提交前核验：证据 JSON 可解析；渲染正文摘要、真实 stats、临时 bare remote 路径与分支状态、文档状态一致，`git diff --check` 通过。

### 2026-10-15 — M1B-FIX-01 `[x]` Codex 卸载 1 字节残留

- 按用户反馈补回归，修复前两条 Codex 路线的 absent 用例均明确失败：卸载后 config.toml 为 `"\n"`。原有换行、空白和正常用户配置往返用例通过，排除粗暴清理空白的修法。
- 原因：agenthooks 新文件直接采用渲染结果，信任托管区末尾额外 LF 不属于删除区域；STYLE 静态块移除后它独自留下。修复现有 `agenthooks/install/render_codex.go` 的生成尾部，使其与 mergeManagedTOML 一致，未重复实现 hook 安装职责。
- 最终回归为两条路线 × 4 种原始状态，共 8 组，均经过重复安装、双向路线切换和最终卸载；新增文件不再残留，原有字节与 0640 权限保持。测试位于 `internal/app/uninstall_test.go`。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX，使用 `/tmp/style-cli-{gopath,gomodcache,gocache}`，`GOPROXY=off GOSUMDB=off make test-go` 与 `make vet-go` 双 module 全量通过；gofmt 和 diff 检查通过。任务、测试、review 已确认完成；真实 B8 仍待用户验证。
- 适用边界：防止新的安装往返遗留换行；历史残留不能与用户原有空白文件自动区分，应按安装前备份判断，不代为清理真实配置。

### 2026-10-15 — M1B-FIX-02 `[x]` 设计路线与信任选项

- 确认旧设计仍用 plugin/user/user-settings，安装命令表声称支持 --preseed-trust，实际 CLI 没有该参数。按已实现契约修正全部相关示意、命令、锁文件、信任和未决项，不增加未经验收的 CLI 功能。
- 路线统一为 codex-plugin/codex-user/claude-user。明确现有 agenthooks 自动生成两种 Codex 摘要；plugin 注册由用户执行，Status 不据生成文件证明信任。另对照 trustState 确认报告信任种类与锁文件 user/preseeded 的映射，避免再次混写。
- `STYLE_DOCS/STYLE-cli-design.md` 原被 .gitignore 忽略，本项仅将该设计文件纳入版本控制，未添加其它资料；补当前 M1a/M1b 状态及实现源文件链接，手册同步。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX、临时 Go 缓存、离线构建 `/tmp/style-cli-m1b-doc-check`。临时 HOME/STYLE_HOME/CODEX_HOME/CLAUDE_CONFIG_DIR 内，help、adapters、未知选项拒绝且无写入、两条路线的真实 CLI install/uninstall、报告/锁/摘要与文档一致；卸载后 config.toml 不存在。复现入口已写入 review；新增源链接、文档契约与 diff 检查通过。
- 主设计开头原有 6 个指向未收录外部资料的相对链接，改为保留文件名/版本的外部来源说明，避免新克隆出现断链；本仓库内的规范和实现使用有效相对链接。

### 2026-10-15 — M1B-FIX-03 `[x]` 未知 effort 残留旧档位

- 对照规范审查发现：harness 报告映射表外的 effort 原值时，`core/state` 只更新 `effort_raw`，旧 `effort` 档位保留，`when.thinking_effort` 继续命中、`{{thinking_effort}}` 渲染过期值。修复前临时探针得到 `tier="high" raw="future-tier"` 且 `thinking_effort: high` 匹配。
- 修复位于 `core/state/state.go`：非空 raw 视为完整读数，档位与近似标记随之替换（可为空）；不带 effort 的事件仍不覆盖状态，满足 §6“从不用空值覆盖”。adapter 与 engine 不变。
- 回归：`core/state/state_test.go:TestReportedUnknownEffortClearsPreviousTier`、`internal/adapter/claude/runtime_test.go:TestUnknownEventEffortClearsPersistedTier`，修复前均失败、修复后通过。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX，使用 `/tmp/style-cli-{gopath,gomodcache,gocache}`、GOPROXY=off/GOSUMDB=off；双 module `make test-go`、`make vet-go`，`go test -race -count=1 ./core/state ./internal/adapter/claude ./internal/runtime`，gofmt 与 `git diff --check` 通过。真实 Claude effort 行为仍属 C1/U-C1。

### 2026-10-15 — M1B-FIX-04 `[x]` Claude 已解析后不再读 CLAUDE_EFFORT

- 对照规范审查发现：`internal/adapter/claude/runtime.go` 在会话已解析且事件不带 `effort.level` 时直接返回，不读 `CLAUDE_EFFORT`。修复前探针中 env=low、state=high 时结果保持 high。规范 §6 要求 Claude 按事件 → `$CLAUDE_EFFORT` → 状态解析，一次性解析只适用于 Codex 配置。
- 修复：删除 Claude 路径的提前返回，每个事件按规范顺序解析；Codex 不变。PostModelSwitch 单测原先依赖短路断言 effort 字段为空，现注入空环境并断言 effort 保持原值（等值写入，无状态变化）。
- 回归：`internal/adapter/claude/runtime_test.go:TestResolvedSessionStillReadsClaudeEffortEnvironment`，修复前失败、修复后通过。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX，使用 `/tmp/style-cli-{gopath,gomodcache,gocache}`、GOPROXY=off/GOSUMDB=off；双 module `make test-go`、`make vet-go`，`go test -race -count=1 ./internal/adapter/claude ./internal/runtime ./internal/adapter/conformance`，gofmt 与 `git diff --check` 通过。新增开销仅一次 `os.Getenv`，未重跑 A20 进程采样。env 与事件生效值不一致时的切换风险留待 U-C1/C1。

### 2026-10-15 — CDEX-06/B6 `[x]`

- 用户要求继续真实验收。本工作会话的用户级插件配置会触发工具 hook，继续与用户 gpt-6-sol 会话分开记账。
- 在独立临时 probe.go 上先新增 TODO，再通过另一 apply_patch 删除。新增时实际收到首句模型警告，12:12:31.929317Z 的 PostToolUse stats 为 no-todo-comments/warn/output；删除时工具完成且没有模型警告、没有新增该规则 stats，原文件字节恢复。
- 观察期间用户追加消息产生了两条 UserPromptSubmit dedup，因此核验按规则筛选而非要求日志总行数不变。证据 JSON 保留观察截止点，后续文档编辑造成的警告不混入本次区间。
- 证据与复现步骤入库 `docs/review/M1B-CODEX-2026-10-15.json`；任务、测试和既有 review 同步完成确认。真实测试保留已安装环境，不运行合成 payload 替代验收；本项无 Go/配置变更，验证 JSON、实际 stats 与警告摘要、文件原始字节、状态一致性和 diff 检查。
- 用户已安装 Claude，并要求先简要安排 M1 收尾再推进：先核对安装基线，完成 Codex 与 Claude 真实矩阵，再处理发现的问题及统一复验。配置操作仍由用户执行，M1a 已完成、M1b 尚未完成。

### 2026-10-15 — M1B-CLAUDE-SETUP-01 `[!]`

- 用户报告 Claude 已安装。只读执行 `claude --version` 得 2.1.300；默认 `$HOME/.claude/settings.json` 的 10 条 STYLE hook 指向 `/var/folders/m5/gv7_10k149l3j4l02mt548zr0000gp/T/go-build1449963545/b001/exe/style`，文件不存在且不可执行。args 只有 provider/owner，没有当前版本的安装状态目录绑定。
- `$HOME/.local/lib/style-m1b/style` 存在；`$HOME/.style-m1b/style.lock.yaml` 当前只记录 Codex。此为安装前置条件阻塞，不代表已运行或通过 Claude C1–C9。
- 复现：运行 `claude --version`；仅提取 settings.json 中 STYLE command/args，对 command 执行 `test -f`/`test -x`；核对 M1b lock 的 harnesses。证据入库 `docs/review/M1B-CLAUDE-2026-10-15.json`，任务、矩阵、review 同步。未打印无关配置或凭据，未写入真实配置。
- 下一步：用户按手册第 1 节重建固定二进制、第 4 节重新安装 Claude 路线；保留原始备份并新开 Claude 会话，之后继续投递/effort 等真实验收。手册同步实际检测版本和配置核对点；本项无 Go 实现变化，提交前检查 JSON、真实路径状态、文档命令语法与 diff。

### 2026-10-15 — M1B-CLAUDE-SETUP-01 / CLAUDE-01/C1 `[x]`

- 用户重新安装并提供 hooks UI：5 类事件、10 条 hook。只读核对所有 command 已指向固定二进制且可执行，args 含 STYLE home/harness home/version，lock 新增 claude-user。旧临时路径阻塞解除，初次失败记录保留。
- 用户回传初始 `Current session: model claude-sonnet-5-5, effort unknown.`，界面选择 high。按要求读取现有 Bash 环境得到 CLAUDE_EFFORT=high 后，下一条提示收到 `effort high`。
- 最新对应会话 f0796ff3-ecf1-44d1-a653-6ef0f3bc20c2 在 12:21:47.708347Z SessionStart 输出 unknown，前两条 prompt 去重；12:23:52.575871Z prompt 输出 high。两次正文加来源文件末尾 LF 的 SHA256 与真实 stats 精确匹配，state 为 high/raw high/approx false。
- 对照主设计 c1，允许首次工具后的下一条提示补上强度，故 C1 通过；启动读数不可用是本次已确认的边界。没有采集 hook 进程自身环境或原始 payload，不能据 Bash 的环境推断 U-C1 已在全部事件成立。
- 原网页工具没有返回可用引用，后直接获取官方 hooks 页面核对工具阶段 effort 说明；验收证据使用用户回传、真实 stats/state 和本仓库 c1，不依赖无法核验的网页引用。
- 证据入库 `docs/review/M1B-CLAUDE-2026-10-15.json`；任务、测试矩阵、手册和 review 同步确认。真实环境保持用户配置，本项无 Go 变更；提交前验证 JSON、路径/args、原始 stats、正文 hash、状态/文档和 diff。后续继续 compact 等独立验收。

### 2026-10-15 — CLAUDE-02/C2 `[x]`

- 同一会话 f0796ff3-ecf1-44d1-a653-6ef0f3bc20c2 在 compact 后 epoch=1，SessionStart 于 12:26:22.31056Z 重新输出 session-info/high；对应 epoch=1 seen 标记存在。
- 按要求分两条提示只引用上下文、不调用工具；用户回传两次 high 正文。真实 12:26:41.883803Z、12:27:42.744059Z prompt 均为 dedup，整个观察区间只有一次 output，正文摘要与两次回答一致。
- 证据区间、stats、scope/seen 和复现步骤入库 Claude JSON 的 C2；任务、矩阵、手册和 review 确认完成。本项无 Go/配置变更；提交前核对真实状态/日志、JSON、正文 hash 和 diff。下一项由用户在同一会话执行 model switch，继续 C3。

### 2026-10-15 — C3 `[!]`；M1B-FIX-05 `[x]`

- 用户截图显示 `/model` 已设置 Haiku 5.5，两次提示仍引用 Sonnet/high。对应 12:30:44.431366Z、12:31:02.377394Z 的真实 stats 都去重旧正文，state 仍为 Sonnet，配置缺少 PostModelSwitch。
- 原因是 app 的 hooksForProvider 只遍历编译后的注入点，没把 model 数据依赖转成观察订阅；现有 codec/runtime 本来能处理切换事件，安装测试却仅手动指定过观察事件，未覆盖完整链路。
- 补真实安装回归，修复前观察事件缺失以及 system 模板/tool 模板/模型条件三类依赖用例失败，静态与无关工具对照通过。现仅在条目 needs 含 model 时加入 PostModelSwitch；通过 renderer 现有 blocking 元数据等待状态写入，避免异步观察与下一提示竞态，输出仍为中性 JSON。trust 移除最后一个依赖时清理订阅。
- 新测试直接使用安装生成的 argv 走 CLI 入口，覆盖启动模型→观察更新→不带 model 的下一提示补发新值→再去重和订阅清理。代码只改 CLI 安装推导，保留 agenthooks 安装/codec 职责，无真实配置修改。
- 最终代码验证：隔离 Go 缓存、清理 CODEX_HOME/CODEX_SANDBOX、GOPROXY=off/GOSUMDB=off；定向 `go test -count=1 ./internal/app -run '^TestInstallClaudeModel' -v`、双 module `make test-go`/`make vet-go`、`go test -race -count=1 ./internal/app -run 'TestInstallClaudeModel|TestTrustRebuildsClaudeSurface'`、gofmt 和 diff 检查通过。
- 另构建 `/tmp/style-cli-m1b-fix05`，在全临时 HOME/STYLE_HOME/CODEX_HOME/CLAUDE_CONFIG_DIR 安装真实示例包，确认 6 类事件/11 条 hook 和同步观察；实际二进制逐次处理合成 startup/switch/两条 prompt，补发新模型后去重，最终卸载无配置残留。此为隔离进程 smoke，不替代真实模型验收；结果记入 Claude JSON。
- 任务、矩阵、设计/手册和 review 已确认实现修复完成；Claude JSON 保留 C3 失败证据。用户需重建固定二进制并重新安装，开新会话确认 PostModelSwitch 后复测；真实 C3 继续 `[!]`，M1 未完成。

### 2026-10-15 — CLAUDE-03/C3 `[x]` 真实复测

- 用户重装 M1B-FIX-05 后，实际 Claude 配置有 6 类事件/11 条 STYLE hook，PostModelSwitch 指向固定二进制且没有 async 标记，args 绑定 M1b 状态目录。未由 agent 修改真实配置。
- 对应会话 fbc11e78-3715-4e9d-88bd-2c8699e0f07b 中，用户 Haiku→Opus 后 12:51:10.45976Z prompt 输出 Opus/unknown，12:51:57.128501Z prompt 去重；再切 Sonnet 后 12:52:06.55888Z prompt 输出 Sonnet/unknown，12:52:52.538231Z prompt 去重。每次新正文只输出一次，用户原样引用分别匹配含末尾 LF 的 SHA256，持久状态与 seen 标记一致。
- 本次 SessionStart 模型和强度均不可用，初始正文为 unknown/unknown；切换后模型正确，强度仍不可用。C3 仅验收新模型单次补发，不增加强度或所有启动场景的通过声明。未采集原始 PostModelSwitch payload，实际配置、状态、输出摘要及模型回传共同构成证据。
- Claude JSON 保留原失败并新增 real_retest；任务、矩阵、手册和 review 同步确认完成。无 Go 变更；验证真实 stats/state/seen、正文摘要、JSON、文档状态和 diff 检查。C4–C9 待验收，M1 未完成。

### 2026-10-15 — CLAUDE-04-PREP `[x]`；C4 `[?]`

- 用户表示没有或不确定原生 code-review 是否可用。新增 `testdata/manual/claude-skills/code-review/SKILL.md`，只报告 M1B_SKILL_LOADED 和实际 hook 附加指令，不包含规则的 findings 数值或实际 review 操作。
- 手册新增独立验收工作目录的用户手动配置命令，不覆盖已有同名文件；规定新会话 A 先 Skill 工具再显式 /code-review，新会话 B 反向，分别验证首次投递和跨入口共享去重。已核对官方 skills 的项目位置、调用默认值和同名优先级；实际支持与加载由用户真实版本确认。
- 验证 fixture 结构/无预填规则正文、临时目录复制与逐字节比较、手册 shell 语法和 diff；未修改真实配置或启动真实模型。review 已确认准备完成，C4 继续待真实验收。

### 2026-10-15 — C4 `[!]`；M1B-FIX-06 `[x]`

- 用户回传 Skill(code-review) 后运行 fork/background review，显式调用也审查 home 而没有 fixture 标记。只读核对 fixture 已正确复制；对应真实 transcript 的 cwd 全部为 `/Users/inxups`，不在验收目录。提取范围仅 cwd、相关 Skill 调用/结果，未使用无关 transcript 指令。
- 另确认真实 PreToolUse/PostToolUse matcher 都没有 Skill。13:01:41.467Z 存在 Skill 工具调用，但观察区间没有隐式 review-skill 记录；13:01:51.794999Z 只有显式 UserPromptExpansion output，正文摘要匹配规则。故 C4 未通过，不以实际 review 的空 findings 作为证据。
- 原因是 hooksForProvider 忽略 Match.Skill。补安装 argv 级回归：before/after × 隐式/显式先后顺序四组在修复前全失败；现按工具评估点添加 Skill，四组共享 session 去重通过，trust 移除 skill 后保留无关 Bash。复用现有 agenthooks codec/renderer/runtime，无真实配置写入。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX，临时 Go 缓存、GOPROXY=off/GOSUMDB=off；定向测试、双 module make test-go/make vet-go、定向 app race、gofmt/diff 通过。构建 `/tmp/style-cli-m1b-fix06` 在全临时配置根安装真实示例，确认 6 类事件/12 条 hook、Skill 的 PostToolUse matcher，两种入口顺序均首次 output 后 dedup，卸载无 settings.json/CLAUDE.md 残留。此为合成 payload 的进程验证，不替代真实 C4。
- 任务、矩阵、手册、既有 review 与 Claude JSON 已记录实现修复和真实失败；手册增加启动目录校验、fixture 描述与标记检查。下一步由用户重建/重装，在正确目录的新会话先确认最小 skill 加载，再继续真实入口去重。M1 未完成。

### 2026-10-15 — CLAUDE-04/C4 `[x]` 真实复测

- 用户重装后实际配置含 PostToolUse 的 `^(Skill)$`，共 6 类事件/12 条 STYLE hook；两个真实新会话的 transcript cwd 均为验收目录，实际 Skill 工具调用与用户回传相符，最小 fixture 标记可见。
- 会话 96e825f7-1e9b-47c8-8a78-8df723880184：13:17:33.310757Z 隐式 Skill 的 tool.post 输出 review-skill；13:17:43.664941Z 显式入口 dedup。会话 89525649-b059-42dd-982e-9e41dd4930dc：13:20:38.846619Z 显式入口输出；13:20:47.965266Z 隐式 Skill 的 tool.post dedup。各 session 一次 output、一次 dedup，共用 rule/body seen 标记。
- 两个入口首次投递时用户均回传 findings 上限原文，含来源 LF 的 SHA256 与实际 stats 一致。第二个会话后续再次复述该文字属于已有上下文，实际日志没有重复注入。验收限默认 after、前台项目 skill；before 仅离线覆盖，fork 不扩大宣称。
- Claude JSON 保留原失败并新增两种顺序的真实复测；任务、矩阵、手册和 review 同步完成。无 Go/配置变更，核验实际 stats/seen、工具 metadata/cwd、正文摘要、JSON、文档 shell 语法和 diff；C5–C9 待验收，M1 未完成。

### 2026-10-15 — CLAUDE-05/C5 `[x]`

- 用户回传 Bash 的 STYLE_M1B_WARN 输出、工具提示 M1B_WARN_USER 和模型引用 M1B_WARN_MODEL，最初称“没有警告”。进一步确认提示实际在 Claude 工具 UI 内容中可见，满足 c5 的用户可见要求，不要求特定警告样式。
- 真实会话 1a0e173f-74cd-4b3d-90c9-040d2cc304c7 的 Bash tool_use 为 13:26:31.689Z；warn-shell 的 PreToolUse output 为 13:26:31.741237Z；正常工具结果为 13:26:32.451Z。模型正文无末尾 LF 的 SHA256 与 stats 一致，独立模型标记未出现在测试提示中。
- 模型报告在工具结果后收到警告，不能据此确定内部接收时机；实际 PreToolUse 评估先于工具结果。本项只验收 Claude PreToolUse 的用户提示与模型投递，不扩展到其他事件，也不混入本工作 Codex 会话的同名探针输出。
- Claude JSON、任务、测试、手册和既有 review 同步确认通过。无 Go/真实配置变更；提交前核对实际工具 metadata/stats、正文摘要、JSON、文档状态与 diff。C6–C9 继续待验收，M1 未完成。

### 2026-10-15 — CLAUDE-06/C6 `[x]`

- 用户回传 prompt 阻断、force 拒绝和 lease 放行。由于本工作 Codex 也装了探针，回传中的触发标记被用户删减；直接读取原 Claude 会话证据，不把删减文字当成原始输出或补造原 prompt。
- 会话 1a0e173f-74cd-4b3d-90c9-040d2cc304c7 在 13:38:24.66853Z、13:38:28.178446Z 两次记录 block-prompt/blocked。对应 harness 记录仅含固定理由，preventContinuation=true、promptKept=false，没有回带原 prompt；正文摘要与规则一致。
- 13:38:53.386Z 的 Bash force 调用被 13:38:53.430153Z no-force-push/blocked 拒绝，工具结果和模型回传均有明确 hook 理由；13:39:18.879Z 的 lease 调用成功返回 Everything up-to-date。允许 lease 是当前设计的正向对照，不采纳模型自行提出的扩大拦截建议。
- 临时仓库先完成正常 push，仅连接同目录 bare remote；验收后 local/remote HEAD 与初始值一致、工作树干净。真实记录、摘要、工具 ID/结果和复现入库 Claude JSON；任务、测试、手册及既有 review 同步完成。无 Go/真实配置变更，核对证据与 diff 后提交；C7–C9 继续待验收，M1 未完成。
- 补充核验既有 wire 回归：清理 CODEX_HOME/CODEX_SANDBOX，使用 `/tmp/style-cli-{gopath,gomodcache,gocache}`、GOPROXY=off/GOSUMDB=off，`go test -count=1 ./internal/runtime -run '^TestClaudePromptBlockSuppressesOriginalPrompt$' -v` 通过。真实 transcript 有不带 timestamp 的快照重复项，证据比对只使用带时间的实际记录。

### 2026-10-15 — CLAUDE-07/C7 `[x]`

- 用户首次在两条消息中分别执行 A/B，两组时间不重叠；会话 cda985ba-95cd-46fe-8fd0-69e66f892ad7 的一次 output、一次 dedup 仅证明串行去重，不用于通过 C7。
- 用户 /clear 后实际会话为 8abb76e3-44ef-427b-a397-00dcae8d5bf8，gen=1、epoch=0。两次 Bash 同属 msg_011Cg48k5htNHmaHdwXmQiR4，不同 tool id，第二次调用在第一次结果前发起，两个真实命令运行区间均为 [1792072394,1792072397]。
- parallel-once 在 13:53:13.626212Z output，13:53:14.116324Z dedup；两者正文 SHA256 相同，session seen 标记仅一个。用户回传模型引用 M1B_PARALLEL_ONCE，与实际正文一致。确认实际并行工具的单次投递，不声称 hook 进程精确同时执行或并发压力测试通过。
- Claude JSON 保留首次串行观察和真实并行复测；任务、测试、手册及既有 review 同步完成。无 Go/真实配置变更，验证实际工具 message/id/时间、输出区间、stats/seen/hash、JSON/文档状态与 diff；C8–C9 待验收，M1 未完成。

### 2026-10-15 — CLAUDE-08/C8 `[~]` 首轮命令拆行

- 用户成功取消 style-runtime 插件和 marketplace 登记，但环境赋值与卸载命令分开输入，两次 uninstall 均报 no matching installed package。没有采集原进程环境，不将推断写成原始运行事实。
- 只读复核：默认 `~/.style` 无包，`~/.style-m1b` 仍有 terse-reviewer/m1b-probes；Claude 12 条 STYLE hook 和 CLAUDE.md 仍存在，settings 与 applied 摘要一致，说明真实卸载尚未完成。插件列表已无 STYLE 条目，无需重复取消登记。
- 复测前已将 owner 恢复记录的原始 settings 保存到临时目录并核对 SHA256，197 字节；CLAUDE.md 原文基线为不存在。初始 manual-backup 早于后来新增的模型/界面设置，不用于删除这些新偏好。具体摘要和来源记入 Claude JSON 的 C8。
- 手册卸载步骤改为每条完整单行 env 命令，用户手动重试后再检查 cmp 与 CLAUDE.md 消失；任务、测试和既有 review 暂记部分完成。无 Go/真实配置变更，C8 不标记通过，C9 未开始。

### 2026-10-15 — CLAUDE-08/C8 `[x]` 真实复测

- 用户依次使用完整单行 env 命令卸载 m1b-probes、terse-reviewer，两次均更新 claude-user/codex-plugin；随后 cmp 和 CLAUDE.md 不存在检查成功，打印 C8_CLAUDE_RESTORED。
- 14:10:58.625139Z 只读复核 settings 与预先校验的本次安装基线完全相同，197 字节、SHA256 fe71f251612b6b782d440703416ab9712cc7dde0cf5ce21c09c41c391f1c2cd0。settings 没有 hooks，CLAUDE.md、owner 恢复记录、M1b lock/index 均不存在，显式 STYLE_HOME 下 list 为空。
- 基线保留用户在最初 absent 备份之后新增的模型/界面偏好。当前安装有 applied 摘要相符的恢复记录和一致的 non-hook 值作为来源，不通过覆盖旧备份伪造恢复；首轮未实际卸载的记录保留。
- Claude JSON、任务、测试、手册及既有 review 同步完成确认。无 Go/真实配置代操作；验证基线来源/字节/hash、卸载残留与文档状态、手册 shell 语法和 diff。C9 待用户重装后继续；不据此完成 Codex B8 或本次没有的外部 hook 真实场景，M1 未完成。

### 2026-10-15 — CLAUDE-09/C9 `[~]`；resume `[x]`；M1B-PERF-01 `[!]`

- 用户重装两包到 claude-user 并恢复旧 session 1a0e173f-74cd-4b3d-90c9-040d2cc304c7，回传 Sonnet/high 上下文。实际 14:16:19.985Z SessionStart:resume 退出 0、stdout `{}`，第一条无工具提示只新增两条 prompt dedup，原四个 seen 标记和 gen=0/epoch=0 不变，prompt_count 5→6；没有重新输出 session.start，resume 分支通过。
- 用户完成采样请求，timestamped transcript 验证 50 个 Bash tool id 均有成功结果，0 失败；49 个 assistant message 中一个含两次重叠调用（第 48/49 次）。无 timestamp 的历史快照不重复计数，未把模型的“顺序”描述当成全串行证据。最终 prompt_count=7、seen 仍相同，仅再新增两条 prompt dedup。
- Claude 2.1.300 debug 原样记录 103 次 STYLE command-hook 完成、0 错误：整体 median=33ms/p95=53ms/max=209ms，PreToolUse 50 条 median=41ms/p95=53ms/max=60ms，PostToolUse 50 条 median=23ms/p95=29ms/max=40ms，resume 1 条 209ms、prompt 2 条 55/66ms。nearest-rank，无预热/慢样本剔除；完整结果未达到 p95<50ms，记录 M1B-PERF-01，C9 不能标记完成。
- 单独排除重叠区间四条 completion 的敏感性分析为 99 条/p95=54ms，也超预算；不替代完整结果。固定二进制、只复制 index、临时新状态和合成输入的进程诊断各 100 次，清理 CODEX_HOME/CODEX_SANDBOX、保留其余环境并指定 CLAUDE_EFFORT=high，Pre/Post p95=24.080709/24.08275ms；临时输出 `/tmp/style-m1b-c9-process-jp90qpc8/report.json`。不同输入/环境/计时边界不能相减得出原因或作为真实 C9 通过依据。
- 原样 103 条 completion 入库 `docs/review/M1B-CLAUDE-C9-HOOKS-2026-10-15.txt`；样本、完整 50 次调用/结果、源摘要、重叠和诊断摘要入库 `docs/review/M1B-CLAUDE-C9-TIMING-2026-10-15.json`。未提交完整 debug/会话内容。新增只读 `testdata/manual/measure_claude_hooks.py`，退出 0 仅表示生成 JSON，`status` 才表达采样判定。
- 验证：`PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_measure_claude_hooks.py' -v` 六项通过；语法编译输出在 `/tmp`；对留存 excerpt 精确复算 103 条及 p95、JSON/status/摘要/引用检查、与真实 transcript 的 50 组调用结果逐项比较及 `git diff --check` 通过。任务、矩阵、手册和既有 review 已确认 resume 完成、性能未通过。本项无 Go/真实配置修改。
- 下一步先查明真实 command-hook 计时边界与 PreToolUse 开销，再选择优化；不直接删除持久化安全语义、不放宽阈值，不将 Codex U5 的 matcher 设计回退套用到 Claude。若需设计变更，按设计 §14.3/§15.1 评审；Claude M1b 和 M1 仍未完成。

### 2026-10-15 — M1B-PERF-01A `[x]`；真实 C9 仍待复测

- 只读检查已安装 Claude 2.1.300 的 command-hook runner，确认计时包含输入准备和命令执行等待、在响应 JSON 校验前结束，当前 args 配置直接 spawn executable/argv。产物摘要和字节定位留在证据 JSON；没有保存完整第三方代码或无关会话数据。原 103 次没有独立进程起止计时，Pre/Post 额外差值仍不能归因。
- 在临时归档源码副本加阶段计时，使用复制的真实两包 index 和合成输入。主路径中状态更新中位数约 11.9ms、engine.Load 约 5.25ms，其中 New 约 1.56ms；阶段数据受插桩影响，仅供定位。未移除 fsync 或改变生命周期/去重。
- 确认 `engine.Load` 原始 JSON 校验后调用 New，再重编码并校验一次；现直接调用私有已校验构建器。New 仍独立校验，两个入口均保留 matcher 编译和 reevaluation 约束。`TestIndexConstructorsKeepValidation` 在基线及修复后均通过，保护 schema、非法正则、reevaluation 和 nil store；既有 wire 错误测试继续通过。
- `BenchmarkLoadIndex` 使用相同入库 golden，三轮中位数约 1.584→0.843ms，分配次数约 26256→13333。前后独立构建，用相同两包 index、各自临时状态、Pre/Post 各 100 次，交错并轮换先后次序，完整进程 p95 从 24.445667/24.338291→22.8505/22.924167ms；400 次全部返回中性 JSON，首次慢启动仍保留。此为离线诊断，不替代真实 C9。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX，临时 Go 缓存和 GOPROXY=off/GOSUMDB=off；双 module `make test-go`、`make vet-go`，`go test -race -count=1 ./core/engine ./internal/runtime` 通过。完整进程样本、基准、摘要、阶段数据和复现入口入库 `docs/review/M1B-PERF-01A-2026-10-15.json`；任务、测试、手册、Claude JSON 与既有 review 同步确认离线优化完成。
- 真实二进制和配置均未代改。用户重建固定程序后，用独立 debug 日志重新采集；M1B-PERF-01 仍为 `[!]`，CLAUDE-09/C9 仍为 `[~]`，不以小幅离线改善宣称 53ms 问题已解决。

### 2026-10-15 — M1B-PERF-01 / CLAUDE-09/C9 `[x]` 真实复测

- 用户回传“采样完成”。只读核对固定二进制：go1.27.0、vcs.revision=a4426a3a081b814695c16e046bdc71561beb1774、vcs.modified=false，SHA256 9c137245d5f240d5c67af020464704e5fe92f09855ced7b1320432e86411975f。由用户更新程序，agent 没有代改真实配置或安装程序。
- 新 debug 文件 `/tmp/style-m1b-c9-perf01a-w1zfttuf.log` 中，15:09:39.216Z–15:12:00.005Z 共 102 条 STYLE command-hook 完成记录，0 错误、0 未解析行。整体 median=27ms/p95=46ms/max=798ms；PreToolUse 50 条 median=37ms/p95=53ms/max=94ms，PostToolUse 50 条 median=23ms/p95=27ms/max=30ms，resume 1 条 798ms、prompt 1 条 25ms。所有匹配样本保留，无预热剔除，原 103 条失败样本未混算。
- 时间戳 transcript 验证新一轮恰有 50 个 Bash tool id、50 个独立 assistant message、50 个成功结果，无重叠；所有命令返回 STYLE_M1B_C9、空 stderr。无 timestamp 的历史快照不重复计数。只留验收调用元数据，不提交完整会话或 debug 文件。
- 同一 session 的 SessionStart:resume 返回 `{}`；gen/epoch 仍 0/0，四个 seen 不变，prompt_count 7→8，stats 16→18，仅两条 UserPromptSubmit dedup。resume 分支继续通过。
- 完成依据沿用手册 §5.2 和既有收集器的整批样本 p95<50ms，没有修改阈值或判定逻辑。整体 46ms 达标；PreToolUse 单类仍 53ms、resume 798ms 如实披露，不扩展为各事件单独保证，不把前后观测差全部归因于优化。
- 原样 102 条 completion 入库 `docs/review/M1B-CLAUDE-C9-RETEST-HOOKS-2026-10-15.txt`；全部样本、50 组调用/结果、程序 build info/摘要、状态变化和限制入库对应 RETEST-TIMING JSON。复算：`python3 testdata/manual/measure_claude_hooks.py docs/review/M1B-CLAUDE-C9-RETEST-HOOKS-2026-10-15.txt --binary /Users/inxups/.local/lib/style-m1b/style`，status=within-budget、102 条、p95=46ms。
- 验证：`PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_measure_claude_hooks.py' -v` 六项通过；留存 excerpt 与 JSON 样本逐项精确复算、程序 revision/hash、50 组实际调用/结果和 resume 状态核对通过。本次只有验收记录变化，不重复无关 Go 测试；优化提交的双 module test/vet 和定向 race 证据保留。
- 任务、测试、手册、Claude JSON 与既有 review 已确认 M1B-PERF-01 和 C9 在上述口径完成，原失败作为历史保留。按用户要求到此停止，等待新指令；Codex M1b 和 M1 尚未完成。

### 2026-10-16 — M1B-EXAMPLE-01 `[x]` 示例 force push 规则精度

- 约定变更：用户 2026-10-16 指示“codex配置你来，继续任务”，Codex 真实配置改由 agent 执行；Claude 真实配置仍按原约定由用户执行。改动前已把当时的 `~/.codex/config.toml` 备份到 `~/.style-m1b/manual-backup-2026-10-16/`（700 权限，含凭据，不入库）。
- 复现：本 Claude 会话装有 STYLE claude-user hook。agent 用 Python heredoc 修改测试文件，heredoc 只在引号字符串中出现 force push 示例，03:03:30Z 被 `terse-reviewer/no-force-push` 真实拦截（stats `result=blocked`），与 2026-10-15 Codex 误报同因。
- 回归先行：新增 `core/compile/example_force_push_test.go`，编译真实示例 index 后用 `when.Compile` 评估 18 例。旧规则失败 8 例：heredoc/单引号/双引号/提交信息/普通 push 后的引号文本共 5 例误拦；子 shell、`$(...)`、反引号中 flag 后紧跟 `)` 或反引号的 3 例漏拦。lease、普通 push 不拦截。
- 修复：`example/terse-reviewer/STYLE.yaml` 改为 `(?m)` 下 git 位于行首或 `; & | ( \`` 之后，git 与 flag 之间不得出现引号、换行、分隔符或括号，flag 后以空白、分隔符、`)`、反引号或行尾结束。规则注释说明仍是正则：以 git 开头的 heredoc 行和 `bash -c "..."` 嵌套 shell 不区分。`UPDATE_GOLDEN=1` 后 `testdata/golden/index.json` 只有两处 src_sha256 和 pattern 变化，plan golden 不变；同步 `STYLE_DOCS/STYLE-spec-compiled.md` 的示例 pattern。
- 验证：清理 CODEX_HOME/CODEX_SANDBOX，临时 Go 缓存、GOPROXY=off/GOSUMDB=off；新测试 18 例通过，双 module `make test-go`、`make vet-go`、gofmt、`git diff --check` 通过。
- 真实环境：已安装副本替换为新 STYLE.yaml 后，`style sync` 退出 3 且 index 不变；`style trust terse-reviewer` 只展示源/渲染 diff、退出 1、index 不变；`trust --yes` 退出 0 并加载新 pattern，两家用户配置字节 hash 不变。随后本 Claude 会话中：同类 heredoc 放行；scratch 仓库接本地 bare remote，真实 force push 于 03:05:42Z 被 hook 拒绝、remote 保持 e7109f3；force-with-lease 放行并推送到 3967cbd。
- 观察：trust 的 rendered diff 按首尾差异整段替换输出，未变行也显示为 -/+，内容正确但冗长；记录，不在本项修改。

### 2026-10-16 — CODEX M1b 第二轮 `[~]`（模型服务阻塞）

- 基线：Codex 无 STYLE 配置（10-15 C8 卸载已清除），hooks.json 不存在，无 style 插件/marketplace。10-15 的 manual-backup/codex/config.toml 含 STYLE 区，不是原始基线；本轮改用 `manual-backup-2026-10-16`。
- 安装：agent 执行 `install example/terse-reviewer --harness codex --route codex=codex-user --set max_findings=10 --yes`，再装 m1b-probes 并 sync。STYLE 静态区在 config.toml 第 1–3 行，信任区含 5 个 trusted_hash；第二包与 sync 后两家 Codex 文件字节不变。doctor healthy=true，无 codex-login-shell 诊断。
- 真实会话：在 `~/.style-m1b/manual-workspace/codex-b` 运行 4 次 `codex exec --json`（清理 CODEX_HOME/CODEX_SANDBOX）。每次 hook 都未弹信任即执行：SessionStart 输出三条规则、首个 prompt 去重；state 与启动参数一致（gpt-6-sol/minimal、gpt-6-sol/low、gpt-6.1-sol/low）。模型请求全部 502 `Upstream access forbidden`；用户修复后第 5 次为 `Upstream rate limit exceeded`。未完成模型回合，不能证明模型收到注入。用户要求“不行就停”，测试暂停。
- TUI：Terminal 面板启动 codex，先出现 0.159.3→0.163.0 升级提示（跳过，未升级），再出现工作区“Trust this folder?”；agent 未代为信任，其后的 hook 信任行为未观察，配置字节不变。expect 无终端能力应答，TUI 无输出，不作证据。
- 状态：B1/B2/B8/B9/B11 记 `[~]` 部分证据，B3/B4/B7/B10/B12 仍 `[?]`。证据 `docs/review/M1B-CODEX-2026-10-16.json`（不含凭据）。本项无 Go 改动，仅验证 JSON 语法、凭据未入库与 `git diff --check`。
- 下一步：服务恢复后在同一 codex-user 安装上依次补模型投递、compact（需 TUI 和用户确认工作区信任）、skill、MCP、resume、PreToolUse 采样，再切插件路线（B10），最后卸载对照 2026-10-16 备份（B8）。

### 2026-10-16 — CODEX-02/B2 `[x]` 注入投递（minimal/high）

- 用户修复模型服务后，在同一 codex-user 安装上运行 `codex exec --json -m gpt-6-sol -c model_reasoning_effort="minimal"` 与 `"high"` 两个新会话。
- 证据来自 Codex 自己的 rollout：两次都有 `content_item_kinds=["hooks.additional_context"]` 的 developer 消息，文字与 state 一致；minimal 含 low-effort 规则，high 正确不含。两次 rollout 同时包含 STYLE 静态 developer_instructions。
- 模型两次拒绝复述注入内容，不以自述为证；以 harness rollout 证明投递，并与 10-15 的 low 证据合并完成 B2。证据 `docs/review/M1B-CODEX-2026-10-16-R2.json` 的 B2。仅文档/证据变化，验证 JSON 语法与 `git diff --check`。

### 2026-10-16 — CODEX-11/B11 `[x]` PreToolUse 真实耗时

- 计时来源：Codex 0.159.3 内置 OTel。仅给单次运行加 `-c otel.trace_exporter={ otlp-http = { endpoint = "http://127.0.0.1:4318/v1/traces", protocol = "json" } }`（metrics 同理），不写入 config.toml；本地只读 sink 每行记录一个导出请求。`codex.hooks.command` span 带 `hook.event_name`、`hook.source=user`、`hook.command_outcome`，由 harness 计时，含 hook 进程启动与 I/O。
- 新增只读收集器 `testdata/manual/measure_codex_hooks.py`（复用 Claude 收集器的 nearest-rank summarize），按 source/handler 过滤，门槛事件默认 PreToolUse、≥100 条、p95 严格 <150ms，报告重叠数；`test_measure_codex_hooks.py` 6 项覆盖慢样本保留、来源/handler/span 过滤、时间窗与重叠、失败 hook、坏导出、空导出。
- 采样：两个新会话各要求串行 50 次 `printf`，模型实际 52、50 次，全部成功。PreToolUse 102 条 median=40.34ms、p95=46.96ms、max=64.2ms；PostToolUse 102 条 p95=30.1ms；SessionStart/UserPromptSubmit 各 2 条约 24–26ms。0 hook 错误、0 重叠，未剔除样本。第二轮期间另有 B4/B12 Codex 会话并发运行（未导出到该 sink），如实记录。
- 入库 `docs/review/M1B-CODEX-B11-SPANS-2026-10-16.jsonl`（只保留 208 个 hook span 与必要属性，已确认无主机/账号字段），同一命令复算 within-budget。不宣称完成设计 U5 的 1000 次基准。
- 验证：`PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_measure_*.py'` 12 项通过；摘录复算与原始 sink 结果一致；`git diff --check` 通过。

### 2026-10-16 — CODEX-04/B4 `[x]` skill 显式与读取去重

- Codex 本机没有 code-review skill。按 Claude C4 做法，在验收工作区 `.agents/skills/code-review/` 放置现有 fixture（cp -n 后 cmp 一致），不改用户级 skill。
- 显式在前（会话 01a142e8-8fba）：`$code-review` 使 UserPromptSubmit 产生 `review-skill@skill.explicit output`，rollout 的 hook 上下文含 findings 上限文字，模型回复 M1B_SKILL_LOADED（随后拒绝复述）；resume 后让模型只执行 `cat .agents/skills/code-review/SKILL.md`，PostToolUse 记 `review-skill@tool.post dedup`。
- 读取在前（会话 01a142e9-dbe0）：`sed -n 1,20p` → `@tool.post output`，rollout 新增同一段 hook 上下文；`head -n 5` → dedup；resume 后 `$code-review` → `@skill.explicit dedup`，模型仍按 skill 回复并引用已有上下文。
- 三种读取形式和显式入口均被识别，同 session 仅首个入口输出。证据 R2 JSON 的 B4；仅文档/证据变化。

### 2026-10-16 — CODEX-12/B12 `[x]` resume

- 会话 01a142ca-fa6d（gpt-6-sol/high）以 `codex exec resume <id>` 恢复三次，均带原 model/effort。第三次加单次 OTel 导出到独立 sink（4319，与 B11 样本隔离）：SessionStart span completed 26.434ms，随后 UserPromptSubmit 23.117ms，证明 Codex 在 resume 时确实调用 hook。
- STYLE 状态：gen/epoch 0/0，两个 seen 文件不变，prompt_count 1→2→3→4；stats 每次仅新增两条 session-info prompt dedup，没有 session.start 输出，符合设计 a8 的 `resume` 默认不触发。
- 历史：rollout 仍是同一文件，四条用户消息按序保留；hook 上下文仅最初 03:39:21Z 一条。`RUST_LOG=debug` 只显示 hook 指标注册，不作为证据。证据 R2 JSON 的 B12；仅文档/证据变化。

### 2026-10-16 — CODEX-03/B3 `[x]` compact 补发一次

- `codex exec` 没有 `/compact`；`-c model_auto_compact_token_limit=20000` resume 未触发压缩，不作证据。TUI 停在工作区信任（agent 不代为信任）。改用 Codex 自带 `codex app-server` stdio JSON-RPC（schema 由 `codex app-server generate-json-schema` 生成）：initialize → thread/resume → thread/compact/start → 两次 turn/start，单次 `-c` 导出 OTel。
- 首次尝试：compact 完成后结束了 app-server，之后用新 exec 进程发 prompt，只触发 SessionStart(resume)，gen/epoch 不变、无补发。Codex 把 SessionStart(compact) 排在同进程下一回合，进程退出即丢失；流程无效，不计失败。0.159.3 未发 `thread/compacted` 通知，以 `contextCompaction` item 判定完成。
- 有效复测（同一进程）：rollout compacted 04:28:52.987Z；下一回合 SessionStart 输出 terse-reviewer/session-info 与 m1b-probes/session-info 各一次，epoch 0→1、新增 `s.0.1.*` seen，rollout 只新增一条 hook 上下文；两条后续 prompt 均 dedup。压缩后 04:28:53.014Z 的模型输入仍含 STYLE 静态 developer_instructions，U11 结论为可见。证据 R2 JSON 的 B3；仅文档/证据变化。

### 2026-10-16 — CODEX-09/B9 `[x]` md 改动经 trust 生效

- 修改 `~/.style-m1b/packages/terse-reviewer/prompts/session-info.md` 为 `Current session (B9 trusted text): …`（原文另存）。操作失误：第一次 sync 漏传 STYLE_HOME，CLI 用默认 `~/.style`（仅 10-15 的旧 adapters 目录，无包）返回 0；核对 `~/.style` 无新写入，不计为结果。正确传入后：sync 退出 3、`trust terse-reviewer` 预览退出 1，两次 build/index.json 均不变。
- trust 前新会话 01a142fa 的 rollout hook 上下文仍为原文字；`trust --yes` 退出 0，Claude settings 与 Codex 两个文件 hash 不变；新会话 01a142fb 的 rollout 为新文字。
- 还原：拷回原文件，sync 退出 3，`trust --yes` 后 sync 0、list hash_matches=true，原文与仓库源一致。证据 R2 JSON 的 B9；仅文档/证据变化。

### 2026-10-16 — CODEX-07/B7 `[x]` MCP create_pull_request 前注入（本地 stub）

- 真实 GitHub MCP 调用会在外部创建 PR，属对外操作，未执行。新增 `testdata/manual/github_mcp_stub.py`：stdio MCP，暴露 create_pull_request，固定返回 `STYLE_M1B_MCP_STUB: no pull request was created`，不联网、不访问 git。STYLE 规则仅按服务名/工具名匹配（`mcp: github/create_pull_request`），因此覆盖同一 hook 路径。
- 首次（01a142fc）：Codex 以 `approval policy is never` 拒绝 MCP 调用，但 PreToolUse 已记 `before-create-pr@tool.pre output`，rollout 04:33:41.618Z 有 hook 上下文。
- 复测（01a142fd）：只对该工具加单次 `-c mcp_servers.github.tools.create_pull_request.approval_mode="approve"`。PreToolUse 04:34:17.790Z output → rollout 04:34:17.791Z 注入 → 04:34:17.821Z 工具成功返回 stub 文字，注入先于调用结果。config.toml 无 github MCP 残留。
- 局限：未使用真实 GitHub MCP 服务器。证据 R2 JSON 的 B7；验证 stub 实际被 Codex 列出与调用、`git diff --check`。

### 2026-10-16 — CODEX-10/B10 `[x]` 插件/用户级路线切换

- 切换前保存 config.toml、hooks.json 快照。`style install … --route codex=codex-plugin` 删除用户 hooks.json、把 5 个 trusted_hash 换为插件键，生成插件目录（plugin.json、hooks/hooks.json、bin/style launcher）；Claude settings hash 不变。
- 复用 10-15 建的 `~/.style-m1b/.agents/plugins/marketplace.json`，`codex plugin marketplace add` / `codex plugin add style-runtime@style` 均成功，installedPath 为 Codex 缓存副本，launcher 指向固定 STYLE_BIN；Codex 只追加 `[marketplaces.style]` 与 `[plugins."style-runtime@style"]`。
- 插件会话 01a142fe：OTel 中 SessionStart/UserPromptSubmit/PreToolUse/PostToolUse 各 1 次，全部 source=plugin，user 0 次，无错误；rollout 注入 session-info 与 M1B_WARN_MODEL；exec 下无信任提示（STYLE 预置插件摘要）。
- 切回：`codex plugin remove`、`codex plugin marketplace remove style` 后 config.toml 与注册前逐字节相同；再装 codex-user 后 config.toml、hooks.json 与切换前快照逐字节相同，doctor codex-user installed/trusted、plugin 未安装。新会话 01a142ff 四类 hook 各 1 次全部 source=user。
- 残留：`~/.codex/plugins/cache/style` 空目录由 Codex 留下，非 STYLE 文件，未删除，记录。证据 R2 JSON 的 B10；仅文档/证据变化。

### 2026-10-16 — CODEX-08/B8 `[x]` 卸载字节往返

- 用户选择“完整卸载后重装”，授权本次卸载同时移除并由 agent 重装 Claude 配置。卸载前保存 Codex config.toml/hooks.json 与 Claude settings.json；Claude 12 条 hook 全为 owner=style。
- 基线：用户在测试中自行把 model/effort 改为 gpt-6.1-sol/medium，10:48 备份不能直接 cmp。脚本按托管标记去掉 config.toml 头部 STYLE 区与末尾 agenthooks 区得到预期字节，与 10:48 备份逐行比较仅 model、model_reasoning_effort 两行不同（只比较哈希，不输出凭据），凭据行一致。
- 卸载：手册 §7 单行 env 依次卸载 m1b-probes、terse-reviewer，均退出 0 并更新两条路线，list 无包。config.toml 与预期逐字节相同，hooks.json 不存在，config 中唯一的 “style” 为用户自己的 `[projects."…/style-cli"]`；Claude settings 去掉 hooks 后与卸载前完全相等，CLAUDE.md 不存在。
- 重装（B1 前半）：`install example/terse-reviewer --harness codex,claude-code --route codex=codex-user --route claude-code=claude-user --set max_findings=10 --yes` 后 Codex 两文件与卸载前逐字节相同（固定订阅面，摘要不变），Claude 恢复 12 条 hook。证据 R2 JSON 的 B8；仅文档/证据变化。

### 2026-10-16 — CODEX-01/B1 `[x]` 全新安装与信任次数

- B8 卸载后全新重装第一包，在 Terminal 面板启动 `codex -c check_for_update_on_startup=false`。用户确认一次工作区“Trust this folder?”，Codex 在 config.toml 写入 `[projects."…/codex-b"] trust_level = "trusted"`；随后直接进入输入框，没有 hook 信任提示。用户按 F2 查看的两条警告：priority 服务档位不受支持、`-c` 覆盖需嵌入模式，均与 hook 无关。
- 用户发送“只回复 B1_TUI…”：会话 01a14304 的 rollout 含静态 developer_instructions 与 SessionStart hook 上下文（只装第一包时仅 terse-reviewer），stats 为 session.start output + prompt dedup，模型回复 B1_TUI。
- 第二包 install → sync → uninstall → sync → install 五步后 Codex config.toml/hooks.json 哈希始终不变，无需重新信任；最终两包均 hash_matches，Claude settings 与 B8 前逐字节相同。
- Terminal 标签 c2 因用户已输入，agent 不能代为关闭，请用户自行关闭。证据 R2 JSON 的 B1；仅文档/证据变化。

### 2026-10-16 — M1b Codex `[x]`，M1 完成

- Codex B1–B12 全部 `[x]`；e2e 条目以两家真实 rollout/会话注入与阻断证据标为 `[x]`。M1a、M1b Codex、M1b Claude、M1B-PERF-01 均已完成。
- 最终验证：清理 CODEX_HOME/CODEX_SANDBOX、临时 Go 缓存、GOPROXY=off/GOSUMDB=off，双 module `make test-go`（30 个包 ok）与 `make vet-go` 通过；`test_measure_*.py` 12 项通过；`git diff --check` 通过。
- 保留项：B0.6 继承宿主 `CODEX_SANDBOX` 时 agenthooks 的 `TestCopilotDetection` 失败，按既定策略只记录（清理后通过），不属于里程碑验收。已知限制：示例 force push 规则仍是正则；B7 用本地 stub；B11 不等于 U5 的 1000 次基准；trust 的 rendered diff 冗长；Codex 留下空插件缓存目录。
- 环境现状：两包仍以 codex-user / claude-user 安装在 `~/.style-m1b`，Codex 工作区 codex-b 已被用户信任；本地 OTLP sink 为临时进程。

### 2026-10-16 — CODEX-07/B7 补做真实 GitHub MCP 调用

- 用户指示“B7 做一次真实的，创建个简单 PR，用完关闭”。从 origin/main（d3c0874）建临时分支 `style-m1b-b7-probe-20261016`，只加一个说明文件后推送；本地 main 的 79 个未推送提交未推送。
- 单次挂载 GitHub 远程 MCP：`-c mcp_servers.github.url="https://api.githubcopilot.com/mcp/"`、`bearer_token_env_var="GITHUB_PAT"`（值取自 `gh auth token`，未打印、未写入配置），并只对 `create_pull_request` 设 `approval_mode="approve"`。
- 会话 01a14310：stats 04:55:27.280Z `before-create-pr@tool.pre output`，rollout 04:55:27.281Z 注入“Before creating a pull request, confirm the branch and test status.”，04:55:28.807Z MCP 调用完成；GitHub 显示 inxups/style-cli#1 创建于 04:55:28Z，注入在真实调用之前。
- 清理：`gh pr close 1` 附评论，closedAt 04:55:53Z、未合并；删除远端与本地临时分支及 worktree，远端只剩 main。已关闭的 PR 记录会留在 GitHub 上，无法删除。证据 R2 JSON 的 `B7.real_run`；仅文档/证据变化。

### 2026-10-16 — CODEX-11/B11 补测设计 U5 基准（731 次，用户叫停）

- 用户要求补 U5（设计 §15.1：固定订阅面连续 1000 次 PreToolUse）。codex-user 路线、gpt-6.1-sol/medium，在 codex-b 串行运行 `codex exec`，每个会话要求逐次调用 shell 100 次 `printf STYLE_M1B_U5`；每次运行单独加 OTel otlp-http json 导出到本地 sink，期间无其他会话导出到该 sink。
- 04:58:53Z–06:00:12Z 共 8 个会话：前 7 个各 100 次，第 8 个第 32 次时按用户“停了吧，够多了”停止（进行中那次的 span 未导出）。PreToolUse 731 条 median=36.8ms、p95=41.6ms、max=44.9ms；PostToolUse p95=28.4ms；全部 1478 条 hook p95=40.3ms；0 错误、0 重叠。各会话 p95 40.1–42.9ms，每会话前 50 次 p95 42.0ms、前 7 个会话最后 30 次 p95 41.45ms，未见随 state 增长变慢。
- 结论：未满 1000 次，U5 记为部分完成；已有样本 p95 远低于 150ms 预算，没有要改设计（按规则生成 matcher）的迹象。第 7 个会话期间模型服务多次重连，发生在调用之间，不影响 hook span。驱动脚本的每轮计数用了 `python3 -I`，破坏收集器同目录导入而为空，改由单独进程计数，不影响样本。
- 入库 `docs/review/M1B-CODEX-U5-SPANS-2026-10-16.jsonl`（1478 个 hook span，只保留 hook 属性，已查无主机/账号字段）；R2 JSON 新增 `U5`。复算命令见 TESTS 的 B11；仅文档/证据变化。

### 2026-10-16 — M1B-EXAMPLE-02 `[x]` force push 规则改为 shell 解析

- 原因：EXAMPLE-01 的正则只看“行首或分隔符后的 git”，看不到引号，以 git 开头的 heredoc 行、引号里出现在 `&&` 之后的 git 仍误拦；`bash -c "…"`、`eval`、`bash <<EOF` 漏拦。本次改动中，本会话尚未更新的 Claude hook 两次把 Bash 命令里测试字符串中的 force push 文字当成真实命令拦截（真实复现），改用编辑工具。
- 方案：新增条件操作 `shell_regex`，不引入外部依赖。`core/when/shell.go` 尽力按 POSIX shell 拆出会执行的简单命令：去引号、续行，分隔符、子 shell、`$(...)`、反引号，`sh|bash|zsh|dash|ksh -c` 脚本与喂给它们的 heredoc，`eval` 参数，未加引号 heredoc 的命令替换；注释、重定向目标、加引号 heredoc、here-string 是数据；去掉前导赋值、保留字和 `command/builtin/exec/nohup/env`。参数按单个空格连接，含空白或特殊字符的参数重新加单引号，引号内的 `--force` 不构成独立参数。嵌套上限 8 层；输入不完整不报错。
- 示例规则改为 `shell_regex`，pattern 锚定单条命令：`^git(?:\s+-[cC]\s+\S+)*\s+push\b.*\s(?:--force|-f|\+\S+)(?:\s|$)`。`core/spec` 接受该操作并同样做 RE2 校验，`schema/index.schema.json` 加入枚举，`STYLE-spec-runtime.md` §4.3 写明读取范围与限制（`sudo`、`xargs`、管道喂给 shell、脚本文件不展开），`STYLE-spec-compiled.md` 示例与 golden 同步（只有 src_sha256、op、pattern 变化）。
- 验证：`shell_test.go` 25 例拆分、嵌套上限、操作语义；`FuzzShellCommands` 30 秒约 779 万次无崩溃或卡死；示例规则 30 例（新增 bash -c、eval、喂给 bash 的 heredoc、未加引号 heredoc 的命令替换、赋值加重定向、续行、引号内 +refspec 均拦截；以 git 开头的 heredoc 行、注释、单个参数内的 --force、echo 均放行）；新增 `bad shell regex` 校验例。双 module `make test-go`、`make vet-go` 通过，`git diff --check` 通过。未在真实 harness 重装复测。

### 2026-10-16 — M1B-TRUST-01 `[x]` trust diff 精简

- 原因：B9 记录 trust 的 diff 只去掉首尾相同行，中间整段 -/+，一个文件里有两处改动时（如 rendered entry 的 src_sha256 与正文）中间未变行全部重复显示，内容正确但冗长。
- 修改：`internal/app/trust_diff.go` 在去掉公共首尾后按行做 LCS 对齐，输出带 3 行上下文的统一 diff，多处改动分成多个 `@@` 段，间隔不超过 6 行的改动合并为一段。改动区域的比较表超过 4M 格（约 2048×2048 行）时退回原来的整段替换，避免大文件或重复行导致工作量失控。头部 `--- approved/` / `+++ current/`、Binary、Empty file、无末尾换行提示保持不变；JSON 输出的 `source_diff`/`rendered_diff` 字段结构不变，只是 diff 文本更短。
- 验证：新增 `internal/app/trust_diff_test.go`：两处分离改动得到两段且中间行不重复；相邻改动合并并正确对齐插入行；新增/删除文件的行号为 0,0；无末尾换行；2100 行全改退回整段。既有 `sync_command_test` 的 trust 预览、Binary/Empty file 断言通过。双 module `make test-go`（30 个包 ok）、`make vet-go` 通过，`git diff --check` 通过。未在真实 trust 流程中重放 B9。

### 2026-10-16 — M1B-EXAMPLE-02 / M1B-TRUST-01 真实环境更新与 Codex 实测

- 按用户指示用 819147a 按手册 §1 重建 `~/.local/lib/style-m1b/style`（先写 style.new 再 mv），sha256 9c137245… → 3982860d…。terse-reviewer 是复制安装（link=no），对已安装包再次 install 被拒（提示用 sync/trust），因此同 B9 把新 STYLE.yaml 复制到 `~/.style-m1b/packages/terse-reviewer`。
- sync 退出 3（1 个文件变化）→ trust 预览退出 1：新 diff 中 source 为一段 `@@ -59,10 +59,9 @@`，rendered 三个 entry 的 src_sha256 与 op/pattern 各成独立小段（共 6 段），不再整段 -/+ → `trust --yes` 退出 0；随后 sync 0、两包 hash_matches、doctor healthy，Codex config.toml/hooks.json 与 Claude settings.json 哈希不变。
- Codex 真实会话 01a1435b（codex-user，gpt-6.1-sol/medium），临时仓库 ex02-repo 只连本地 bare 远端：`cd ex02-repo; bash -c 'git push --force origin main'` 被 PreToolUse 拦截；写入含以 git 开头 force push 行的引号 heredoc 文档运行成功；`--force-with-lease` 运行成功（Everything up-to-date）；直接 `git -C ex02-repo push --force origin main` 被拦截。stats 两条 blocked，远端 main 仍为 6c88e18。用旧 EXAMPLE-01 pattern 对同样四条命令复核：第 1 条旧规则会放行、第 2 条会误拦，新规则两处都改正。
- 同一 STYLE_HOME 也服务本 Claude 会话的 claude-user hook，更新后本会话随之使用新规则；Claude 配置文件未改动。证据 `docs/review/M1B-EXAMPLE-02-REAL-2026-10-16.json`；本项仅文档/证据变化，验证 JSON 语法与 `git diff --check`。

### 2026-10-16 — M1B-EXAMPLE-03 force push 规则补漏

- 起因：复核 EXAMPLE-02 pattern 时用 Python 对同一正则复算，`git push -fu origin main`、`git --git-dir=.git push --force origin main`、`/usr/bin/git push --force origin main`、`git --no-pager push -f origin main` 均不匹配。
- 修改：`example/terse-reviewer/STYLE.yaml` 的 no-force-push pattern 接受带路径的 git、push 前的任意全局选项（带独立值的 `-C`/`-c`/`--git-dir`/`--work-tree`/`--namespace`/`--attr-source`），以及含 `f` 的短选项簇（`f` 前有 `o` 时视为 `-o` 的值）。`UPDATE_GOLDEN=1` 重生成 `testdata/golden/index.json`（src_sha256 ff1753d3… → fb5d9905…，pattern 变化），`STYLE_DOCS/STYLE-spec-compiled.md` 示例同步。
- 验证：`core/compile/example_force_push_test.go` 由 30 例增至 46 例（新增 9 拦截、7 放行）。双 module `make test-go`（30 个包 ok）、`make vet-go`、`git diff --check` 通过。
- 未做：真实安装 `~/.style-m1b` 仍是 EXAMPLE-02 版本，未 sync/trust；`-o -f`（`-f` 作为 push option 的值）会误拦，git alias 与 `sudo` 包装不覆盖。

### 2026-10-16 — M1B-EXAMPLE-03 真实环境更新与 Codex 实测

- 按用户指示把 80a6fd8 的 `example/terse-reviewer/STYLE.yaml` 复制到 `~/.style-m1b/packages/terse-reviewer`。固定程序未重建：819147a 之后唯一的 Go 改动是 `core/compile/example_force_push_test.go`，规则本身是包数据，sha256 仍为 3982860d…。
- 更新前 sync 0；复制后 sync 退出 3（1 个文件变化）→ trust 预览退出 1（共 6 段：source `@@ -60,8 +60,10 @@`，两个 harness 的 no-force-push 各有 src_sha256 与 pattern 小段，codex on-review-prompt 一段 src_sha256）→ `trust --yes` 退出 0；随后 sync 0、两包 hash_matches、doctor healthy，Codex config.toml/hooks.json 与 Claude settings.json 哈希不变。
- Codex 真实会话 01a14364（codex-user），临时仓库 ex02-repo 只连本地 bare 远端：`git -C ex02-repo push -fu origin main`、`git --no-pager -C ex02-repo push -f origin main`、`git --git-dir=ex02-repo/.git push --force origin main`、`/usr/bin/git -C ex02-repo push --force origin main` 均被 PreToolUse 拦截（stats 四条 blocked）；`push -u` 与 `push --follow-tags` 运行成功（Everything up-to-date，`-u` 在临时仓库设置了上游跟踪）。远端 main 仍为 6c88e18。用 EXAMPLE-02 pattern 对同样命令复核：四条拦截命令旧规则全部会放行。
- 同一 STYLE_HOME 也服务本 Claude 会话的 claude-user hook，本会话随之使用 EXAMPLE-03 规则；Claude 配置文件未改动。证据 `docs/review/M1B-EXAMPLE-03-REAL-2026-10-16.json`；本项仅文档/证据变化，验证 JSON 语法与 `git diff --check`。

### 2026-10-16 — B0.6 继承宿主环境的全量测试

- 复现：继承本 Claude 会话环境，加 `CODEX_SANDBOX=seatbelt`、`CODEX_HOME=<临时目录>` 或二者同时，`go test -C agenthooks -count=1 ./...` 都只有 `TestCopilotDetection` 失败（`codec_copilot_test.go:387: env detection = "codex"`），根 module 不受影响。原因：`agenthooks/detect.go` 的 `detectFromEnv` 先查 Cursor/Codex/Gemini/OpenCode 变量，再查 Copilot，测试只设置了 `COPILOT_CLI` 和 `CLAUDE_PLUGIN_ROOT`。
- 修改 agenthooks（用户指示完成 B0.6，取代原先“只记录不修改”的策略）：只改测试，在 `TestCopilotDetection` 开头清空优先于 Copilot 的变量，做法与同仓库 `TestDetectClaudeCodeMarker`、`TestUndetectableProviderNoOps` 相同。检测顺序和生产代码不变，职责边界不受影响。
- 验证：上述三种 Codex 变量组合，以及 `CURSOR_AGENT=1 GEMINI_CLI=1 OPENCODE=1`，两个 module 全量 `go test -count=1 ./...` 都无失败。清理环境的双 module `make test-go`（30 个包 ok）、`make vet-go`、`git diff --check` 通过。

### 2026-10-16 — M1B-SHELL-01 shell_regex 展开包装命令和管道脚本

- 起因：用户要求处理 force push 规则的剩余漏洞，其中 `sudo`、`xargs`、`find -exec`、管道喂给 shell 属于 shell_regex 的读取范围，先在 `core/when/shell.go` 补上，规则 pattern 另作 EXAMPLE-04。
- 修改：带选项的 `sudo`/`doas`/`env`/`nice`/`timeout`/`stdbuf`/`xargs`/`time`/`exec` 跳过自身选项（短选项簇里取值的字母可带附着值或下一个词；列出的长选项可带独立值；`timeout` 再跳过时长）后另作一条命令读，结果再走同一流程，所以嵌套包装和 `sudo bash -c` 都能读到；`env -S` 的值按空白拆开。`find` 的 `-exec`/`-execdir`/`-ok`/`-okdir` 读到 `;` 或 `+`。解析时记录单个 `|`/`|&` 的上游命令，shell 没有脚本文件参数（或带 `-s`）时读上游 `echo`/`printf`（展开 `%s`、`%%`、`\n`、`\t`、`\\`）的输出和只读 heredoc/here-string 的 `cat`。here-string 改为挂在命令上，喂给 shell 时当脚本读。不带选项的简单包装仍直接去掉，`time` 从保留字移到包装命令。规范 `STYLE_DOCS/STYLE-spec-runtime.md` §4.3 同步。
- 验证：`core/when/shell_test.go` 拆分用例 25 → 43 例，原 25 例输出不变；FuzzShellCommands 30 秒约 722 万次输入无崩溃；双 module `make test-go`（30 个包 ok）、`make vet-go`、`git diff --check` 通过。

### 2026-10-16 — M1B-EXAMPLE-04 force push 规则剩余漏洞

- 修改：`example/terse-reviewer/STYLE.yaml` 的 no-force-push pattern 在 push 之后逐词读参数。`-o` 及以 o 结尾的短选项簇只能和它的值一起被跳过，所以 `-o -f`、`-uo --force`、`-of` 不再算 force；其他含 f 的短选项簇、`--force`、新增的 `--mirror`、`+refspec` 算 force。命令名另接受带引号的含空格路径 `'…/git'`。`UPDATE_GOLDEN=1` 重生成 golden，`STYLE_DOCS/STYLE-spec-compiled.md` 示例同步。
- 验证：`core/compile/example_force_push_test.go` 新增 11 个拦截例（`--mirror`、push option 值之后的 `-f`、`-fo`、带空格路径、sudo、嵌套 nice/timeout、xargs、find -exec、echo 管道进 bash、cat heredoc 管道进 sh、here-string 喂给 bash）与 5 个放行例（`-o -f`、`-uo --force`、`--mirrors`、带空格路径无 force、echo 管道进 tee），共 61 例。双 module `make test-go`、`make vet-go`、`git diff --check` 通过。
- 更正：此前记录的用例数有误，EXAMPLE-02 实为 29 例（记为 30），EXAMPLE-03 实为 45 例（记为 46）。本次重新计数并改正了 TASKS 中的数字，旧 PROGRESS 条目保持不变。
- 未做：长选项 `--push-option`/`--repo`/`--receive-pack` 后单独一个形如 `-f` 的词仍会被当成 force（保守误拦）；git alias、脚本文件内容不覆盖；真实安装尚未 sync/trust 到此版本。

### 2026-10-16 — M1B-SHELL-01 / M1B-EXAMPLE-04 真实环境更新与 Codex 实测

- 按用户指示用 88b25b3 按手册 §1 重建 `~/.local/lib/style-m1b/style`（先写 style.new 再 mv），sha256 3982860d… → 1f2b1691…，重建后 sync 0。旧程序备份在本会话 scratchpad `style.pre-example04`。
- 复制新 STYLE.yaml 到 `~/.style-m1b/packages/terse-reviewer` → sync 退出 3 → trust 预览退出 1（共 6 段）→ `trust --yes` 退出 0；随后 sync 0、两包 hash_matches、doctor healthy，Codex config.toml/hooks.json 与 Claude settings.json 哈希不变。
- Codex 真实会话 01a14377（codex-user），临时仓库 ex02-repo 只连本地 bare 远端，事先给远端设 `receive.advertisePushOptions=true`：`sudo -n git -C ex02-repo push --force origin main`、`echo 'git -C ex02-repo push --force origin main' | bash`、`git -C ex02-repo push --mirror origin` 被 PreToolUse 拦截（stats 三条 blocked）；`git -C ex02-repo push -o -f origin main` 运行成功（Everything up-to-date）。远端只有 refs/heads/main 6c88e18，未变。按 EXAMPLE-03 的读取结果和 pattern 复核：前三条旧规则会放行，第四条会误拦。
- 同一 STYLE_HOME 也服务本 Claude 会话的 claude-user hook，本会话随之使用新程序和新规则；Claude 配置文件未改动。证据 `docs/review/M1B-EXAMPLE-04-REAL-2026-10-16.json`；本项仅文档/证据变化，验证 JSON 语法与 `git diff --check`。

### 2026-10-16 — DOCS-01 任务清单和测试矩阵过时表述修正

- 按用户指示复查两份清单，只改表述和位置，不改任何条目的状态或证据。
- 任务清单：19 个 `[x]` 条目原先排在“后续 backlog”标题下。5 个 M1a 补项（A13/T52、RV-CONFORMANCE-01/04、RV-CLI-HOOK-01、RV-A20-01）移到回顾跟进项末尾；14 个 M1b 条目移入新的“M1b 跟进项”段，按处理顺序排列；backlog 只留 M2–M4。条目总数 73 不变，所有 `[x]` 条目的标题前后一致。
- 修正与现状矛盾的表述：M1b Claude 的“Codex M1b 和 M1 仍未完成”；RV-T3X-01 的“A10/A11 保持部分状态”；RV-CONFORMANCE-04、M1B-PREP、M1B-FIX-01、CLAUDE-04-PREP 中仍写“待验收/待用户结果”的部分，改为指向后来的验收条目。
- 测试矩阵：B1.6、B1.8、B1.21、B1.22、A11、R01–R08 段落、B2、B5、C6 的“仍未完成/待验证/未修复”改为注明由哪个条目完成或修正。新增两处如实说明：Codex 子 agent context 投递（设计 U7）未实测，Claude workspace trust 仍按设计报告为未验证。
- 两份文件的更新时间改为 2026-10-16。只改文档。

### 2026-10-16 — DOCS-02 设计 §15.1 未决项按 M1b 结果回填

- 按用户指示，在 `STYLE_DOCS/STYLE-cli-design.md` §15.1 表格新增“M1b 结果（2026-10-16）”列，并在表前说明这一列的含义。只依据测试矩阵和 review JSON 中已有的真实证据，没有新增实测。
- 结论分四类：
  - U3 接受：B10 插件路线，B1 用户级路线的 exec 与 TUI。
  - U11 可见：B3。
  - U-F9 已由 T42 关闭。
  - 部分验证：U2（手动登记可用，但没有自动化）、U5（731/1000 次，p95 41.6ms）、U10（只验证了 `-c`/`-m`）、U-C1、U-C3、U-C5（只验证了 PreToolUse）、U-T1（只验证了 resume）、U-V（实测版本未写入示例包的 requires）。
  - 未测：U6、U7、U9、U-C2、U-C4、U-T2–U-T6。
- 原有各列与做法不变，适配器能力、默认值和报告都没有因此修改。只改文档。

### 2026-10-16 — M1B-U7 Codex 子 agent context 投递实测

- 用户指示开始 U7。探针规则 `subagent-marker` 加入 `testdata/manual/m1b-probes/STYLE.yaml`（`target: event`、`event: subagent.start`，正文无条件）。plan 中 Codex 为 native，Claude 为 skipped。
- 真实安装：固定程序 1f2b1691（88b25b3）未重建。复制到 `~/.style-m1b/packages/m1b-probes` 后 sync 3 → trust 预览 1 → `trust --yes` 0 → sync 0，doctor healthy，index 有 codex subagent.start 条目；Codex config.toml/hooks.json、Claude settings.json/CLAUDE.md 哈希不变。
- 会话 01a1438b-9ee2（codex-user，gpt-6.1-sol/medium）用 `spawn_agent` 启动子线程 01a1438b-b9ea。
  - 子 agent scope 的 stats 07:09:53.101Z 记 `subagent-marker@subagent.start output`，摘要与正文一致。
  - 子线程 rollout 07:09:53.102Z 有内容逐字相同的 `hooks.additional_context` developer 消息。
  - 父线程 main scope 与 rollout 中没有这条注入。
  - 结论：U7 成立，F7 留在 M1。
- 附带核对：子 agent 通过 code-mode `exec` 工具运行命令。今天的主会话 rollout 也都走这个工具（861 次），不是新的工具路径。
- 还原：已安装副本恢复原字节，sync 3 → trust --yes 0 → sync 0，两包 hash_matches，index 无 subagent 条目，lock 与四个配置文件哈希和探针前一致。仓库 fixture 保留新规则，README 注明它会进入每个 Codex 子 agent。
- 文档：测试矩阵新增 CDEX-U7，B1.21、A11、e2e 同步；任务清单新增 M1B-U7；设计 §15.1 U7 与运行时规范事件表、阶梯说明同步。不覆盖嵌套子 agent、fork 启动、子 agent 内的工具规则。

### 2026-10-16 — M1B-CR-01 shell_regex 跳过 shell 选项的值

- 来源：只看代码的审查。`core/when/shell.go` 的 `shellScriptArgument` 只把 `-o`/`+o` 当带值选项，`bash --rcfile /dev/null -c 'git push --force origin main'` 只拆出外层一条命令，示例规则的 `^git` 锚定匹配不到，force push 放行；`-O extglob` 同理。`readsScriptFromStdin` 认 `--rcfile` 但漏了 `-O`。
- 修复：新增 `shellOptionValues`，两处共用。`--rcfile`、`--init-file` 取一个值；`-`/`+` 开头的短选项簇中每个 `o`/`O` 取一个值。本机 bash 5.3.15 实测 `-eo pipefail -c`、`-oe pipefail -c`、`-co pipefail '…'`、`-cO extglob '…'`、`+O extglob -c`、`-c -O extglob '…'` 都执行了脚本，与该规则一致。
- 测试：`shell_test.go` 新增 3 例，`example_force_push_test.go` 新增 3 个拦截例；修复前 6 例失败。`go test ./core/...`、`go vet ./core/when ./core/compile`、gofmt 通过（临时 GOPATH/GOMODCACHE/GOCACHE，去掉 CODEX_HOME/CODEX_SANDBOX）。
- 文档：运行时规范 §4 `shell_regex` 说明补充选项值的跳过规则，并在不展开列表中写明 `fish` 等未列出的 shell。未改 agenthooks。真实安装未更新（固定程序未重建）。

### 2026-10-16 — M1B-CR-02 tool.input 条件值不做 HTML 转义

- 来源：只看代码的审查。`internal/adapter/{codex,claude}/runtime.go` 的 `flattenJSONFields` 对对象和数组用 `json.Marshal`，默认 HTML 转义；根路径 `tool.input` 先写入 compact 原文，随后又被这个转义结果覆盖。临时测试中 `{"command":"a && b > c <d"}` 的 `tool.input` 变成 `a \u0026\u0026 b \u003e c \u003cd`，`tool.input.command` 正常。
- 影响：对 `tool.input` 或 `tool.input.<对象>` 写含 `&`、`<`、`>` 的条件匹配不到，模板不受影响（validate 禁止事件变量进模板）。
- 修复：新增 `encodeJSONText`（`json.Encoder` + `SetEscapeHTML(false)`，去掉末尾换行），替换三处 `json.Marshal`。键排序与数字、布尔的表示不变，所以现有 golden 不变。未改 agenthooks。
- 测试：两家新增 `TestEnrichKeepsShellCharactersInToolInput`，暂存修复后单独运行确认失败，恢复后 `go test ./internal/...`、gofmt 通过（隔离缓存，去掉 CODEX_HOME/CODEX_SANDBOX）。真实安装未更新。

### 2026-10-16 — M1B-CR-03 hook 命令不写相对路径

- 来源：只看代码的审查。`internal/adapter/codex/route.go` 与 `internal/adapter/claude/route.go` 在 `command` 为空时用 `os.Args[0]`，生产代码从不传 command。隔离 HOME 复现：在程序目录执行 `./style install <pkg> --harness codex --route codex=codex-user --yes`，hooks.json 的命令以 `./style --style-home=…` 开头。
- 影响：harness 从别的工作目录启动 hook 时找不到程序，hook 进程启动失败，harness 按非阻断错误继续，规则不生效且没有提示（程序没能运行，STYLE 自己的 fail-closed 也无从生效）。
- 修复：新增 `internal/adapter/command.go:HookCommand`。空命令取 `os.Args[0]`；绝对路径和不含分隔符的名字原样返回（名字交给 PATH 查找，避免把包管理器的版本目录写死）；其余相对路径用 `filepath.Abs` 转换。两条路线改为调用它。插件路线 hooks.json 用 `$PLUGIN_ROOT/bin/style`，不受影响。
- 测试：`command_test.go`（5 种写法与空命令），两家 route 测试在 `os.Args[0]=./style` 时检查配置中的绝对路径；暂存路线修复后两条路线测试失败。`go test ./internal/...`、vet、gofmt 通过（隔离缓存，去掉 CODEX_HOME/CODEX_SANDBOX）。重建临时程序后隔离 HOME 端到端安装，hooks.json 命令为绝对路径。
- 真实安装由绝对路径 `~/.local/lib/style-m1b/style` 安装，不受影响，未更新。未改 agenthooks。

### 2026-10-16 — M1B-CR-04 包快照不收录 .git 与 .DS_Store

- 来源：只看代码的审查。`internal/app/lifecycle_store.go:snapshotPackage` 用 `filepath.WalkDir` 收录包目录下所有普通文件。在隔离 HOME 中复现：
  - 包目录 `.git` 里有 300 KB 文件时，安装失败：`.git/objects/pack.bin: package file exceeds size limit`。
  - 换成小 `.git` 后安装成功，但 `packages/m1b-probes/` 中有 `.git/HEAD` 与 `.DS_Store`，lock 的 files 也记了这两项。
  - `--link` 包在源目录出现 `.DS_Store` 后，sync 报 `1 changed files: .DS_Store`。
- 修复：遍历时跳过名为 `.git`、`.hg`、`.svn`（目录用 SkipDir，文件和链接直接略过，包括 worktree 的 `.git` 文件）和 `.DS_Store` 的条目，它们不复制、不哈希。安装、sync、list、trust 共用这个快照，所以行为一致。规范层读取的 STYLE.yaml 和规则路径不受影响。
- 兼容：如果已有安装的 lock 记录了这类路径，升级后 sync 会报告这些路径已删除，需要 trust 一次。真实安装 `~/.style-m1b` 的 lock 中没有这类路径。
- 测试：新增 `package_snapshot_test.go` 两例，暂存修复后都因 `file-too-large` 失败。根 module 与 agenthooks 全量测试、vet、gofmt 通过（隔离缓存，去掉 CODEX_HOME/CODEX_SANDBOX）。未改 agenthooks。

### 2026-10-16 — M1B-CR-05 state.Update 不再每次 hook 都做 F_FULLFSYNC

- 来源：只看代码的审查，用户指示修复。`internal/runtime/runtime.go` 对每个事件都调用 `core/state.Update`。`Update` 总是把 session.json 和 scope.json 各写一遍，只为刷新 `updated_at`。`writeJSON` 每个文件调用一次 `File.Sync`，再对目录调用一次 `Sync`。Go 在 Darwin 上把 `Sync` 实现为 `fcntl(F_FULLFSYNC)`（`internal/poll/fd_fsync_darwin.go`），所以每个 hook 要做 4 次清空磁盘缓存的同步。临时测试测得单次 Update p50 约 14ms、p95 约 16ms，与 PERF-01A 插桩得到的 11.9ms 一致。
- 修复（`core/state/state.go`）：
  - 读取时记录文件是否存在；比较除 `updated_at` 外的字段，只重写确有变化或尚不存在的文件，写入时才更新 `updated_at`（该字段没有读者，现在表示最后一次变更）。
  - 都没变时用 `os.Chtimes` 刷新 scope.json 的修改时间，Cleanup 按修改时间判断活跃的语义不变。
  - 实际写入仍是同目录临时文件加 rename；Darwin 上改用 `syscall.Fsync`（`sync_darwin.go`），其他平台仍用 `File.Sync`（`sync_other.go`）。
- 取舍：PERF-01A 曾明确不删 fsync。本次保留了原子写和 fsync，只去掉无变化时的写入，并在 Darwin 上不再清空磁盘缓存。断电时可能丢失最近一次计数或 effort 更新；最坏情况是一次重复注入，或下一次事件重新解析 effort。状态文件 7 天过期，不是配置或安装数据。安装、sync 使用的 `writeAtomic` 不变，仍用 `File.Sync`。
- 附带效果：多数工具事件不再写 session.json，并行 hook 之间的读改写覆盖窗口也随之缩小，但跨进程竞争本身没有处理。
- 测试：
  - 新增 `TestUpdateRewritesOnlyChangedStateFiles`：未变化的事件不改两份文件的字节，只刷新 scope 的 mtime；prompt 只写 scope；model 变化写 session；新 agent 首次写 scope。暂存修复后失败。
  - 根 module 与 agenthooks 全量测试、`go vet ./...`、`GOOS=linux go vet ./core/state`、`GOOS=windows go build ./core/state`、`go test -race -count=1 ./core/state ./core/engine ./internal/runtime`、gofmt 通过。均使用临时 GOPATH/GOMODCACHE/GOCACHE，并去掉 CODEX_HOME/CODEX_SANDBOX。
- 计时（离线，同一台机器）：
  - 进程内 200 次 Update：未变化 p50 0.16ms / p95 0.29ms；prompt p50 0.33ms / p95 0.57ms。修复前约 14ms / 16ms。
  - 隔离 HOME 下的 codex-user 安装（m1b-probes），真实 hook 进程（`agenthooks run --provider=codex`），每次清空 state 后先发 SessionStart，再发 200 次 PreToolUse 和 50 次 UserPromptSubmit：
    - 修复前：PreToolUse p50 18.7ms / p95 21.4ms；UserPromptSubmit p50 19.1ms / p95 21.6ms。
    - 修复后：PreToolUse p50 7.0ms / p95 8.2ms；UserPromptSubmit p50 8.1ms / p95 10.5ms。
  - 这是离线进程计时，不替代真实 harness 的 U5/C9 测量；固定程序和真实安装未更新。

### 2026-10-16 — DOCS-03 当前 adapter 实现总览

- 用户要求将当前 adapter 全部内容整理为 Markdown。新增 `docs/STYLE-ADAPTERS-CURRENT.md`（源码基线 dd7438ec），按分层、公共接口、检测/安装路线、事件/能力、system/replace、fallback、工具/MCP/skill、字段/effort、运行时、输出/限制、验证和未决项组织。
- 逐项区分 Facts 声明、codec 可接收、实际安装订阅和已有真实证据。明确 Codex 固定 surface、Claude 动态 surface；PermissionRequest 虽有 runtime 映射但未默认订阅；Claude SubagentStart 不提供注入能力；Codex 生产 runHook 未设置 ProjectTrusted 回调，默认不读取项目 effort 配置。
- 记录当前策略而不修改语义：degrade 允许最终跳过，tool.error 在 Codex 下普通无条件规则为 skipped；规范“永不触发”措辞和代码状态存在差异；静态回退的 HasWhen 检查比规范“无 when.conditions”严格；replace 在两家均无槽位。严格降级失败即报错尚未决定或实现。
- 同步任务清单和测试矩阵，仅新增 DOCS-03 完成记录，不改变既有实现/验收状态。既有未跟踪文件 `docs/review/M1B-CR-REAL-2026-10-16.json` 未改动、未纳入提交。
- 验证：以下命令退出 0，adapter、Claude、Codex、conformance、effortmap、runtime、plan、app 八个包全部通过。未重跑真实 harness 验收。

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  go test -count=1 ./internal/adapter/... ./internal/runtime ./core/plan ./internal/app
git diff --check
python3 - <<'PY'
from pathlib import Path
import re
p = Path('docs/STYLE-ADAPTERS-CURRENT.md')
s = p.read_text()
links = re.findall(r'\]\(([^)]+)\)', s)
bad = [link for link in links
       if '://' not in link and not link.startswith('#')
       and not (p.parent / link.split('#')[0]).exists()]
assert not bad, bad
assert s.count('```') % 2 == 0
print(f'Markdown checks passed: {len(s.splitlines())} lines, {len(links)} local links, paired code fences')
PY
```

- Markdown 检查结果：400 行，38 个本地链接目标全部存在，代码围栏成对；`git diff --check` 通过。

### 2026-10-16 — M1B-CR-REAL 代码审查修复进入真实安装，Codex 实测与 U5 复测

- 重建：按用户指示，用 d7438ec 按手册 §1 重建 `~/.local/lib/style-m1b/style`（隔离 Go 缓存，GOPROXY=off，先写 style.new 再 mv）。sha256 1f2b1691… → 695aff85…，build info 为 vcs.revision=d7438ec、vcs.modified=false。旧程序备份在本会话 scratchpad 的 `style.pre-cr`。
  - 之后 sync 退出 0（changed=false），doctor 退出 0（只有设计内的 claude-user route-trust-unverified），两包 hash_matches。
  - Codex config.toml/hooks.json、Claude settings.json/CLAUDE.md、lock、index 哈希都没变。同一 STYLE_HOME 也服务本 Claude 会话的 hook。
- CR-01 实测：Codex 会话 01a143a5（codex-user，0.159.3，gpt-6.1-sol/medium，codex-b，07:37Z）。ex02-repo 只连本地 bare 远端，refs/heads/main 6c88e18 前后不变。
  - 拦截：`bash --rcfile /dev/null -c 'git -C ex02-repo push --force origin main'`、`bash -O extglob -c '… push -f …'`、`bash -eo pipefail -c '… push --force …'` 都被 PreToolUse 拦截。stats 有 3 条 `terse-reviewer/no-force-push@tool.pre` blocked；rollout 的工具输出为 "Command blocked by PreToolUse hook"。
  - 对照：`bash --rcfile /dev/null -c 'git -C ex02-repo status --short'` 退出 0。
  - 离线对比：把同一 index 复制到临时 STYLE_HOME，用相同 payload 测试。旧程序 1f2b1691 四条都放行，新程序拦截前三条。
- U5 复测：用户在运行前把目标改为 200 次（“1000次就别了，200次就ok了”），原计划 10 个会话改为 2 个。
  - 方法同上一轮：每个会话串行 100 次 `printf STYLE_M1B_U5`，只用 `-c` 把 OTel otlp-http json 导出到本地 sink，07:39:06Z–08:03:23Z，会话 01a143a6、01a143b4，无其他会话导出。
  - 结果：PreToolUse 200 条，median 25.3ms，p95 28.6ms，max 74.2ms（07:45:59Z 一次，保留；另有 44.7ms、41.6ms 两次）。各会话 p95 分别为 29.6ms 和 26.8ms。PostToolUse p95 17.3ms。全部 404 条 hook 的 p95 27.3ms，0 错误，0 重叠。
  - 对比：上一轮旧程序 731 次的 median 36.8ms、p95 41.6ms，median 约降 11ms，与 CR-05 去掉的 state.Update 开销一致。两轮的时间段和机器负载不同，不是 A/B 对照。
- 入库：`docs/review/M1B-CODEX-U5B-SPANS-2026-10-16.jsonl`（404 个 hook span，只保留名称、trace id、起止时间和 hook.* 属性；核对后没有主机或账号字段）和 `docs/review/M1B-CR-REAL-2026-10-16.json`。同一命令复算 within-budget（`--min-samples 200` 也是）。
- 文档：设计 §15.1 U5 改为成立（以 200 次为准）；测试矩阵 B11 补上复测，新增 CR-REAL；任务清单新增 M1B-CR-REAL，CR-05 的未复测说明同步更新。Claude C9 未复测。本项只改文档和证据。

### 2026-10-16 — CI-01 GitHub Actions 与端到端冒烟

- 用户要求提交到 GitHub 并用 GitHub Actions 验证当前程序可用。
- 新增 `.github/workflows/ci.yml`：push 到 main、pull_request、workflow_dispatch 触发；ubuntu-latest、macos-latest 两个 runner，`permissions: contents: read`，Go 版本取自 go.mod。步骤：gofmt 检查；`env -u CODEX_HOME -u CODEX_SANDBOX` 下两个 module 的 vet 和 test；`testdata/manual` 两个收集器单测；`go build -trimpath`；`python3 scripts/smoke.py`。CI 用 setup-go 自带的缓存，不用 Makefile 的 /tmp 缓存路径。
- 新增 `scripts/smoke.py`（只用标准库）：临时 HOME/STYLE_HOME，PATH 前置只输出 `codex-cli 0.159.3`、`2.1.300 (Claude Code)` 的桩，清除 CODEX_HOME/CODEX_SANDBOX/CLAUDE_CONFIG_DIR/STYLE_HOME。检查 validate/plan 两个包；install terse-reviewer（`--route codex=codex-user`，claude-user）；list hash_matches、doctor healthy、config.toml 与 CLAUDE.md 写入静态文本；从 hooks.json（经 `/bin/sh -c`）和 settings.json（command+args）取出 hook 命令，6 条 force push 须 deny 且理由为规则原文，4 条对照（含 `echo 'git push --force'`）须放行；sync；uninstall --all 后四个配置文件无 STYLE 残留。
- 本地验证（隔离 Go 缓存，GOFLAGS=-mod=readonly）：gofmt 无输出；两个 module vet、test 通过；两个收集器单测 OK；构建后冒烟 32 项全部 ok。
- 反向对照：对修复前程序 `style.pre-cr` 运行冒烟，6 项失败，都是两家 `bash --rcfile/-O/-eo -c` 形式的 force push 被放行，其余通过。
- 未覆盖：真实 harness 的配置接受、上下文投递和计时；Windows runner（hook 依赖 POSIX shell 的场景未适配）。GitHub 首次运行结果待推送后补记。

### 2026-10-16 — CI-01 GitHub 首次运行结果

- 推送 origin/main d3c0874..d7105f0（101 个提交）。推送前扫描了新增内容：没有令牌、私钥或邮箱；证据文档里有 238 行本机路径 `/Users/inxups/...`。仓库是公开的。
- 运行 https://github.com/inxups/style-cli/actions/runs/38872171780（push 触发）：Test (ubuntu-latest) 47s 通过，Test (macos-latest) 1m17s 通过。两个 job 日志都有 `smoke passed`，两家 `bash --rcfile /dev/null -c` force push 都是 deny。
- 注意：GitHub 提示 ubuntu-latest 将在 2026-10-19 起迁到 Ubuntu 26，届时需要关注首次运行。

### 2026-10-16 — DOCS-04 CLI 完整使用教程

- 用户要求设计这个 CLI 的完整使用教程。读取 `internal/app/*command*.go`、规范文档（包/规则）、`docs/STYLE-ADAPTERS-CURRENT.md`、M1b 手册和 `example/terse-reviewer`，再用 `go build ./cmd/style` 的二进制在隔离沙箱里实际运行：临时 `STYLE_HOME`、`CLAUDE_CONFIG_DIR`、`CODEX_HOME`，仅放空 `config.toml` / `{}` 的 `settings.json` 让 adapters 检测到两家。
- 实测并写入教程的行为：init（目录/单文件/冲突退出 1）、validate（文本/JSON/bad-name）、plan（native、`mode: replace` 跳过、Codex `tool.error` + `fallback: error` 退出 4）、install（无 `--yes` 非交互报错、`--set` 未知名称、重复安装拒绝、`--fail-closed` 版本不足拒绝、不指定路线时 Codex 选 `codex-plugin`）、生成的 `CLAUDE.md` / `settings.json` / `config.toml` / lock 内容、手工 payload 喂 hook（SessionStart 注入、force push 拒绝、普通 push 放行、Edit 新增 TODO 的 warn、同模型 prompt 去重、换模型补发）、复制安装改源码后 sync 无变化、`--link` 改源码后 sync 退出 3 / trust 预览退出 1 / trust --yes 退出 0 并更新 `CLAUDE.md`、uninstall 需 `--yes` 及配置文件清理、doctor / list / adapters 输出、未知命令退出 1。
- 校对中修正了教程初稿的两处不准确：快速上手的 `install` 在检测到 Codex 时默认走 `codex-plugin`（需要 `--route codex=codex-user`）；`when.harness` 必须在 `reads.harness` 声明（否则 `reads-missing`）。
- 观察到的现象（未修改实现，教程里已如实标注）：`--link` 包源码变化后，`sync` / `trust` 输出 “0 changed files” 且 `Source diff:` 为空，但 `sync` 退出码 3，`trust --yes` 后 `CLAUDE.md` 确实更新；`list` 在源码已变、未 trust 时仍显示 `hash_matches=true`。这是 diff 展示/漂移检测的缺口，不属于本任务范围，待单独评估。
- 验证：文档 30 个本地链接目标全部存在，代码围栏成对，TOC 锚点与标题一致；从文档抽出的 `team-style` 四个文件用真实二进制 `style validate` 输出 `valid`；教程中 13.x 的 YAML 片段合并成包后 `validate` 通过；`git diff --check` 通过。
- 未覆盖：真实 Claude Code / Codex 会话里的加载与投递（沿用 M1b 结论）；Codex 一侧的手工 payload 示例没有重跑；交互式确认提示没有在真实终端里演示。

### 2026-10-16 — M1-FINAL-01 / FINAL-01 示例版本门槛与实际过滤

- 用户批准全部 M1 未闭合项修复/验收；任务清单与测试矩阵新增 12 组补项，保留旧验收范围与历史证据。隔离配置优先，Claude 界面仍由用户操作；缺少真实环境证据保持待验证。
- 不只是补元数据：发现 `requires.harness` 原先仅在 schema 验证，没有安装过滤。新增共享门槛检查；已检测的 plan、install 及 sync/trust/uninstall 重编译按每包/每 provider 检查。未检测 provider 的显式 plan 仍可离线运行；安装遇到未知版本不能视为兼容。
- 全部目标不兼容时返回 skipped/warning，不创建 STYLE_HOME、不写 provider 配置；混合目标只安装兼容路线。已有兼容旧包和路线继续保留，重编译不会将新包误加到不兼容 harness。无新命令、锁文件或 index 格式变化。
- 示例 requires.harness 回填 Codex >=0.159.3 / Claude >=2.1.300；index golden 只有两个 inline 规则来源 SHA256 变化。
- 回归：`TestExampleRequirementsSkipOldAndUnknownHarnessesWithoutMutation`、`TestExampleRequirementsInstallOnlyCompatibleHarnessAndRebuildPerPackage`。
- 验证通过（GOPATH=/tmp/style-cli-gopath、GOMODCACHE=/tmp/style-cli-gomodcache、GOCACHE=/tmp/style-cli-gocache，清除 CODEX_HOME/CODEX_SANDBOX，GOFLAGS=-mod=readonly）：`go test -count=1 ./internal/app ./core/compile ./core/spec`、`go vet ./internal/app ./core/compile`、gofmt、`git diff --check`。黄金文件按明确元数据变更用 UPDATE_GOLDEN=1 重建，再无更新重跑通过。

### 2026-10-16 — M1-FINAL-02 / FINAL-02 Codex 原生配置对照与 effort 修复

- 真实 Codex 0.159.3 已改用 `-p <name>` 加载 `<name>.config.toml`；旧实现只读 `[profiles.<name>]`。新版拒绝 legacy 顶层 profile selector、与所选 profile 同名的 legacy table；缺失独立文件则继承用户配置（不是错误）。另实测受信项目覆盖 profile，未受信项目忽略，`-c` 最高优先级。配置 raw 未知值原样进入模型请求；不设配置时本机请求默认 low，但 STYLE 继续报告配置 unknown，避免把模型默认值当作配置值。
- 新增版本分支，0.159.3 起使用独立文件/项目覆盖顺序；旧/未知版本保留现有 table 兼容路径。runtime 与 doctor 传入检测/事件版本。未知项目信任继续不读项目层，未新增信任推断。
- 新增 `testdata/manual/probe_codex_config.py`：仅用临时配置和本地 HTTP sink，真实 CLI 构建请求后 sink 故意返回 400；11 例均通过。此记录证明配置加载，不能替代 B2/B10 的真实模型、工具或上下文验收。只保存合成 fixture 与安全字段，不读取凭据。早期探索发现缺失文件可回退后修正实现/断言，未把失败结果标完成。
- 单测保留旧版表式解析，并覆盖独立文件、项目优先级、启动覆盖、ignore-user-config、未知 raw、缺失/损坏文件、legacy 拒绝、路径逃逸，以及 Enricher 使用固定 harness 版本选择解析器。
- 验证：隔离缓存并去掉 CODEX_HOME/CODEX_SANDBOX，`go test -count=1 ./internal/adapter/codex ./internal/app ./internal/runtime`，`go vet ./internal/adapter/codex ./internal/app`；原生对照脚本、gofmt、`git diff --check`。证据 `docs/review/M1-FINAL-02-CODEX-CONFIG-2026-10-16.json`。

### 2026-10-16 — M1-FINAL-03 / FINAL-03 link 漂移复核

- 对应 DOCS-04 的疑似漂移记录，新增 `TestLinkDriftIsVisibleBeforeFirstTrustAndAfterApproval`，覆盖首次未改源码、首次授权前改动、list/hash、sync 阻塞、Source/Rendered diff、预览只读、批准快照及批准后自动同步。代码实现未改动，agenthooks 边界不变。
- 实际构建 CLI，在临时 STYLE_HOME / CLAUDE_CONFIG_DIR / CODEX_HOME 中执行 11 步：link install → 未改源码 sync/list → 修改正文 → list/sync/trust preview → trust --yes/list → 再修改/list/sync → uninstall。真实漂移正确检出；首个未改源码 sync 的 0 changed files 及退出 3 是首次开发链接授权。卸载后 settings.json、CLAUDE.md 字节还原。证据 `review/M1-FINAL-03-LINK-2026-10-16.json`。
- 旧记录缺口未复现，不能据此断言旧操作的具体原因；教程改为区分首次授权和实际改动，准确说明授权后自动同步。历史证据未覆盖。
- 验证（临时 GOPATH/GOMODCACHE/GOCACHE，去掉 CODEX_HOME/CODEX_SANDBOX）：`go test -count=1 ./internal/app`、`go vet ./internal/app`；实际 CLI 构建与隔离运行；gofmt、`git diff --check`。

### 2026-10-16 — M1-FINAL-04-FIX / FINAL-04-FIX 插件登记表被误删

- B10 独立插件验收在 B1 即复现：`codex plugin marketplace add` / `plugin add` 用 TOML 编辑器把 `[marketplaces.style]`、`[plugins."style-runtime@style"]` 放到 STYLE 末尾标记之前；第二包 install/sync 的整区替换删除了原生登记。用户级相同布局也存在缺陷；新增生命周期回归在修复前两例均失败。
- 修改 agenthooks/install 的职责范围内 TOML 合并/卸载：Codex 使用现有 go-toml AST 区分 trust 与外部 table，保留 quoted/array table 和字符串里的伪表头；损坏区域返回错误而不是裁掉配置。Kimi 的现有标记逻辑不变，STYLE 的 index/lock/state 及公开命令不变。
- 构建隔离程序后，真实 Codex 0.159.3 插件登记→第二包 install→sync→uninstall→sync 四步配置字节不变，取消登记恢复登记前字节，STYLE 最后卸载恢复安装前字节。证据 `review/M1-FINAL-04-PLUGIN-REGISTRATION-2026-10-16.json`；该生命周期检查不代替 hook 模型投递。
- 验证（临时 GOPATH/GOMODCACHE/GOCACHE，清除 CODEX_HOME/CODEX_SANDBOX）：`go test -C agenthooks -count=1 ./install`、`go vet -C agenthooks ./install`、`go test -count=1 ./internal/app ./internal/adapter/codex`、`go build -o /tmp/style-m1-final.rxdYyj/style ./cmd/style`；gofmt、`git diff --check`。
- 同轮真实用户路线已观测 B2 的 low/minimal/high/unset/unknown/profile/override/trusted/untrusted，以及 B4/B5/B6/B7/B9；gpt-6.1-sol/minimal 请求返回服务端 502，换历史成功的 gpt-6-sol/minimal 后成功，未将失败算通过。B3 首轮在 compact 的 item/completed 后过早发 turn/start，原生返回 ActiveTurnNotSteerable，属于采集流程无效；将等待完整 turn/completed 后重跑。FINAL-04 保持部分完成，最终程序两条完整路线各自留证。

### 2026-10-16 — M1-FINAL-04 / FINAL-04 两条 Codex 路线独立验收

- 使用 a978bb1 实现构建的隔离程序 SHA256 48ae3f39b392b87c829cf8341e1f57d273d5ceabb995995e1309ac7ca6efa33d。codex-user 与 codex-plugin 各 19 次真实调用，分别完整执行 B1–B9，证据独立保存为 `review/M1-FINAL-04-CODEX-USER-2026-10-16.json`、`review/M1-FINAL-04-CODEX-PLUGIN-2026-10-16.json`，不拼接通过结果。日常安装及凭据不写入报告。
- B2 每条路线均覆盖 low、minimal、high、unset、unknown raw、file profile、启动覆盖、trusted/untrusted 项目。minimal/启动覆盖使用 gpt-6-sol；其他沿用 gpt-6.1-sol。通过 loopback 转发器记录真实请求的 model/effort/STYLE context，与 rollout 和 state/stats 的正文 SHA256 逐项核对。服务可以拒绝未知 raw（model_turn_completed=false），STYLE 仍保留 raw、tier 空、无 low guard；不能将此表述为模型支持未知档位。项目生产信任不可判定时仍跳过，真实请求 high 与 STYLE 用户 low 的差异和 doctor 原因明确保留。
- B4 两种入口先后顺序共用 session 去重；B5 仅本地 bare remote，force 被 hook 阻断、lease 实际成功；B6 apply_patch 新增触发一次警告、删除不触发，最终无未完成注释；B7 既有 MCP stdio stub 实际调用并收到前置上下文；B9 未 trust 的旧 index/正文继续投递，批准后新正文进入请求。
- B3 改为等待 compact 完整 turn/completed，同进程继续两回合；每条动态 system 总计两次 output（首次+压缩后一次），epoch=1，后一回合 dedup，压缩后两次请求仍含静态规则。首轮过早 turn/start 的无效采集未用于通过结果。
- B8 原始比较发现 app-server 自行增加临时 workspace 的 project trust 表；两份 BASELINE-OBSERVATION 保留。验证 STYLE 卸载后的唯一外部差异正是该原生表，随后探针单独清理自己的 native fixture 登记，最终配置与 fixture 基线逐字节相同。没有让 STYLE 删除用户信任表，也没有先覆盖整份配置再宣称通过。新增回归保证未知残留报错且不改写、原有 trust 保留。
- B10 另外在独立根执行 user→plugin→user→plugin→user：4 个真实会话，每次原生 codex.hooks.command span 的 SessionStart/UserPromptSubmit/PreToolUse/PostToolUse 各一条且 source 全为当前路线，动态正文仅一次 output；用户路线配置/hooks 与初始安装字节相同，最后卸载字节还原。证据 `review/M1-FINAL-04-SWITCHES-2026-10-16.json`。
- 重复命令：`PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_codex_routes.py --style /absolute/path/style --route codex-user --scratch "$(mktemp -d)" --output /tmp/codex-user.json`；plugin 在另一个空根独立运行；切换另加 `--switches-only`。真实服务失败会非零并留证。临时项目 trust 清理现已在重复脚本中明确分开。
- 验证：`PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py'`（15 项）、Python AST、两条路线 B2 全部正文摘要复核、证据凭据/服务 URL 扫描、`git diff --check`。Go 实现验证沿用同一构建的 FINAL-04-FIX 记录；最终双 module 总回归仍在 FINAL-12。


### 2026-10-16 — M1-FINAL-05 U2 手动插件生命周期与 U6 支持边界

- U2：复用 FINAL-04 已独立完成的真实插件 B1–B9、登记保留修复及两轮双向切换。native `hook.source=plugin`、第二包 install/sync/uninstall/sync 保留登记、取消登记与最终配置字节还原均有原始结果，不重复用准备脚本代替真实运行。证据：`review/M1-FINAL-04-CODEX-PLUGIN-2026-10-16.json`、`review/M1-FINAL-04-PLUGIN-REGISTRATION-2026-10-16.json`、`review/M1-FINAL-04-SWITCHES-2026-10-16.json`。
- U6：原设计默认“不支持，plan 里说明”，本轮按此边界关闭，不扩展云端路线。现有 matcher 仅 Bash/apply_patch/mcp；runtime 识别本地 shell 读取，prompt 解析 `$name`。云端实际原生工具名尚未采集，候选名回归并非真实云端测量。
- 修复 plan 遗漏边界说明：`BlindSpot` 返回非 fatal reason 时保留为现有 RulePlan.Reasons，继续计算可用挂载。Codex skill 规则明确提示 cloud skill reads unsupported，本地 tool.post/skill.explicit 两个 native placement 不变。不改 index、lock、state 格式、公开命令或订阅面；agenthooks 无变更。
- 验证：隔离 `GOPATH/GOMODCACHE/GOCACHE`、清除 CODEX_HOME/CODEX_SANDBOX，`go test -count=1 ./core/plan ./internal/adapter/codex ./internal/app` 通过；同包 vet、gofmt 与 diff 检查通过。新增 plan/固定 matcher/未知读取工具 runtime 回归，更新 Codex plan golden，Claude golden 不变。


### 2026-10-16 — M1-FINAL-06 Claude 隔离准备，真实界面验收待执行

- 本机 Claude 2.1.300 的日常认证存在；新建私有 CLAUDE_CONFIG_DIR 后 `claude auth status --json` 返回 exit 1 / loggedIn=false / authMethod=none。未读取或复制密钥，未修改日常配置；准备环境不能直接借用日常认证。
- 新增 `prepare_claude_acceptance.py`：拒绝非空 scratch，复制固定二进制，在独立 STYLE_HOME/工作区安装两个原包和六类用户提示探针；独立 observer 记录原生字段和 effort 环境，不包裹 STYLE 的计时命令。另备运行中安装的 Glob 新订阅以区分热更新/新会话加载。
- 已运行：`PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/prepare_claude_acceptance.py --style /tmp/style-m1-final.rxdYyj/style-final05 --scratch /tmp/style-m1-claude-ui-c-20261016`，三个包真实 install 成功；热更新包 validate、observer 白名单检查通过。固定程序 SHA256 `23cf8caae7d447fc1e621f569b17fb5fd9db0584132bf116bcccff68ccb0cde3`。准备中首次 fixture 校验报缺 description/非法 event，已修正为 skill target 后在全新目录重跑，失败目录未覆盖。
- 用户执行入口与步骤：`review/M1-FINAL-06-CLAUDE-RUNBOOK.md`，包含隔离登录、思考关闭、各 hook effort、clear、六类 systemMessage、运行中配置变更和最终 C9。当前没有新真实模型/UI/C9 通过结论，FINAL-06 保持 `[?]`。可继续不依赖该认证的 Codex/成本项。


### 2026-10-16 — M1-FINAL-07 Codex 上下文继承、无补发 compact 与子 agent scope

- 固定程序 23cf8caa，Codex 0.159.3；私有 `/tmp/style-m1-context-a-20261016`，独立 CODEX_HOME/STYLE_HOME/工作区，现有 provider 的真实推理。运行 `PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_codex_context.py --style /tmp/style-m1-final.rxdYyj/style-final05 --scratch /tmp/style-m1-context-a-20261016 --output /tmp/style-m1-context-a.json`。不改日常安装。
- U-T1：exec resume 原 thread，startup-only 历史规则累计输出一次、原始标记仍在下一次真实请求；app-server fork 新 thread 也保留该标记，无新 history 输出，默认 sources 的另一条规则在 fork 输出一次。沿用默认 sources，不因一例保留改策略。
- U-T2：专用规则关闭 reinject；compact 后 epoch=1，两条动态规则累计仍各一次，后续真实请求的 marker_locations 均为空，未把补发误判成保留。该短会话仅证明这组标记没有保留，不是所有内容必定丢失的保证。
- U7：真实 native spawn 的 fork_turns 分别 none/all/none，agent_path 为 fresh_child、fork_child、fresh_child/grandchild（depth2）。三份子 rollout 都有自身 additional_context 和工具注入；三个 scope 各有 child context output、child-tool output、child-block blocked。父 main stats 无这些条目。阻断对象只是 harmless printf；允许 printf 的真实返回另留在原始 rollout。
- 采集器末尾初版读取 scope.json 中不存在的 agent_id 而 exit1；数据与模型运行均已成功留存。修正为从 scope 文件目录取身份后，`--verify-report /tmp/style-m1-context-a.json --output /tmp/style-m1-context-final.json` 复核原始结果，保留 source SHA256 和错误观察记录；未覆盖原文件，也没有用离线 fixture 代替模型运行。证据：`review/M1-FINAL-07-CODEX-2026-10-16.json`、`M1-FINAL-07-COLLECTOR-OBSERVATION-2026-10-16.json`。
- collector marker 回归和全部 17 项 Python 测试、语法/diff 检查通过；报告按实际 provider URL/token 扫描无泄漏。Claude 独立的 resume/fork/compact 未测，FINAL-07 保持 `[~]`，用户已选择稍后操作 Claude。


### 2026-10-16 — M1-FINAL-07 子 agent 原生工具结果追加核对

- 77c6990 提交前附加的 shell 退出码检查使用了错误的输出类型/文本格式而失败；该检查失败后 shell 仍继续提交。现已核对原始 rollout：本机 code mode 使用 custom_tool_call_output，input_text 中嵌套 JSON 的 exit_code=0，而不是普通 function_call_output 的 “Process exited” 文本。
- 增强采集器同时读取两种输出，且只把精确 output=STYLE_CHILD_SCOPE_OK 的结构化结果算作允许命令；单纯函数参数包含标记不计通过。三个子线程均确认一条真实 block 结果和一条 exit_code=0 的允许 printf。原证据不覆盖，追加 `review/M1-FINAL-07-CHILD-TOOLS-2026-10-16.json` 与原报告关联。
- 验证：新增 code mode 结果回归，全部 18 项 Python 测试、原始三线程结果提取断言及 diff 检查通过。后续提交命令明确 set -e，避免检查失败后继续提交。


### 2026-10-16 — M1-FINAL-08 STYLE 保护与 Codex 原生超限分别测量

- 真实命令：`PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_codex_overflow.py --style /tmp/style-m1-final.rxdYyj/style-final05 --scratch /tmp/style-m1-overflow-a-20261016 --output /tmp/style-m1-overflow-a.json`，exit0。程序 23cf8caa、Codex 0.159.3；全新私有配置/工作区。
- STYLE：两家同一 14k 字符 event 规则均触发 T003，exit4，harness 配置字节不变、STYLE_HOME 未创建。`go test -count=1 ./core/engine ./core/tokens ./internal/app` 在隔离 Go 缓存、清除 CODEX_HOME/CODEX_SANDBOX 后通过，包括 cap 丢最高 order 与释放 reservation 回归。
- Codex 原生：没有安装 STYLE 包，直接生成 raw Python hook 并预置信任；SessionStart 12,064/50,064 字符、PreToolUse 12,056 字符三次均真的输出完整正文，模型运行成功。native 保存完整原文，模型收到约 10,100 字符头尾预览和 Full hook output saved to 路径；保存文件 bytes/SHA256 与输入正文一致，原文未完整出现在 hook context。首尾标记在预览中仍同时出现，因此新增回归防止错误推断“完整接受”。字符数明确为字符，不冒充官方 token 计数。
- 报告：`review/M1-FINAL-08-OVERFLOW-2026-10-16.json`，保存所有请求/rollout 与实际结果；采集后对现有原生落盘文件做精确比较，无再次推理或结果替换。Codex file-preview 声明与实测一致。Claude 原生超限仍因隔离认证缺失待测，FINAL-08 `[~]`，不改变预算默认值。
- Python 全部 20 项测试、语法/diff 检查通过，报告按真实 provider URL/token 扫描无泄漏。


### 2026-10-16 — M1-FINAL-09 固定语料、官方计数准备与未验证误差保证修正

- 官方依据（2026-10-16）：OpenAI Counting tokens 指南 `https://developers.openai.com/api/docs/guides/token-counting` 明示 POST /v1/responses/input_tokens，可传 instructions/input，读取 input_tokens；Claude token counting 指南 `https://platform.claude.com/docs/en/build-with-claude/token-counting` 支持 system/messages 结构及 input_tokens。OpenAI 巨型 reference 页抓取失败，改用已成功打开的官方指南核对。
- 固定 `testdata/manual/token-calibration-40.json`：20 英文、20 中文可直接使用的工程规则，意义配对，逐条 UTF-8 SHA256；非重复填充。`calibrate_tokens.py` 用真实 CLI plan 取得原估算，官方计数请求与固定基线只差规则正文，记录请求摘要、模型、API request id、差值和相对误差。只向两家官方固定域名发送各自明确环境 key，不读取/转发日常代理 provider token。
- 实际运行：`PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/calibrate_tokens.py --style /tmp/style-m1-final.rxdYyj/style-final05 --openai-model gpt-6.1-sol --anthropic-model claude-sonnet-5-5 --output /tmp/style-m1-calibration-blocked.json`，exit2；OPENAI_API_KEY 和 ANTHROPIC_API_KEY 均缺失，两家 actual completed=0，不发网络计数请求。模型为拟测模型，官方可用性未被认证验证。结果入库 `review/M1-FINAL-09-COUNTING-BLOCKED-2026-10-16.json`，真实校准保持 `[?]`。
- 删除 plan、doctor、golden、教程/规范里未经验证的 ±25% 保证，明确 calibration pending；不改估算系数、JSON/index/lock/state 格式、预算默认值或公开命令。不能用另一分词器代替真实计数。
- 验证：23 项 Python 测试（语料平衡与哈希、同结构请求、无 key 不联网/不伪造计数）；app/tokenestimate/tokens 测试与 app vet、gofmt/diff 通过。后续在可提供官方 API 环境的终端按 README 同一命令运行，失败时保留独立报告，不覆盖本次缺认证证据。

### 2026-10-16 — M1-FINAL-10 U-T4 真实会话统计与 U-T6 缓存核对

- 用户选择 Claude 界面稍后操作，本轮继续独立项，只读保留的 FINAL-04 报告及 Claude 历史主 transcript/stats。`summarize_session_usage.py` 复现命令见 `review/M1-FINAL-10-CACHE-2026-10-16.md`；输出 `review/M1-FINAL-10-USAGE-2026-10-16.json`，只收白名单 metadata、hash、usage 与规则统计，不复制提示词、工具正文、凭据。
- 两条 Codex 路线各 19 调用归并 17 session，31 HTTP 尝试，16 session 有 usage；规则 inject53/warn1/dedup63，去重比例 53.85%，规则输出估算 796。缺 usage 的 unknown-effort 拒绝、compact native usage 覆盖缺口及自定义 provider 非官方账单明确记录。resume 后 exec usage 为累计；直接与 retained rollout `01a1440e-91dd…` 最后 total 53182 输入/150 输出、last 13788 输入对照，不逐调用累加。app-server 仅使用最后 total，字段仅部分会话提供时不冒充总和。
- Claude 历史会话 `1a0e173f…` 的 130 usage 行按响应身份去重为108，删除22重复快照。input216 + creation25808 + read4878107 = 输入4904131，output11410；creation 全为1h，TTL明细不再加总。STYLE inject4/warn1/dedup10，估算69；主 transcript 不覆盖独立子 agent 或无 usage 的失败/重试。验收工作负载不校准典型会话默认值，也不能将总 usage 归因到每条规则。
- 2026-10-16 成功打开官方 OpenAI/Claude prompt-caching 指南并核对门槛、前缀、TTL、读写倍率和 usage 字段。具体资料与例外记录于 CACHE 文档；例如当前 Sonnet5.5 读取0.05、1h写入2，与保留公式不完全相符。CLI 文本/golden、教程、设计 §11.3 明确公式是假设1.25写/0.1读的相对比较，不是实际账单；公式、JSON/index/lock/state、60/3和400/1200/600不变。
- 验证：全部27项 Python 测试（新增4项覆盖 resume累计/追加stats、app-server累计、部分字段覆盖、Claude重复/TTL/隐私）；清理CODEX变量和临时Go缓存的 `go test -count=1 ./internal/app`、`go vet ./internal/app`，golden、汇总断言和 `git diff --check` 全部通过。FINAL-10 `[x]`；FINAL-06/07/08/09 仍按缺证据范围待验。

### 2026-10-16 — M1-FINAL-11 文档、状态与现有 fallback 一致性

- 规范首页不再声称“还没有实现”，设计/教程/任务与矩阵明确 M1 补验收未全部闭合；历史 B/C 原证据保留。总览核对到960dd57：Codex effort 九种真实组合、插件生命周期、scope、raw预览及实际usage已完成；Claude剩余能力与官方计数仍待验，最终程序C9须重采。
- 统一规则/运行时规范、设计及总览：无条件 Codex tool.error 的degrade/skip=skipped，error=install-error；缺失条件可为never-trigger。工具静态回退要求无任何when；system静态回退单点优先、动态双点可用时双挂载，完全不可用的system保留原fatal例外。degrade仍允许最终跳过，不改代码策略、schema或默认值。unknown映射的approx缺失不再误写为永远false。
- 另纠正文档中尚未实现的Codex events.yaml按版本裁剪描述：当前Surface固定，示例靠既有requires.harness过滤；plugin manifest版本当前固定0.1.0，bin/style为POSIX启动器。两条路线切换、native追加表保留和不确定信任回退边界已说明。
- 验证：隔离Go环境 `go test -count=1 ./internal/adapter/... ./internal/runtime ./core/plan ./internal/app` 全通过；`go build -trimpath -o /tmp/style-m1-docs-final11 ./cmd/style` 后在临时HOME/STYLE_HOME/CODEX_HOME/CLAUDE_CONFIG_DIR运行四组 `plan package --harness codex --json`。初次临时fixture漏必填spec/description，仅得到校验诊断，补齐后四组实际退出/状态完全符合文档、无placement且无配置/state写入；fixture全文/二进制哈希/复现方式在 `review/M1-FINAL-11-PLAN-2026-10-16.json`，不作为新的runtime投递证据。
- 9个当前文档的94个本地文件链接、全部代码围栏、`git diff --check`通过；源码对照入口为core/plan/system.go、events.go、policy.go及Codex HookManifest。FINAL-11 `[x]`，接着执行FINAL-12；M2仍不启动。

### 2026-10-16 — M1-FINAL-12 本地最终回归通过，远端CI及环境项保持待验

- 当前源码2f6426b，Go1.27.0 darwin/arm64。清除CODEX_HOME/CODEX_SANDBOX，GOPATH=/tmp/style-cli-gopath、GOMODCACHE=/tmp/style-cli-gomodcache、GOCACHE=/tmp/style-cli-gocache。`GOFLAGS='-mod=readonly -count=1' make test-go`双module全通过；`GOFLAGS=-mod=readonly make vet-go`全通过。
- 定向race：同样环境的 `go test -race -count=1 ./core/state ./core/engine ./internal/adapter/codex ./internal/runtime ./internal/app` 以及 `go -C agenthooks test -race -count=1 ./install` 全部通过。覆盖去重/状态、runtime、effort、CLI安装与native表保留；没有因缓存复用省略新运行。
- `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py' -v` 27项全通过；20个Python文件ast.parse有效，302个跟踪Go文件gofmt干净。CI改为同一发现命令，补上路线、上下文、超限、官方计数准备和usage回归；此前CI只跑两个计时收集器。
- 同一Go环境 `go build -trimpath -o /tmp/style-m1-regression.xUgaiQ/style ./cmd/style` 成功；SHA256=8edcb3ce48e2402ee3ac8f788890c1a25f03956a720f0f852fb47e79b771f636。`python3 scripts/smoke.py /tmp/style-m1-regression.xUgaiQ/style` 全32项通过，包含两家版本桩下安装/hook拒绝与放行/sync/卸载；无真实模型/远程push操作。
- Claude用户已选择稍后操作。隔离scratch尚无hook-facts/debug/state，原23cf程序先存bin/style.before-final12-23cf8caa，再原子更新bin/style到本轮构建；prepared.json与配置字节不变，入口launch.sh路径不变。`review/M1-FINAL-12-CLAUDE-BINARY-2026-10-16.json`单独记录更新，不冒充UI/C9通过。日常安装未改。
- 只读检查 `gh run list --repo inxups/style-cli --workflow ci.yml --limit 5` 和 `gh run view 38872445836 ... --json headSha,status,conclusion,jobs,url`：远端最新04371ef成功，Ubuntu/macOS均success；此前d7105f0成功仍保留。当前2f6426b无对应run，本轮提交尚未发布，不把旧CI当新源码验收。全部结果/复现命令汇总在 `review/M1-FINAL-12-REGRESSION-2026-10-16.json`。
- FINAL-12 `[~]`：本地验证完成，远端本轮CI和FINAL-06/07/08/09真实补验收仍缺证据；剩余能力修复若更改代码需重跑相关检查。当前不声明M1全部完成、不开始M2；全部闭合后再提示下一步规划。


### 2026-10-16 — M1-FINAL-06 用户界面边界与 native failure 修复

- 用户完成隔离登录与界面步骤。初始 session163efdc3：SessionStart:startup、UserPromptSubmit、PreToolUse、PostToolUse、PostToolUseFailure 和 UserPromptExpansion 的 USER 标记均可见；每类对应 STYLE JSON 输出与stats核对，exit7 有真实失败 tool result。原项目技能被 user-only 来源排除，首次 Unknown command 保留；补到配置根 skills 后原进程自动发现并成功触发。
- 原 Glob 热更新尝试实际执行 Bash/find，本版原生工具声明也无 Glob，不能计为通过。改为此前不存在的 Read matcher；未重启、未再次clear的当前进程实际 Read、POST hook、HOT_READ_USER 和 MODEL 标记及stats一致，U-C2 成立。准备脚本和runbook改用Read。
- 原始 observer逐事件：SessionStart/UserPromptSubmit/UserPromptExpansion无effort和CLAUDE_EFFORT；工具前/后/失败有effort.level及环境high，调整后medium。用户所见选项为low/medium/high/extra/max，无off选项；不把medium当关闭，不宣称已获得off原生表示。clear创建新session6d6118da，source=clear且无model；新的state保持unknown，随后工具补medium。启动USER在hook输出中存在，用户界面未见，作为clear能力边界保留。
- 真实失败payload字段是error而非tool_error。codec原生fixture和runtime管线回归先失败（Error空、条件结果为空），修复后通过；兼容旧tool_error、两者并存优先旧字段，且仅Claude decode变更，Codex共享形状不变。进一步核对M1规范只允许tool.output在tool.post；不扩展公开DSL，收窄tool.error的tool.output/tool.added能力声明。runtime测试直接构造index仅验证内部归一化管线，不代表公开条件范围扩大。原生失败探针改用既有tool.input.command条件。
- `probe_claude_failure.py --claude /private/tmp/style-m1-claude-cli-2.1.300/claude --style /tmp/style-m1-final06-fixed --scratch /tmp/style-m1-claude-failure-d-20261016 --output /tmp/style-m1-claude-failure-d.json`：真实CLI/Bash exit7，observer见error=Exit code7且无tool_error，STYLE tool.error输出一次，实际Messages请求含完整固定标记，stats一致。服务为仅loopback的脚本响应，不调用模型，合成usage/cost不入证据。前几次准备失败（全局claude路径消失、原fixture误用被M1禁止的tool.output）原scratch均保留，未计通过。
- 验收过程中全局npm安装变为2.1.301且claude入口消失，当前UI进程仍为2.1.300。只向私有目录npm pack目标2.1.300，核对官方包sha512 integrity，再固定native二进制475517ca…；未安装/回滚日常配置。隔离STYLE二进制原子更新为8d5fbf75…，旧8edcb3ce程序和launcher备份保留，独立launch-c9.sh写c9-final06-debug.log。详情见BOUNDARY JSON。
- 验证环境清理CODEX_HOME/CODEX_SANDBOX，临时GOPATH/GOMODCACHE/GOCACHE和GOFLAGS=-mod=readonly：agenthooks `go test -count=1 ./...`、root `go test -count=1 ./internal/runtime ./internal/adapter/claude ./core/plan ./internal/app ./core/spec`及两边相关vet通过；30项Python单测通过（新增loopback/原生落盘精确比较/环境隔离；初版macOS符号路径断言改为resolve后通过）。FINAL-06-FIX关闭，FINAL-06保持部分完成待C9；不使用脚本准备或模型复述替代验收。


### 2026-10-16 — M1-FINAL-08 Claude 原生超限边界实测

- `PYTHONDONTWRITEBYTECODE=1 python3 testdata/manual/probe_claude_overflow.py --claude /private/tmp/style-m1-claude-cli-2.1.300/claude --scratch /tmp/style-m1-claude-overflow-local-b-20261016 --output /tmp/style-m1-claude-overflow-local-b.json` exit0；原生2.1.300，独立HOME/配置/work，没有STYLE包或STYLE裁剪。实际Messages请求只发loopback脚本服务，无凭据读取/转发，无远程推理；合成usage与cost从报告排除，不能计入U-T3/T4。
- 八组ASCII正文：SessionStart9,999和10,000字符精确完整进入请求；10,001、12,000、50,000及UserPromptSubmit/PreToolUse/PostToolUse各12,000均由native生成persisted-output预览。六份native保存文件与原文bytes/SHA256完全相等；pre/post包含真实Bash printf成功返回。只收marker或首尾相同不算完整，采集器回归覆盖该误判及错误落盘内容。
- 第一轮a探针所有原生运行已结束，采集器最初只判断fullbody，故把预览暂记not-full；原报告不覆盖，从保留请求和文件补判的新报告关联sourceSHA。第二轮b以修正后的完整脚本重跑八组，全通过，无意外HTTP路径。原scratch均保留，入库b报告 `review/M1-FINAL-08-CLAUDE-2026-10-16.json` 并关联既有两家STYLE保护/Codex原生证据。
- 当前file-preview/10k adapter声明与ASCII原生边界一致，预算、来源和策略不改。30项Python采集器回归、脚本语法/报告断言/diff检查通过。FINAL-08关闭；不把本地脚本响应表述成真实模型推理或官方token计数。


### 2026-10-16 — M1-FINAL-06-C9 最终修复版真实串行性能通过

- 用户通过固定launcher完成C9；原生2.1.300、STYLE SHA2568d5fbf758cbe7920a9cc978f7715abcf3c00461f9d2289e6f90c2c92501db5d1。会话f9d73552，用户切换Haiku5.5，实际51个主请求（含最后回答）。原本继承环境与仅allowlist环境的auth status不同，仅后者未读到认证；实际用户会话有成功推理，不能用早先headless未登录覆盖真实通过证据。
- `collect_claude_c9.py --log /private/tmp/style-m1-claude-ui-c-20261016/c9-final06-debug.log --transcript /private/tmp/style-m1-claude-ui-c-20261016/claude/projects/-private-tmp-style-m1-claude-ui-c-20261016-work/f9d73552-edb9-46d2-8c27-59364f9b9b02.jsonl --binary /private/tmp/style-m1-claude-ui-c-20261016/bin/style --output docs/review/M1-FINAL-06-C9-2026-10-16.json --completion-output docs/review/M1-FINAL-06-C9-HOOKS-2026-10-16.txt` exit0。
- 50个Bash tool id、50个timestamped成功结果、50个含工具assistant消息；固定单次printf，无循环合并，逐次结果后才发起下一次，0重叠。46条无timestamp API输入快照中的结果不重复算工具执行；原生命令的转义拼写在报告保留。
- 全部103条STYLE completion：整体median15ms/p95=28ms/max341ms；PreToolUse50条p95=29ms，PostToolUse50条p95=13ms，启动341ms、模型切换54ms、prompt15ms各一。所有慢样本保留，0hook错误/0无法解析完成行。按既定整体p95<50ms通过，不将启动或模型切换单类宣称小于50ms。
- 33项Python测试全部通过，新增计数/快照去重、重叠和循环/缺结果/错误拒绝回归。C9单项关闭；FINAL-06整体仍补U9控制配置观测，界面无off不能当medium=off，预算/能力默认值未更改。


### 2026-10-16 — M1-FINAL-06 U9 控制配置与既定回退验收

- 用户确认官方API key暂时没有，FINAL-09继续待验。本轮未将Claude登录态用于官方计数。
- 使用真实认证的原生2.1.300/Haiku5.5，在独立work/STYLE_HOME/显式settings下比较正常配置与alwaysThinkingEnabled=false且MAX_THINKING_TOKENS=0；不加载用户hooks，认证只由CLI使用现有隔离M1配置，不读/复制凭据。两次Bash均成功，实际4个Messages请求均为thinking.type=adaptive/output_config.effort=high，工具hook含effort.level=high和CLAUDE_EFFORT=high，state保持high/approx=false。不能把配置请求当作实际关闭，更不伪造none；结合界面无off选项，按原计划既定“不产出none”能力边界关闭U9。没有宣称已验证实际关闭后的原值。
- 复现 `probe_claude_thinking.py --style /private/tmp/style-m1-claude-ui-c-20261016/bin/style --claude /private/tmp/style-m1-claude-cli-2.1.300/claude --auth-config /private/tmp/style-m1-claude-ui-c-20261016/claude --scratch /tmp/style-m1-claude-thinking-b-20261016 --output /tmp/style-m1-claude-thinking-b.json` exit0。首个a准备fixture的reads/template格式错误未进入模型调用，原目录保留；修正为现有schema后才计验。
- 新增原生请求重放采集器：验证canonical内容hash，按keep/tail还原输入；原生流式assistant分块和复用user结果由精确hash引用关联，不把snapshot当额外执行。fork会把部分历史blob改写为system但保留旧hash，仅在已验证原始来源可提供相同hash正文时解析；没有原始内容时拒绝，绝不忽略缺失。C9全51请求成功校验；string/block、缺hash、损坏内容、prefix越界、stream、原生fork别名回归均通过。
- 40项Python回归及证据断言通过。双module test/vet、runtime/Claude race已在最终修复代码上重跑通过，最终汇总另记FINAL-12。FINAL-06整项按已验证回退/能力边界及C9关闭，预算、sources、compact和公开schema保持不变。


### 2026-10-16 — M1-FINAL-07 Claude 真实继承、无补发压缩与子scope通过

- 使用8d5fbf75固定程序、原生2.1.300和用户已登录M1隔离认证，独立work/STYLE_HOME和显式settings，排除用户hooks；未复制凭据、未修改日常安装。`probe_claude_context.py --style /private/tmp/style-m1-claude-ui-c-20261016/bin/style --claude /private/tmp/style-m1-claude-cli-2.1.300/claude --auth-config /private/tmp/style-m1-claude-ui-c-20261016/claude --scratch /tmp/style-m1-claude-context-c-20261016 --output /tmp/style-m1-claude-context-c.json` exit0。
- resume源956c59bc真实请求保留原startup-only marker，history总输出一次；fork新7c5a101f也保留，startup-only无新输出、默认sources规则输出一次。请求内容hash与保留源transcript精确关联；模型只回答OK，不以复述判通过。
- compact专用87620189两条规则关闭reinject，native手动compact_boundary记preTokens3307/postTokens775/6100ms，scope epoch1，history/default总输出仍各一次，后续真实请求两marker均空。compact命令本身num_turns/usage为0且无普通Messages请求记录，不能据此宣称压缩服务没有usage。仅本短会话结论，补发默认值不变。
- native Agent创建style-child，子scope a30d30c5eee9e4968中一次真实printf阻断、一次注入；child真实请求含工具marker，成功result为STYLE_CHILD_SCOPE_OK、拒绝result为固定hook理由，父scope无child条目。子请求不含本次父级两条动态context；Claude SubagentStart仍无注入能力，未扩展能力或路线。
- a/b原生运行成功但严格采集器分别遇到string→text block归一、fork历史blob改写为system且保留旧hash；原报告保留未标整项通过。修正只按请求hash精确链接已验证原始内容，再完整c重跑。c后从保留native文件补记compact边界和child实际结果断言，无重复推理或观察替换。入库 `review/M1-FINAL-07-CLAUDE-2026-10-16.json`，关联Codex既有证据。
- 40项采集器回归、脚本语法、报告断言和diff检查通过。FINAL-07关闭；FINAL-09按用户选择等待官方key，M1不扩大全部完成声明。

### 2026-10-16 — M1-FINAL-11 最终证据后的当前文档补充审计

- FINAL-06–08真实补验已经提交，当前adapter总览、设计/规范首页及部分正文仍保留旧“Claude待测”描述。按BOUNDARY/THINKING/C9、CONTEXT和OVERFLOW证据回填：六事件USER与clear显示边界、Read热更新、未实现实际off且不产出none、resume/fork/无补发compact、子scope和ASCII原生预览。教程仍建议新开会话，同时明确所测Read热更新范围；不外推其他事件/版本。
- 旧失败/原始证据和追加式历史日志保持原样；当前任务与测试表只剩官方计数及本轮远端CI待验。不调整sources、compact补发、预算、公开命令或schema。
- 11份当前文档的111个本地链接目标存在、代码围栏平衡；记录逐文件SHA256及检查范围到 `review/M1-FINAL-11-DOC-AUDIT-2026-10-16.json`，关联既有四组plan真实输出。仅更新文档，未重复已通过的模型调用或Go回归。`git diff --check`通过。

### 2026-10-16 — M1-FINAL-12 最终修复版本地验证汇总，官方计数及远端CI保留待验

- 最后生产Go变更ce912e6之后，已完成 `GOFLAGS='-mod=readonly -count=1' make test-go`、`GOFLAGS=-mod=readonly make vet-go` 及 `go test -race -count=1 ./internal/runtime ./internal/adapter/claude`，双module及改变路径均通过（FINAL-06/U9时执行）；之前六包race证据仍保留。所有Go验证清除CODEX_HOME/CODEX_SANDBOX，使用 `/tmp/style-cli-{gopath,gomodcache,gocache}`。本轮核对ce912e6→HEAD没有Go/module差异，不因文档提交重复整套测试。
- FINAL-07提交前完整40项Python采集器回归通过；最终检查302份tracked Go无gofmt输出、30份manual/smoke Python语法有效，全部FINAL JSON可解析。11份当前文档111个本地链接/围栏另见FINAL-11审计。15个FINAL主/补条目审计为13项有证据关闭、09官方计数待验、12远端CI待验。
- 在clean d276b17上 `env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOFLAGS=-mod=readonly go build -trimpath -o /tmp/style-m1-final12-i20hx2za/style ./cmd/style` exit0。构建SHA256 a45ab727ac9046cfeca7e04f5afa116d004527985b5dd7c8cb54d40d7f587be4；`PYTHONDONTWRITEBYTECODE=1 python3 scripts/smoke.py /tmp/style-m1-final12-i20hx2za/style` exit0/32项通过。固定C9用8d5fbf75，后续干净构建VCS元数据不同；生产Go修复相同，未替换原C9程序或证据。
- 工作流补齐与Makefile一致的临时GOPATH/GOMODCACHE/GOCACHE，YAML解析/路径断言和实际 `go env -json GOPATH GOMODCACHE GOCACHE GOFLAGS`一致；原有Ubuntu/macOS、双module、全部采集器和smoke步骤保留。此项只验证配置，未冒充远端执行。
- 只读 `gh run list --repo inxups/style-cli --workflow ci.yml --commit b5caed2...`结果为空；最新公开run仍38872445836/04371ef success。当前批次未推送，不用旧run替代。汇总追加 `review/M1-FINAL-12-REGRESSION-FINAL-2026-10-16.json`并关联旧REGRESSION，任务/矩阵12保留[~]。
- 用户明确“暂时没有，保留计数待验”：FINAL-09继续[?]，未用登录态、合成usage或其他估算器代替官方计数。除官方计数和本轮远端CI外的M1补项均已由独立证据关闭；不宣称M1全部完成，不开始M2。

### 2026-10-16 — M1-FINAL-12 用户授权发布，最终源码双平台CI通过

- 本地实现/证据与62b9346提交全部可审阅、工作树干净后，用户明确授权“推送并验证CI”。先fetch并确认origin/main为祖先，再普通fast-forward推送04371ef→62b9346，无force操作。
- 真实GitHub Actions run38894349556（head62b9346ce73a27cb7fcfc57738c7ba0d1efb7ea7）completed/success。Test(macos-latest)、Test(ubuntu-latest)均success；逐步核对format、双module vet/test、全部采集器、build和smoke均成功。完整日志各出现一次Ran 40 tests和smoke passed，共两套，不用旧04371ef绿灯替代。
- `gh run view 38894349556 --repo inxups/style-cli --json databaseId,headSha,status,conclusion,jobs,url,createdAt,updatedAt` 与 `--log`核对通过；原日志留私有/tmp/style-m1-final12-i20hx2za/ci-38894349556.log。逐job/step元数据、日志SHA256及断言追加入 `review/M1-FINAL-12-CI-2026-10-16.json`，关联最终本地REGRESSION，保留所有历史待验和失败记录。
- FINAL-12任务/矩阵改[x]，当前首页/总览/教程同步。15个FINAL主/补条目已有14项关闭，唯一FINAL-09按用户“暂时没有key，保留计数待验”继续[?]；M1不宣称全部闭合，M2未开始。
- 本次收尾仅文档/证据，Go、采集器、示例和workflow与通过CI的62b9346完全相同；提交推送后继续核验最终文档HEAD的CI。`git diff --check`、JSON/本地链接检查和工作树检查通过。

### 2026-10-16 — 规范与设计基线迁入 docs/spec

- 按用户指示，把 `STYLE_DOCS/` 下已跟踪的 `STYLE-spec*.md`（6 份，含 `STYLE-cli-design.md`）用 `git mv` 迁到 `docs/spec/`，并新纳入原被忽略的 20 张 `svg/*.svg`；删除与 `svg/` 逐文件 diff 一致的 `STYLE-cli-diagrams-svg.zip` 和 `.DS_Store`。`STYLE_DOCS/STYLE.yaml` 是早期草稿，已被 `example/terse-reviewer/STYLE.yaml` 取代，未迁移，仍在 `.gitignore` 下。SVG 内嵌的 C2PA 元数据只含来源凭证，无本机路径或个人信息。
- 链接：迁入文档里 `../docs/X` 改为 `../X`，其余仓库根相对路径加一层 `../`；教程、adapter 总览和任务清单改指 `spec/` 或 `docs/spec/`；规范入口增加流程图目录链接。本条以上的日志条目、`review/*.json` 证据和 2026-10-13 两份评审/设计文档里的 `STYLE_DOCS/...` 是历史路径，按原样保留，对应文件现位于 `docs/spec/`。
- 验证：`docs/**/*.md` 相对链接检查只剩 `STYLE-spec-compiled.md` 指向 `style-gen/`、`style-review/` 的 3 个链接，这三个目标迁移前就不存在（见 `STYLE-M1-REVIEW-2026-10-13.md`），本次未新增断链。清理 CODEX_HOME/CODEX_SANDBOX、使用临时 GOPATH/GOMODCACHE/GOCACHE 的 `go test ./... ./agenthooks/...` 全部通过；`git diff --check` 通过。仅移动文档，未改 Go、采集器、示例或 workflow。
- CI：用户授权后，确认 origin/main 是祖先，普通快进推送 bf1f92e→9439987，没有使用 force。GitHub Actions run 38895447558（head 94399870b6f667a95b525d33c5beb6c7c262a029）completed/success；Test(ubuntu-latest)、Test(macos-latest) 的 format、vet、test、采集器、build 和 smoke 步骤都是 success。核对命令：`gh run view 38895447558 --repo inxups/style-cli --json databaseId,headSha,status,conclusion,jobs,url`。

### 2026-10-16 — M2-00 建立 M2 任务与测试文档 `[x]`

- 依据：用户确认 M1 完成，M2 范围限定为 CLI 设计 §14.4 五项（`target: native`、`style stats`、跨包去重 T8、Claude `~/.claude/rules/` 静态位置 U-C4、output style 做 replace 评估），不提前拉入 M3；文档沿用 M1 三件套；真实环境沿用当前约定（Codex 由 agent 执行，Claude Code 界面项由用户执行）。
- M1 关闭：唯一未闭合的 FINAL-09 官方计数按用户决定作为已知遗留移交 M2-CARRY-01，继续 `[?]`，证据仍为 `review/M1-FINAL-09-COUNTING-BLOCKED-2026-10-16.json`。M1 任务/测试首页改为已关闭并只读归档，FINAL-09 条目原状态保留并指向 M2-CARRY-01，backlog 的 M2 行指向新文档。
- 改动：新建 `docs/STYLE-M2-TASKS.md`（基线、遗留、设计定稿 D1–D5、M2a 实现 T10–T60、M2b 准备、里程碑验收、backlog）与 `docs/STYLE-M2-TESTS.md`（BASE、CARRY、A1–A9、B1–B4、C1–C4、CI、DOCS）；`AGENTS.md` 改为指向当前里程碑文档，完成里程碑时提示下一步规划。`docs/STYLE-M2-MANUAL.md` 留给 M2-PREP。
- 验证：改动文档的本地相对链接目标全部存在、代码围栏成对，`git diff --check` 通过。仅文档改动，未运行 Go 测试；Go 基线由 M2-01 执行。
- 下一步：M2-01 基线回归，然后 M2-D1 native 规则格式设计。

### 2026-10-16 — M2-01 M2 起点基线回归 `[x]`

- 起点：clean HEAD 6ec25f62b190fe93dea96c9af13c059b68222f9b（M2-00 仅文档，Go/module/采集器/workflow 与通过 CI 的 62b9346 相同），go1.27.0 darwin/arm64。所有 Go 命令清除 CODEX_HOME/CODEX_SANDBOX，使用 `/tmp/style-cli-{gopath,gomodcache,gocache}`，步骤与 `.github/workflows/ci.yml` 对齐。
- `gofmt -l $(git ls-files '*.go')` 无输出；`GOFLAGS='-mod=readonly -count=1' make test-go` exit 0（30 个包 ok，6 个包无测试文件，无 FAIL）；`GOFLAGS=-mod=readonly make vet-go` exit 0；`PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py'` Ran 40 tests OK。
- `env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache GOFLAGS=-mod=readonly go build -trimpath -o <scratch>/style ./cmd/style` exit 0，SHA256 48a86c05f79d7239b0595866e084eee4396377d27407fa900c35ca6a979c0fa5（含 VCS 元数据，换提交后哈希会变，只作本起点对照）；`PYTHONDONTWRITEBYTECODE=1 python3 scripts/smoke.py <scratch>/style` exit 0，32 项 ok，smoke passed。
- 日志与二进制留在会话 scratch，不入库。M2-01 / M2-BASE-01 改 `[x]`；后续 M2 改动以此为回归对照（M2-A9）。
- 下一步：M2-D1 native 规则格式设计，设计结论回写 `docs/spec` 后交用户评审，评审通过前不进入 M2-T10。

### 2026-10-16 — M2-D1 native 规则格式设计回写规范 `[~]`

- 依据：读 `core/spec/validate_rules.go:171`、`validate_package.go:86`（M1 写死 `codex`/`claude-code`）、`core/plan/plan.go:48`（`skipped-m2`）、`agenthooks/install/install.go:531-612`（按 `--owner=` 字段识别归属、`hookCommand` 只生成 `agenthooks run …`）、`render_codex.go`/`trust.go`（信任键 `<source>:<event>:<组>:<handler>` 与 `DefinitionHash`）、`render_claude.go`（`kindToClaude`、exec 形式）。关键约束：作者命令原样写入时不带 `--owner=style`，现有卸载认不出。
- 用户决定（AskUserQuestion，均选推荐项）：归属用命令前缀 no-op `: --owner=style --native=<包>/<id>; <命令>`；`codex-user` 照常预置 `trusted_hash`，信任关口是 STYLE 披露与确认；每个 harness 一个映射；M2 只支持 `event`/`matcher`/`command`/`timeout`，Claude `async`/`onFailure` 与非 command hook 留 M3。
- 回写：`spec/STYLE-spec-rules.md` 新增 §3.10（字段、`command` 约束、六个诊断码 `native-mapping`/`native-field`/`native-event`/`native-matcher`/`native-command`/`native-timeout`、plan 状态 `native-hook` 取代 `skipped-m2`、token 单列不估算、写入顺序与归属前缀），§3.3/§3.4 表同步；`spec/STYLE-spec-runtime.md` §8.6 增加 native 披露、非交互必须 `--yes`、native 变化 `sync` 退出 3（`--link` 也不例外）、trust 预览单列差异、list/doctor 核对；`spec/STYLE-cli-design.md` §3 增加 `NativeEvents()`、`HookSpec.Command/NativeTag` 和注册表注入 validate，§6 安装流程、§9.3 Codex native 事件与信任键序号、§10.2 Claude native 条目、§11.3 报告示例、§13 新增 F11（agenthooks `install.HookSpec.RawCommand/RawTag`，空值时渲染逐字节不变）。
- 我自行定的、需评审确认的点：native 变化对 `--link` 包也阻塞；matcher 两家都只开放工具类事件；省略 timeout 按 60 秒；正文可选且豁免 `empty-body`；包级 `defaults` 不作用于 native；Windows 写法随 M3。
- 验证：仅文档；本地链接与围栏检查、`git diff --check` 通过。未改 Go，M2-T10 在评审通过前不开始。

### 2026-10-16 — M2-D1 评审通过 `[x]`，细化 M2-A1/A2/A4

- 用户明确“M2-D1 评审通过，标 [x]，细化 M2-A1/A2/A4 后开始 M2-T10”。上一条记录中自行定的点（`--link` 也阻塞、matcher 只开放工具类事件、timeout 默认 60、正文可选、`defaults` 不作用于 native、Windows 随 M3）一并按规范定稿。
- 测试矩阵：M2-A1 列出六个新 invalid fixture、各码子情况、合法用例、单文件/目录一致与注册表注入单测；M2-A2 列出两家 golden 期望、token 单列、fallback 三种结果与报告不含命令；M2-A4 列出披露字段、非交互退出 1、`sync` 退出 3（含 `--link`）、trust 预览差异与 list/doctor 三态。
- 验证：仅文档，`git diff --check` 通过。下一步 M2-T10。

### 2026-10-16 — M2-T10 native schema 与 validate `[x]`

- 实现：新增 `core/spec/native.go`。`Validate` 对 native 规则做与 harness 无关的检查：规则引擎字段报 `native-field`，非 STYLE 字段的顶层键都当 harness 映射（无映射或非映射报 `native-mapping`），映射内只接受 `event`/`matcher`/`command`/`timeout`，`command` 按规则 3.10 的 POSIX 词法约束检查，`timeout` 必须是 1–600 的整数。native 规则跳过 `empty-body`、内联 file/text 必填、模板、lint 和 target/action 组合检查，包级 `defaults` 不再作用于它。`ValidateNative(pkg, catalog)` 按注册表检查 harness 键（未注册报 `native-field` 并列出已注册名）、事件（`native-event`）和 matcher（`native-matcher`）。`NativeRules` 解析映射供 T11/T12 使用。
- 注册表：`adapter.Definition` 新增 `Native spec.NativeHarness`，`Registry.NativeCatalog()` 汇总；`internal/adapter/codex/native.go`、`claude/native.go` 给出 CLI 设计 §9.3/§10.2 的事件表与 matcher 方言（Codex 正则；Claude `*` 或正则）。`internal/app` 的 validate、install、已装包快照与 sync 准备统一走 `Service.validateSpec`（`Validate` + `ValidateNative`）；删除写死的 `isNativeProviderField` 和 `validRuleFieldValue` 里的 harness 分支。`core` 仍不导入具体 adapter。
- 测试：`core/spec/validate_native_test.go` 替换旧的 `validate_native_single_file_test.go`，49 例在目录包与单文件包上逐例比对错误码，另有默认值、`from` 整体覆盖、非 native 规则写 `codex:` 报 `invalid-schema`、假注册表注入、`NativeRules` 解析；`testdata/invalid/` 新增 `native-mapping`/`native-field`/`native-event`/`native-matcher`/`native-command`/`native-timeout` 并加入 A1 清单；`internal/app/native_catalog_test.go` 核对注册表接线与两家事件/matcher 表。变异检查：令 `nativeCommandProblem` 恒返回空，13 个命令用例转红，已还原。
- 验证：`gofmt -l` 无输出；`GOFLAGS='-mod=readonly -count=1' make test-go` exit 0（30 个包 ok）；`GOFLAGS=-mod=readonly make vet-go` exit 0；`go build -trimpath` 后 `scripts/smoke.py` 32 项通过。plan/compile golden 未变（native 仍为 `skipped-m2`，由 T11 改）。
- 审查发现：`when.harness.name`、`requires.harness`、`recommends` 仍写死 harness 名，记为 M2-RV-01 `[!]`，待用户决定是否纳入 M2。下一步 M2-T11。

### 2026-10-16 — M2-T11 native plan 与 token 报告 `[x]`

- plan：`core/plan/native.go` 的 `planNative` 按适配器名取 `spec.NativeRules` 的映射。没有映射时报 `skipped`（原因 "no native mapping for this harness"，不看 fallback）；适配器实现 `plan.NativeAdapter` 且成功时报 `native-hook`，附 `RulePlan.Native{event, matcher, timeout}`；写不了时 `skip`/`degrade` 报 `skipped`，`error` 报 `install-error` 并阻塞。`skipped-m2` 状态删除。`RequirementFor` 对 native 提前返回，不带 action、needs、events，包级 `defaults` 与正文模板都不参与；`RulePlan.Action` 加 `omitempty`，文本报告写成 `- id: native-hook (native)` 和 `-> native <事件>[ matcher …]; timeout Ns`。compile 的 `checkPlan` 对 native 期望 action 为空。
- adapter：`adapter.NativePlanningFacts` 与 `planAdapter.NativeHook` 委托；`codex`/`claude` 的 `Facts.NativeHook` 给出实际 timeout（省略为 60；Codex `SessionEnd` 与 agenthooks 一致压到 3）。
- token：`AnalyzeWithLayout` 跳过 native 规则，不进 per-rule、always-on、事件合计、T001/T003；`native-hook` 的规则列入 `Report.Unestimated`（JSON `native_unestimated`），文本报告加一行 `native hook output not estimated: <规则> (<事件>)`。
- 示例：`codex-only-notify` 正文改为作者说明。两家 plan golden 重新生成，差异只有 native 一行、per-rule 中去掉 native、Codex 多一行不估算说明；其余估算与 M2-01 起点逐字相同。
- 测试：`core/plan/native_test.go`（native-hook、无映射、fallback 三态与不支持 native 的适配器、requirement 为空、文本不含命令或正文），`internal/adapter` 委托测试，两家 `native_test.go` 的 timeout 解析，`internal/app/native_plan_test.go`（A2 JSON 契约，fallback error 时 install 退出 4 且不建目录，plan 输出原因；skip/degrade 时 install 退出 0）。
- 验证：gofmt 无输出；`GOFLAGS='-mod=readonly -count=1' make test-go` exit 0（30 个包 ok）；`GOFLAGS=-mod=readonly make vet-go` exit 0；采集器 40 项 OK；`-trimpath` 构建后 smoke 32 项通过。
- 过渡状态：T12 之前 native-hook 只在 plan 中出现，install 不写 native 条目。下一步 M2-T12。

### 2026-10-16 — M2-T12 native adapter 与安装路线 `[x]`

- 接手起点为 clean 62605ac（Claude Code 已完成 T10/T11）。两家 `Facts.Native` 实现 `adapter.NativeWriter`，沿用 `NativeHook` 的事件/matcher/实际 timeout，生成作者命令和 `<包>/<id>` tag；安装编译另存 native hook，按 priority（含小数）、包安装序号、包内序号排序，不写入 runtime index。安装、重编译、部分卸载和 doctor 路线核对都使用完整 hook 集合。
- agenthooks 边界：按已批准 F11 扩展 `install.HookSpec.RawCommand/RawTag`，仅 POSIX Codex/Claude 支持；生成 `: --owner=style --native=<tag>; <作者命令>`，绕过 runner、插件启动器、异步/filter/fail-closed 参数。Claude 原生命令不进动态 `Surface` 合并，不影响 STYLE 自身订阅和 fail-closed；`*` matcher 保留。字段为空时现有 renderer 测试保持通过。
- 发现并修复已有合并问题：外部 matcher 组先于 STYLE 组时，原先 Codex `trusted_hash` 键仍指向渲染前的序号。新增 agenthooks `rebaseCodexTrust`，在无写入的安装规划中用同一 owner 合并规则计算外部组偏移，含路线切换的虚拟文件视图；哈希内容不变，键对应最终布局。CLI 设计 §9.3 回写实际索引约定。Codex 路线 status 保留盘上 native 命令核对 runner 面与信任，doctor 再与包的批准状态核对，避免合法 native 被误报 route-missing。
- 可重复证据：`internal/app/native_install_test.go` 的 `TestA3NativeInstallRoundTripAndRouteSwitch` 在 codex-plugin/codex-user/claude-user 上验证 owner、默认 60/指定 19 秒、priority 顺序、同优先级包顺序、两个包安装与单包卸载重编译、幂等、Codex 路线往返无残留、外部配置字节和 0640 权限还原；解析实际 TOML，将各 native 的最终组序号与该命令的 `DefinitionHash` 逐条对应。`agenthooks/install/native_test.go` 实际用 `sh -c` 执行渲染命令，stdin 字节相同且退出 7；覆盖含引号 tag、原生事件全集和 SessionEnd 3 秒限制。
- 验证：清除 CODEX_HOME/CODEX_SANDBOX，临时 GOPATH/GOMODCACHE/GOCACHE；`GOFLAGS='-mod=readonly -count=1' make test-go` 双 module exit 0；`GOFLAGS=-mod=readonly make vet-go` 双 module exit 0；加强信任键断言后单独重跑 `go test ./internal/app -run '^TestA3Native' -count=1` 通过。gofmt 与 `git diff --check` 通过；plan/compile golden 无改动。M2-A3 改 `[x]`；尚未宣称真实 harness 验收。
- 下一步：T13 安装披露、native 差异强制信任（含开发 link）、list/doctor 条目状态；共享 conformance 扩展仍由 T60 跟踪。

### 2026-10-16 — M2-T13 native 披露、信任与诊断 `[x]`

- install：把确认关口移到全部只读规划结束、首次写入之前，新增接收 detached `InstallResult` 的确认回调。文本 CLI 在写入前输出 capability 与 native 命令（harness/规则/事件/matcher/实际 timeout/作者原文/路线与信任方式）；JSON 的未确认调用返回同样的预览并退出 1。无 `--yes` 不创建 STYLE_HOME、不改 harness 字节；`--yes` 或确认回调只应用已捕获的源文件/配置快照。
- approval/sync/trust：批准快照新增 native 渲染字段（仍 format 1，lock 的 approval 哈希认证；T12 快照可从已认证源字节重建 native 基线）。native 源指纹覆盖所有 harness 映射、增删、priority/规则顺序和声明 timeout，普通包与已信任 link 的 native 变化都使 sync 退出 3；不把命令写进 sync 摘要。trust 单列前后 event/matcher/实际与声明 timeout/command，披露路线信任；批准一个包时，其他未批准 native link 保留旧快照。普通 link 规则仍可自动同步。
- doctor：agenthooks `install.InspectNative` 只读比对 owner/tag、原生事件、matcher、命令、timeout、handler 属性与重复条目，沿用同一 `DefinitionHash` 和最终组序号检查 Codex 用户路线信任；两家路线经可选 `adapter.NativeInspector` 委托。app 以批准快照为基准逐条输出 ok/missing/modified 与信任 match/missing/mismatch/unreadable，缺失/改动给 warning。native 配置或信任修复不由无确认的 sync 自动执行，改走 trust。list 文本/JSON 显示每包实际 native 条目数。
- 可重复证据：`internal/app/native_lifecycle_test.go` 的 `TestA4*` 共 6 个安装预览、48 个源变更、22 个 doctor/修复场景及 4 个边界用例。验证未确认时整个安装树字节不变；确认期间二次改源只安装已确认命令、回调不能篡改 transaction；T12 基线兼容；另一 link 的命令/lock 哈希未被连带批准；SessionEnd 10→20 秒仍实际 3 秒时必须重新信任且预览显示两种 timeout。`go test ./internal/app -run '^TestA4' -count=1` exit 0。
- 验证：所有 Go 命令清除 CODEX_HOME/CODEX_SANDBOX，使用临时 GOPATH/GOMODCACHE/GOCACHE。最终字段补齐后 `GOFLAGS='-mod=readonly -count=1' make test-go`、`GOFLAGS=-mod=readonly make vet-go` 双 module exit 0；`go test -race -count=1 ./internal/app` exit 0；两家 adapter 的 race 在本条路线实现后已通过，最后只增加 app 报告的声明 timeout 字段。Python 采集器 40 项 OK。此前本条实现的 `-trimpath` 构建与 smoke 32 项通过（/tmp/style-m2-native.xD9rcb/style，SHA256 df30b7fc…85bcc）；最终 native 批次构建由 T60 再记录。gofmt、当前文档本地链接/围栏和 `git diff --check` 通过，plan/compile golden 无改动。
- M2-T13 / M2-A4 改 `[x]`；真实 Codex/Claude 验收仍未执行。下一步 T60 shared conformance 扩展，然后 D2 `style stats` 设计评审。

### 2026-10-16 — M2-T60 native 共享 conformance `[x]`

- `internal/adapter/conformance/routes_test.go` 保留原路线测试，另复用同一往返流程加入两个同事件 native 命令：codex-plugin/codex-user/claude-user × 无尾换行/一个/两个尾换行，共 9 次。验证只读 plan、条目不合并、native 状态、Codex 用户路线哈希、重复安装/卸载幂等，最终外部配置字节与 0640 权限还原、零额外文件。
- 新增 `native_test.go`，给现有 payload 包追加 native 声明，对两家共 46 个 payload 经真实 codec/runtime 检查：stdout 与原 golden 相同且通过白名单；runtime index 不增加 native 条目；统计记录除运行时间外与无 native 基线逐字段相同。初次完整 index 比较失败，定位为 inline 规则的 `src_sha256` 认证整个 STYLE.yaml，追加声明必然改变摘要；测试改为先核对并替换这一可计算摘要，其他字段仍严格比较，未改生产代码、未更新 golden。
- 验证：清除 CODEX_HOME/CODEX_SANDBOX，临时 GOPATH/GOMODCACHE/GOCACHE；`GOFLAGS=-mod=readonly go test -race -count=1 ./internal/adapter/conformance` exit 0（8.022s），`go vet ./internal/adapter/conformance` exit 0。生产代码为 6c30d35，本条仅新增测试；双 module fresh test/vet、app/两家 adapter race 与采集器 40 项沿用紧接之前的 T13 证据。
- 最终 native 批次：同一隔离 Go 环境执行 `go build -trimpath -buildvcs=false -o /tmp/style-m2-native-final.W5m9T5/style ./cmd/style` exit 0；显式禁用 VCS 元数据以免本条未提交的测试影响构建摘要。SHA256 `5ab3ad8efebf0cd08fa37728d921224fd71bc13e74e6ffb042bf9e9818ba2316`；`PYTHONDONTWRITEBYTECODE=1 python3 scripts/smoke.py /tmp/style-m2-native-final.W5m9T5/style` 32 项通过。二进制只留 /tmp，不入库。gofmt、本地文档链接/围栏、`git diff --check` 通过。
- M2-T60 改 `[x]`，A3 补共享证据，A9 记 native 阶段通过但保持 `[~]` 等后续 M2 实现后最终复验。真实 native 投递不由本条离线测试替代。下一步 D2 stats 输出设计，按任务清单在用户评审后进入 T20。

### 2026-10-16 — M2-D2 stats 输出草案 `[~]`，待用户评审

- 依据：`core/state/stats.go` 的真实字段为 at/provider/event/point/entry/action/result/est_tokens/body_sha256，旧 §11.5 的 ts/kind/est 示意已落后于实现；`internal/runtime/runtime.go` 记录当次模型侧估算，`core/engine/record.go` 不保存正文；`doctor_stats.go` 仅读近 7 天 output/blocked、有估算与摘要的 top3，旧记录计数单列，遇坏行原先提前停止；state 会话目录名来自现有 component，scope 不保存可恢复的原始 ID。
- 草案回写 CLI 设计 §4/§11.5.2：`--by rule|session` 默认 rule，`--since` 默认 168h、正 duration 或 RFC3339，until 单次捕获且边界包含；按 provider/规则或 provider/目录 session 分组，后者合并全部 agent。区分 outputs/blocks/dedup/cap_drops，历史 token 仅累加已有估算与摘要的实际输出；缺成本记 unestimated。文本/JSON format 1、排序、空报告、质量计数和退出码均给出契约，不把记录条数当作请求数或账单。
- 拟定坏行/超长行有界跳过继续统计、partial 与计数可见；根 symlink/I/O/取消/溢出退出 1。stats 完全只读，不触发 adapter 检测、index 读取或清理。doctor 提取共享读取器，正常 top3/legacy/repeated_body 保持，坏行继续统计并保留 warning；agenthooks 与 runtime 写入格式不变。M2-A5 写入待评审验收清单，尚无 fixture/golden 或测试通过声明。
- 验证：仅文档，代码/二进制仍为 T60 的已验证版本；当前文档本地链接、围栏、示例 JSON 结构和 `git diff --check` 通过。D2 保持 `[~]`，依任务清单“用户评审通过后才进入对应实现条目”，T20 未开始。


### 2026-10-16 — M2-D2 评审通过 `[x]`

- 上一轮已提交 D2 草案并请求评审，用户回复“继续”；按此同意原草案的默认 168h、rule/session 分组、只读/有界扫描、六字段计数、质量报告与 doctor 兼容契约，进入 T20。未扩大到尚未评审的 D3–D5 实现。
- D2 改 `[x]`，CLI 设计 §11.5.2 改为实施契约，A5 按契约验收；T20/A5 尚未标完成。
- 验证：仅文档状态更新，`git diff --check` 通过；当前草案已有的本地链接和 JSON 示例未改。下一步实现 T20。


### 2026-10-16 — M2-T20 stats 命令与 M2-A5 `[x]`

- 实现：`internal/app/stats.go`/`stats_command.go` 接入 `style stats [--by rule|session] [--since <duration|RFC3339>] [--json]`，默认 168h、单次捕获 until、UTC/包含边界；按 provider+规则或 session 目录键汇总六个非负 int64 字段、稳定排序、JSON format 1。只累计历史 output/blocked 自带估算与摘要，缺任一字段保留次数并记 unestimated，dedup/cap-drop 不累加候选成本；不依赖 index、已装包或 adapter 检测。
- 共享读取：`core/state/stats_read.go` 固定扫描 session/agent 两层目录，用 `os.Root` 固定目录句柄、Lstat 与 inode 核对拒绝路径替换，跳过后代 symlink、拒绝非法根；512 字节缓冲分片读取，>=512 字节行有界丢弃后继续，严格校验真实记录字段、时间、枚举与摘要。坏行/超长行/跳过 symlink 在 quality 计数，partial 时仍输出有效汇总；I/O/取消/计数溢出返回错误，CLI 在完整读取后才写报告，不回显原始坏行或敏感字段。agenthooks、runtime writer 与留存策略未改。
- doctor：`doctor_stats.go` 复用同一读取器，正常最近 7 天 top3、legacy 与 repeated_body 保持；坏行后有效记录继续汇总，给 stats-invalid warning，仍保留 doctor 自己的清理职责。
- 证据：`testdata/stats/m2` 固定 2026-10-16T00:00:00Z，含两家+未注册历史 provider、不同评估点、main/child、多会话与边界相邻 1ns；`TestA5StatsRuleAndSessionGoldens` 先独立核对每行及合计 11 outputs / 3 blocks / 1 dedup / 1 cap_drop / 96 estimated_tokens / 3 unestimated，再比四份文本/JSON golden。重生成命令为隔离 Go 环境下 `STYLE_UPDATE_GOLDENS=1 go test ./internal/app -run '^TestA5StatsRuleAndSessionGoldens$' -count=1`；本次随后去掉更新开关重跑全部测试通过。
- 边界证据：`TestScanStats*`/`TestReadStatsLines*` 覆盖未知字段、非法元数据/JSON/截断、511/512/513/1MiB 行、旧成本字段、深度与 5 处 symlink、消失文件、权限/I/O、扫描中取消与质量计数溢出。`TestA5Stats*` 覆盖 duration/RFC3339/UTC/窗口、非法参数 stdout 空、空报告不建目录、坏行隐私标记不泄露、标识符转义、真实 component 哈希会话键、聚合溢出、全树字节/权限/mtime 不变和过期会话未清理；doctor 用同一 fixture 验证 top3 与三条旧记录，并验证坏行后继续统计/告警。
- 验证：所有 Go 命令清除 CODEX_HOME/CODEX_SANDBOX，临时 GOPATH/GOMODCACHE/GOCACHE。`GOFLAGS='-mod=readonly -count=1' make test-go`、`GOFLAGS=-mod=readonly make vet-go` 双 module exit 0；`go test -race -count=1 ./core/state ./internal/app` exit 0（state 1.774s / app 21.528s）；Python 采集器 40 项 OK。`go build -trimpath -buildvcs=false` 成功，二进制 `/var/folders/m5/gv7_10k149l3j4l02mt548zr0000gp/T/style-m2-stats.xy26oody/style` SHA256 `63984423c125e04326864407f3074362a6f1b8c84bc8f39cc29f75e53f3fbe80`；扩展 `scripts/smoke.py` 36 项通过，新增空 home 只读、真实 hook 日志的 rule/session 汇总与卸载后历史仍可读。临时产物不入库，既有 plan/index/stdout golden 未改。
- gofmt、本地文档链接/围栏、四份新 golden JSON 结构与 `git diff --check` 通过。T20/A5 改 `[x]`；A9 保持 `[~]` 等剩余 M2 功能完成后复验。下一步 D3 跨包去重设计，评审通过前不进入 T30。


### 2026-10-16 — M2-D3 跨包去重草案 `[~]`，待用户评审

- 接 T20 clean 08aaa61 读取现有实现：`core/compile/build.go`/`entries.go` 在所有包里逐 placement 构造并按 order 排序，`body_sha256` 认证渲染前模板，静态变量在 compileBody 中替换；`core/tokens/installed.go` 当前直接合并包级桶和分隔符；`internal/app/plan.go` 仅做单包 plan；`core/state/dedup.go` 以 rule+渲染正文抢占，block/every/subagent.start 不抢占。仅按模板哈希或某次输出同文折叠，会丢失条件、selector、warn 用户文字或动态变量变化。
- 草案回写运行时 §7.5、编译产物 §9.6、CLI 设计 §7.1，native 规则段与 A6 同步。拟采用编译阶段、静态变量替换后全文+全部执行语义等价，按 order 保留；native、不同语义、运行时偶然同文与同包多重声明类保持。保留 index format 1 和 agenthooks 边界；组合报告标 deduplicated_into，全局预算及有效成本只计一次，单包源级校验独立。
- 明确待用户确认的权衡：不引入跨包持久 repeat 键，卸载保留方后接替规则可能在旧会话再注入一次；跨包比较只在组合安装规划中进行，独立 plan 不读已装包。所有这些边界均为建议，未擅自进入 T30。
- 验证：仅文档；6 份当前文档的围栏、43 个存在的本地目标及新增标题锚点、`git diff --check` 通过；编译产物文档原有 3 个缺失的外部资料路径（style-gen 两项、style-review 一项）与 HEAD 一致，未纳入本条改动。未修改 schema/Go/测试产物，代码仍为 T20 已验证版本。D3 保持 `[~]`，T30/A6 未标完成。


### 2026-10-16 — M2-D3 评审通过 `[x]`

- 上一轮已提交 D3 草案并请求评审，用户回复“继续”；按该契约同意编译阶段完整正文/执行语义去重、同包多条保守保留、native 排除、源级预算独立与有效组合成本、保留方切换可能再次注入的边界，进入 T30。
- D3 改 `[x]`，规范草案改为实施契约，A6 保留未完成状态；没有扩大到尚需真实环境结论的 D4/D5。
- 验证：仅文档状态与锚点同步，`git diff --check` 通过。下一步实现 T30。


### 2026-10-16 — M2-T30 跨包去重与 M2-A6 `[x]`

- 编译：`core/compile/dedup.go` 在所有包构造、静态替换和排序之后收集每条规则的完整 placement 集合；每个 harness 独立，以最终静态字节/slot/mode 或规范化 runtime 执行字段、编译片段、warn 双通道和能力判等。摘要只分桶，再逐结构比较（含 nil/空条件差异），不依赖原始 body/source 摘要。按小数 priority/包序号/规则序号保留第一条，原 provenance 不重写；同包多条等价声明的整个类保留并解释，native 不参与。`Index.Static`/`Harnesses` 与 `StaticBodies` 同步过滤，packages 保留全部，index schema/format 1、runtime/state 与 agenthooks 均未修改。
- 报告与预算：`Result.Deduplicated`/`DeduplicationWarnings` 只返回冷路径关系。install/trust 包含请求包及关系中的参与包，plan/token 折叠行增加可选 `deduplicated_into`，保留状态/单条估算，文字标“no additional cost”，无正文/settings 泄露。先运行单包源级检查，再编译并重算有效 events/always_on/session/T003；保留源级 T001/T002/T004–T007，分隔符和 block/other 最大分支重算。静态 T001 仍在 validate 退出 1，T002/T004 安装预算与 T003 退出 4；规范补清阶段差异，均不写安装树。单包 plan 不读 STYLE_HOME。
- 核心证据：`TestCrossPackageDedup*` 覆盖 3 包 × 2 harness、分数 priority、序号与反转输入、完整 dynamic system/skill 入口、静态替换、同包多重声明、skipped/never 与能力排除；`TestDedupPreservesDifferentCompiledSemantics` 逐项覆盖 22 种执行语义/正文形状差异，nil/空 alternatives 同 JSON 哈希桶仍不合并。`core/tokens/dedup_test.go` 验证源级诊断不变、有效桶零贡献，以及 tokens/chars 两种 T003 单位的分隔符/分支边界。
- app 证据：`TestA6Dedup*` 覆盖三路线分别卸载保留方/折叠方、两条 Codex 路线切换、幂等、doctor、普通 trust/已信任 link 的正文/priority 改动、原文件字节与 0640 还原；原单包 plan 对比不变。组合报告两份 `testdata/golden/dedup-install-codex.{txt,json}`，先核对字段/成本与正文不泄露再更新 golden，随后无更新开关重跑。12 并发调用只有 1 次 session 输出、11 次 canonical dedup；compact/clear、turn、渲染正文变化、warn every、block every、subagent.start 不占位、条件不命中、cap-drop 释放、旧会话保留方卸载后接替身份可再输出均验证。
- 边界：1002-token 两份等价声明去重前 2005 > Codex T003 的 2000，去重后 1002，可安装；不同 repeat 或运行时偶然同文仍退出 4，安装树完全不变；源级 T001/T002/T004 不能被另一个包抵消。相同 native 命令仍全部安装。同包歧义的组合报告解释保留原因。原 A18 T003 fixture 原先正文相同，现第二份改为等长不同首字母，继续独立验证真实增长为 2005 时拒绝；未改任何 M1 归档文档。
- conformance：`TestCrossPackageDedupPreservesAllCodecGoldensAndRuntimeStats` 为真实语料追加等价包，两家 46 个 codec payload 的 canonical 条目、原 stdout golden/白名单和 stats 记录完全一致（仅排除运行时间），编译折叠方不生成虚构去重行；既有 index/plan/runtime golden 未改。
- 验证：全部 Go 命令清除 CODEX_HOME/CODEX_SANDBOX，临时 GOPATH/GOMODCACHE/GOCACHE。`GOFLAGS='-mod=readonly -count=1' make test-go`、`GOFLAGS=-mod=readonly make vet-go` 双 module exit 0；`go test -race -count=1 ./core/compile ./core/tokens ./core/engine ./core/state ./internal/app ./internal/adapter/conformance` exit 0（app 22.219s，conformance 9.701s）；随后新增 turn/正文 claim 专项 race 通过，生产代码只补 API 注释。采集器 `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py'` 40 项 OK；初次误用不存在的 tools/collect-tokens 路径只得到 discovery ImportError，已改用上述仓库命令。
- 最终构建：`go build -trimpath -buildvcs=false` 成功，二进制 `/var/folders/m5/gv7_10k149l3j4l02mt548zr0000gp/T/style-m2-dedup-final.kkbpbqzx/style` SHA256 `c35d85bd70c4bd575e5eaf401662870b45867b419995e2747a5917c57952ab8c`；扩展 smoke 40 项通过（36→40，新增实际 CLI 组合报告、单份静态/index、doctor、卸载折叠方不改保留产物）。缓存、二进制与临时探针均不入库。gofmt、5 份当前文档围栏/39 个存在的本地目标/D3 锚点与 JSON golden、`git diff --check` 通过；编译规范原有 3 处缺失外部材料链接未改。
- T30/A6 改 `[x]`，A9 补本阶段回归并保持 `[~]` 等剩余 M2 任务最终复验。下一步 D4/D5 Claude 真实环境结论与手册准备；没有用离线测试替代真实 harness 证据。


### 2026-10-16 — M2-PREP-01 Claude 设计探针准备 `[x]`；D4/D5 `[~]`

- 接 T30 clean 74a473d，按依赖推进 D4/D5。新增 `testdata/manual/probe_claude_m2_design.py`：protocol 模式复用既有 loopback ClaudeProbe；prepare-ui 模式创建空 scratch 下独立 HOME/config/work、用户 rules/CLAUDE.md/三样式、InstructionsLoaded 白名单观察器、四个分模式启动器与显式 login.sh。只为准备读取 `claude --version`，不自动登录或启动界面/模型会话；env -i 不继承凭据、日常配置或 Codex 检测变量。0700/0600，非空目录拒绝，特殊 shell 路径用 shlex。
- 版本固定：`/Users/inxups/.nvm/versions/node/v24.21.0/bin/claude --version` 为 2.1.301，未升级、未声称 latest。2026-10-16 复核官方 memory/hooks/output-styles Markdown（URL 与 SHA256 在协议报告）；web 入口未返回内容，Python HTTPS 遇本机 CA 校验失败，改用系统 curl 正常校验获取，没有关闭 TLS 验证。没有复制官方全文进仓库。
- 补充原生协议：最终 `/tmp/style-m2-claude-design-final-20261016`，显式 SIMPLE_SYSTEM_PROMPT=0，default/existing/replace/keep 各新会话一份真实 CLI Messages 请求、exit 0，无未知端点。用户 rules 与 CLAUDE.md 的完整连续正文每份各一次（不只匹配 marker），仅选中 style 全文出现一次，其他 style 均无；候选 style 位于 messages，system 长度分别 26512/23229/23229/26550，replace 仍保留与 default 相同 20615 字符的系统片段（连接文本块比较）。支持 rules 候选和 output style 不作为整份 system 替换的评估方向，但未定稿。
- InstructionsLoaded 为异步观察事件；较早短 prompt 对照抓到 User/session_start 的 rules 路径，最终短生命周期 headless 观察日志为空，未将空日志当作未加载；最终以实际请求全文证实投递。所有原始 request/debug/stdout 留在私有 scratch，只提交 `docs/review/M2-D4-D5-PROTOCOL-2026-10-16.json` 的摘要、计数、定位和命令。loopback 的合成 usage 不用于官方 token 计数，M2-CARRY-01 不变。
- 用户界面准备：`/private/tmp/style-m2-claude-ui-20261016/prepared.json` 为 acceptance=false，独立 login.sh 与 launch-default/existing/replace/keep.sh；没有启动这些脚本。`docs/STYLE-M2-MANUAL.md` 写明信任、短请求、/config 样式观察、证据回传与复现步骤；用户界面项遵守“Claude Code 界面项由用户执行”。M2-PREP 保持 `[~]`（其他最终验收 runbook 等 T40/T50），D4/D5 `[~]`，C1/C4 `[?]`，T40/T50 未开始。
- 验证：`PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py'` 45 项 OK（新增五项），覆盖观察器敏感内容排除、marker 不等于全文、三样式、特殊路径脚本语法/权限、仅 --version 无会话执行与拒绝覆盖。真实本地协议四请求再次逐项复算、源码 SHA256 与请求 system/messages 摘要、UI receipt/脚本权限和语法、本地文档链接/围栏/JSON、Python AST、`git diff --check` 通过。无 Go/agenthooks/生产行为修改，沿用 T30 的双 module test/vet/race 与 smoke 40 项证据。
- 下一步需要用户按 M2 手册执行隔离 Claude 界面步骤；agent 收集证据后提出 D4 迁移和 D5 回退/实现的正式结论供评审，不能依据本条准备或本地 stub 提前关闭真实界面验收。


### 2026-10-17 — M2-PREP-01 OAuth 启动环境修复；C1/C4 `[~]`

- 用户首次登录报告 `OAuth error: Couldn't save your login. If your Mac's keychain is locked, unlock it and log in again.`。没有据此断言钥匙串已锁定，也没有把 OAuth 或界面验收记为成功。先读取当前任务/测试/手册和工作树，起点 clean `8e2de6d`。
- 复现原因：原 prepare-ui 为隔离配置改写 HOME，macOS `security default-keychain -d user` 在该生成环境退出 1，stderr 为 `A default keychain could not be found.`；保留真实 HOME 的最小环境同一只读查询退出 0，返回用户 login.keychain-db。没有读取凭据值、写入/删除钥匙串条目或自动解锁。固定 Claude 2.1.301 的本机代码也通过 `security` 保存 OAuth；官方 authentication/env-vars 资料确认独立 CLAUDE_CONFIG_DIR 的用途和凭据作用域，页面摘要记录在修复报告。web 未返回正文，改用系统 curl 正常 TLS 校验，不关闭验证。
- 修复：prepare-ui 保留真实 HOME/USER/LOGNAME，继续 env -i 排除 API key、OAuth token、CODEX_HOME/CODEX_SANDBOX 和宿主安全存储覆盖变量；CLAUDE_CONFIG_DIR、ANTHROPIC_CONFIG_DIR、XDG_CONFIG_HOME 指向私有 scratch，避免因保留 HOME 而复用默认 profile 配置。登录和四种会话均固定到临时工作目录。不改本地 loopback 协议模式的假 HOME，不改 STYLE/agenthooks 生产行为。
- 现有 `/private/tmp/style-m2-claude-ui-20261016` 的五个脚本已原位修复；旧脚本和 prepared.json 先备份到 `auth-home-fix-before-20261016/`。8 个已有设置/fixture/日志文件 SHA256 前后相同；prepared.json 追加环境策略和私有修复收据位置，acceptance 继续 false。原协议证据保留历史版本，未以新源码摘要重写旧报告。修复归档为 `docs/review/M2-PREP-01-AUTH-2026-10-17.json`。
- 验证：`PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py'` 47 项 OK。新增两项实际执行五个生成脚本（含空格/引号路径），核对真实 HOME/USER/LOGNAME、临时配置、工作目录、凭据/Codex 检测变量排除和 auth login 参数；核对单独重写启动器不覆盖设置或已有观察记录。五个实际修复脚本 `sh -n` 通过，修复环境的只读默认钥匙串查询 exit 0；这不证明钥匙串解锁/可写或 OAuth 保存成功。
- 手册写明重新执行原 login.sh，看到 Login successful 后继续四种界面步骤；若同样失败，再检查 login 钥匙串解锁状态并回报错误。M2-PREP-01 脚本准备仍 `[x]`；C1/C4 由待执行改 `[~]`，表示真实界面受登录失败阻塞并待重试。D4/D5 维持 `[~]`，不进入 T40/T50。无 Go 修改，本条不重复运行已通过的 T30 Go 回归。


### 2026-10-17 — M2-C1/C4 Claude 订阅额度阻塞 `[~]`

- 用户报告“我claude订阅没额度了”，作为用户提供的环境状态记录；没有发起额外模型请求复核、推断额度恢复时间或把额度错误当成功能失败。原 OAuth 保存修复后仍未重试确认，两个阻塞原因分别保留。
- 影响：D4/D5 真实界面证据与评审仍未完成；C1/C4 维持 `[~]`，T40/T50 依赖设计结论而未开始。C2/C3 仍为后续待验，不伪造已运行记录；已有 native/stats/去重离线通过证据不受用户额度报告影响。M2-CARRY-01 仍是两家官方 API key 缺失，与订阅额度问题分开。
- 恢复步骤：额度可用后，若隔离登录仍需完成，执行 `/private/tmp/style-m2-claude-ui-20261016/login.sh`，再按手册依次运行 `launch-default.sh`、`launch-existing.sh`、`launch-replace.sh`、`launch-keep.sh`。现在不重复登录/发测试消息；已有 scratch、日志、协议报告与失败证据保留。没有创建轮询或自动重试任务。
- 独立推进范围：Codex B1–B4 所需 T10–T13/T20/T30/T60 已完成，可以先准备和执行这部分独立验收，不把尚未评审的 Claude 迁移/replace 纳入。
- 验证：仅任务/测试/手册状态与追加日志改动；当前文档本地链接、围栏和 `git diff --check` 检查，无代码/fixture/通过状态变化，不重复 Go/模型测试。


### 2026-10-17 — M2-PREP-02 Codex 独立验收准备 `[x]`

- Claude 订阅额度阻塞后，只推进依赖已完成 T10–T13/T20/T30/T60 的 Codex B1–B4。新增 `testdata/manual/probe_codex_m2.py`，复用 NativeProbe 的私有 CODEX_HOME/STYLE_HOME/work、loopback 转发与已配置的真实 Responses 服务；配置/日志不进入 Git，报告仅含白名单原生观察、完整正文摘要与次数、rollout 摘要、OTel span 和 STYLE stats。未修改 NativeProbe、STYLE 或 agenthooks 的生产边界。
- 探针分别支持 codex-user 和 codex-plugin：SessionStart 与 Bash PreToolUse native 各一次，Read matcher 不命中；同事件 STYLE 与 native handler 各一次；生产 stats 按规则/会话逐行与日志对账，native 输出不混入 STYLE 计数；两包等价声明只编译/投递保留方一次，stats 不虚构折叠行。卸载核对配置字节/权限、hooks 文件无残留，并以额外真实会话验证 native 不再执行；插件登记使用隔离本地 marketplace，并单独由原生命令移除。
- 手册 §4 补构建/两路线命令、证据边界和失败保留。脚本不覆盖旧 scratch；不会把 exec 的哈希/插件接受等同于 TUI 工作区信任。没有调用 Claude、提前关闭 D4/D5 或 T40/T50。
- 复核：已读取 OpenAI Docs 的 hooks/config-reference 原生接口说明，使用已固定的本机 codex-cli 0.159.3，而非依据官网当前默认覆盖旧版本；本机已配置 Responses provider，未探查/输出其连接值。STYLE 固定 T30 构建仍存在，SHA256 `c35d85bd70c4bd575e5eaf401662870b45867b419995e2747a5917c57952ab8c`。
- 验证：`PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py'` 50 项 OK，新增三项验证 marker 不等于完整正文、用户/工具角色不算 developer 投递、重复/失败回合拒绝；stats 合计/每行/额外折叠行/partial 拒绝；真实执行原生 observer，确认只保存 fixture/event/session_id 和 0600。源码 AST、当前文档本地链接/围栏与 `git diff --check` 通过。此提交只标准备通过，B1–B4 未宣称真实运行成功。


### 2026-10-17 — M2-B1 native hook 真实触发 `[x]`

- 真实结果：codex-user/plugin 两路线的 SessionStart、Bash PreToolUse 各一次，Read matcher 不触发；白名单观察、OTel、rollout 和请求全文相符。卸载后额外真实会话无 hook/span/正文，配置字节与权限还原。只声明 exec 对用户哈希/插件登记的接受，不声明首次 TUI 工作区信任。
- 原生命令与证据：`probe_codex_m2.py --style <固定 T30 构建> --route codex-user|codex-plugin --scratch <各自空目录> --output <私有报告>`，两路线脚本均 exit 0、四项 check 均 true。源码提交 f76b37e，固定 codex-cli 0.159.3；STYLE SHA256 `c35d85bd70c4bd575e5eaf401662870b45867b419995e2747a5917c57952ab8c`。实际参数、时间、源码/原始报告摘要和本条事实见 `docs/review/M2-B1-CODEX-2026-10-17.json`。
- 归档仅提取本条白名单结果，不写连接配置、provider 地址、鉴权信息或原始 prompt；真实推理使用用户已有 Responses 服务，未调用 Claude 或改变额度相关状态。原始日志在两份私有 scratch 中保留；未修改日常安装。
- 验证：重算两份原始报告的 SHA256，核对源码/固定二进制摘要与本条双路线 passed=true、抽取结果逐字段相同；JSON 可解析、当前文档链接/围栏、追加式日志和 `git diff --check` 通过。源码/采集器沿用 f76b37e 已通过的 50 项，无 Go 生产代码改动，不重复已通过的 Go 回归。


### 2026-10-17 — M2-B2 native 与 STYLE hook 共存 `[x]`

- 真实结果：codex-user/plugin 两路线在 SessionStart 和 PreToolUse 上各有两个 completed span（native 与 STYLE 各一），来源正确；四份完整正文各一次，STYLE runtime 两条 output，native 不进入规则日志。信任范围沿用 B1。
- 原生命令与证据：`probe_codex_m2.py --style <固定 T30 构建> --route codex-user|codex-plugin --scratch <各自空目录> --output <私有报告>`，两路线脚本均 exit 0、四项 check 均 true。源码提交 f76b37e，固定 codex-cli 0.159.3；STYLE SHA256 `c35d85bd70c4bd575e5eaf401662870b45867b419995e2747a5917c57952ab8c`。实际参数、时间、源码/原始报告摘要和本条事实见 `docs/review/M2-B2-CODEX-2026-10-17.json`。
- 归档仅提取本条白名单结果，不写连接配置、provider 地址、鉴权信息或原始 prompt；真实推理使用用户已有 Responses 服务，未调用 Claude 或改变额度相关状态。原始日志在两份私有 scratch 中保留；未修改日常安装。
- 验证：重算两份原始报告的 SHA256，核对源码/固定二进制摘要与本条双路线 passed=true、抽取结果逐字段相同；JSON 可解析、当前文档链接/围栏、追加式日志和 `git diff --check` 通过。源码/采集器沿用 f76b37e 已通过的 50 项，无 Go 生产代码改动，不重复已通过的 Go 回归。


### 2026-10-17 — M2-B3 stats 真实汇总 `[x]`

- 真实结果：codex-user/plugin 两路线的生产 style stats rule/session 每行及总量与真实 runtime 记录一致：2 outputs、26 estimated_tokens；对应两份 STYLE rollout 全文，另外两份 native 注入单列排除。窗口内 session 身份正确，quality.partial=false。
- 原生命令与证据：`probe_codex_m2.py --style <固定 T30 构建> --route codex-user|codex-plugin --scratch <各自空目录> --output <私有报告>`，两路线脚本均 exit 0、四项 check 均 true。源码提交 f76b37e，固定 codex-cli 0.159.3；STYLE SHA256 `c35d85bd70c4bd575e5eaf401662870b45867b419995e2747a5917c57952ab8c`。实际参数、时间、源码/原始报告摘要和本条事实见 `docs/review/M2-B3-CODEX-2026-10-17.json`。
- 归档仅提取本条白名单结果，不写连接配置、provider 地址、鉴权信息或原始 prompt；真实推理使用用户已有 Responses 服务，未调用 Claude 或改变额度相关状态。原始日志在两份私有 scratch 中保留；未修改日常安装。
- 验证：重算两份原始报告的 SHA256，核对源码/固定二进制摘要与本条双路线 passed=true、抽取结果逐字段相同；JSON 可解析、当前文档链接/围栏、追加式日志和 `git diff --check` 通过。源码/采集器沿用 f76b37e 已通过的 50 项，无 Go 生产代码改动，不重复已通过的 Go 回归。


### 2026-10-17 — M2-B4 跨包去重真实投递 `[x]`

- 真实结果：codex-user/plugin 两路线均保留两个 packages、只有 m2-first/shared 一个编译入口；组合报告标 m2-second/shared 折叠到保留方。真实请求/rollout 正文各一次，stats 仅保留方一次 output，卸载后配置字节与权限还原。
- 原生命令与证据：`probe_codex_m2.py --style <固定 T30 构建> --route codex-user|codex-plugin --scratch <各自空目录> --output <私有报告>`，两路线脚本均 exit 0、四项 check 均 true。源码提交 f76b37e，固定 codex-cli 0.159.3；STYLE SHA256 `c35d85bd70c4bd575e5eaf401662870b45867b419995e2747a5917c57952ab8c`。实际参数、时间、源码/原始报告摘要和本条事实见 `docs/review/M2-B4-CODEX-2026-10-17.json`。
- 归档仅提取本条白名单结果，不写连接配置、provider 地址、鉴权信息或原始 prompt；真实推理使用用户已有 Responses 服务，未调用 Claude 或改变额度相关状态。原始日志在两份私有 scratch 中保留；未修改日常安装。
- 验证：重算两份原始报告的 SHA256，核对源码/固定二进制摘要与本条双路线 passed=true、抽取结果逐字段相同；JSON 可解析、当前文档链接/围栏、追加式日志和 `git diff --check` 通过。源码/采集器沿用 f76b37e 已通过的 50 项，无 Go 生产代码改动，不重复已通过的 Go 回归。
- M2b Codex 验收汇总改 `[x]`。D4/D5、C1/C4 的 Claude 额度/OAuth 阻塞、T40/T50 未定稿、最终 M2 回归/CI/文档同步和官方计数遗留均未因此关闭。


### 2026-10-17 — M2-DOCS-02 已实现功能文档同步 `[x]`

- 起点 clean e6dd778。Claude 额度/OAuth 阻塞与 D4/D5 评审依赖未变，先推进已完成 T10–T13/T20/T30 的文档。教程补 native 完整包、stats 用法和六字段口径、跨包等价去重及保留方卸载边界；修正旧的“native 只校验”和“stats 未实现”、link native 重新批准与 sync 退出 3 的说明。
- adapter 总览补 native 的规划/渲染/归属职责、与固定订阅面的并存、信任/输出限制、编译等价语义、stats 留存与只读边界，以及 Codex B1–B4 双路线实测范围。当前设计 §14.4 写明 M2 部分进展；编译规范的 Codex replace 表从未实现的 model_instructions_file 方案改为当前无槽位，与 core/plan 和两家 Facts 一致；Claude 仍为 CLAUDE.md，D4/D5 未关闭。静态缓存文字改为条件情景，三处未分发外部设计材料改用明确的历史材料名，避免失效链接。
- 验证：固定 T30 二进制 SHA256 c35d85bd70c4bd575e5eaf401662870b45867b419995e2747a5917c57952ab8c，在独立 /private/tmp/style-m2-docs-20261017.jdmdko4y 逐字提取教程 §4.4 命令，用真实 CLI 完成 validate、两家 plan、codex-user/claude-user 安装和卸载；均退出 0，原 settings.json/config.toml 字节与 0640 还原、hooks.json 无残留。首次草稿试跑发现缺少包 description，补齐后全流程通过；没有运行 native 事件或远端模型请求。§10.4 三条 stats 命令退出 0、空记录语义正确；7d 退出 1/stdout 空。结果、命令摘要和四份内容文档摘要见 docs/review/M2-DOCS-02-2026-10-17.json。
- 四份内容文档 116 个本地链接/锚点、围栏成对；任务/测试/追加日志和 git diff --check 另行核对。仅文档与本地证据，不改生产 Go、agenthooks 或 M1 归档，不重复已通过的 Go/模型验收。M2-DOCS 改为部分完成，新增 DOCS-02 独立可验收条目；DOCS-01 保留部分状态，等待 Claude 设计和最终 M2 结果回写。


### 2026-10-17 — M2-D4/D5 具体方案待评审 `[~]`

- 接 clean 2ad3f99。在 Claude 订阅额度/OAuth 保存待确认期间，把已有协议事实整理成 docs/review/M2-D4-D5-DESIGN-2026-10-17.md，并在 CLI 设计 §10.4、编译规范静态位置说明、任务/测试和手册关联。此为可评审方案，不是实现或真实界面通过；D4/D5 仍 `[~]`，T40/T50 未开始，当前依赖未擅自改变。
- D4 建议配置根目录下每包 rules/style-<名字>.md，拟议槽位 claude_rules；首次成功重建 Claude 路线时迁移已批准正文，单纯搬迁无需另加 trust，原有源码/native 批准保持。明确文件归属/生成摘要、外部同名文件不接管、漂移/标记/path 冲突退出 1、旧 CLAUDE.md 外部字节/权限还原、复用现有安装事务与失败回滚；保留加载排序实测和崩溃原子性的实际边界。A7 列出新旧布局、4 种触发路径、文件尾/Unicode、去重接替、多包批准隔离、冲突和回滚候选验收。
- D5 建议不将 output style 作为 replace、不修改用户样式，维持 degrade/skip 跳过及 error 拒绝。草案区分 --allow-replace 授权检查与实际能力；评审后 T50 可按不实现关闭，A8 仍须补齐 skip 和样式/设置不变的真实 CLI 证据，不能直接标成功。C4 界面切换仍阻塞。
- 为继续离线实现，单独提出调整 T40 前提为“本设计评审通过 + 已有原生请求证据”，真实用户界面 C1/C3/C4 留待额度可用后补；该依赖调整明确等待用户决定，不用本次“继续”追认尚未展示的方案。任务清单的规范先行约定要求用户评审通过后进入实现，因此在具体方案可审阅后请求设计确认。
- 验证：从历史原生协议 JSON 独立核对四个 phase 的 exit 0/单请求、rules 全文一次、选中样式互斥且均在 messages、system 长度 26512/23229/23229/26550 和候选公共片段 20615；报告 SHA256 9d23d962b8e3f517ac4015d8778322975a37c7e742832104295a71829d3dcc7e。核对现有 core/plan 的 TestBuildRejectsReplaceWhenStaticLocationIsUnavailable 与 Claude Facts 测试源码、route/ApplyFiles/sync/trust 的事务和批准职责；没有重跑模型请求或把 stub usage 当官方计数。本地文档链接/锚点/围栏、追加日志、git diff --check 通过；无生产代码、agenthooks 或 M1 归档修改，不重复既有 Go 回归。


### 2026-10-17 — M2-D4/D5 设计评审通过；允许先实现 T40/T50

- 用户对 61413eb 展示的三项具体决定回复“同意”：每包 rules 文件、首次重建自动迁移且单纯迁移无需额外 trust；output style 不做 replace、保留现有 fallback；以设计评审和已有四份原生请求证据先实现，Claude 界面验收后补。
- 先回写设计定稿、CLI/编译规范与任务依赖；细化 A7 的新旧布局/迁移入口/字节权限/冲突回滚/批准隔离验收，A8 的 degrade/skip/error、授权和样式设置不变验收。D4/D5 总条目保留 [~] 明确真实界面尚未闭合；C1/C3/C4、M2 最终回归/CI 和官方计数均未因此关闭。
- 验证：设计的用户批准记录与历史协议证据对应，文档本地链接/锚点/围栏与 git diff --check 通过。仅设计/验收文档修改，未改生产代码或 M1 归档。本提交完成后进入 T40，不再重复请求已授予的批准。


### 2026-10-17 — M2-T40 / M2-A7 Claude rules 与旧安装迁移 `[x]`

- 接 3b1295b 用户批准的 D4。Claude append 新槽位为 claude_rules，每个有效静态包写 rules/style-<名字>.md；空有效正文/纯动态/折叠包不生成文件，保留方卸载后接替。确定性版本/包名头尾；新文件 0644、更新保留权限；锁文件以可选 static_layout/static_files 保存布局和整文件摘要，批准快照沿用源字节与编译表示。index format、runtime/repeat 状态键不变，schema 同时接受 claude_md/claude_rules。
- 首次成功 install/sync/trust/uninstall 重建时迁移旧块，报告包名和新旧路径，卸载目标直接移除。只读命令不迁移；doctor 按锁的旧布局重编译，认同有效旧安装并提示 static-migration-pending。只含 rules 的配置也可被 detector 识别。单纯搬迁无需额外 trust；trust 指定包时其他未批准普通/link 包只使用批准基线，源码哈希和 native 命令未连带批准。
- adapter 先检查完整 rules/旧块集合：安装归属、版本/包名头尾、整文件摘要、同名外部占用、缺失/漂移、symlink、旧标记和路径/权限冲突。文件名/相同正文/伪造头尾不构成归属，不新增 force-adopt。失败保留旧载体与其他配置、index、lock、批准记录；旧块外的 LF/CRLF/无尾换行/Unicode 字节与权限还原，STYLE-only 旧文件删除。
- agenthooks 变更原因/边界：nested rules 路径需要在读取和应用前检查根/各现有组件。仅把原 containment 检查扩展并导出 CheckPath，拒绝 symlink/非普通组件，在暂存和最终替换前重查；沿用 ApplyFiles 的 snapshot、单文件 rename 和失败回滚。adapter 仍负责静态归属/渲染，app 仍组合配置/index/lock/批准事务，未移动 codec/engine 或 native owner 职责。不声明进程崩溃时数据库式原子性。
- 可重复 Go 证据：internal/app/claude_rules_test.go 的 TestA7*（新装/去重接替、5 入口 × 4 文件尾共 20 场景、普通/link 批准隔离、冲突、漂移、陈旧快照）；internal/adapter/claude/rules_test.go（格式/摘要/字节/权限、空正文、指定 owner 删除、取消回滚）；agenthooks/install/path_test.go（7 类不安全路径、应用中引入 symlink 后回滚先前变更）。首次测试发现编译器白名单/index schema 未接受新槽位及旧用例仍读取 CLAUDE.md，均修正；陈旧事务用例的权限预期改为读取实际保留权限后通过。
- 实际升级命令：python3 scripts/check_m2_rules_migration.py --old-binary <T30 固定二进制> --binary /tmp/style-m2-rules.wG0JrxQX/style --report docs/review/M2-A7-RULES-MIGRATION-2026-10-17.json。旧 SHA256 c35d85bd70c4bd575e5eaf401662870b45867b419995e2747a5917c57952ab8c；新 SHA256 38573d25583cb105dae11adf49ae55009c14487a501007399c8ec1513bae7184。16/16（4 写入入口 × 4 文件尾）通过，plan/list/adapters/doctor/stats 五个只读命令不迁移、doctor 前后健康、重复 sync changed=false、外部 bytes/mode 与最终卸载一致。脚本只执行 STYLE 与版本 stub，不启动真实 harness、不访问日常配置/凭据。旧版本可用 git archive 74a473d 到临时目录后按下述环境构建；报告保留当时源码 HEAD/未提交实现事实，不改写旧证据摘要。
- 标准环境均 env -u CODEX_HOME -u CODEX_SANDBOX GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache GOCACHE=/tmp/style-cli-gocache：go test -count=1 ./...；go test -C agenthooks -count=1 ./...；go vet ./...；go vet -C agenthooks ./...；go test -race -count=1 ./internal/app ./internal/adapter/claude ./internal/adapter/conformance ./core/compile ./core/engine；go test -C agenthooks -race -count=1 ./install 均通过。最后 detector/空正文补充后相关 Claude/app/conformance 重跑通过。一次 agenthooks race 命令误将 -C 放在 -race 后被 Go 拒绝，改为 -C 首参数后通过；不把误用当成功。python3 -m unittest discover -s testdata/manual -p 'test_*.py' 50 项 OK；-trimpath 最终构建、scripts/smoke.py 40 项通过。
- 教程、adapter 总览、CLI/编译规范同步新槽位、归属、迁移、回退与真实验收边界；golden 仅将 Claude plan 的 static:claude_md 改为 static:claude_rules，未修改既有 runtime stdout golden。任务 T40/A7 关闭；D4/C1/C3 仍保留界面待补，T50/A8、最终回归/CI 与官方计数不因本条关闭。


### 2026-10-17 — M2-T50 / M2-A8 output style replace 回退 `[x]`

- 接 clean 16d5c1c，按 D5 用户批准结论不提供 output style replace、不改生产 adapter 能力。新增 scripts/check_m2_replace.py，实际运行两家 × degrade/skip/error × 文本/JSON 共 12 场景，不能只以源码断言或“不适用”关闭。
- degrade/skip 的 plan/install 为 skipped、无 placement，index 无静态/动态替代条目；error 的 plan blocked/退出 4、install 退出 4。所有场景缺 --allow-replace 都退出 1 并给授权提示；加授权不会创造槽位。失败时完整 STYLE_HOME/两家配置树的 bytes/mode/mtime 不变，STYLE_HOME 未创建。成功时整棵 Claude 用户 output-styles/settings 树在安装中保持；卸载后两家原配置 bytes/mode 精确还原，用户 outputStyle 选择仍为 user-existing。
- 验证命令：python3 scripts/check_m2_replace.py /tmp/style-m2-rules.wG0JrxQX/style --report docs/review/M2-A8-REPLACE-FALLBACK-2026-10-17.json，12/12 passed；固定二进制 SHA256 38573d25583cb105dae11adf49ae55009c14487a501007399c8ec1513bae7184。脚本以最小环境运行并显式将 CODEX_HOME/CLAUDE_CONFIG_DIR/STYLE_HOME 指到临时目录；这与 Go 回归清理宿主变量的约定一致，未保留宿主检测环境。版本 stub 仅答 --version，不启动真实 harness、不读取日常配置/凭据。
- 同一脚本加入 Ubuntu/macOS CI 的构建后二进制检查，但本条没有远端执行结果，不关闭 M2-CI-01。本条无 Go 生产变更，沿用 T40 已通过的双 module test/vet/race、50 采集器与 40 smoke；新增脚本实际运行、文档本地链接/锚点/围栏和 git diff --check 通过。T50/A8 关闭；C4 的真实界面样式切换/冲突观察仍因额度/OAuth 保持 [~]，D5 总条目不改为真实实测通过。


### 2026-10-17 — M2-PREP-03 Claude 最终隔离验收准备 `[x]`

- 接 clean 4976713。T40/T50 完成后补齐手册 §5 与 prepare_claude_m2_acceptance.py，生成私有固定二进制/观察器、两份等价静态包、SessionStart/Bash/Read 三 native、四份外部配置基线。安装后只保留 first/shared 静态，second 不生成文件；STYLE/登录/界面/恢复检查启动器以最小环境保留真实系统身份，独立 CLAUDE_CONFIG_DIR/STYLE_HOME/profile，排除调用者凭据。仅准备，不调用 auth login 或启动真实模型。
- 新增三个测试：观察日志白名单排除 prompt/tool_input/密钥；拒绝非空 scratch 并保留证据；实际执行带空格/引号路径的启动器，确认 HOME/USER 与隔离配置、凭据排除、shell 语法与权限；运行复制的 observer 验证原生 JSON 形状，再核对恢复检查拒绝管理文件残留与权限漂移。PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py'，53 项 OK。
- 实际 CLI 干跑：固定 -trimpath -buildvcs=false 构建 /tmp/style-m2-final.Vi9x9bcm/style，SHA256 32aef36dcb67d8cd0c57e123016094e1f82e5cce8adca37a9b6ae56c990cf81d；标准临时 Go 三缓存，清理 CODEX_HOME/CODEX_SANDBOX。准备两个包、1 个静态入口/折叠文件不存在、list native_count 为 3/0、doctor healthy；生产 uninstall 后 verify-restored 校验四份外部 bytes/mode 还原、管理 rules/lock/index 删除。首次干跑汇总误读 list 的 native_hooks 键，改为现有 native_count 后全部通过，未把字段错误当成功。
- Claude 只执行固定路径的 --version，仍为 2.1.301；不查询额度、不重试 OAuth、不改日常配置/凭据。另生成未启动会话的 /private/tmp/style-m2-claude-acceptance-20261017.d30zgr28，prepared.json acceptance=false；版本/二进制/观察器源摘要、干跑结果与 53 项测试记录在 docs/review/M2-PREP-03-CLAUDE-2026-10-17.json。没有手工注入 observer 的结果被用于 C2/C3 真实验收。
- 手册明确额度恢复后登录保存确认、C2 真实 native 事件/matcher、C3 InstructionsLoaded 或请求完整正文与顺序证据、卸载恢复和新会话不再触发的步骤。只关闭 M2-PREP/PREP-03，C1/C4 仍 [~]、C2/C3 仍 [?]；原设计探针现场不覆盖，最终 CI 与官方计数继续等待。


### 2026-10-17 — M2-A9 / M2-DOCS-03 最终本地回归与状态同步 `[x]`

- 起点 clean 5dd6742，生产实现仍为 16d5c1c、replace 验收 4976713。最终标准环境（清理 CODEX_HOME/CODEX_SANDBOX；临时 GOPATH/GOMODCACHE/GOCACHE 同前）下，go test -count=1 ./...、go test -C agenthooks -count=1 ./...、make vet-go 全部退出 0；go test -race -count=1 ./core/compile ./core/engine ./core/state ./core/tokens ./internal/app ./internal/adapter/... 与 go test -C agenthooks -race -count=1 ./install 均通过，覆盖现有 M1 回归及三路线 conformance。tracked Go 文件 gofmt -l 无输出。
- PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py'：53 项 OK。go build -trimpath -buildvcs=false 重建 /tmp/style-m2-final.Vi9x9bcm/style-rebuilt，与已准备 UI 的 style 二进制 cmp 相同，SHA256 均为 32aef36dcb67d8cd0c57e123016094e1f82e5cce8adca37a9b6ae56c990cf81d。scripts/smoke.py 40 项、scripts/check_m2_replace.py 12 场景、scripts/check_m2_rules_migration.py 16 组实际旧安装升级全部通过。最终运行没有远端模型/登录/计数调用。
- 最终事实和命令列表：docs/review/M2-A9-LOCAL-REGRESSION-2026-10-17.json。M1 归档相对 6ec25f6 未变；runtime stdout golden 未改。M2 整体新增/变更的 golden 限于 native plan、stats、组合 dedup 报告；T40 的 Claude plan 只变静态槽位名，不重写历史原生证据或报告摘要。
- 更新教程/adapter 基线与规则规范的 Claude 静态载体/compact 验收边界，补采集器 README 的 M2 入口；修正任务/测试摘要，关闭 M2a 和 A9。M2 本轮相对 6ec25f6 修改的 14 份当前 Markdown 共 173 个本地链接/锚点及代码围栏通过；JSON 可解析、固定二进制/准备摘要一致、git diff --check 通过。DOCS-03 只关闭当前实现与离线状态同步，DOCS-01 总项仍等最后真实结果。
- M2 仍未完成：Claude C1/C4 因订阅额度/OAuth 保存待确认保持 [~]，C2/C3 等用户真实界面；当前提交未推送，Ubuntu/macOS 最终 GitHub Actions 未执行；M2-CARRY-01 官方计数仍缺 API key。M2-FINAL 改部分完成（本地通过、远端 CI 待补）。M2-RV-01 的旧 core/spec harness 名硬编码风险保持既有待决定状态，未混入本次实现。待这些既有验收条件满足后再关闭 M2 并提示 M3 规划，当前不宣称里程碑完成。


### 2026-10-17 — M2-C1 Claude 用户级 rules 真实交互验收 `[x]`

- 接 clean eb6ff86。用户报告 Claude 恢复，并在操作方式问题中明确选择“你通过交互终端代跑”；更新当前手册的操作约定。固定 Claude 2.1.301 的路径/二进制摘要与准备时一致，既有设计 scratch 的 auth status 为 loggedIn=true、claude.ai/firstParty；新安装 scratch 尚未登录，未复制或读取凭据。
- 实际以 launch-default.sh --ax-screen-reader --model sonnet --tools '' --session-id 5f94ea4b-8e82-44c6-94fc-a5e15ea8a50b 启动原生交互终端。界面显示 Sonnet 5.5 / Claude Pro，短请求回复 OK，无工具调用、OAuth 或额度错误；/config 显示 Output style=default，退出 0。既有临时工作目录已经信任，本次未出现首次工作区信任提示，不扩大该范围。
- InstructionsLoaded 对同一 session 的用户级 rules 文件记录 memory_type=User、load_reason=session_start，一次；同时保留用户 CLAUDE.md 加载事件。以原生事件证明规则发现，不以模型回复证明投递。白名单事件、固定 fixture/原生 transcript 摘要、启动命令和界面观察归档 docs/review/M2-C1-CLAUDE-2026-10-17.json；原始日志/登录数据留在私有 scratch。
- 验证：从实际会话 transcript 核对 OK 与零工具调用，精确核对规则事件的五个字段/次数及二进制摘要；报告 JSON、文档本地链接/围栏与 git diff --check 通过。没有生产代码变更，沿用已通过的最终本地回归。C1 关闭；D4 仍等 C3，C4、C2/C3、最终 CI 与官方计数继续。


### 2026-10-17 — M2-C4 / M2-D5 output style 真实交互验收 `[x]`

- 接 clean 8a9d219。继 C1 的 default 会话，依次用 launch-existing.sh、launch-replace.sh、launch-keep.sh 启动三个全新 first-party 已认证交互终端，不 resume；固定 Claude 2.1.301 与 Sonnet 5.5。每次 /config 分别显示 M2Existing、M2Replace、M2Keep，真实短请求均回复 OK、零工具、退出 0；未出现样式冲突或额外样式确认。
- 三份原始终端记录和 native transcript 保存在 /private/tmp/style-m2-claude-live-20261017.5fzqcx0b 与原隔离配置中；逐一核对实际 style 行、OK、session ID/正常退出。六份用户 settings/CLAUDE.md/rules/三样式的 bytes/mode 与启动前私有 baseline 精确相同；磁盘 outputStyle 始终 M2Existing。没有复制/读取凭据或改日常配置。
- C4 结局为“回退方案生效”。界面仅证明样式切换/冲突行为；整份 system replace 仍依据已有实际 CLI 完整请求对照：候选仍保留 20,615 字符公共 system 片段。结合 A8 的真实 CLI fallback 验证，D5 关闭，保持已批准的不提供 replace 槽位方案。报告 docs/review/M2-C4-CLAUDE-2026-10-17.json 不归档原始 prompt/登录/连接数据。
- 验证：三会话终端与 transcript、六文件 bytes/mode、固定二进制/历史协议报告摘要、JSON 可解析、文档本地链接/围栏、git diff --check 通过；无 Go 生产变更，未重复已通过的离线回归。C2/C3、D4、最终 CI 与官方计数继续。


### 2026-10-17 — M2-C2 Claude native 真实交互/卸载验收 `[x]`

- 接 clean 7b16584。复用已认证的设计隔离配置，不读/复制凭据；在 /private/tmp/style-m2-claude-live-20261017.5fzqcx0b 创建独立 STYLE_HOME/工作目录，复制 PREP-03 固定 SHA256 32aef36d…cf81d 的 STYLE 与观察器导入闭包，安装两份等价静态包/三 native。保存既有六份文件的私有 bytes/mode 基线；list 为 3/0 native、doctor healthy。初次终端记录实际显示该新工作目录的信任确认，agent 在用户明确授权下接受，没有 bypassPermissions。
- 安装态 session b9e583ed-805b-4a34-bcb2-6a31b932ff92：实际一次 Bash printf，工具成功输出固定文本、最终 DONE。native 观察 start/SessionStart 与 shell/PreToolUse 各一次，tool_name=Bash，Read 为零；原生 hook_success 各退出 0，hook_additional_context 各含对应完整 fixture 正文一次。无手工 observer 注入替代真实触发。
- style.sh uninstall --all --yes --json 退出 0；六份用户配置/规则/样式 bytes/mode 精确还原，管理 rules/lock/index 删除。全新 session 2e4e01f7-0a27-4b5f-a9e7-5c347937156b 再执行一次 Bash 并 DONE，native 日志仍 2 行且摘要不变、该 session 的 hook 完成/上下文附件为零。白名单证据、启动器/固定程序摘要和恢复事实见 docs/review/M2-C2-CLAUDE-2026-10-17.json。
- 附加请求重放：既有 replay_requests 对本次 native API blob 报 native message content hash mismatch，因此不作为通过证据；本项按矩阵以原生 hook 完成/上下文、事件及卸载恢复验收。一次私有 post 检查把 scratch 根目录中的 style-m2-claude- 也当成管理文件名，改为检查 basename 后与真实两条用户加载事件相符；未改变生产代码或模型结果。
- 验证：重新从两个原生 transcript/终端/观察日志提取并严格断言实际事件、工具、上下文和恢复；53 项采集器单测通过，JSON、文档链接/围栏和 git diff --check 通过。无 Go 生产变更；C3 的静态去重/加载与顺序证据单独归档，D4、最终 CI/官方计数尚未关闭。


### 2026-10-17 — M2-C3 / M2-D4 Claude 静态规则真实交互验收 `[x]`

- 接 clean c09b117，复用 C2 已完成的真实安装/卸载两个会话；未为文档重复发送模型请求。安装时 index 保留 m2-claude-first/second 两包，组合报告将 second/shared 折叠到 first/shared；仅保留方 rules 文件存在，完整固定静态正文一次。源包/固定 STYLE/静态文件摘要已在删除前采集。
- 安装态 session b9e583ed-805b-4a34-bcb2-6a31b932ff92 的 InstructionsLoaded 对生产管理规则记录 User/session_start 一次；原生 instructions 附件中完整静态正文一次，实际文件顺序为用户 CLAUDE.md、用户 style-m2-user-probe.md、STYLE style-m2-claude-first.md。按附件记录顺序，不从异步日志完成时间或文件名推断全局优先级。
- 卸载后两个管理路径、lock/index 均删除，CLAUDE.md 等六份用户文件逐字节/权限还原；全新 session 2e4e01f7-0a27-4b5f-a9e7-5c347937156b 只加载原用户 memory/rules，实际 Bash 工具和 DONE 证明新会话正常完成。报告 docs/review/M2-C3-CLAUDE-2026-10-17.json 仅白名单/摘要，无原始 prompt 或鉴权。附加 API 重放的既有哈希校验失败继续明确为未采用，不绕过校验。
- C3/D4 结局“符合预期”；与 C1/C2/C4 合并，M2b Claude 真实环境改 [x]，D4/D5 实测闭合。没有把 M1 的 CLAUDE.md compact 事实外推到新 rules。M2 全里程碑仍等远端 Ubuntu/macOS CI、最终文档状态和官方 token 计数遗留说明；既有 RV-01 风险状态不改。
- 验证：从已固定的安装态事实及卸载态 transcript/事件重新交叉断言两包、一文件、全文/加载次数、真实顺序和恢复；C2 同轮 53 项采集器通过，所有报告 JSON/摘要、当前文档本地链接/围栏、M1 只读归档与 git diff --check 通过。无生产代码变更。


### 2026-10-17 — M2-DOCS-04 Claude 真实验收与当前文档同步 `[x]`

- 接 clean 02d57c2。教程、adapter 总览、CLI/编译/规则规范、当前手册、采集器 README 和任务/测试摘要同步 C1–C4 与 D4/D5 真实结果，移除已过期的当前额度/OAuth 阻塞说明。设计评审第 1–4 节及历史协议/准备 JSON 保留原事实，另追加真实验收补记；PROGRESS 只追加，M1 档案不改。
- 核对四份 Claude 验收报告引用的 42 处实际私有文件摘要：41 处与当前文件精确相同，C1 InstructionsLoaded 全日志摘要匹配后续追加前的完整行前缀。再次确认固定 STYLE/Claude 摘要、六份用户文件还原、六个独立真实交互会话均已正常退出；C1/C4 设计目录与 C2/C3 新工作目录的信任范围分别记录。
- 未采用的附加请求重放明确保留 native message content hash mismatch，不把模型回复或绕过哈希的重放用于投递证明。C2/C3 按矩阵采用原生 hook 完成/完整上下文、加载事件/附件和卸载恢复；新 rules compact 未声称。官方计数仍缺独立 API key，远端 Ubuntu/macOS CI 未执行，RV-01 仍为既有风险；M2 不提前关闭或转入 M3。
- 验证：本轮 53 项采集器已通过；四报告 JSON/证据摘要和所有 M2 变更当前 Markdown 的本地链接/锚点/围栏、M1 归档未变、PROGRESS 追加前缀、git diff --check 均核对。仅文档/真实证据归档，无生产代码、agenthooks 或依赖变更，不重复已通过的 Go 回归和模型请求。

### 2026-10-17 — M2-CI-01 首轮远端失败与测试环境修正 `[~]`

- 用户要求进行远端 CI。起点 clean 3102f69，发布到专用分支 codex/m2-remote-ci-20261017，workflow_dispatch 触发现有 ci.yml；没有创建 PR。run 38999049830 的 headSha 精确匹配，Ubuntu/macOS 均在 TestA2NativePlanReports 失败：plan --json exit=1 stderr=style plan: no harness selected。format/vet 已通过，后续采集器/build/smoke/replace 被跳过，不能计为通过。
- 原因是 A2 的 Service.Plan 已显式选择两家，而 JSON CLI 调用漏了 --harness，默认 auto 依赖本机已安装的 harness/用户配置。隔离编译旧 app.test，再以 env -i、空 HOME/PATH 执行同一测试，稳定退出 1 并复现同一报错；不是生产计划能力失效。
- 原测试补空 HOME/PATH、清理 CODEX_HOME/CODEX_SANDBOX/CLAUDE_CONFIG_DIR，并对 JSON 命令显式 --harness=codex,claude-code。相同隔离条件下新 app.test 退出 0；标准临时 Go 三缓存、清理宿主 Codex 变量，go test -count=1 ./internal/app（16.561s）及 go vet ./internal/app 均通过。生产代码、agenthooks 和 workflow 未改。
- 原始远端日志、元数据及本地前后对照留在 /tmp/style-m2-ci.tD4GCHgQ；白名单运行/逐步骤元数据、日志摘要、原因与验证归档 docs/review/M2-CI-01-2026-10-17.json。M2-CI-01 保持 [~]，提交修正后重跑；不覆盖失败事实，不提前关闭最终回归或官方计数。

### 2026-10-17 — M2-CI-01 / M2-FINAL 远端 Ubuntu/macOS 通过 `[x]`

- 修正提交 2aded84d5e5b8dd4cc5ce5ead19343260dc55c3f 快进发布至专用分支 codex/m2-remote-ci-20261017，gh workflow run ci.yml --repo inxups/style-cli --ref codex/m2-remote-ci-20261017 触发 run 38999430973。gh run view 的 headSha 精确相同，整个 run 与两个 job 均 completed/success。
- 两台 runner 的 Check formatting、Vet、Test、Test acceptance collectors、Build、Smoke test built binary、Check replace fallback and user style preservation 全部 success。逐 job 日志分别核对 root/agenthooks 的测试输出、Ran 53 tests/OK、40 条 smoke ok/smoke passed、M2-A8: 12/12 replace fallback cases passed；没有把跳过或旧 run 结果当成功。
- 实际镜像 ubuntu-24.04 / macos-26-arm64，工具链 go1.25.0 linux/amd64 / darwin/arm64（按 go.mod 设置）；Go 三缓存为 /tmp/style-cli-gopath、/tmp/style-cli-gomodcache、/tmp/style-cli-gocache。vet/test/build 命令均 env -u CODEX_HOME -u CODEX_SANDBOX，GOFLAGS=-mod=readonly。未改 CI 条件或减少检查。
- docs/review/M2-CI-01-2026-10-17.json 保留首轮 38999049830 的失败、同环境本地前后对照，追加成功 run 的完整逐步骤元数据、两个 job 的日志断言、源码树对象和 SHA256。原日志留 /tmp/style-m2-ci.tD4GCHgQ；可用 gh run view 38999430973 --repo inxups/style-cli --json headSha,status,conclusion,jobs,url 和 --log 重查。
- 关闭 M2-CI-01/M2-FINAL，当前教程、adapter 总览、设计、手册及任务/测试状态同步。官方计数仍因缺两家独立 API key 保持 M2-CARRY-01 [?]、0/40；M2-RV-01 既有风险未改，总文档与里程碑收尾仍单列。本次结果记录只改文档/证据，生产实现、测试和 workflow 与已通过的 2aded84 相同；提交发布后继续核对文档 HEAD 的远端 CI。
- 提交前核对两轮 JSON/五份实际证据摘要及修正测试摘要、已测源码树对象、PROGRESS 追加前缀、M1 归档未变；14 份 M2 当前 Markdown 的 193 个本地链接/锚点和围栏、git diff --check 全通过。没有以文档提交触发额外模型请求或官方计数。

### 2026-10-17 — M2-ALT-01 无 API key 的离线/订阅补充验证 `[x]`

- 接 clean 81b1a4f。用户无两家官方 API key，批准“本地分词对照＋沿用订阅实测、继续 M2 收尾”的方案；官方计数独立保留，不标通过。新增两个手工采集器，仅依赖临时 venv 的可选 tiktoken，不改 Go 生产代码、agenthooks、工作流或产品依赖。
- 临时目录 /tmp/style-m2-token-alt.UFcIgt7y，以临时 Go 三缓存并清理 CODEX_HOME/CODEX_SANDBOX 重新构建，二进制 SHA256 32aef36dcb67d8cd0c57e123016094e1f82e5cce8adca37a9b6ae56c990cf81d 与最终生产构建相同。系统 Python3.14.7 的裸 urllib 访问 PyPI 因本地证书链失败；标准 pip 在临时 venv 保持 TLS 校验安装 tiktoken0.12.0 成功，没有关闭 TLS 检查。
- 固定 40 条正文/哈希全部复核，生产 CLI 的逐条估算合计 1,205。o200k_base 的纯正文/固定前缀增量均合计 816，逐条 MAPE49.8051%；cl100k_base 均合计972，MAPE36.5474%。语言子组、每条数据及两张完整 token-ID/bytes 映射摘要归档；这些偏差仅相对具名本地编码，不能推断 Claude 或具体 OpenAI 模型准确性、账单或误差上界。生产系数不改。
- 编码表首次下载后，在全新 Python 进程将 socket.socket.connect 替换为抛错，暖缓存真实执行完整离线采集；与首次结果逐字节相同，SHA256 c25ce49e7ca6e07765ca4ec7c5cddd90f515f9d76644a25eb30bc3b788aeef50。语料、旧官方校准器和原始真实验收报告未改。
- 从 C1/C2/C4 报告引用的六个原生 transcript 重新核对精确 SHA256，白名单提取八个真实 usage 响应；每个响应只留最后快照，缓存创建/读取不重复相加。只读私有会话，不读凭据、不重新登录、不新增模型请求。会话工具/指令/缓存状态不同，不用安装前后差分推断逐规则成本。
- 59 项采集器通过（新增六项覆盖平均误差与总量区别、丢语料/正文漂移拒绝、非正参考拒绝、响应去重/缓存/正文白名单、原始日志摘要漂移拒绝）。结果见 docs/review/M2-ALT-01-2026-10-17.md 及两份 JSON；文档/证据核对和 diff-check 通过后提交。M2-CARRY-01 仍 [?]、0/40/家；总文档收尾单独完成。

### 2026-10-17 — M2-DOCS-01 / M2 里程碑关闭 `[x]`

- 接 clean 154506f。M2-ALT-01 已独立验收并提交；用户批准的无 key 补充验证完成，官方计数保持两家各0/40的非阻塞遗留，不使用本地编码/订阅 usage 冒充官方结果。M2-RV-01 核心硬编码仍 [!]，本轮不扩大到生产重构；下一步规划由用户决定范围和优先级，没有标记解决。
- 154506fc072bc4066081e6a0b94d20ce604fc479 快进发布到既有专用分支 codex/m2-remote-ci-20261017，run 39000483969 completed/success，Ubuntu/macOS 所有必需步骤 success。逐 job 日志核对59项采集器、40条smoke、12/12回退及双module test/vet/build，未新增 tiktoken CI依赖。完整元数据/日志摘要和逐步骤断言进入 docs/review/M2-DOCS-01-2026-10-17.json；原始日志留 /tmp/style-m2-token-alt.UFcIgt7y。
- 同步任务/测试、教程、adapter总览、真实环境手册和CLI设计的最终状态，修正设计首页仍称M1补验收中及U-C4表仍未测的旧摘要。当前M2五项范围、离线/两家真实环境、最终CI和文档总验收均完成；计数与RV-01在关闭说明中显式交接，M1归档/历史JSON保持原事实，PROGRESS只追加。
- 总验收核对M2当前Markdown的本地链接/锚点/围栏、全部所引JSON/私有来源摘要、生产二进制/语料/编码表、已测源码树对象、PROGRESS追加前缀与M1归档；git diff --check通过。本次只修改文档/证据，生产实现、测试和workflow与已通过CI的154506f一致。
- M2完成并关闭，提示用户进行下一步M3规划；不自动开始新里程碑、不新增功能重构、不推送main。

### 2026-10-17 — M3-00 建立 M3 任务与测试文档 `[x]`

- 接 clean `b7af4e75d0967ed5f7e8728f86f80bc7a930114c`。用户已确认完整覆盖 M3 backlog、分阶段推进，RV-01 列为功能开发前置必修；本轮仅建立任务与验收文档，具体接口语义继续逐项设计定稿。
- 新建 `docs/STYLE-M3-TASKS.md`（44 项任务，含 D0–D11、T00–T80、遗留/风险、准备、真实验收和收尾）与 `docs/STYLE-M3-TESTS.md`（39 项测试，含 BASE、A0–A18、P1/P2、B1–B4、C1–C6、TOKEN、W、CI、FINAL、DOCS）。覆盖主设计全部 M3 范围，并补 native async/onFailure/非 command 的设计评估；M3 手册留待 PREP-01 建立，不新增未就绪链接。`AGENTS.md` 切换 M3，主设计首页与 §14.4 新增 M3 入口，保留 M2 最终事实。
- M3-CARRY-01 承接两家官方计数各 0/40 的非阻塞待验，既有本地编码/订阅 usage 不替代官方结果；`--count=exact` 产品验收另列，不继承旧校准的自动豁免。M3-RV-01 仍 `[!]`，由 M3-D0/M3-T00 修复并以 M3-A0 验收，尚未执行。其他缺环境的必需真实验收保持 `[?]`，不因准备或 stub 通过而关闭。
- 验证命令：`python3 /tmp/style-m3-doc-check-20261017.py --base b7af4e75d0967ed5f7e8728f86f80bc7a930114c --closed`。临时检查器检查五份当前文档的 50 个本地链接/锚点及代码围栏、三段 POSIX 命令的 `zsh -n`、44 项任务/39 项测试的编号唯一性与双向映射、任务目标/依赖/产出/测试字段、依赖无环、所有后续功能的架构前置关系、完整范围与状态；均通过。迁移脚本要求 `--old-binary`/`--binary`，文档已固定 `74a473d` 旧布局与当前构建两个二进制，核对旧 commit 存在及实际脚本参数，未执行迁移或 Go 构建。
- `git diff --check b7af4e75d0967ed5f7e8728f86f80bc7a930114c` 通过；相对起点的变更仅五份文档。`git diff --name-only b7af4e75d0967ed5f7e8728f86f80bc7a930114c -- 'docs/STYLE-M1*' 'docs/STYLE-M2*' 'docs/review/M1*' 'docs/review/M2*'` 无输出，77 份历史文档/证据未变。`PROGRESS.md` 原始字节前缀保持一致，起点前缀 SHA256 为 `7c2a69d2439f327dc332ee85cc6d8b9e7a7f9504643af4c631d31bbe153fd8ff`；无生产源码、agenthooks、依赖或 workflow 变更。
- 仅关闭 M3-00，未运行 Go 测试、远端 CI、登录、模型请求或官方计数；M3-01 基线仍 `[ ]`。下一项为 M3-01，然后 M3-D0/M3-T00；本轮不继续实现功能。

### 2026-10-17 — M3-01 / M3-BASE-01 隔离环境基线回归 `[x]`

- 接 clean `7586cca5744c697c1cc84a7cc0409e20cc39af40`。macOS 27.0.1 / arm64、Go 1.27.0、Python 3.14.7；清理宿主 `CODEX_HOME`、`CODEX_SANDBOX`、`CLAUDE_CONFIG_DIR`、`STYLE_HOME` 及外部 `GOFLAGS`/`GOWORK`，Go 使用 `/tmp/style-cli-{gopath,gomodcache,gocache}`。`GOFLAGS='-mod=readonly -count=1' make test-go`（30 个有测试的 package，无 cached）、`GOFLAGS=-mod=readonly make vet-go` 均通过，双 module 未修改。
- `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py'` 首轮 59 项中 58 过、1 错：本地 HTTP fixture 在 `127.0.0.1:0` 的 `socket.bind` 被沙箱以 EPERM 拒绝。仅对采集器申请沙箱放行，原测试未改，第二轮 59/59 通过；Go 已通过项未重跑。失败/成功日志与退出码均保留，没有以跳过或 stub 通过代替未测条目。
- 标准构建 `GOFLAGS=-mod=readonly GOWORK=off go build -trimpath -buildvcs=false -o /tmp/style-m3-baseline-20261017.lbp47_dq/build/style ./cmd/style`（沿用上述清理环境和三缓存）SHA256 为 `66ed7d4c21eb1e03a6b4b1c93269535ed96be58cc694d90c57dd5843582103c4`。依次新执行 `scripts/smoke.py BINARY` 40/40、`scripts/check_m2_replace.py BINARY` 12/12；`git archive 74a473d` 导出临时旧源码并同环境构建，再以 `scripts/check_m2_rules_migration.py --old-binary OLD --binary BINARY` 完成 16/16，旧二进制 SHA256 `4b2a46049758faef4c0a7e7e3045b877fe0503d0479b349fc5bf929adff8d958`。临时 HOME/配置与版本 stub 隔离，无真实 harness/模型请求。
- 数量与 M2 最终对照一致。新标准构建为 `GOWORK=off`，agenthooks 记录为 `v0.0.0` 加本地 replace；额外定向 workspace 构建记录为 `(devel)`，与既有 M2 二进制逐字节相同、SHA256 `32aef36dcb67d8cd0c57e123016094e1f82e5cce8adca37a9b6ae56c990cf81d`，避免将构建模式差异误作源码回归。相对 M2 最终 CI 源码 `154506f`，cmd/core/internal/agenthooks、Go 依赖文件、Makefile、脚本与采集器对象未变。
- 归档 `docs/review/M3-BASE-01-2026-10-17.json`：固定起点 HEAD/源码树、环境、命令/退出码/耗时、逐 package 输出、40 项 smoke、16/12 组 fixture 事实、两次采集器与构建元数据、原始日志 SHA256。私有完整记录留 `/tmp/style-m3-baseline-20261017.lbp47_dq`；旧 M2 脚本报告中的 T40 未提交/C4 待验是历史硬编码标签，报告已注明不作为本轮状态。
- 验证命令 `python3 /tmp/style-m3-01-doc-check-20261017.py --base 7586cca5744c697c1cc84a7cc0409e20cc39af40 --closed` 通过：源码/二进制/日志摘要、52 个当前 Markdown 本地链接/锚点及围栏、三段命令语法、44 项任务/39 项测试的编号与依赖、`PROGRESS.md` 追加前缀（SHA256 `f620d519baa131e533a5cb064a1ada712f2454fd1e4b8a301077b116927b51e0`）、77 份 M1/M2 归档未变及 `git diff --check`。仅关闭 M3-01/M3-BASE-01；M3-RV-01 仍 `[!]`，官方计数各 0/40、真实验收/远端 CI 状态未变。下一项 M3-D0，设计经用户评审后才进入 M3-T00。

### 2026-10-17 — M3-D0 注册表校验边界评审稿 `[~]`

- 接 clean `323acf81ad63b417ea27b29064f9473efbb7a12b`，M3-01 已独立提交且基线通过。逐项检查 core/spec 的三处名称 allowlist、Registry/Service、validate/plan/install/sync/trust/卸载保留包与去重预算调用链；当前无 M3 生产实现，不沿用基线结果关闭新验收。
- 新建 `docs/review/M3-D0-REGISTRY-VALIDATION-2026-10-17.md`，并回写 CLI §1/§3.1、包顶层字段和条件规范。拟由同一 Registry 产生 `ValidationCatalog`（名称全集/native 子集），Service 用 `ValidateWithCatalog` 完整校验；零目录不回落内置值，普通注册但无 native 与未知名分别处理，目录包/单文件包共用门禁。
- 兼容选择明确留待用户批准：`spec.Validate(pkg)` 保留 Go 签名，职责改为结构/lint 校验；完整有效性调用迁移到带目录入口。三处未知名 code/message、默认两家 CLI 输出/退出码/诊断排序保持兼容。预算路径传递完整源诊断，validate 预算与 plan auto 遍历注册项；原生指纹/批准基线继续保留解析比较用途。agenthooks 的协议、配置渲染、owner/卸载职责不变。
- 全 core 名称搜索额外发现 engine/process.go 两处 Claude prompt suppression 判断，与名称成员关系校验不同。本稿保留既有原生输出安全约束，显式交由 M3-D8 评估能力边界；T00 不宣称所有 core 特例清零。另确认依赖 guard 尚未覆盖 adapter 子路径，纳入 A0 的架构回归。
- M3-A0 细化 A0-01–A0-10（四种目录、普通/native 差异、坏值与诊断兼容、API 边界、快照/无探测、全部入口/auto、预算回退、失败恢复、依赖边界及兼容回归），全部待执行。D0 保持 `[~]` 待用户评审，T00/A0 仍 `[ ]`、RV-01 仍 `[!]`；不新增真实模型请求、远端 CI 或官方计数。
- 文档检查 `python3 /tmp/style-m3-d0-doc-check-20261017.py --base 323acf81ad63b417ea27b29064f9473efbb7a12b --closed` 通过：8 份当前 Markdown 的 81 个本地链接/锚点、围栏与四段 shell 语法，44 项任务/39 项测试映射与依赖，15 份源码的 24 条调查事实、十组待执行断言与评审门禁状态。`PROGRESS.md` 原字节前缀 SHA256 `b1e13a652462af8fd1f4c22c22e662c7e3e62ead163735aa56e82a3c41d82235` 保持一致，77 份 M1/M2 归档未变、生产/测试/agenthooks/依赖未变，`git diff --check` 通过。设计检查只证明文档和当前源码调查一致，不标记 M3-A0 通过。

### 2026-10-17 — M3-D0 注册表校验边界定稿 `[x]`

- 接 clean `2768f385245fe5aa6ed1eea55a328ec3ed2a9d97`。用户在上一轮明确询问是否按评审稿推进 T00 后回复“继续”，确认完整目录接口、`Validate` 保持签名但仅做结构/lint 的兼容选择及 T00 范围；按该确认关闭 D0，无需重复请求批准。
- 详细设计、CLI §3.1、包/条件规范与任务状态同步为已定稿、待实现；A0 十组断言、T00 和 RV-01 仍未通过。agenthooks 边界、engine 既有输出判断及各真实验收保持原约束。
- 文档检查 `python3 /tmp/style-m3-d0-approved-check-20261017.py --base 2768f385245fe5aa6ed1eea55a328ec3ed2a9d97 --closed` 核对本地链接/锚点、围栏、编号/依赖、批准与待实现状态、追加日志和 77 份 M1/M2 归档；源码与依赖未变，`git diff --check` 通过。未以本次定稿冒充 A0 回归；下一项 M3-T00。

### 2026-10-17 — M3-T00 / M3-A0 统一注册表校验 `[x]`

- 接用户已确认设计的 clean `7a577d5ff1ab566f8adaf7659e61d688695fa7a3`。实现 `spec.ValidationCatalog`、`ValidateWithCatalog` 和 `Registry.ValidationCatalog()`：名称全集与 native 子集从同一注册表投影，Service 保存独立容器，零目录无内置回退。三处普通名称不再写死两家；注册但无 native 与未知名分别诊断，额外 Native 键不能扩充合法名称。`Validate` 保留 Go 签名，仅做结构/lint；CLI/app 入包统一使用完整入口，未新增真实 harness。
- validate 的预算与 plan auto 遍历注册项；plan、安装编译、sync/trust/保留包重编译和去重报告沿用完整源诊断。低层 native 校验入口仍保留，解析/指纹基线不扩大为写操作。补齐整个 adapter 子路径的 core import guard；agenthooks 的 codec/配置渲染/owner/卸载、锁/index/state 格式和 engine 输出安全判断未改。
- 新增 14 个 M3A0 测试函数，四包 race 中 205 个叶子用例全部通过（211 条含分组 pass，完整四包共 790 条 pass，无 fail/skip）；包含四种目录/三种包格式/六种名称拼写/字符串与列表的 144 组组合、三处字段跨 11 个入口的 33 组 bytes/mode/mtime 保全、并发快照、注册/检测/选择分离、普通/native 差异、真实去重前后 lint 和空源诊断回退。native 校验的 marker 命令未执行，detector 未调用。
- 新执行 `make test-go GOFLAGS='-mod=readonly -count=1'`（双 module、30 个有测试 package，均非 cached）、`make vet-go GOFLAGS=-mod=readonly`、`go test -json -race -count=1 ./core/spec ./core/tokens ./internal/adapter ./internal/app`，全部通过；临时三缓存并清理 CODEX_HOME/CODEX_SANDBOX/CLAUDE_CONFIG_DIR/STYLE_HOME/GOFLAGS/GOWORK。59 项 Python 采集器按已知回环端口要求放行执行并通过，未删测或关闭断言。
- 保留两次前期失败：首轮新去重测试漏设报告 route，触发 route.Name 空指针，补齐测试 double 后定向通过；首轮全量检查中旧 Codex-only 注册 fixture 却安装了声明 Claude 要求的示例，新门禁正确拒绝。fixture 改为两家均注册、Claude 明确未检测到，生产校验未放宽；随后 fresh test/race 全过。原命令、退出码与失败/成功日志摘要均在报告，不掩盖失败。
- `GOWORK=off go build -trimpath -buildvcs=false` 当前二进制 SHA256 `8d4e933ccdc6d79167294b0998e0bfe76668cfbf0a268a64d25dc3f3b5ad6967`；独立导出 `74a473d2e238af44c346da3807f46e1c9993176d` 重新构建真旧布局二进制 SHA256 `4b2a46049758faef4c0a7e7e3045b877fe0503d0479b349fc5bf929adff8d958`。新执行 40/40 smoke、16/16 真旧迁移、12/12 replace 回退；与 M3 基线 CLI 的 32 组文本/JSON stdout/stderr/退出码逐字节相同。8 个隔离复制 import guard fixture 正确拒绝 5 个整个 adapter/agenthooks 根或子路径，允许 3 个不匹配前缀，未改仓库来制造失败。
- `docs/review/M3-A0-2026-10-17.json` 固定源码起点加 22 份 Go 改动、源码范围指纹 `b4513fa557eb30a63758437aff58dc4149163b5c2c61a2b963ab6f7f89b31d8d`、14 个函数/十组验收映射、环境、命令/退出码/日志摘要、兼容案例和构建摘要；完整私有产物留 `/tmp/style-m3-a0-20261017.0ttt7bk2`。旧 M2 兼容脚本的 T40 未提交/C4 待验常量已注明为历史标签，不能当作当前源码或真实验收状态。
- 文档/证据验证命令 `python3 /tmp/style-m3-t00-check-20261017.py --base 7a577d5ff1ab566f8adaf7659e61d688695fa7a3 --closed` 核对链接/锚点/围栏/命令语法、44 任务/39 测试映射与依赖、全部摘要/日志/新旧案例及当前源码一致性；PROGRESS 原始前缀 SHA256 `4b976e73deb835ee25fec4030f291c0e8bb6fc889b060c4cfbf9467902f2ea1f` 不变，77 份 M1/M2 归档、agenthooks、依赖、脚本/旧 fixture/golden 未改，`git diff --check` 通过。关闭 T00/A0/RV-01；下一项 D1 设计，官方校准仍各 0/40、全部真实验收与远端 CI 保持待验，未新增模型调用。
