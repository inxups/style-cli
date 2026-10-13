# STYLE M1 已完成工作回顾

日期：2026-10-13

结论：T10–T14 的基础实现已经形成清楚的分层，`agenthooks/` 仍负责 hook 协议和 provider 事实，STYLE core 负责包、合并、校验、模板和条件语义；这条边界应继续保留。已完成的测试没有发现 P0/P1 问题，但边界探针发现 8 个 P2 校验/解析问题。建议先修复这些问题，再进入 T15、CLI、engine 和 adapter 集成。当前停止功能推进，M1 尚未完成。

## 修复跟进

每项只有在回归测试通过后标记 `[x]`。以下状态覆盖报告中的历史发现；原观察结果保留用于对照。

| 编号 | 状态 | 修复结果与证据 |
|---|---|---|
| R01 | [x] | YAML decoder 完整消费输入，拒绝第二文档或非法尾部；`core/spec/frontmatter_test.go`，`go test -count=1 ./core/spec` 通过 |
| R02 | [!] | 待修复 |
| R03 | [!] | 待修复 |
| R04 | [!] | 待修复 |
| R05 | [!] | 待修复 |
| R06 | [!] | 待修复 |
| R07 | [!] | 待修复 |
| R08 | [!] | 待修复 |

## 已完成范围

| 条目 | 提交 | 结果 |
|---|---|---|
| T10 | `b965681` | 根 module、独立 `agenthooks` module、workspace 和双 module 测试入口完成 |
| T11 | `b731f54` | 目录包、单文件包、frontmatter、`from:` 文件和 glob 解析完成 |
| T12 | `da1da7f` | 默认值、规则覆盖、`reads` 并集和停用规则合并完成 |
| T13 | `a91c497` | schema、路径、模板变量、条件、lint 和诊断完成 |
| 规范拆分 | `632d5fa` | 规范入口、包结构、规则、运行时、编译产物五篇文档完成 |
| T14 | `ff32b99` | `core/template`、`core/when`、spec 校验/lint 复用模板解析器完成 |

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

T14 的定向测试覆盖模板默认值和转义、静态/动态分类、引号内分隔符、条件 AND/OR、模型 glob、effort 比较、agent 范围、缺失字段、provider 永不匹配、matcher 复用和 JSON 往返。完整测试通过只说明已覆盖行为保持一致；它们没有覆盖下面列出的包输入边界。

## 需要修正的 8 个边界问题

优先级均为 P2：会让无效包通过 validate、阻止有效规则通过校验，或让错误延迟到后续编译/运行阶段。

| 编号 | 复现输入 | 当前观察 | 修正方向 |
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

探针是观察工具，当前行为本身不会让命令失败；修复时应把 R01–R08 转成失败回归测试，并把错误码和诊断路径固定下来。

## 维护性和文档结论

- `core/spec` 已按 loader、frontmatter、文件安全、merge、package/rule/condition/template/reads/lint/value 校验拆分。`validate_rules.go` 仍约 299 行，包含相关的 schema、ID、正文和有效值逻辑；后续若继续增长，可再拆 schema 与语义，但现在不需要重写。
- 规范拆分保留了章节编号和正文顺序，最大单文件实际为 242 行；进度日志中曾写 243 行，是切片时保留的 EOF 空行造成的计数差异，本次记录以实际文件计数为准。
- 规范入口和 CLI 设计仍有“尚未实现”的历史状态文字。`STYLE_DOCS/STYLE-cli-design.md` 被忽略、未跟踪，新 clone 中规范到它的链接不可用；示例 `example/terse-reviewer/STYLE.yaml`、附录引用的 `style-gen/cross-harness.md`、`thinking-effort.md` 和 `style-review/solution-2026-10-12.md` 也缺失。由负责 plan/docs 的对话统一处理状态文字和引用。
- `agenthooks/` 在 T10–T14 没有源代码改动；根 module 通过本地 `replace` 使用它，符合“hook 直接在 agenthooks、CLI 另起 module”的约定。

## 暂停点

T14 已在 `ff32b99` 提交并验证。T15 尚未开始；A1–A22、adapter conformance、Codex/Claude 真实环境和 M1a/M1b 仍未完成。下一次继续时，优先顺序是 R01–R08 的修正与回归测试，然后再规划 T15 和 CLI/engine 的下一步。
