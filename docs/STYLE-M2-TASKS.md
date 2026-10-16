# STYLE CLI M2 任务清单

更新时间：2026-10-17

本文件是 STYLE CLI M2 的持久化任务清单。每项任务完成后必须同时补充证据（命令、fixture、golden 文件或真实运行记录），不能只根据对话上下文标记完成。测试矩阵见 [STYLE-M2-TESTS.md](STYLE-M2-TESTS.md)，执行记录追加到 [PROGRESS.md](PROGRESS.md)。

当前总状态：M2a 离线实现与 A1–A9 验收已完成，包含 native、stats、跨包去重、Claude 每包 rules/旧安装迁移和 replace 回退；Codex B1–B4 在 user/plugin 两路线通过。2026-10-17 Claude 恢复，用户授权 agent 通过交互终端代跑；既有隔离目录认证和真实请求成功，C1/C4 已通过，C2/C3 继续。最终本地 test/vet/race/53 采集器/构建/40 smoke 通过，远端 Ubuntu/macOS CI、完整文档最终状态与官方 token 计数仍未闭合。M1 已关闭并只读归档（[任务](STYLE-M1-TASKS.md)、[测试](STYLE-M1-TESTS.md)）；M2 未宣布完成。

## 范围

M2 只做 [CLI 设计 §14.4](spec/STYLE-cli-design.md) 的五项，不提前拉入 M3 条目：

1. `target: native`（规则写原生 hook，不经过规则引擎）。
2. `style stats`（按规则和会话汇总注入次数与估算 token）。
3. 跨包去重（CLI 设计 T8：正文完全相同的规则只留一份）。
4. Claude 静态位置改用用户级 `~/.claude/rules/`（用户已批准依据原生请求证据先实现，C1/C3 界面验收后补）。
5. 评估 Claude output style 能否做 `replace`。

M1 中已记录的已知限制（如 force-push 示例规则对 `--push-option` 后单独 `-f` 的保守误拦）不属于 M2。

真实环境约定：Codex 真实配置由 agent 执行。2026-10-17 用户明确授权 Claude Code 界面项改由 agent 通过交互终端代跑；保留隔离 runbook 与原生证据，登录/钥匙串需要用户交互时单列。

## 状态约定

- `[x]` 已完成，且有可重复证据。
- `[ ]` 尚未开始。
- `[~]` 部分完成或被环境阻塞。
- `[!]` 已知问题或设计风险，尚未解决。
- `[?]` 等待真实环境验证。

## 编号约定

- `M2-D*` 设计定稿，`M2-T*` 实现，`M2-CARRY-*` 上一里程碑遗留，`M2-RV-*` 审查/回顾发现，`M2-PREP` 真实环境准备。
- 提交消息带对应编号，例如 `docs(M2-D1): ...`、`feat(M2-T10): ...`。
- 每条写明目标、依赖、产出和对应的测试编号；状态变化时同步测试矩阵和进度日志。

## 当前基线

- [x] **M2-00 建立 M2 文档**：新建本文件与 [STYLE-M2-TESTS.md](STYLE-M2-TESTS.md)；M1 任务/测试首页改为已关闭并只读归档，FINAL-09 移交 M2-CARRY-01；`AGENTS.md` 改为指向当前里程碑文档。仅文档改动。证据：本地链接/围栏检查与 `git diff --check`，见 `docs/PROGRESS.md` M2-00。
- [x] **M2-01 基线回归**：在 M2 起点 HEAD 用隔离环境运行 `make test-go`、`make vet-go`、`-trimpath` 构建和 `scripts/smoke.py`，记录提交哈希、二进制 SHA256 与结果，作为 M2 改动的对照。依赖：M2-00。测试：M2-BASE-01。证据：起点 6ec25f6 全部通过，见 `docs/PROGRESS.md` M2-01。

## 遗留项

- [?] **M2-CARRY-01 官方 token 计数（原 M1-FINAL-09）**：固定 40 条中英各半语料、正文/请求哈希和生产 CLI 估算已记录；两家官方 API key 缺失，实际计数 0/40。有 key 时按 `testdata/manual/` README 的命令重跑，不用登录态、合成 usage 或其他估算器替代。不阻塞 M2 其他条目；M2 关闭时须再次显式说明状态。证据：`docs/review/M1-FINAL-09-COUNTING-BLOCKED-2026-10-16.json`。测试：M2-CARRY-01。

