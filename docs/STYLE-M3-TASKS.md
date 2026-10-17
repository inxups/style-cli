# STYLE CLI M3 任务清单

更新时间：2026-10-17

最新授权：用户于 2026-10-17 要求“所有评审全部通过，完全完成M3”。T31/T32 恢复实施，后续设计先记录契约后直接推进；下文早期“暂缓/待用户评审”叙述保留决策背景，以本条和逐项证据为准。授权不替代实现、测试或真实环境验收。

本文件是 STYLE CLI M3 的持久化任务清单。每项完成后同步状态和可重复证据，测试矩阵见 [STYLE-M3-TESTS.md](STYLE-M3-TESTS.md)，执行记录追加到 [PROGRESS.md](PROGRESS.md)。M1、M2 已关闭并只读归档；上一里程碑的事实见 [M2 任务清单](STYLE-M2-TASKS.md)、[M2 测试矩阵](STYLE-M2-TESTS.md) 和 [M2 总验收报告](review/M2-DOCS-01-2026-10-17.json)。

当前总状态：基线、注册表、诊断、来源/update/frozen、项目/云端/Claude 插件、refresh 与事件内容围栏均已实现并完成对应离线 A0–A10；D0–D6 已定稿。T41/A10 的命令和源码摘要见 [报告](review/M3-A10-2026-10-17.json)，使用方式见 [事件内容说明](STYLE-EVENT-CONTENT.md)。当前进入 D6 provider/unknown。真实本地 B/C、后续运行时/计数/平台/CI 与最终收尾尚待执行；用户明确延期的三类真实环境单独保留待验。

## 范围与推进顺序

