# STYLE CLI M3 测试清单

更新时间：2026-10-17

本文件记录 M3 测试矩阵与当前证据状态，任务及依赖见 [STYLE-M3-TASKS.md](STYLE-M3-TASKS.md)，运行记录追加到 [PROGRESS.md](PROGRESS.md)。[M1 测试](STYLE-M1-TESTS.md)、[M2 测试](STYLE-M2-TESTS.md) 已只读归档；历史通过结果不替代 M3 的新验收。

当前状态：M3-00 建档检查、M3-BASE-01 新基线已通过；A0–A18、准备、CI 和最终文档验收尚未执行。Codex B1–B4、Claude C1–C6、exact TOKEN-01 与 Windows W1 均待真实环境。M3-CARRY-01 官方校准仍两家各 0/40；核心硬编码风险尚未修复，M3-A0 为功能开发前置必做验收。

## 状态与验收规则

- `[x]` 已通过且有可重复证据；`[ ]` 尚未实现或尚未执行。
- `[~]` 部分验证或环境阻塞；`[!]` 失败或设计风险；`[?]` 等待真实环境。
- 每项记录源码提交、命令、环境、fixture/golden 或原生结果及退出码。未执行的预期不得写成结果。
- 设计定稿后细化对应断言、具体命令和新格式；本矩阵不预先批准未定 schema、配置优先级或失败策略。
- 准备自测、协议 stub、真实 harness 调用和模型投递分别取证。真实投递以原生加载/回调、完整上下文或请求证据交叉核对；模型回复和生成配置存在不能单独证明投递。

## 测试环境约定

标准 Go 验证沿用临时 `GOPATH`、`GOMODCACHE`、`GOCACHE`，清理 `CODEX_HOME`、`CODEX_SANDBOX`；检测测试还应隔离 HOME/PATH 和 `CLAUDE_CONFIG_DIR` 等输入，显式选择 harness，避免依赖本机安装。保留外部变量用于复现时另记环境和结果。

根 module 与 agenthooks module 的标准命令：

```sh
GOFLAGS='-mod=readonly -count=1' make test-go
GOFLAGS=-mod=readonly make vet-go
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_*.py'
```

Makefile 已为 Go 命令设置 `/tmp/style-cli-{gopath,gomodcache,gocache}` 并清理上述 Codex 变量。构建和定向测试也必须显式使用同样环境，例如：

```sh
m3_build_dir=$(mktemp -d /tmp/style-m3-build.XXXXXXXX)
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  GOFLAGS=-mod=readonly GOWORK=off \
  go build -trimpath -buildvcs=false -o "$m3_build_dir/style" ./cmd/style
PYTHONDONTWRITEBYTECODE=1 python3 scripts/smoke.py "$m3_build_dir/style"
PYTHONDONTWRITEBYTECODE=1 python3 scripts/check_m2_replace.py "$m3_build_dir/style"
```

旧 rules 迁移需要真正使用旧布局的二进制，不能把当前二进制同时当作旧版本。沿用 M2-T40 固定的 `74a473d`，在同一 shell 中导出到临时目录并构建，再执行 16 组升级：

```sh
m3_legacy_dir="$m3_build_dir/legacy-source"
mkdir "$m3_legacy_dir"
git archive 74a473d | tar -x -C "$m3_legacy_dir"
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  GOFLAGS=-mod=readonly GOWORK=off \
  go -C "$m3_legacy_dir" build -trimpath -buildvcs=false -o "$m3_build_dir/style-legacy" ./cmd/style
PYTHONDONTWRITEBYTECODE=1 python3 scripts/check_m2_rules_migration.py \
  --old-binary "$m3_build_dir/style-legacy" --binary "$m3_build_dir/style"
```

上述为仓库根目录的 POSIX 基线命令；浅克隆缺少旧 revision 时先取得该提交，不能跳过迁移检查后标通过。Windows 原生命令由 M3-D11/M3-T70 落实；交叉编译或在 POSIX 上运行 fixture 不能替代 M3-W1。race 测试针对改动的 engine/state/app/adapter 和 agenthooks install 路径执行，平台不支持时明确记录。固定时钟、输入与排序，临时配置/缓存/产物不入库。