## 设计定稿

规范先行：每项先写设计结论并回写 `docs/spec`，用户评审通过后才进入对应实现条目。实测类设计项（D4、D5）的结论按 M1 惯例记录为**符合预期**、**回退方案生效**或**阻塞**。

- [x] **M2-D1 native 规则格式**：设计已回写规范，用户评审通过（2026-10-16），M2-A1/A2/A4 已按设计细化。用户已定：归属用命令前缀 no-op `: --owner=style --native=<包>/<id>;`；`codex-user` 预置 `trusted_hash`；每个 harness 一个映射；M2 只支持 `event`/`matcher`/`command`/`timeout`。结论见 `spec/STYLE-spec-rules.md` §3.10、`spec/STYLE-spec-runtime.md` §8.6、`spec/STYLE-cli-design.md` §3/§6/§9.3/§10.2/§11.3/§13 F11。原始待定项：
  - `codex:` / `claude-code:` 下的字段：原生事件、matcher、command/args、timeout、Claude `onFailure` 等，以及单文件包与目录包的写法。
  - validate 诊断码（缺映射、未知 harness 键、非法原生事件、空命令等）。
  - plan 状态替换 `skipped-m2`；token 报告口径（STYLE 无法估算原生命令输出，如何标注）。
  - 安装报告披露、sync/trust 差异展示、owner 标记与卸载。
  - 与 Codex 固定订阅面、Claude 动态订阅面并存时的合并与排序。

  回写：`spec/STYLE-spec-rules.md` §3.3、`spec/STYLE-spec-runtime.md` 信任一节、`spec/STYLE-cli-design.md` §3 `Native()`。依赖：M2-01。
- [x] **M2-D2 `style stats` 输出**：用户在上一轮草案评审请求后回复“继续”（2026-10-16），按该契约进入 T20。设计已回写 CLI 设计 §4、§11.5.2：默认 rule/最近 168h，`--by session` 合并 main 与子 agent，`--since` 接受正 duration 或 RFC3339；只读留存日志，六个计数字段明确 output/blocked/去重/容量丢弃与缺估算；JSON format 1、稳定排序，坏行/超长行跳过计数、I/O 错误退出 1。doctor 复用读取器但保留 top3/旧记录/清理职责，坏行改为继续统计并告警。草案中的用户可见决策按此定稿。依赖：M2-01。证据：`docs/PROGRESS.md` M2-D2 评审通过。
- [x] **M2-D3 跨包去重（T8）**：用户在草案评审请求后回复“继续”（2026-10-16），按运行时 §7.5、编译产物 §9.6、CLI 设计 §7.1 契约进入 T30。比较静态变量替换后的完整正文及全部执行语义，按现有 order 保留一份；native 与运行时偶然同文不合并；同一类内任一包多条等价声明时保守保留。单包 plan/源级预算继续独立，组合报告标 `deduplicated_into`、有效成本和 T003 只计一次。沿用 rule 状态键，卸载保留方后接替规则可在旧会话再次注入。依赖：M2-01。证据：`docs/PROGRESS.md` M2-D3 评审通过。
- [~] **M2-D4 U-C4 实测与静态位置决策**：设计于 2026-10-17 获用户“同意”。[定稿](review/M2-D4-D5-DESIGN-2026-10-17.md) 明确每包 `rules/style-<名字>.md`、首次路线重建自动迁移且单纯迁移不额外 trust、文件归属/冲突与外部字节还原。用户批准以四份真实 CLI 请求的 rules 全文证据及设计评审作为 T40 实现前提，C1 已通过真实交互会话，C3 安装/卸载继续验收；任务保持 `[~]` 表示该实测未闭合，不阻止 T40。回写：CLI 设计 §10.4 与编译规范静态位置。证据：`review/M2-D4-D5-PROTOCOL-2026-10-16.json`、`docs/PROGRESS.md` M2-D4/D5 评审通过。
- [x] **M2-D5 output style 做 replace 评估**：2026-10-17 真实交互终端完成 default、M2Existing、M2Replace、M2Keep 四个新会话；样式选择正确、请求成功、无冲突/额外样式确认，用户六份文件 bytes/mode 和既有 outputStyle 不变。[C4 证据](review/M2-C4-CLAUDE-2026-10-17.json) 结合已有完整请求中保留的 20,615 字符公共 system 片段，结局为“回退方案生效”。维持已批准[定稿](review/M2-D4-D5-DESIGN-2026-10-17.md)：不提供整份 system replace，不改用户样式，T50/A8 fallback 已通过。

