# STYLE CLI 工作进度

更新时间：2026-10-15

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