## 文档与基线

- [x] **M3-00 文档建档验收**（M3-00）：新任务/测试文档覆盖主设计完整 M3 清单及 native 评估；所有任务有目标、依赖、产出和对应测试。五份当前文档的 50 个本地链接/锚点与围栏、三段 POSIX 命令的 `zsh -n`、44 项任务与 39 项测试的编号/映射、依赖无环和架构前置关系检查通过；当前入口与状态一致，77 份 M1/M2 历史文档/证据未变，进度日志仅追加，生产源码未改，`git diff --check` 通过。命令和结果见 `docs/PROGRESS.md` M3-00；建档时仅关闭本编号，未运行 Go 基线或真实模型请求。
- [x] **M3-BASE-01 起点回归**（M3-01）：在 clean 建档提交 `7586cca5744c697c1cc84a7cc0409e20cc39af40`、macOS 27.0.1 arm64 / Go 1.27.0 / Python 3.14.7 执行上述标准环境命令。双 module fresh test（30 个有测试的 package，均非 cached）与 vet 通过，采集器 59/59、smoke 40/40、`74a473d` 真实旧二进制迁移 16/16、replace 回退 12/12，数量未变。临时三缓存及宿主变量清理、逐步命令/退出码/日志摘要与 fixture 结果见 [新报告](review/M3-BASE-01-2026-10-17.json)。采集器首轮因 `127.0.0.1:0` 监听被沙箱拒绝而 58 过、1 错；仅采集器放行重跑后 59/59，原失败不覆盖。三份脚本统一使用 `GOWORK=off` 二进制 SHA256 `66ed7d4c21eb1e03a6b4b1c93269535ed96be58cc694d90c57dd5843582103c4`；另用 workspace 构建对照，与 M2 `32aef36d…cf81d` 逐字节相同。测试前后工作树干净，未变更源码/golden、未调用真实 harness 或远端 CI。

## 遗留项

- [?] **M3-CARRY-01 官方校准**（M3-CARRY-01，承接 M2-CARRY-01）：固定 40 条中英各半语料，在两家官方计数接口上取得同结构基线增量，逐条对照生产估算。独立 API key 仍缺，当前各 0/40；依 [采集器说明](../testdata/manual/README.md) 和 [原阻塞报告](review/M1-FINAL-09-COUNTING-BLOCKED-2026-10-16.json) 有条件时重跑，固定模型/语料/请求摘要并保留失败。非阻塞状态沿用，不因建档通过，也不以 [M2 本地编码/订阅补充](review/M2-ALT-01-2026-10-17.md) 替代。

## M3a：离线验收