## M2a 离线实现

每条依赖对应设计项定稿；实现前后都检查工作树，保留 agenthooks 职责边界，`core` 仍不导入具体 adapter。

- [x] **M2-T10 native schema 与 validate**：`core/spec` 按 D1 校验 native 字段与诊断码，目录包与单文件包一致。依赖：M2-D1。测试：M2-A1。实现：`core/spec/native.go`（`Validate` 做与 harness 无关的形状检查，`ValidateNative` 按注册表查 harness 键、事件和 matcher；`NativeRules` 解析映射），adapter `Definition.Native` 与 `Registry.NativeCatalog()`，app 层所有入包路径改用 `Service.validateSpec`；删除写死的 `isNativeProviderField`。证据见 `docs/PROGRESS.md` M2-T10。
- [x] **M2-T11 native plan 与 token 报告**：`core/plan`、`core/tokens` 输出 native 状态和报告，更新两家 plan golden。依赖：M2-T10。测试：M2-A2。实现：`plan.NativeHook` 状态取代 `skipped-m2`，`RulePlan.Native`（事件、matcher、实际 timeout，不含命令）；可选接口 `plan.NativeAdapter`，adapter 侧 `NativePlanningFacts` 由两家 `Facts.NativeHook` 实现（默认 60 秒，Codex `SessionEnd` 压到 3 秒）；native 规则从 token per-rule、预算和 lint 中移除，`tokens.Report.Unestimated` 单列。过渡状态：T12 之前 native-hook 只出现在 plan，尚未写入 harness 配置。证据见 `docs/PROGRESS.md` M2-T11。
- [x] **M2-T12 adapter `Native()` 与路线**：两家 `Facts.Native` 生成命令、归属 tag 和实际 timeout；安装编译在 runtime index 之外按 priority/包序号/规则序号收集 native hook，经三条路线交给 agenthooks 的 `RawCommand`/`RawTag` 渲染、owner 合并、卸载和 Codex 信任哈希。修复已有 foreign matcher 组时信任键序号未随合并偏移的问题。依赖：M2-T11。测试：M2-A3；双 module fresh test/vet 通过，见 `docs/PROGRESS.md` M2-T12。披露与 native 变更信任由下一项 T13 补齐。
- [x] **M2-T13 CLI native 生命周期**：install 在写入前提供完整 capability/native 预览与确认回调，文本/JSON 披露命令和路线信任；批准快照保存 native 渲染字段，sync 对普通包/link 的 native 字段、增删、声明 timeout 和顺序变化均退出 3，配置/信任修复也必须 trust 确认。trust 单列 native 前后差异，保留其他包的未批准 link native 字节；list 显示条目数，doctor 以批准快照核对一致/缺失/改动及 codex-user 信任哈希。依赖：M2-T12。测试：M2-A4；双 module fresh test/vet、相关 race、采集器通过，见 `docs/PROGRESS.md` M2-T13。
- [x] **M2-T20 `style stats` 命令**：实现默认 168h 的 rule/session 文本与 JSON format 1，六字段计数、稳定排序与质量报告；`core/state.ScanStats` 固定两层目录、有界逐行严格解码并跳过坏行/超长行/symlink，全程只读，I/O/取消/溢出退出 1 且不输出成功报告。doctor 复用扫描器，保持正常 top3/legacy/repeated_body，坏行继续有效聚合并告警。依赖：M2-D2。测试：M2-A5；固定 fixture/四份 golden、双 module fresh test/vet、state/app race、采集器 40 项与扩展 smoke 36 项通过，见 `docs/PROGRESS.md` M2-T20。
- [x] **M2-T30 跨包去重实现**：compile 在静态替换、规范化与排序后按整条规则的全部 placement/执行语义等价折叠，保留最小 order；同包多重声明类和 native 保留，关系仅用于冷路径报告，index format 1 与 runtime 状态键不变。组合 install/trust 展示参与包的 `deduplicated_into`、原单条估算和零新增成本；源级预算独立，有效 events/always_on/session/T003 重算。依赖：M2-D3。测试：M2-A6；两份组合报告 golden、双 module fresh test/vet、compile/tokens/engine/state/app/conformance race、采集器 40 项、扩展 smoke 40 项通过，见 `docs/PROGRESS.md` M2-T30。
- [x] **M2-T40 Claude rules 静态位置与迁移**：已实现 `claude_rules`、每包 rules、安装记录布局/摘要、首次路线重建迁移、迁移报告与旧 doctor 兼容；冲突/漂移拒绝、外部字节/权限还原、批准隔离和回滚通过。A7 的 20 个旧布局生命周期场景与实际旧/新二进制 16 组升级、双 module fresh test/vet、相关 race、50 项采集器与 40 项 smoke 通过，见 `docs/PROGRESS.md` M2-T40 和 [升级报告](review/M2-A7-RULES-MIGRATION-2026-10-17.json)。依赖：M2-D4 设计批准与已有原生请求证据；C1/C3 界面验收后补，不据此关闭。
- [x] **M2-T50 output style replace**：按已批准 D5 不新增槽位，保留 replace 回退。实际构建 CLI 的两家 × degrade/skip/error × 文本/JSON 共 12 场景验证 plan 状态/退出码、无 placement、缺授权退出 1、授权不创造能力、error 退出 4 且无写入；完整用户样式树/outputStyle 设置及安装卸载外部字节/权限保持。脚本加入 CI；仅本地已通过，不宣称远端 CI 通过。证据：[A8 报告](review/M2-A8-REPLACE-FALLBACK-2026-10-17.json)、`docs/PROGRESS.md` M2-T50。C4 界面观察继续 `[~]`。
- [x] **M2-T60 conformance 扩展**：共享路线测试新增三路线 × 三种文件尾的 9 次 native 安装往返，核对状态、独立命令、用户路线信任、幂等、卸载无残留、外部字节与权限还原；46 个真实 codec payload 在附加 native 声明后仍匹配原 stdout golden/白名单，runtime index 只有源文件摘要变化，统计记录除时间外与基线相同。依赖：M2-T12。测试：M2-A3、M2-A9；conformance race/vet、最终 native 构建与 smoke 32 项通过，见 `docs/PROGRESS.md` M2-T60。

