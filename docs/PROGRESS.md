# STYLE CLI 工作进度

更新时间：2026-10-13

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