- [ ] **M3-A0 注册表与架构边界**（M3-D0、M3-T00、M3-RV-01）：用仅注册假 harness、删去某家和两家现有注册表的 fixture，覆盖 `when.harness.name`、`requires.harness`、`recommends` 及 native 校验。目录/单文件包、所有 app 入包路径给出一致诊断；未知名称按定稿报错，core 不导入具体 adapter/agenthooks，旧包/诊断排序保持兼容。通过才关闭风险并开始后续功能。
- [ ] **M3-A1 explain**（M3-D1、M3-T10）：固定两家原生 payload、index 和隔离状态，解释命中/不命中、缺值、repeat、跨包去重、容量丢弃与最终输出；结果与真实 runtime/codec 相符。覆盖坏 payload/index、未知 harness/event 和 native 边界；以目录字节/权限/mtime 对照证明不改真实 state/config、不执行 native 命令，输出范围按 D1 固定。
- [ ] **M3-A2 probe**（M3-D1、M3-T11）：以隔离回调 fixture 验证关联、真实回调与仅安装的区分、超时/中断/未触发诊断及失败清理；他人配置与已有 STYLE 安装保持完整，不残留探针。此项仅验离线生命周期，实际 harness 调用由 B1/C1 验收。
- [ ] **M3-A3 Git 来源**（M3-D2、M3-T20）：用临时本地 Git 仓库模拟固定 revision、分支推进、同名/不同内容、目录与单文件包。覆盖不可达/缺 revision、非法包、路径逃逸/symlink 和取得中断；锁与内容摘要一致，失败不留半安装。取得源码不执行仓库代码；凭据与来源中的敏感字段不进入报告。
- [ ] **M3-A4 update**（M3-D2、M3-T21）：fixture 覆盖无变化、普通内容变化、native 变化、多个包和已授权 link 的交互。核对预览/批准边界、来源与锁/index/配置一致、取消/部分失败完整回滚；不会连带批准未选包或跳过既有 native 信任。
- [ ] **M3-A5 frozen**（M3-D2、M3-T22）：在隔离根重复锁定安装，核对 revision/摘要/有效规则可重复；覆盖锁缺失/不匹配、缓存缺失、离线条件、远端分支移动和本地/link 来源的定稿行为。失败结果明确，锁/已安装配置不发生未批准变化。
- [ ] **M3-A6 项目级路线**（M3-D3、M3-T30）：两家 fake/native 配置 fixture 覆盖用户/项目共存、不同项目同名包、嵌套目录、多 clone、范围显式选择与错误根。plan/install/list/sync/trust/doctor/uninstall 使用同一范围；index/state 不串用，现有文件字节/权限还原，native 归属与原生信任边界一致。
- [ ] **M3-A7 云端交付产物**（M3-D3、M3-T31）：按定稿对隔离 clone/项目产物验证必要文件、运行时路径与加载前提，不依赖本机用户配置或私有临时绝对路径。缺配置/运行时和不支持环境给出明确结果；本项仅验产物与可移植性，云端实际生效另见 C3。
- [ ] **M3-A8 Claude 插件路线**（M3-D3、M3-T32）：生成产物与 agenthooks 原生配置契约一致；覆盖重复安装、用户/项目/插件路线切换、多包、登记前后状态、native 及静态规则。卸载恢复外部文件/权限，无重复 handler 或残留；生成文件不等同于已登记或被真实 harness 加载。
- [ ] **M3-A9 refresh**（M3-D4、M3-T40）：固定回合序列验证补发边界、零/非法参数及与 repeat、compact、resume/fork、子 agent、并发去重的组合。plan 预算、runtime 实际输出和 stats 一致；旧规则与旧 state 升级按设计兼容，不产生重复补发。
- [ ] **M3-A10 事件内容围栏**（M3-D5、M3-T41）：覆盖提示、工具输入/输出中伪造围栏、嵌套标记、控制字符、Unicode、多行和超长内容。校验 reads/字段可用性、转义/截断与最终预算；缺字段和错误处理按设计一致。hook 日志、stats、doctor 中不泄露原始提示或工具正文，原生 stdout 仍符合白名单。
- [ ] **M3-A11 provider 来源**（M3-D6、M3-T42）：按 adapter 声明的来源验证已知/缺失/未知服务值与切换行为，不以 harness 名、模型别名或 URL 猜测 provider。plan/explain/runtime 对可用性一致；旧 `provider-unimplemented` 行为的变更有明确 fixture 和诊断依据。
- [ ] **M3-A12 unknown 匹配**（M3-D6、M3-T43）：覆盖批准字段上的缺失、空字符串、未知原始值、显式 `unknown` 和与已有值组合；比较表达式、否定条件和不支持字段按定稿处理。未使用新语法的旧规则仍遵循既有缺值语义，plan/compile/runtime/explain 一致。
- [ ] **M3-A13 用户 effort 映射**（M3-D7、M3-T44）：验证用户配置与内置映射的定稿优先级、无效/冲突项、未知 raw、approx 与会话缓存更新。隔离配置下 doctor/plan 说明来源，fixture 逐行核对两家原生值；缺值时不沿用旧档位，不伪造原生未提供的思考开关。
- [ ] **M3-A14 规则级 fail-closed**（M3-D8、M3-T45）：覆盖受支持/不支持事件与路线、混合规则、模板/状态/超时故障、index 缺失/损坏及既有安装级开关。错误分类、原生输出与退出码按定稿，默认放行兼容；无法识别规则的故障不声称已实现规则级判断，正常未命中不误拦。
- [ ] **M3-A15 exact 模式**（M3-D9、M3-T50）：固定本地编码及计数接口 fixture，核对计数对象、模型/编码披露、文本/JSON 与确定性排序。覆盖缺凭据/未知模型/编码、取消、限流/网络/接口错误和计数失败；不把估算静默标为 exact，不泄露 key。默认估算模式与旧预算兼容；stub 不能关闭 TOKEN-01 或 CARRY-01。
- [ ] **M3-A16 native 扩展**（M3-D10、M3-T60）：按评估结论覆盖获支持的 `async`/`onFailure`/非 command 类型及拒绝项、版本门槛、目录/单文件包和 from 覆盖。核对安装披露、sync/trust、owner/信任/卸载、原生执行约束及旧四字段兼容；native 仍不被虚构为 STYLE runtime stats/token 记录。
- [ ] **M3-A17 Windows 平台**（M3-D11、M3-T70）：平台 fixture 与 Windows 原生测试覆盖盘符/空格/Unicode/长路径、启动器引用、支持 shell、native 归属、stdin/退出码、并发状态锁和文件事务。验证失败回滚、外部文件/权限语义和多路线切换；POSIX 路径的既有行为保留。
- [ ] **M3-A18 conformance 与升级**（M3-T80）：重跑两家 codec/stdout 白名单和共享路线 conformance；覆盖旧包/锁/index/state、旧 Claude 布局与 native/stats/跨包去重/replace 回退。新功能组合验证隔离、排序、预算、去重与错误恢复；golden 变化必须关联批准设计，不用整体重录掩盖无关回归。