## 审查发现

- [!] **M2-RV-01 core/spec 仍写死 harness 名**：T10 只把 native 的 harness 键改成注册表提供；`when.harness.name`（`core/spec/validate_conditions.go:14`）、`requires.harness`（`validate_package.go:115`）、`recommends`（`validate_package.go:235`）仍写死 `codex`/`claude-code`，与 CLI 设计 §1“核心不硬编码字符串”不符。M1 起就存在，不影响 M2 功能；是否在 M2 内改为同一注册表由用户决定。

## M2b 真实环境

- [x] **M2-PREP 真实环境手册**：D4/D5 设计探针（PREP-01）、Codex B1–B4（PREP-02）及 T40/T50 后的 Claude C2/C3 最终安装验收（PREP-03）已备齐。手册包含固定构建、私有基线、登录/界面启动、白名单加载/native 证据、卸载后新会话和恢复校验；采集器测试及实际 CLI 准备/卸载干跑通过。此项只关闭准备，不将 Claude 真实结果标成功。
- [x] **M2-PREP-01 Claude 设计探针准备**：用户界面隔离启动器、白名单 InstructionsLoaded 观察器、四模式本地协议补充证据和手册；探针七项、全部采集器 47 项通过，真实 CLI 四请求 exit 0 且全文核对通过。首次 OAuth 保存失败后，修正临时 HOME 导致找不到默认钥匙串的问题，保留系统身份并隔离 Claude/profile 配置；现有五脚本已备份并更新，实际 OAuth 保存仍待用户重试，C1/C4 记环境阻塞 `[~]`。它不关闭 C1/C4 或 D4/D5，不提前实现 T40/T50。证据：`docs/review/M2-D4-D5-PROTOCOL-2026-10-16.json`、`docs/review/M2-PREP-01-AUTH-2026-10-17.json`、`docs/PROGRESS.md` M2-PREP-01。
- [x] **M2-PREP-02 Codex 独立验收准备**：新增 `probe_codex_m2.py`，复用既有隔离配置/真实服务转发，覆盖 user/plugin 两路线的 B1–B4，并补手册 §4；三项新增采集器断言测试、全部 50 项通过。只标脚本/手册准备，不因此标真实验收通过；无 STYLE/agenthooks 生产代码修改。证据：`docs/PROGRESS.md` M2-PREP-02。
- [x] **M2-PREP-03 Claude C2/C3 最终验收准备**：`prepare_claude_m2_acceptance.py` 固定私有 STYLE/观察器、隔离配置并保留系统身份，生成两包/三 native/一份去重静态文件及登录/界面/STYLE/还原检查启动器；三个新增测试通过，实际 CLI 干跑的 doctor/折叠/3 native/四文件恢复通过。独立待用户目录已生成，未登录或启动模型会话。证据：[准备报告](review/M2-PREP-03-CLAUDE-2026-10-17.json)、手册 §5；C2/C3 不据此关闭。
- 真实验收条目见测试矩阵的 M2-B*（Codex）与 M2-C*（Claude Code）。

