# STYLE CLI M1 测试清单

更新时间：2026-10-13

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
- [x] **B1.7 T20 `core/plan`**：隔离环境下 `go test -count=1 ./core/plan ./core/spec ./core/template ./core/when` 覆盖 Requirement 的事件/能力/数据需求，native/degraded/partial/skipped/never-trigger/install-error 状态，tool 替代时机、条件数据缺失、block→warn 且不走静态回退、warn 部分能力、skill 隐式+显式事件、fallback skip/error、动态/静态 system、replace 降级、静态槽位缺失、harness 过滤、可选模板值告警、文本与 JSON 报告不泄漏正文并保持确定性，以及 Load→Validate→Merge→Build 整合。`make test-go`、`make vet-go`、`go mod tidy -diff`、gofmt、diff 检查通过。证据：`core/plan/plan_test.go`、`docs/PROGRESS.md` T20/B1.7。A2 的 CLI golden 和 token 部分仍未完成。
- [x] **B1.8 T21 `core/tokens`**：隔离环境下 `go test -count=1 ./core/tokens ./core/spec ./internal/tokenestimate` 覆盖估算公式与 Unicode rune 数、静态包/规则/settings/vars 值、64/128 字符运行时占位符、默认值/非法模板、warn 首句和 `model_text`、prompt block 不计模型输入、动态 system/static fallback、T001/T002/T004 严重程度与限值、adapter Layout 原生事件/工具聚合、block 与 inject/warn 分隔符及最坏情况、跨包 T003 token/字符 80% 边界、共享 T005/T006/T007、稳定 JSON 和正文不泄漏。双 module `make test-go`、`make vet-go`、`go mod tidy -diff`、gofmt 和 `git diff --check` 均通过；agenthooks 没有改动。证据：`core/tokens/*_test.go`、`core/spec/validate_lints_test.go`、`internal/tokenestimate/estimate_test.go`、`docs/PROGRESS.md` T21/B1.8。T003 的完整调用仍由后续 adapter/install 集成提供已安装包清单和 adapter 上限；CLI golden/A18 尚未完成。

## M1a：STYLE 离线验收

### 包格式和 validate

- [ ] **A1** 示例包 validate 退出 0；所有 invalid fixture 对应错误码退出 1：`dup-id`、`from-and-inline`、`inline-missing-id`、`file-and-text`、`file-into-rules`、`body-has-frontmatter`、`rule-file-pkg-field`、`empty-body`、`override-target`、`defaults-field`、`undeclared-var`、`reads-missing`、`event-var-in-template`、`sensitive-reserved`、`condition-field-wrong-event`、`bad-regex`、`bad-effort-expr`、`action-target`、`block-after`、`replace-non-system`、`bad-name`、`bad-id`、`path-escape`、`symlink`、`file-too-large`、`bad-encoding`、`marker-in-body`。
- [ ] **A2** 示例包 `style plan` 与 Codex/Claude golden 逐字一致，含 token 报告。
- [ ] **A3** 示例包生成的 `index.json` 与 golden 逐字一致，并通过 schema。
- [x] **A21** `warn` 首句提取覆盖中英文、无标点、多行和开头空行；新增 ASCII 句末条件、CRLF 和动态模板值用例。证据：`core/warn/sentence_test.go`、B1.6。
- [ ] **A22** 非交互安装没有 `--yes` 退出 1；replace 没有 `--allow-replace` 被拒绝。

### engine、state、codec 和输出

- [ ] **A4** Codex/Claude payload fixture 的 hook stdout 与 golden 一致。
- [ ] **A5** inject/warn 合并顺序正确；命中 block 时只输出拒绝。
- [ ] **A6** session/turn/every、正文哈希变化、block 不去重、warn 每次发送。
- [ ] **A7** 20 个并发进程对同一去重键恰好一个输出。
- [ ] **A8** startup/resume/clear/compact 来源和 gen/epoch 行为正确。
- [ ] **A9** model/effort 改变后动态 system 重新评估且只补发变化正文。
- [ ] **A10** Codex/Claude effort 映射、未知值、比较表达式和来源优先级正确。
- [ ] **A11** 主 agent/subagent/any 默认范围和 subagent.start 去重行为正确。
- [ ] **A15** 所有输出通过白名单；Claude prompt block 带 `suppressOriginalPrompt`。
- [ ] **A17** 损坏 index 默认空输出、记录日志；Claude fail-closed 返回失败码。
- [ ] **A19** 动态正文超过 harness 上限时从 order 大的一端裁剪并释放占位。
- [ ] **A20** index-only 热路径 p95 小于 50ms（不含 shell 启动）。

### 安装、信任和 token

- [ ] **A12** 普通包改动由 sync 阻塞并由 trust 确认；link 包行为符合设计；trust 基于同一份内存字节。
- [ ] **A13** install/uninstall 保留外部配置、重复安装幂等、换路线无残留。
- [ ] **A14** Codex 静态托管区位置正确；用户自定义 developer instructions 时降为动态；Claude 标记块正确增删。
- [ ] **A16** core 不直接 import `adapter/codex` 或 `adapter/claude`。
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
