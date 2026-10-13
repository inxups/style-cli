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