范围以 [CLI 设计 §14.4](spec/STYLE-cli-design.md#144-之后) 为准，另承接 [规则 §3.10](spec/STYLE-spec-rules.md#310-target-native) 留待 M3 的 native 扩展评估；不能只沿用 M2 页末的简略 backlog 而漏掉 Claude 插件路线、精确计数和 `unknown` 匹配。

| 顺序 | 阶段 | 任务 |
|---|---|---|
| 1 | 基线与架构 | M3-00、M3-01、M3-D0、M3-T00 |
| 2 | 诊断 | M3-D1、M3-T10、M3-T11：`explain`、`probe` |
| 3 | 来源与更新 | M3-D2、M3-T20–T22：Git 来源、`update`、`--frozen` |
| 4 | 安装路线 | M3-D3、M3-T30–T32：项目级路线、Claude 云端会话、Claude 插件路线 |
| 5 | 运行时 | M3-D4–D8、M3-T40–T45：`refresh`、事件内容围栏、`provider`、`unknown`、用户 effort 映射、规则级 `fail: closed` |
| 6 | 计数与平台 | M3-D9–D11、M3-T50、M3-T60、M3-T70：`--count=exact`、native 扩展评估、Windows |
| 7 | 收尾 | M3-T80、M3-PREP-01/02、M3-REAL-01–04、M3-CI-01、M3-FINAL、M3-DOCS-01 |

按表内顺序和各条依赖逐项推进，完成一个可独立验收的任务后验证、更新文档并提交，再进入下一项。2026-10-17 用户在 T30/A6 完成后要求“继续推进其他 M3 阶段计划”，D4 的前置据此改为已完成的 T30；T31/T32 保留暂缓，不阻止运行时及后续阶段的设计推进，也不视为已完成或已批准。设计需要原生探针时可先做该设计范围的隔离调查，不能据此跳过实现或真实验收；准备工作可以随对应设计展开，前置实现未通过时不能关闭最终验收。

M3 继续完整支持 Codex 与 Claude Code；其他 harness 的完整接入留在 M4。M2 的 replace 回退、token 估算系数和既有默认语义继续有效，变更须由对应设计项给出依据。Codex 会话中途 `/model` 强度不可见等未被 M3 范围纳入的限制，不在本轮自动扩展。

## 状态、编号与证据

- `[x]` 已完成，且有命令、fixture、golden 或真实运行证据。
- `[ ]` 尚未开始或尚未执行；`[~]` 部分完成或被环境阻塞。
- `[!]` 已知问题或设计风险；`[?]` 等待真实环境验证。
- `M3-D*` 为设计定稿，`M3-T*` 为实现，`M3-CARRY-*` 为遗留，`M3-RV-*` 为审查风险，`M3-PREP-*` 为真实验收准备；测试编号沿用 BASE、A、B、C、CI、DOCS，并增加 TOKEN、W。
- 每条写明目标、依赖、产出和测试编号。设计项先回写相关 `docs/spec`，再进入实现。2026-10-17 用户明确“开始推进，所有评审全部通过，完全完成M3”，批准后续设计由 agent 定稿推进，并恢复 T31/T32；这项授权不替代测试和真实环境证据。
- 提交消息带任务或测试编号，例如 `docs(M3-00): ...`、`feat(M3-T10): ...`。未通过的测试保持原状态，记录复现命令、原因、影响和下一步。

## 当前基线

- [x] **M3-00 建立 M3 文档**：目标是完整建档并切换当前里程碑入口。依赖：M2 已关闭、用户已确认 M3 范围。产出：本任务清单、测试矩阵、`AGENTS.md` 当前入口、主设计状态和追加日志；M1/M2 归档不改。测试：M3-00。证据：五份当前文档的本地链接/锚点、围栏、44 项任务与 39 项测试的编号/映射、依赖无环、归档/追加日志及 `git diff --check` 通过，见 `docs/PROGRESS.md` M3-00；建档时仅关闭本项。
- [x] **M3-01 基线回归**：目标是固定 M3 改动前的可重复对照。依赖：M3-00。产出：建档提交 HEAD、环境、双 module fresh test/vet、采集器、固定构建 SHA256、40 项 smoke、16 组旧 rules 迁移与 12 组 replace 回退的运行结果。测试：M3-BASE-01。证据：[新运行报告](review/M3-BASE-01-2026-10-17.json) 固定 clean `7586cca`、Go 1.27.0 darwin/arm64；双 module fresh test/vet、59 项采集器及三份脚本全部通过，数量与 M2 对照一致。`GOWORK=off` 构建 SHA256 为 `66ed7d4c21eb1e03a6b4b1c93269535ed96be58cc694d90c57dd5843582103c4`，旧迁移二进制取自 `74a473d`；采集器首轮回环端口沙箱阻塞和放行后通过均保留。M2 历史结果未代替本轮执行，未运行真实 harness 或远端 CI。

建档起点为 clean `b7af4e75d0967ed5f7e8728f86f80bc7a930114c`。M2 最终 CI 固定在 `154506f`、run `39000483969`，Ubuntu/macOS 的双 module test/vet、59 项采集器、40 项 smoke 与 12 组回退均通过；M3 新基线已按 M3-BASE-01 单独执行。现有三条安装路线、native、stats、跨包去重、Claude rules/迁移和 replace 回退构成兼容性基线。

## 遗留与风险

- [?] **M3-CARRY-01 官方 token 计数**（承接 M2-CARRY-01，原 M1-FINAL-09）：目标是完成固定 40 条、中英各半语料的两家官方同结构基线增量对照。依赖：独立 API key 和可用计数模型。产出：模型、语料/请求摘要、逐条计数及与生产估算的比较；缺 key 时仍各 0/40，非阻塞遗留不因 M3 建档关闭。测试：M3-CARRY-01。原阻塞见 [报告](review/M1-FINAL-09-COUNTING-BLOCKED-2026-10-16.json)，[M2 补充验证](review/M2-ALT-01-2026-10-17.md) 不替代官方结果，不承诺模型误差范围。
- [x] **M3-RV-01 核心 harness 名硬编码**（承接 M2-RV-01）：目标是消除 `when.harness.name`、`requires.harness`、`recommends` 校验对两家名称的写死依赖。依赖：M3-D0、M3-T00。产出：注册表驱动的统一校验和回归证据；已由 M3-T00 修复并通过 M3-A0，见 [验收报告](review/M3-A0-2026-10-17.json)；三处校验不再维护内置名称名单。engine 的两处既有 Claude 输出判断继续由 D8 评估，不属于本风险的名称成员关系范围。测试：M3-A0。来源：[条件校验](../core/spec/validate_conditions.go)、[包级校验](../core/spec/validate_package.go)、[注册表](../internal/adapter/registry.go)。

M3-CARRY-01 与 `--count=exact` 产品验收分别记录；前者的非阻塞决定不自动豁免后者的真实能力验证。其他新增环境阻塞也不自动豁免里程碑验收。2026-10-17 用户另明确延期 Claude 托管云端、Windows 主机真实运行、Anthropic 独立 API 计数（见本文件末尾范围决定）。

## 设计定稿

本节列出需要回答的问题，不预先确定新 schema、JSON 格式、退出码或配置优先级。每项产出均须说明接口、数据流、失败行为、兼容策略和测试细化；涉及外部能力时固定版本、来源和实测范围，结论记为符合预期、回退方案生效或阻塞。任何范围调整单独记录用户决定，不能静默删项。

- [x] **M3-D0 注册表校验边界**：目标是明确通用校验如何接收合法 harness 集合，兼容现有调用与诊断。依赖：M3-01。产出：[定稿设计](review/M3-D0-REGISTRY-VALIDATION-2026-10-17.md) 与 [CLI §3.1](spec/STYLE-cli-design.md#31-m3-d0-注册表校验边界)，已覆盖名称全集/native 子集、结构/完整校验接口、三处名称及预算/重编译调用链、诊断兼容和十组验收，并回写包/条件规范。测试：M3-A0。2026-10-17 用户在评审问题后回复“继续”，确认 `Validate` 的结构校验兼容选择与实现范围；文档检查通过，见 `PROGRESS.md` M3-D0 定稿记录。M3-T00 已实现、M3-A0 已通过，结果见 [验收报告](review/M3-A0-2026-10-17.json)。
- [x] **M3-D1 诊断契约**：目标是明确 `style explain --harness h --event e --payload f.json` 和 `style probe [--harness h]` 的输入、输出及成功证据。依赖：M3-T00。产出：[定稿契约](review/M3-D1-DIAGNOSTICS-2026-10-17.md) 与 [CLI §4.1](spec/STYLE-cli-design.md#41-m3-d1-诊断契约评审稿)、运行时 §8.9、编译产物 §9.7；拟定原生 event/默认空状态/显式 snapshot 与 facts、共享 trace、绑定 index 的可选编译说明、现有 hook 观察及租约清理，A1/A2 各细化 12 组。测试：M3-A1、M3-A2、M3-B1、M3-C1。源码调查固定 clean `bf27863`，文档检查见 [D1 文档报告](review/M3-D1-DOCS-2026-10-17.json)；2026-10-17 用户在明确评审问题后回复“继续”，确认本稿三项产品选择；[定稿检查](review/M3-D1-APPROVED-2026-10-17.json) 通过，关闭 D1。M3-T10/M3-A1、M3-T11/M3-A2 均已独立完成；B1/C1 真实验收保持待验。
- [x] **M3-D2 来源与锁定契约**：目标是明确 Git 来源、`update`、`install --frozen` 的来源表达、revision 锁定和授权边界。依赖：M3-T11。产出：[评审稿](review/M3-D2-SOURCES-2026-10-17.md) 与 [CLI §4.2](spec/STYLE-cli-design.md#42-m3-d2-来源与锁定评审稿)、包 §1.8、编译产物 §10.1；拟定显式公共 HTTPS/完整 ref/原始对象快照、私有导出缓存、lock 1/2 兼容、明确选包更新/未选批准基线、共享提交保护与回滚、整锁离线 frozen。测试：M3-A3–A5。调查固定 clean `a8fad75a17d0dbc1a38cc5825ec17cfda5b780ab`，本地 Git 2.54.0 的 12 项设计探针通过，前两次 fixture 失败保留；文档检查见 [报告](review/M3-D2-DOCS-2026-10-17.json)。A3/A4/A5 各细化 12 组断言；2026-10-17 用户在明确评审问题后回复“继续”，确认公共 HTTPS、lock 2/明确选包更新和当前整锁离线恢复的三项选择；[定稿检查](review/M3-D2-APPROVED-2026-10-17.json) 通过，仅关闭 D2。
- [x] **M3-D3 安装范围与新路线**：目标是明确用户级/项目级安装隔离，以及 Claude 云端和插件载体的可行路径。依赖：M3-T22。产出：项目根/存储/index/state 选择、同名包和多 clone 边界、原生信任与卸载恢复、Claude 云端交付条件及插件注册方案；固定原生证据后回写路线设计。测试：M3-A6–A8、M3-B2、M3-C2–C4。T22 推进期间已按本清单的隔离调查约定完成[原生调查](review/M3-D3-INVESTIGATION-2026-10-17.md)（Codex 0.159.3 与 Claude Code 2.1.301，均用 stub 模型；Claude 运行经用户授权，真实 `~/.claude` 未变），只固定事实和待决问题；现已复核原稿并补做五个静态加载调查场景；[评审稿](review/M3-D3-ROUTES-2026-10-17.md)已回写规范，A6/A7/A8 分别细化 12/6/6 组，2026-10-17 用户明确“先只做项目级吧，继续”，项目部分定稿并允许进入 T30/A6；云端/插件继续待评审，本项保持部分完成。定稿检查见 [项目批准报告](review/M3-D3-PROJECT-APPROVED-2026-10-17.json)。项目部分现已由 T30 实现并通过 [A6](review/M3-A6-2026-10-17.json)，整个 D3 的设计已由用户于 2026-10-17 全部批准；T31/T32 恢复实施，功能验收仍独立。
- [x] **M3-D4 refresh 语义**：目标是明确隔 N 回合补发如何计回合、何时重置及如何影响预算。依赖：M3-T30（用户授权继续其他阶段，T31/T32 暂缓）。产出：[评审稿](review/M3-D4-REFRESH-2026-10-17.md)及规则/运行时/编译/CLI 规范入口，涵盖提交回合独立时钟、同正文滚动间隔、repeat/compact/resume/fork/子 agent、跨进程保护、format 1/2 门槛与完整预算；A9 细化为 12 组。测试：M3-A9、M3-B3、M3-C5。证据：[设计检查报告](review/M3-D4-DOCS-2026-10-17.json)固定源码调查、307 条旧行为回归与 8 项基线探针、官方文档摘要和文档检查；用户于 2026-10-17 回复“ok”，确认本稿三项选择，[定稿检查](review/M3-D4-APPROVED-2026-10-17.json)通过，定稿当时仅关闭 D4，允许进入 T40；后续 T40/A9 已另经实现验收，B3/C5 保持待验。
- [x] **M3-D5 事件内容围栏**：目标是明确可进入模板的事件字段及其不可信内容边界。依赖：M3-T40。产出：显式使用方式、围栏转义、长度限制、reads 与预算处理、日志脱敏边界；回写模板和运行时安全章节。测试：M3-A10、M3-B3、M3-C5。证据：[D5 定稿](review/M3-D5-EVENT-CONTENT-2026-10-17.md)明确 event. 显式引用、reads.event、512 字符截断与单行 JSON 围栏、warn 显式 model_text、index 3 和八组 A10；按用户全部评审授权直接进入实现，设计通过不替代 A10。
- [x] **M3-D6 provider 与 unknown**：目标是区分模型服务 provider 与 harness 名，并确定缺失数据的显式匹配范围。依赖：M3-T41。产出：可核验的数据来源、缺失/空值/未知原始值区别、`unknown` 的适用字段与比较行为，以及旧规则兼容；回写条件、数据来源和编译规范。测试：M3-A11、M3-A12、M3-B3、M3-C5。定稿：[D6 契约](review/M3-D6-PROVIDER-UNKNOWN-2026-10-17.md)明确配置请求、Codex provider 专属层级、Claude 环境白名单、按需缓存/重放与 index 4、四字段 unknown；十组验收待实现。
- [ ] **M3-D7 用户 effort 映射**：目标是明确用户映射的配置入口、覆盖关系和可追溯性。依赖：M3-T43。产出：与内置映射、raw/approx、未知值及会话缓存的关系，doctor/plan 说明；回写 effort 章节和 CLI 设计。测试：M3-A13、M3-B3、M3-C5。
- [ ] **M3-D8 规则级 fail-closed**：目标是明确 `fail: closed` 的适用动作、事件、可判定错误和原生能力门槛。依赖：M3-T44。产出：与现有 Claude 安装级 `--fail-closed` 的关系、混合规则处理、index 不可读时的可实现边界和不支持路线的报告；评估 D0 调查记录的 engine prompt suppression 名称判断及其能力边界，回写运行时与安装设计。测试：M3-A14、M3-B3、M3-C5。
- [ ] **M3-D9 精确计数边界**：目标是明确 `--count=exact` 用于哪些命令、计什么文本以及精确性的适用范围。依赖：M3-T45；不依赖 M3-CARRY-01 完成。产出：本地 OpenAI 分词与 Anthropic 计数接口的适配边界、模型/编码披露、凭据与联网要求、失败行为和估算模式兼容；回写 token 设计，不等同于账单保证。测试：M3-A15、M3-TOKEN-01。
- [ ] **M3-D10 native 扩展评估**：目标是评估 Claude `async`、`onFailure` 和非 command hook。依赖：M3-T50。产出：逐项支持/不支持结论及来源/实测、归属和信任方案、披露与旧四字段兼容策略；回写规则 §3.10 与路线设计。测试：M3-A16、M3-B4、M3-C6。评估完成不等于未支持的类型已实现。
- [ ] **M3-D11 Windows 支持边界**：目标是确定 Windows 路径、命令执行、归属标记、文件事务和支持路线。依赖：M3-T60。产出：shell/启动器、native stdin/退出码、权限/锁与原子写入适配、平台 CI 和真实验收方案；回写 CLI/规则设计。测试：M3-A17、M3-W1、M3-CI-01。

## 离线实现

设计定稿后实现。core 只消费通用能力和数据；原生事件、payload/响应 codec、hook 配置渲染与 owner 合并继续归 agenthooks。需要调整边界时，在对应任务记录原因、影响和双 module 测试。

- [x] **M3-T00 统一 harness 校验**：目标是修复 M3-RV-01。依赖：M3-D0。产出：注册表注入所有相关校验路径、假 harness 回归和现有诊断兼容；保持 core 不依赖具体 adapter。测试：M3-A0。证据：[完整报告](review/M3-A0-2026-10-17.json)。`ValidationCatalog` 由 Registry 生成隔离快照，完整入口统一校验普通名称/native 子集；app 入包及预算/重编译共用诊断，auto/预算遍历全部注册项。14 个新测试函数、205 个叶子用例、33 组文件保全、32 组旧/新 CLI 字节对照与 8 个架构 guard fixture 通过；双 module fresh test/vet、相关四包 race、59 采集器、40 smoke、16 旧迁移、12 回退均通过，agenthooks/旧 golden 未变。允许进入 M3-D1。
- [x] **M3-T10 explain**：目标是按 D1 提供可重复的离线解释。依赖：M3-D1。产出：CLI/app、共享 Match/Process trace、私有状态重放、所选 snapshot、绑定 facts、真实 codec wire 与管理退出码；build/explain.json 绑定 index 并参与安装/sync/trust/卸载事务，旧安装只读 index-only 回退。测试：M3-A1。证据：[完整报告](review/M3-A1-2026-10-17.json)、[使用说明](STYLE-EXPLAIN.md) 与 [固定 fixture/golden](../testdata/explain/)。26 个新测试函数、145 个叶子用例及 29 组生命周期与普通 runtime 对照通过；双 module fresh test/vet、相关八包和 agenthooks 全 module race、59 采集器、40 smoke、16 真旧迁移、12 回退通过。agenthooks 新增可选离线依赖/结构化结果观察，继续独占 codec/事件表与配置渲染；默认 wire 与旧格式/golden 保持；probe 现已另经 T11/A2 通过，真实验收保持待验。
- [x] **M3-T11 probe**：目标是按 D1 观察已安装 STYLE runtime 回调。依赖：M3-D1、M3-T10。产出：CLI、agenthooks 归属/订阅预检、私有短租约/安全收据、OS 出生身份与 executable/路线绑定、多目标/超时/中断/漂移、清理和崩溃恢复；不自动改配置、检测版本或请求模型。测试：M3-A2；真实验收另见 M3-B1、M3-C1。证据：[完整报告](review/M3-A2-2026-10-17.json)、[使用说明](STYLE-PROBE.md) 和内嵌 fixture。28 个新函数/181 个叶子用例，三条现有路线离线 OS 进程链、15 组普通 runner 对照、真实信号及 PID/租约竞争通过；双 module fresh test/vet、相关六包和 agenthooks 全 module race、59 采集器、40 smoke、16 真旧迁移、12 回退及最终 CLI 通过。两家各 200 次普通 hook 进程采样 p95 为 9.56ms/8.71ms，低于 50ms 门槛；B1/C1、Windows、远端 CI 与官方计数未关闭。
- [x] **M3-T20 Git 来源**：目标是安装锁定的远程包。依赖：M3-D2。产出：显式公共 git+https、规范 ref/包路径、固定 OID 原始快照与完整导出缓存；validate/plan/install 共用校验、Git 来源预览和 lock 1/2 保真，sync/trust 拒绝 Git 漂移，管理写入共用提交保护。测试：M3-A3。证据：[完整报告](review/M3-A3-2026-10-17.json)、[使用说明](STYLE-SOURCES.md) 和 [旧二进制门禁脚本](../scripts/check_m3_git_source.py)。30 个新函数/202 个叶子用例，真实 SHA-1/SHA-256 仓库与受控 TLS、三路线生命周期/回滚、17 项 built CLI/旧版门禁均通过；双 module fresh test/vet、相关五包及 agenthooks/install race、59 采集器、40 smoke、16 真旧迁移、12 回退通过。agenthooks 仅增加 renderer 管理输入路径枚举，继续负责协议/渲染/owner/文件事务。该次 T20 验收未关闭 T21 update、T22 frozen、真实投递、Windows 或远端 CI。
- [x] **M3-T21 update**：目标是按 D2 更新既有来源并保留授权边界。依赖：M3-T20。产出：明确选包、更新预览、版本/来源与 native 变更确认、未选批准基线、联合编译和多包事务/回滚错误报告，不连带批准其他包。测试：M3-A4。证据：[完整报告](review/M3-A4-2026-10-17.json)、[使用说明](STYLE-SOURCES.md#明确选包更新) 与 [built CLI 脚本](../scripts/check_m3_update.py)。31 个新函数/244 个叶子用例、16 份新增 golden、逐包/逐文件故障恢复、三路线 native/旧安装与 24 项 built CLI/基线 runtime 对照均通过；双 module fresh test/vet、app/source 和 agenthooks 全 module race、59 采集器、40 smoke、16 真旧迁移、12 回退及 17 Git 门禁通过。agenthooks 保留协议/渲染/owner/事务职责，仅完善类型化恢复错误、备份 mode/mtime 和恢复失败保留；T22/A5、真实投递、Windows 和远端 CI 不在本项关闭。
- [x] **M3-T22 frozen 安装**：目标是按锁文件重现安装。依赖：M3-T21。产出：按 D2 定稿实现 `install --frozen` 的整锁离线恢复及本地/link/Git 兼容，保全锁字节/绑定/批准边界，缺必要内容或结果不匹配给出明确结果。测试：M3-A5。证据：[完整报告](review/M3-A5-2026-10-17.json)、[使用说明](STYLE-SOURCES.md#整锁离线恢复) 和 [built CLI 脚本](../scripts/check_m3_frozen.py)。21 个新函数/209 个叶子用例、16 份新增 golden、三路线离线恢复与逐包/逐文件回滚通过；双 module fresh test 3,163 条 pass/0 fail，vet、app/source race 1,299 条 pass、59 采集器、30 frozen CLI、24 update CLI、40 smoke、16 真旧迁移、12 回退及 17 Git 门禁通过。锁 bytes/mode/mtime/身份不变、零 Git/检测/缓存写入；agenthooks 无改动。无法重现旧 rendered 变更集摘要的部分配置修复及缺失 Claude 已登记 rules 保守拒绝，程序路径变更须先普通安装更新绑定。跨根/项目仍属 D3，真实投递/Windows/CI 未关闭。
- [x] **M3-T30 项目级路线**：目标是支持 Codex/Claude 的本地项目安装。依赖：M3-D3 项目部分（已获用户确认）。产出：显式 `--project`、两条项目路线、规范根私有隔离、lock 3/声明/binding、用户同名冲突阻断、clone 整批重现确认及管理/诊断的一致生命周期。测试：M3-A6；真实验收 M3-B2、M3-C2。证据：[完整报告](review/M3-A6-2026-10-17.json)、[使用说明](STYLE-PROJECTS.md)、[CLI 检查脚本](../scripts/check_m3_project.py) 与 44 份新增 golden。33 个新函数/166 个叶子用例、18 项 built CLI/真实 Git worktree 检查、逐包/逐文件故障恢复与旧程序 format 3 拒绝通过；双 module fresh test 3,347 pass/0 fail、vet、相关 race、68 采集器及全部旧安装/来源门禁通过，24 项既有 opt-in skip 单列。agenthooks 保留协议/配置渲染/owner/文件事务职责，仅扩展项目 hook 与用户 trust 分根和归属摘要。仅关闭 T30/A6；D3 部分完成，云端/插件暂缓，真实 B2/C2、Windows 和远端 CI 待验。
- [x] **M3-T31 Claude 云端交付**：目标是按 D3 提供经证据确认的项目交付方式（2026-10-17 用户恢复完整 M3 范围）。依赖：M3-T30。产出：所需项目产物、加载条件、路径/运行时可移植性检查和操作说明；无可用真实环境时明确待验，不能用本地配置存在代替云端生效。测试：M3-A7、M3-C3。证据：[A7 报告](review/M3-A7-2026-10-17.json)、[使用说明](STYLE-CLOUD.md)与 [CLI 检查](../scripts/check_m3_cloud.py)。54 条新 pass/race、根 module 2,989 pass、vet、8 项 built CLI 与 40 smoke 通过；明确导出批准 bundle、相对来源、私有云状态、摘要/版本拒绝、全预览、取消恢复、重导出与最后卸载。仅关闭产物验收，真实 C3 保持 `[?]`。
- [x] **M3-T32 Claude 插件路线**：目标是按 D3 支持插件安装与卸载（2026-10-17 用户恢复完整 M3 范围）。依赖：M3-T31。产出：插件产物、登记/启用边界、与用户/项目路线切换及残留检查，复用 agenthooks 渲染。测试：M3-A8、M3-C4。证据：[A8 报告](review/M3-A8-2026-10-17.json)、[插件说明](STYLE-CLAUDE-PLUGIN.md)与 [CLI 检查](../scripts/check_m3_claude_plugin.py)。本地 marketplace/launcher、只读登记检查、互斥切换、多包/native 更新、lock 3 旧程序门禁与取消恢复通过；双 module fresh test 3,555 pass/0 fail、vet、专项 race、8 项插件 CLI、8 项云端 CLI、40 smoke 与旧迁移/replace 回归通过。仅关闭离线 A8，C4 真实登记/投递仍待验。
- [x] **M3-T40 refresh**：目标是按 D4 支持周期补发。依赖：M3-D4。产出：校验/plan/compile/runtime/state/stats 的一致实现和兼容 fixture。测试：M3-A9、M3-B3、M3-C5。证据：[完整报告](review/M3-A9-2026-10-17.json)、[使用说明](STYLE-REFRESH.md) 与 [built CLI 门禁](../scripts/check_m3_refresh.py)。29 个通过的新测试函数/85 个叶子用例（98 条含分组 pass），另有进程 helper 由父测试启动验证；覆盖滚动时钟/原生 ID、旧 claim 升级、生命周期、跨包去重、OS 锁/崩溃/清理、写入与回滚故障、只读 snapshot、严格 format 1/2 和五路线审批事务。双 module fresh test 3,445 pass/0 fail、vet、相关 race、68 采集器、50 refresh CLI 及全部旧门禁通过；五路线各 50 次进程采样 p95 为 9.269–16.919ms。24 项既有 opt-in skip 与 helper 的父进程非工作模式 skip 单列；agenthooks/go.mod/旧 golden 不改。仅关闭 T40/A9；真实 B3/C5、Windows、远端 CI 与官方计数不关闭。
- [x] **M3-T41 事件内容模板**：目标是按 D5 开放经围栏处理的事件内容。依赖：M3-D5。产出：字段访问、转义/截断、模板与预算支持及敏感数据日志检查。测试：M3-A10、M3-B3、M3-C5。证据：[A10 报告](review/M3-A10-2026-10-17.json)和[使用说明](STYLE-EVENT-CONTENT.md)。11 个新函数、最终专项/race 57 pass；根 module 全量 3,079 pass/0 fail，35 项五路线 CLI、8 云端/8 插件 CLI、40 smoke 与 refresh 回归通过。agenthooks/旧 golden 未变，真实 B3/C5 待验。
- [ ] **M3-T42 provider 条件**：目标是按 D6 使用有来源依据的模型服务信息。依赖：M3-D6。产出：adapter 数据声明、状态更新和条件评估，区分 harness 标识与服务 provider。测试：M3-A11、M3-B3、M3-C5。
- [ ] **M3-T43 unknown 匹配**：目标是按 D6 支持显式缺失值匹配。依赖：M3-T42。产出：校验、编译、匹配及 explain/plan 说明，旧规则缺值行为兼容。测试：M3-A12、M3-B3、M3-C5。
- [ ] **M3-T44 用户 effort 映射**：目标是按 D7 支持可追溯的用户配置。依赖：M3-D7。产出：配置读取、映射与缓存失效、raw/approx 和 doctor/plan 诊断。测试：M3-A13、M3-B3、M3-C5。
- [ ] **M3-T45 规则级 fail-closed**：目标是按 D8 实现被支持的失败策略。依赖：M3-D8。产出：规则校验、能力规划、运行时错误处理及安装级兼容；不支持的能力明确拒绝或按批准设计回退。测试：M3-A14、M3-B3、M3-C5。
- [ ] **M3-T50 exact 计数**：目标是按 D9 提供显式计数模式。依赖：M3-D9。产出：计数实现与文本/JSON 报告、可选依赖/凭据处理和确定性测试；缺 key 或联网失败不能标为计数成功。测试：M3-A15、M3-TOKEN-01。
- [ ] **M3-T60 native 扩展结论落地**：目标是落实 D10 的逐项结论。依赖：M3-D10。产出：获支持字段/类型的校验、plan、配置/信任生命周期；不支持项的诊断与依据，保留旧四字段兼容。测试：M3-A16、M3-B4、M3-C6。
- [ ] **M3-T70 Windows 适配**：目标是按 D11 完成支持范围内的平台实现。依赖：M3-D11。产出：路径、启动器、native 命令、状态锁与文件事务适配及 Windows CI 接线。测试：M3-A17、M3-W1、M3-CI-01。
- [ ] **M3-T80 conformance 与升级回归**：目标是核对新能力组合及 M2 兼容性。依赖：M3-T70。产出：共享路线/协议回归、旧包/锁/index/state 的升级 fixture、组合能力和失败回滚覆盖，保留旧 golden 的有据变更说明。测试：M3-A18。

## 真实环境准备与验收

本轮不创建未就绪的手册链接；`STYLE-M3-MANUAL.md` 由 PREP-01 建立，PREP-02 补充特殊环境。沿用隔离配置、固定二进制和私有基线方法；原始 prompt、工具正文和凭据不入库，归档白名单、摘要及可复核的必要事实。交互操作沿用既有授权范围，登录/钥匙串等需要用户交互时单列。

- [ ] **M3-PREP-01 两家真实验收准备**：目标是准备可复核的 Codex/Claude runbook。依赖：M3-D1、M3-T11；后续功能按各自设计批准后补齐。产出：M3 手册、隔离启动/观察/恢复工具、固定构建与采集器自测。测试：M3-P1。只关闭准备，不关闭 B/C 条目。
- [ ] **M3-PREP-02 云端、计数与 Windows 准备**：目标是明确特殊环境的执行条件。依赖：M3-PREP-01、M3-D3、M3-D9、M3-D11。产出：环境可用性、项目交付/账号/计数模型/Windows shell 前提、白名单采集和恢复步骤；缺环境记录阻塞，不以 stub 替代真实验收。测试：M3-P2。
- [?] **M3-REAL-01 Codex 真实验收**：目标是验证受支持路线上的实际调用与投递。依赖：M3-T80、M3-PREP-01。产出：固定版本下的诊断、项目隔离、运行时和 native 证据，以及卸载恢复；既有 user/plugin 两路线独立核对，新增项目路线另测。测试：M3-B1–B4。
- [?] **M3-REAL-02 Claude 真实验收**：目标是验证本地用户/项目/插件与云端支持范围。依赖：M3-T80、M3-PREP-01、M3-PREP-02。产出：真实回调、加载/投递、信任、运行时与卸载恢复证据；本地与云端分开判定。测试：M3-C1–C6。
- [?] **M3-REAL-03 exact 真实计数**：目标是验证 D9 宣称的计数能力。依赖：M3-T50、M3-PREP-02，以及对应编码/接口环境。产出：真实计数结果、模型/编码版本、文本与请求摘要及报告范围；与旧校准遗留分别记录。测试：M3-TOKEN-01。
- [?] **M3-REAL-04 Windows 真实验收**：目标是验证 Windows 上实际安装和 hook 执行。依赖：M3-T80、M3-PREP-02。产出：原生 shell、受支持 harness 路线、stdin/退出码、状态并发和卸载恢复证据；交叉编译不视为通过。测试：M3-W1。

## 里程碑收尾

- [ ] **M3-CI-01 跨平台 CI**：目标是核对最终受支持平台的发布形态。依赖：M3-T80。产出：Ubuntu/macOS/Windows 的双 module test/vet/build、采集器和平台 smoke；记录精确 headSha、逐 job 结果及失败修正，适用时验证 `GOWORK=off`。测试：M3-CI-01。
- [ ] **M3-FINAL 最终回归**：目标是关闭完整交付范围的验收。依赖：M3-REAL-01–04 中未延期的验收、M3-CI-01。产出：离线、升级/失败恢复、两家真实环境、exact 与 Windows 的一致证据；缺环境的必需验收不能自动转为非阻塞。测试：M3-FINAL。M3-CARRY-01 保留独立非阻塞状态。
- [ ] **M3-DOCS-01 总文档验收**：目标是同步最终能力和限制并关闭 M3。依赖：M3-FINAL。产出：规范、教程、adapter 总览、M3 手册与任务/测试/进度记录一致，所有状态有证据，归档不改；关闭时提示用户规划下一里程碑。测试：M3-DOCS-01。

M3-T00/M3-A0、M3-D1、M3-T10/M3-A1、M3-T11/M3-A2、M3-D2 和 M3-T20–T22/M3-A3–A5 已完成。M3-D3 的项目部分已获用户确认，M3-T30/M3-A6 已完成。云端 T31/A7 与插件 T32/A8 暂缓，整个 D3 仍为部分完成；用户现已授权继续其他 M3 阶段，D4 已确认并定稿，T40/A9 已完成；2026-10-17 用户恢复完整 M3 范围并通过全部后续评审，当前先推进 T31/A7、T32/A8，再按依赖完成 D5–D11 与收尾。

## 后续 backlog

- [ ] **M4**：Gemini、OpenCode、Cursor、Copilot、Kimi 等额外 harness，每家独立 adapter；相关开关/预算型 effort 来源随对应 harness 研究。
- [ ] **M3 之后再议**：Codex 会话中途 `/model` 强度变化的可靠来源；现有只读配置边界继续有效。

## 2026-10-17 完整实施授权

用户明确“开始推进，所有评审全部通过，完全完成M3”。此前 T31/T32 的暂缓决定由本条新授权覆盖；D3 §6–7 定稿，后续 D5–D11 由 agent 先记录接口、失败行为和验收断言，再直接实施。真实验收仍需实际执行；缺环境不得改为通过或自动豁免。

## 2026-10-17 特殊真实环境延期

用户在收到所需环境清单后明确“这些等以后再测试，先排除这些，继续”。据此延期 Claude 托管云端 C3、Windows 主机实际 harness 验收 W1/REAL-04、Anthropic 独立计数 API 的 TOKEN-01 子项及相应 REAL-03 子项；三者保持 `[?]` 并作为后续真实验收，不在本次 M3 完成门槛内，不标记通过。T31/T50/T70 等实现与离线验收、Windows CI、可本地执行的精确编码和其他本地真实 B/C 验收仍在当前范围。FINAL/DOCS 按调整后的完整范围判定，报告须显式列出延期项目与用户决定。

T31/A7、T32/A8 现已完成离线实现验收；当前进入 D5/T41。D3 设计已定稿，早期暂缓记载不再代表当前状态。

T41/A10 已完成；当前按依赖推进 D6/T42/T43，T31/T32 早期暂缓记载已由最新授权覆盖。