## 准备验证

- [ ] **M3-P1 两家 runbook/采集器**（M3-PREP-01）：固定私有 STYLE/harness/观察器摘要，隔离配置与工作目录，验证启动、关联、脱敏、安装/卸载干跑和 bytes/mode 恢复。采集器用 fixture 证明拒绝来源摘要漂移、缺原生证据与手工伪造回调；仅关闭准备。
- [ ] **M3-P2 特殊环境 runbook**（M3-PREP-02）：明确 Claude 云端项目运行条件、exact 的 API key/编码模型条件和 Windows 原生 shell/runner 条件。验证证据白名单、失败保留与恢复步骤可执行，环境不具备时记录缺项；准备检查不计入 C3/TOKEN-01/W1 成功。

## M3b：Codex 真实环境

既有 `codex-user`、`codex-plugin` 两路线独立记录，不拼接为一套通过；项目路线新增单独证据。记录固定版本、二进制和配置基线、原生 hook 来源/回调、完整上下文或请求摘要及卸载恢复；只声明实际观察到的信任范围。

- [?] **M3-B1 probe 与 explain**（M3-T10、M3-T11、M3-REAL-01）：真实会话触发可关联探针；采集原生 payload 后按 D1 在隔离状态解释，与实际命中/去重/输出核对。未触发、退出与清理结果明确，卸载后新会话无探针残留。
- [?] **M3-B2 项目路线与来源生命周期**（M3-T20–T22、M3-T30、M3-REAL-01）：在隔离项目中用锁定来源安装，核对项目配置被真实 harness 接受、实际投递和用户/其他项目隔离；更新/frozen 与 native 变更信任沿定稿执行。多 clone 信任不外推，卸载后配置与原有用户规则还原。
- [?] **M3-B3 运行时扩展**（M3-T40–T45、M3-REAL-01）：以固定真实事件序列核对 refresh、围栏、provider/unknown、effort 映射和受支持失败策略。原生回调/上下文、state、stats 与 explain 相符；不支持能力按批准设计证明边界，不把模型回复当投递证据。
- [?] **M3-B4 native 与既有路线回归**（M3-T60、M3-T80、M3-REAL-01）：真实触发仍受支持的 native 及 STYLE hook，核对来源、matcher/timeout、信任与共存。新 native 能力仅按 D10 的 Codex 支持范围取证；卸载恢复外部文件，后续新会话不再触发。

