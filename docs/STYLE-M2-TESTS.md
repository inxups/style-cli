# STYLE CLI M2 测试清单

更新时间：2026-10-17

本文件记录 M2 测试矩阵和每项当前状态。测试通过必须有命令输出、fixture/golden 文件或真实环境记录作为证据。任务清单见 [STYLE-M2-TASKS.md](STYLE-M2-TASKS.md)，M1 测试矩阵已只读归档（[STYLE-M1-TESTS.md](STYLE-M1-TESTS.md)）。

当前状态：M2-BASE-01、M2-A1–A6 已通过，M2-A9 已完成 native/stats/跨包去重阶段回归、等待后续 M2 功能完成后复验，D4/D5 已完成设计探针准备和本地协议补充检查（M2-PREP-01），真实界面验收因用户报告 Claude 订阅额度用尽暂挂，OAuth 保存重试仍未确认。M2a 断言为初稿，标“D* 定稿后细化”的条目在对应设计项通过评审时补全具体 fixture、golden 和退出码。

## 状态约定

- `[x]` 已通过。
- `[ ]` 尚未实现或尚未执行。
- `[~]` 已执行但受环境影响，或只完成部分验证。
- `[!]` 发现失败或设计风险。
- `[?]` 需要真实 Codex/Claude 环境。

## 测试环境约定

与 M1 相同。为避免宿主环境中的 `CODEX_HOME`、`CODEX_SANDBOX` 等变量改变 provider 检测，标准命令使用：

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

仓库根目录的 `make test-go`、`make vet-go` 分别对根 module 与 `agenthooks/` 执行上述命令。保留宿主环境运行用于复现时，单独记录环境和结果。

## 基线

- [x] **M2-BASE-01**（M2-01）：M2 起点 HEAD 的 `make test-go`、`make vet-go`、`-trimpath` 构建与 `scripts/smoke.py` 通过；记录提交哈希、二进制 SHA256 和输出摘要。结果：6ec25f6 上 gofmt 无输出、双 module test/vet exit 0、采集器 40 项 OK、构建 SHA256 `48a86c05…0fa5`、smoke 32 项通过，见 `docs/PROGRESS.md` M2-01。

## 遗留项

- [?] **M2-CARRY-01**（原 M1 FINAL-09）：两家官方 token 计数接口对固定 40 条语料的同结构基线增量，与生产 CLI 估算逐条对照。当前两家 API key 缺失，0/40；有 key 时按 `testdata/manual/` README 重跑。证据：`docs/review/M1-FINAL-09-COUNTING-BLOCKED-2026-10-16.json`。

## M2a：离线验收