## 里程碑验收

- [x] **M2a 离线验收**：M2-A1–A9 全部有可重复证据；最终双 module fresh test/vet、相关 race、53 项采集器、固定构建/重建一致、40 smoke、16 组升级与 12 组回退通过。证据：[A9 本地最终回归](review/M2-A9-LOCAL-REGRESSION-2026-10-17.json)。
- [x] **M2b Codex 真实环境**：M2-B1–B4，codex-user/plugin 两路线真实服务、原生观察/OTel/rollout/stats 证据均通过，见测试矩阵和 `docs/review/M2-B*-CODEX-2026-10-17.json`。
- [ ] **M2b Claude Code 真实环境**：M2-C1–C4。
- [~] **M2-FINAL 最终回归**：最终本地双 module fresh test/vet、相关 race、53 项采集器、固定构建/重建一致、40 smoke、迁移/回退 CLI 通过；当前提交未推送，最终 Ubuntu/macOS CI（M2-CI-01）仍待执行。
- [~] **M2-DOCS 文档同步**：当前实现/设计/离线结果已同步（DOCS-02/03），本地链接/围栏通过；Claude/CI/计数的最终事实尚缺，补齐后再关闭总文档验收 DOCS-01。
- [x] **M2-DOCS-02 已实现功能文档同步**：[教程](STYLE-TUTORIAL.md) 和 [adapter 总览](STYLE-ADAPTERS-CURRENT.md) 补 native、stats、跨包等价去重，移除 M1 过时支持状态；CLI 设计 §14.4 与编译规范静态位置表反映当前实现，明确 Claude 待定与最终验收边界。真实固定二进制跑通教程新增 native 包的 validate/双 harness plan/install/uninstall 和三个 stats 命令；配置字节/0640 还原，文档链接/锚点/围栏检查通过。证据：`docs/review/M2-DOCS-02-2026-10-17.json`、`docs/PROGRESS.md` M2-DOCS-02。本项不关闭 D4/D5、Claude 真实验收或最终文档总验收。
- [x] **M2-DOCS-03 rules/回退与离线最终状态**：教程、adapter 总览、CLI/编译/规则规范同步 claude_rules、旧迁移/归属/冲突和真实界面边界；C2/C3 最终手册与采集器入口完成。核对本轮 M2 改过的全部当前 Markdown 本地链接/锚点、围栏、证据 JSON、M1 归档未变和 diff-check；未完成的 Claude/CI/计数仍明确保留。证据：`docs/PROGRESS.md` M2-A9/DOCS-03。

M2 全部闭合后提示用户进行 M3 规划。

## 后续 backlog

- [ ] **M3**：`explain`、`probe`、`update`、git 来源、`--frozen`、项目级路线、刷新注入、事件内容围栏、provider、规则级 fail-closed、用户 effort 映射、Windows。
- [ ] **M4**：Gemini、OpenCode、Cursor、Copilot、Kimi 等额外 harness 适配器。