## M3b：Claude Code 真实环境

本地用户/项目/插件与云端分别记录。沿用隔离 runbook 和原生证据；交互操作沿既有授权范围执行，登录/钥匙串等需要用户交互时单列。固定版本、实际工作区信任范围和文件基线，安装/卸载分别启动新会话核对。

- [?] **M3-C1 probe 与 explain**（M3-T10、M3-T11、M3-REAL-02）：真实交互会话产生可关联原生回调，隔离解释与真实输出相符；核对未触发/超时诊断、清理与配置还原，不以采集器手工调用代替原生事件。
- [?] **M3-C2 本地项目路线**（M3-T30、M3-REAL-02）：原生加载/指令附件或请求证据证明项目规则生效，用户规则与其他项目隔离；核对信任、动态/native 与静态位置、卸载恢复和新会话无残留。
- [?] **M3-C3 云端项目交付**（M3-T31、M3-REAL-02）：在实际云端会话中验证定稿支持的交付路径、规则发现、hook 运行和完整投递；记录 clone/运行时条件及卸载或移除后的新会话。无真实环境保持待验，本地 fixture 不代替云端结果。
- [?] **M3-C4 插件路线**（M3-T32、M3-REAL-02）：实际登记/启用插件、原生加载并触发目标规则；与已有用户/项目路线无重复投递，核对原生信任边界、禁用/卸载和外部文件恢复。
- [?] **M3-C5 运行时扩展**（M3-T40–T45、M3-REAL-02）：真实事件序列核对 refresh、事件围栏、provider/unknown 和用户 effort 映射；对规则级 fail-closed 注入定稿允许的故障并验证工具行为，恢复后新会话正常。用原生证据与 state/stats 对照，明确静态规则 compact 等未覆盖边界。
- [?] **M3-C6 native 扩展**（M3-T60、M3-REAL-02）：对 D10 获支持的能力验证实际原生事件、输出/失败行为、版本门槛及与 STYLE hook 共存；不支持项保留结论。核对 trust/卸载恢复及新会话不再触发，旧四字段命令继续可用。

## 计数与 Windows 真实验收

- [?] **M3-TOKEN-01 exact 能力**（M3-T50、M3-REAL-03）：对定稿支持的本地编码和真实计数接口运行生产 CLI，记录计数文本/请求摘要、模型/编码版本和结果；缺 key/模型/编码/网络时保留失败与待验，输出语义不扩大为全请求/账单精度。该产品验收与固定 40 条的官方估算校准 CARRY-01 分开。
- [?] **M3-W1 Windows 原生运行**（M3-T70、M3-REAL-04）：在支持的 Windows shell 与实际 harness 下执行安装、原生 hook、诊断、规则投递、状态并发与卸载恢复。记录 OS/shell/harness 版本和二进制摘要；原生 stdin/退出码、文件恢复及新会话无残留均需证据，交叉编译不计通过。

## CI 与最终验收

- [ ] **M3-CI-01 跨平台 CI**（M3-CI-01）：最终 headSha 在 Ubuntu/macOS/Windows 的适用 format/vet、双 module fresh test/build、采集器、smoke 与旧迁移/回退检查全部通过；平台脚本使用原生临时缓存和环境清理。记录逐 job/步骤结果，跳过项不得算通过，适用时验证 `GOWORK=off` 发布形态。
- [ ] **M3-FINAL 最终回归**（M3-FINAL）：M3-BASE-01、A0–A18、P1/P2、B1–B4、C1–C6、TOKEN-01、W1 与 CI-01 按定稿范围全部验收；关联同一最终源码/二进制或说明等价性，无未解释 golden/兼容性差异。CARRY-01 继续独立非阻塞，其他缺环境项不能自动豁免。
- [ ] **M3-DOCS-01 总文档验收**（M3-DOCS-01）：规范、命令帮助/示例、教程、adapter 总览和 M3 手册反映最终已验行为；任务/测试/日志证据互相对应，链接/围栏/摘要核对通过，历史归档未改。M3-FINAL 通过后关闭里程碑，并提示用户进行下一步规划。