- [x] **M2-A1 native validate**（M2-T10，按规则 3.10）：结果：`core/spec` 的 `TestNativeRuleValidationMatchesAcrossPackageFormats`（49 例，目录/单文件逐例比对）及默认值、`from` 覆盖、非 native 键、注册表注入、`NativeRules` 解析测试；`TestA1ValidateExampleAndEveryInvalidFixture` 含六个新 fixture；`internal/app` 的 `TestNativeCatalogComesFromRegisteredAdapters`；双 module test/vet 通过，见 `docs/PROGRESS.md` M2-T10。示例 `codex-only-notify` 已符合 3.10，正文说明随 T11 的 golden 一起更新。
  - 验收清单：`testdata/invalid/` 新增 `native-mapping`、`native-field`、`native-event`、`native-matcher`、`native-command`、`native-timeout` 六个 fixture，纳入 `TestA1ValidateExampleAndEveryInvalidFixture` 的清单；每个在文本/JSON 两种模式下退出 1、stderr 为空、输出含对应码。
  - `core/spec` 表驱动用例覆盖各码的子情况：没有 harness 键、harness 值是列表或标量（`native-mapping`）；规则上写 `action`/`when`/`repeat`/`event`、顶层写 `gemini:`、映射里写 `async`/`onFailure`/`type`/`args`（`native-field`）；缺 `event`、非字符串、Codex 写 `UserPromptExpansion`、Claude 写 `PostModelSwitch`（`native-event`）；空串、首尾空白、写在 `Stop`/`UserPromptSubmit` 上、Codex 非法正则、Claude `*` 合法（`native-matcher`）；缺失、非字符串、含换行/回车/NUL、首尾空白、以 `;`/`&`/`|`/`)`/`#` 开头、未转义 `\` 结尾、引号不成对（`native-command`）；`0`、`601`、`1.5`、字符串（`native-timeout`）。
  - 合法用例退出 0：示例包（`codex-only-notify` 更新为 3.10 写法）；同时写两家映射、只写一家、带 matcher 与 timeout、正文为空的 native 规则；包级 `defaults.action: warn` 不使 native 报错；`from:` 整体覆盖某个 harness 映射。
  - 非 native 规则上写 `codex:` 仍报 `invalid-schema`。
  - 单文件包（`STYLE.md` frontmatter）与等价目录包对上述合法/非法用例给出相同的码和退出码。
  - harness 键与事件名来自注册表：用只注册一个假 harness 的注册表做单测，`codex` 键即报 `native-field`；`core/...` 不导入具体适配器的依赖测试仍通过。
- [x] **M2-A2 native plan**（M2-T11，按规则 3.10）：结果：文本 golden `testdata/golden/plan-{codex,claude-code}.txt` 已更新（仅 native 行与“native hook output not estimated”一行变化，其他数字与 M2-01 起点相同）；JSON 契约由 `internal/app` 的 `TestA2NativePlanReports` 断言（无 JSON golden 文件）；fallback 三种结果由 `core/plan` 的 `TestUnwritableNativeMappingAppliesFallback` 与 `internal/app` 的 `TestA2NativeFallbackErrorBlocksInstallWithoutChanges`（install 退出 4、STYLE_HOME 与 Codex 配置目录都未创建）覆盖，见 `docs/PROGRESS.md` M2-T11。
  - 两家 plan golden（文本与 JSON）：`codex-only-notify` 在 Codex 上为 `native-hook`，附事件 `Stop`、实际 timeout 60；在 Claude 上为 `skipped`，原因"没有该 harness 的 native 映射"。输出中不再出现 `skipped-m2` 或"跳过（M2）"。
  - token 一节单列"native hook 输出不估算"及规则/事件；常驻、单条、按原生事件合计与 M2-01 起点相比不含 native，`T001`/`T003` 不计 native。
  - `fallback`：用假适配器让 `Native()` 返回错误，`skip`/`degrade` 得 `skipped`，`error` 得 `install-error` 且 install 退出 4、不写任何文件。
  - plan 报告（文本/JSON）不含 native 规则正文，也不含命令；命令只在 install 披露、trust 预览和 doctor 里出现。
- [x] **M2-A3 native 安装往返**（M2-T12/T60）：`internal/app` 的 `TestA3NativeInstallRoundTripAndRouteSwitch` 覆盖三条路线 install → uninstall 的配置字节/权限还原、他人条目保留、重复安装幂等、Codex 路线往返无残留、多包按 priority/包序号排序及单包卸载后重编译；核对 native owner、matcher、默认/指定 timeout、原生命令不入 runtime index，并逐条核对含 foreign 组的 Codex 最终组序号与 `trusted_hash`。`agenthooks/install` 的 `TestRawNativeCommandsBypassRunnerAndPreserveShellContract` 实际用 `sh` 核对 stdin 原样和 exit 7、引号 tag、无 runner/async/filter/fail-closed 参数；`TestCodexRawNativeEventsAndSessionEndTimeout` 覆盖全部 Codex native 事件和 SessionEnd 3 秒限制。T60 的共享 `TestNativeRouteLifecycleRestoresForeignBytes` 另覆盖三路线 × 三种文件尾的 9 次安装往返、逐条状态与信任、幂等及零残留。双 module fresh test/vet、共享 conformance race/vet 通过，见 `docs/PROGRESS.md` M2-T12/T60。
- [x] **M2-A4 native 披露与信任**（M2-T13，按运行时 8.6）：`internal/app/native_lifecycle_test.go` 的 `TestA4*` 已覆盖下列断言：6 个三路线文本/JSON 安装确认场景；48 个三路线普通/link 包的命令、事件、matcher、timeout、增删、priority 和显式默认 timeout 变化场景；22 个 doctor 一致/缺失/字段改动/重复条目/信任哈希与确认修复场景；另覆盖捕获快照与回调隔离、T12 批准基线兼容、另一个 link 包不被连带批准、普通 link 规则仍自动 sync、SessionEnd 声明 timeout 改变而实际仍 3 秒仍需确认。双 module fresh test/vet、app 与两家 adapter race、采集器 40 项通过，见 `docs/PROGRESS.md` M2-T13。
  - 安装报告逐条列出 harness、规则、事件、matcher、实际 timeout、作者命令原文（不含 `: --owner=style --native=…;` 前缀），并写明路线信任方式（`codex-user` 预置、`codex-plugin` 插件信任、`claude-user` 工作区信任）。
  - 非交互、无 `--yes`、本次会新增 native 条目时退出 1，harness 配置、锁文件、index 逐字节不变；带 `--yes` 成功。
  - 修改 native 的命令、事件、matcher、timeout 任一项，或增删 native 规则后，`sync` 退出 3；`--link` 包同样退出 3。`trust` 预览单列"native 条目差异"，给出改前改后四个字段；未确认时不写入，确认或 `--yes` 后配置与新渲染一致，`codex-user` 的 `trusted_hash` 同步更新。
  - `style list` 显示每个包的 native 条目数；`style doctor` 对一致、缺失、被手工改动三种状态分别输出正常、告警、告警，`codex-user` 另报 `trusted_hash` 是否匹配。
- [x] **M2-A5 stats**（M2-T20，按已批准 D2 契约）：结果：`core/state/stats_read_test.go` 与 `internal/app/stats_test.go` 覆盖以下断言；`testdata/stats/m2` 固定时钟/两家与历史 provider/多会话多 agent fixture，对照 `testdata/golden/stats-{rule,session}.{txt,json}` 四份 golden。双 module fresh test/vet、state/app race、采集器 40 项和扩展 smoke 36 项通过，见 `docs/PROGRESS.md` M2-T20。
  - 固定时钟、两家 provider、多规则/评估点、多个会话/main/子 agent 的 `stats.jsonl` fixture；rule/session 文本与 JSON golden 逐字节比较。核对 output/blocked/dedup/cap-drop、六个数值合计、相同规则名跨 provider 分组、估算排序与平手顺序、已卸载包历史仍可读。
  - 默认 168h、正 duration、带时区 RFC3339、两端边界、过旧/未来记录过滤；非法参数/未来 since/未知参数退出 1 且 stdout 为空。无 state/窗口无记录退出 0、rows 为 []、不建目录。
  - 缺估算/摘要的旧记录保留次数、增加 unestimated，不读取当前 index 或重新估算；静态/native 无日志时不杜撰成本。与 doctor 的正常 top3/legacy/repeated_body 结果兼容。
  - 坏 JSON、截断尾行、空行、非法字段/时间/摘要/负估算/未知字段及 >=512 字节行逐项 fixture；跳过后下一行仍计入，quality 精确、partial=true、退出 0。根/后代 symlink、权限/I/O、数值溢出与取消覆盖；致命错误退出 1、stdout 无成功报告。
  - 整树字节/权限/mtime 对照证明 stats 只读且不触发 state 清理；坏行里的正文/提示词/native 命令标记不出现在 stdout/stderr，标识符控制字符转义。doctor 坏行继续有效聚合并给 stats-invalid warning。
- [x] **M2-A6 跨包去重**（M2-T30）：`core/compile/dedup_test.go` 的三个包/两个 harness、完整入口与严格语义差异测试，`core/tokens/dedup_test.go` 的源级诊断与有效桶/分隔符测试，`internal/app/install_dedup_test.go` 的三路线、trust/link、预算无写入、并发/生命周期/保留方切换测试通过；两份 `dedup-install-codex` 文本/JSON golden 固定组合报告。`TestCrossPackageDedupPreservesAllCodecGoldensAndRuntimeStats` 在两家 46 个 payload 上验证原 stdout golden、白名单和 stats 记录（仅忽略时间），不为编译别名产生记录。双 module fresh test/vet、相关 race、采集器 40 项与 smoke 40 项通过，详见 `docs/PROGRESS.md` M2-T30。
  - 以下验收清单均有上述可重复测试支撑：
  - 两个/三个包的等价静态/动态规则；priority（含小数）、包安装序号、包内序号与输入遍历顺序测试，index/StaticBodies 仅保留选定规则全部入口、packages 仍完整，组合 plan 文本/JSON 标 deduplicated_into，单包 plan 原 golden 保持。
  - settings/包名静态替换、空白/换行/Unicode、warn 两份正文/首句标志、不同变量名/默认值、action/条件/工具/MCP/skill/repeat/sources/reinject/reevaluate/能力差异逐项排除误合并；同包多条等价声明时整个类保留；相同 native 命令仍全部安装。
  - 源级 T001/T002/T004 检查保留（静态 T001 在 validate 阶段退出 1，T002/T004 安装预算退出 4）；组合有效 events/always_on/session 情景与 T003 只计一次并重算分隔符；不同语义或仅运行时偶然同文不降预算；去重前超 80% 而去重后合法时可安装，真实超限仍退出 4 且无写入。
  - runtime 条件/重复/并发/compact/gen/epoch/block/warn/subagent.start/cap-drop 回归；等价组仅保留方产生日志，stats 不虚构折叠行。无去重组的现有黄金输出保持。
  - 卸载保留方/被折叠方、trust/link sync 改正文/priority、三条路线切换与重复编译；接替规则按新 rule 身份在已有会话可再注入；doctor 认同重建结果，最终外部字节/权限精确还原。
- [ ] **M2-A7 Claude rules 静态位置**（M2-T40，仅 D4 成立时）：写入/删除 `~/.claude/rules/style-<名字>.md`；从 `CLAUDE.md` 标记块迁移后再卸载，两处文件逐字节还原。D4 不成立时本条以 D4 证据标注不适用并关闭。
- [ ] **M2-A8 output style replace**（M2-T50，仅 D5 可行时）：断言随 D5 定稿；不可行时本条以 D5 证据标注不适用并关闭。
- [~] **M2-A9 M1 回归**：native、stats 与跨包去重阶段已通过双 module fresh test/vet、app/state/adapter/conformance race、采集器 40 项、构建与 smoke（stats 阶段 36 项，去重阶段扩为 40 项）。T60 的 `TestNativeRulesLeaveRuntimeOutputsAndStatsUnchanged` 在两家共 46 个 payload 上核对原 stdout golden、输出白名单、runtime 条目与统计记录（只排除时间差）；inline 源文件摘要随新增声明改变是预期，其他条目字段不变。T60 未更新任何 golden。M1 的 A1–A22、adapter conformance 在后续 M2 实现后仍需最终复验，见 `docs/PROGRESS.md` M2-T13/T60/T20/T30。

## M2b：Codex 真实环境

Codex 真实配置由 agent 执行，步骤见 M2-PREP 建立的 `docs/STYLE-M2-MANUAL.md`。投递以 Codex rollout / OTel span 为证，不以模型自述为据。

- [x] **M2-PREP-02 Codex 独立探针准备**：`test_probe_codex_m2.py` 三项通过（完整正文/角色/重复/失败拒绝、stats 行与总量和折叠行拒绝、原生观察白名单），全部采集器 50 项 OK。仅准备通过，B1–B4 仍需真实运行结果。

- [x] **M2-B1 native hook 真实触发**：codex-user/plugin 两路线的 SessionStart、Bash PreToolUse 各一次，Read matcher 不触发；白名单观察、OTel、rollout 和请求全文相符。卸载后额外真实会话无 hook/span/正文，配置字节与权限还原。只声明 exec 对用户哈希/插件登记的接受，不声明首次 TUI 工作区信任。证据：[M2-B1 双路线报告](review/M2-B1-CODEX-2026-10-17.json)；见 `docs/PROGRESS.md` M2-B1。
- [x] **M2-B2 native 与 STYLE hook 共存**：codex-user/plugin 两路线在 SessionStart 和 PreToolUse 上各有两个 completed span（native 与 STYLE 各一），来源正确；四份完整正文各一次，STYLE runtime 两条 output，native 不进入规则日志。信任范围沿用 B1。证据：[M2-B2 双路线报告](review/M2-B2-CODEX-2026-10-17.json)；见 `docs/PROGRESS.md` M2-B2。
- [?] **M2-B3 stats 真实汇总**：真实会话产生的 `stats.jsonl` 经 `style stats` 汇总后，注入次数与 rollout 中 hook 上下文条数一致。
- [?] **M2-B4 跨包去重真实投递**：两个包含相同正文规则时，真实会话只注入一次。

## M2b：Claude Code 真实环境

- [x] **M2-PREP-01 设计探针准备**：`test_probe_claude_m2_design.py` 七项通过，全部采集器 47 项通过；新增实际执行五个生成脚本核对真实 HOME/系统身份、独立配置、凭据变量排除，以及重写脚本时保留既有证据。首次 OAuth 保存失败的只读复现：原临时 HOME 下默认钥匙串查询退出 1，修正后退出 0；不据此推断解锁或 OAuth 保存成功，见 `review/M2-PREP-01-AUTH-2026-10-17.json`。此前固定 2.1.301 CLI loopback 四模式均 exit 0、各一份请求、rules 全文各一次，替换/keep 和既有 style 互斥全文命中。只有准备/补充检查通过，C1/C4 不因此标通过。

Claude Code 界面项由用户执行，agent 提供手册与隔离 runbook。

- [~] **M2-C1 U-C4 实测**（M2-D4）：用户级 `~/.claude/rules/*.md` 是否被加载，以 `InstructionsLoaded` 或原生请求为证；记录结局。本地协议已证实全文一次；首次 OAuth 保存失败的启动器 HOME 问题已修正，但保存成功尚未确认。2026-10-17 用户报告订阅额度用尽，暂停真实请求，额度可用后按手册继续。
- [?] **M2-C2 native hook 真实触发**：Claude Code 真实会话中 native 命令按事件和 matcher 执行；卸载后 `settings.json` 逐字节还原。
- [?] **M2-C3 rules 静态位置真实生效**（仅 D4 成立时）：安装后规则文件被加载，卸载后文件移除、`CLAUDE.md` 与安装前一致。
- [~] **M2-C4 output style 实测**（M2-D5）：output style 能否替代 system prompt、与用户已有 output style 的冲突行为；记录结局。本地完整 prompt 对照与官方资料支持不作为整份替换位置；与 C1 同受首次 OAuth 保存失败和用户报告的订阅额度用尽阻塞，保留启动环境修复结果，额度可用后再登录/切换实测。

## 持续集成与文档核对

- [ ] **M2-CI-01**：M2 最终提交的 GitHub Actions 在 Ubuntu/macOS 上 format、vet、双 module test、采集器测试、build、smoke 全部 success。
- [ ] **M2-DOCS-01**：M2 修改过的当前文档本地链接目标全部存在、代码围栏成对，`git diff --check` 通过。
