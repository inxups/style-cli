# STYLE CLI M3 测试清单

更新时间：2026-10-17

最新授权：用户于 2026-10-17 要求“所有评审全部通过，完全完成M3”。T31/T32 恢复实施，后续设计先记录契约后直接推进；下文早期“暂缓/待用户评审”叙述保留决策背景，以本条和逐项证据为准。授权不替代实现、测试或真实环境验收。

本文件记录 M3 测试矩阵与当前证据状态，任务及依赖见 [STYLE-M3-TASKS.md](STYLE-M3-TASKS.md)，运行记录追加到 [PROGRESS.md](PROGRESS.md)。[M1 测试](STYLE-M1-TESTS.md)、[M2 测试](STYLE-M2-TESTS.md) 已只读归档；历史通过结果不替代 M3 的新验收。

当前状态：M3 于 2026-10-18 按用户确认范围完成并关闭。M3-00/BASE、A0–A18、P1/P2、B1–B4、C1/C2/C4/C5/C6、本地 exact、三平台 CI、FINAL 和 DOCS-01 全部通过；见[最终报告](review/M3-FINAL-2026-10-18.json)与[文档关闭核对](review/M3-DOCS-01-2026-10-18.json)。用户延期的 C3、W1/REAL-04 和 Anthropic 独立 API 保持待验，未算通过；M3-CARRY-01 官方校准仍两家各 0/40、非阻塞，RV-01 已关闭。

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

本节记录首次离线验收当时的范围。后续真实与平台补验见 M3b 和 CI/FINAL；早期“待验”描述不覆盖当前汇总状态。

- [x] **M3-A0 注册表与架构边界**（M3-D0、M3-T00、M3-RV-01）：用仅注册假 harness、删去某家、两家现有注册表及空目录的 fixture，覆盖 `when.harness.name`、`requires.harness`、`recommends` 及 native 校验。目录/单文件包、所有 app 入包路径给出一致诊断；普通名称全集与 native 子集、快照隔离、结构/完整 API、预算重复路径和 auto 遍历均需证明。core 不导入具体 adapter/agenthooks，旧包/诊断排序及失败后 bytes/mode/mtime 保持兼容。[D0 设计 §6](review/M3-D0-REGISTRY-VALIDATION-2026-10-17.md#6-m3-a0-验收细化) 的 A0-01–A0-10 已逐项通过，见 [本轮报告](review/M3-A0-2026-10-17.json)。macOS 27.0.1 / Go 1.27.0、临时三缓存及清理环境下，新执行双 module fresh test（30 个有测试 package，均非 cached）/vet、core/spec/tokens/adapter/app 四包完整 race；14 个新测试函数、205 个叶子用例，含 144 组目录/单文件/from 的精确名称组合与 33 组无效候选文件保全。与固定基线 CLI 的 32 组文本/JSON 输出和退出码逐字节相同；8 个隔离 import fixture 证明整个 adapter/agenthooks 前缀受限。59 项采集器、40 smoke、16 组 `74a473d` 真实旧版迁移、12 组回退通过；当前二进制 SHA256 `8d4e933ccdc6d79167294b0998e0bfe76668cfbf0a268a64d25dc3f3b5ad6967`。首轮测试中两个 fixture 问题及修正后通过均保留；生产校验未放宽、agenthooks/协议/旧 golden 未改，未调用真实 harness、远端 CI 或官方计数。关闭 M3-RV-01，允许进入 D1。
- [x] **M3-A1 explain**（M3-D1、M3-T10）：按 [D1 §8 的 A1-01–A1-12](review/M3-D1-DIAGNOSTICS-2026-10-17.md#8-待执行的验收细化)（已获用户确认）固定两家原生 payload/index、空 state/只读 snapshot 和绑定 facts。逐项核对共享短路 trace、缺值、来源/agent、repeat、block 优先、多点评估、cap-drop/释放、编译折叠与 static/native 边界，实际 codec wire 和管理退出码分开。覆盖解码起零外部读取/进程/网络、坏/超限/漂移输入、取消/清理失败、并发隔离和 bytes/mode/mtime；explain.json 与 index 同事务及旧安装 index-only 回退另验。文本/JSON 确定性、协议白名单和兼容回归均以 [本轮报告](review/M3-A1-2026-10-17.json) 验收：26 个新函数、145 个叶子用例（全部无 fail/skip）、29 组生命周期/普通 runtime 对照、两家完整 JSON golden、只读/并发/失败与恢复、编译说明回滚和事务窗口均通过。macOS 27.0.1 / Go 1.27.0、临时三缓存及清理环境下，双 module fresh test（31 个有测试 package，无 cached）/vet、相关八包与 agenthooks 全 module race、59 项采集器、40 smoke、16 真旧迁移、12 回退通过；agenthooks 既有可选真实 e2e 的 skip 单列，不计本次离线通过。当前二进制 SHA256 为 a08d25a1f61c205023c7501d2f67f46a2a4d8684d65eb2f0f3ce0c806ebd3cff，旧 index/lock/state schema、普通 wire golden 与历史归档未改。probe 现已另经 A2 通过；两家真实验收/Windows/CI 保持待验。
- [x] **M3-A2 probe**（M3-D1、M3-T11）：按 [D1 §8 的 A2-01–A2-12](review/M3-D1-DIAGNOSTICS-2026-10-17.md#8-待执行的验收细化) 验收现有 STYLE hook 的 lease/run ID/根/事件/runtime/原生 executable 和路线来源绑定，见 [完整报告](review/M3-A2-2026-10-17.json) 与 [使用说明](STYLE-PROBE.md)。28 个新函数、181 个叶子用例（201 条含分组 pass，无新 fail/skip），覆盖三路线离线 OS 祖先链、直接/伪造/版本/解码拒绝、index 不可读、部分完成、SIGINT/SIGTERM/SIGKILL、PID 复用、并发 busy/回滚、迟到写入/替换、漂移与 cleanup 失败。白名单/上限/FIFO/隐私、配置/信任/包/lock/index/说明 bytes/mode/mtime 和 15 组普通 runner 对照通过；后者分别以无观察器、无租约、活动/坏租约各运行两次，仅归一化 state 的 updated_at，stats/seen/模式和 wire/原生退出码核对。macOS 27.0.1 / Go 1.27.0、临时三缓存及清理环境下，双 module fresh test（32 个有测试 package，无 cached）/vet、相关六包及 agenthooks 全 module race、59 采集器、40 smoke、16 真旧迁移、12 回退和最终 CLI 均通过。普通 hook 各 200 次进程 p95 为 Codex 9.56ms、Claude 8.71ms，均小于 50ms；最终二进制 SHA256 为 `6461c60459091a63662cc101d7e266ca8cf48cb130e65705a7438376d3ee967f`。十次前期失败及修正/重跑保留，23 项既有 opt-in 真实 e2e skip 单列；不计入新验收。仅关闭 A2，真实 B1/C1、Windows、远端 CI 和官方计数保持待验。 真实验收发现的新版 index 诊断误报已修正，见[补充回归](review/M3-A2-INDEX-FOLLOWUP-2026-10-18.json)：共享 reader 接受 1–7、拒绝未知格式，回调仍不单独证明执行。
- [x] **M3-A3 Git 来源**（M3-D2、M3-T20）：按已定稿 [D2 §9 的 A3-01–A3-12](review/M3-D2-SOURCES-2026-10-17.md#9-待执行的验收细化) 覆盖目录/单文件、HEAD/ref/OID、SHA-1/SHA-256、原始 blob 与归档属性差异、路径/模式/大小限额、配置/协议/凭据隔离。临时 Git 与 loopback TLS fixture 验证真实取得、不可达/缺 revision/中断/磁盘失败、缓存完整性/并发、lock 1/2 和真实旧二进制门禁、同名幂等、包/native/预算批准、来源保真及共享提交保护；失败无半安装、取得不执行仓库代码，普通 hook 不启动 Git。生产入口验收见 [完整报告](review/M3-A3-2026-10-17.json) 和 [使用说明](STYLE-SOURCES.md)：30 个新函数/202 个叶子用例全部通过（230 条含分组 pass、无新 fail/skip）；17 项真实新旧二进制检查核对 format 2 拒绝/目录身份与 bytes/mode/mtime 保全、普通 hook wire 及无 Git 的非法输入拒绝。macOS 27.0.1 / Git 2.54.0 / Go 1.27.0、临时三缓存及清理环境下，双 module fresh test（33 个有测试 package、无 cached）/vet、相关五包及 agenthooks/install race、59 采集器、40 smoke、16 真旧迁移、12 回退均通过；sync 收紧持锁范围后 app race 再通过。24 项既有 opt-in skip（23 个真实 e2e、1 个性能）单列，不计本项验收；最终二进制 SHA256 为 `175979dcf6bf2317e4c2b67917a7bf28e848456bd218e6b012c7cc57bda32150`。前期 fixture/测试调整、回环端口沙箱拒绝和误调用 vet 目标均保留。仅关闭 A3，真实投递、Windows、CI、官方计数状态不变。
- [x] **M3-A4 update**（M3-D2、M3-T21）：按已定稿 [D2 §9 的 A4-01–A4-12](review/M3-D2-SOURCES-2026-10-17.md#9-待执行的验收细化) 覆盖明确选包/无变化/预览确认、branch/tag/固定 OID/换 ref、本地原路径/link、版本/settings/native/replace、未选批准字节与效果变化阻止、跨包去重/预算、任一失败整批不安装。验证捕获后分支/输入漂移、各包与 approved/settings/index/explain/lock/hook/static 故障回滚、并发 guard、旧安装/三路线、报告和隐私；不连带批准未选 link/native。证据：[完整报告](review/M3-A4-2026-10-17.json)、[使用说明](STYLE-SOURCES.md#明确选包更新) 和 16 份新增 text/JSON golden。31 个新函数、244 个叶子用例（269 条含分组 pass，零新 fail/skip）覆盖 SHA-1/SHA-256 真实 Git、固定 OID 不回退、新 settings default/必填及影子值拒绝、旧批准保守边界、native 顺序/全量披露、多包联合效果、16 类确认漂移与私有输入 symlink、各包及文件 bytes/mode/mtime 恢复和失败备份。最终双 module fresh test 2,935 条 pass/0 fail、vet、app/source 与 agenthooks 全 module race、59 采集器通过；24 条既有 opt-in skip 单列。最终二进制运行 24 项更新 CLI/旧安装/基线 wire、40 smoke、16 真旧迁移、12 回退和 17 Git 门禁通过，SHA256 `7da4b1d4d7288c6305793d5db3677b325eb6c3c7f8e9d0b9a28f7f6afd55bf3a`。前期 fixture/原始锁构造、TLS 沙箱及错误的 M1 runtime 对照选择的失败和修正保留；最终未跳过 A4 用例。该次仅关闭 A4；A5、真实 B/C、Windows、CI 与官方两家各 0/40 未在该次关闭。
- [x] **M3-A5 frozen**（M3-D2、M3-T22）：按已定稿 [D2 §9 的 A5-01–A5-12](review/M3-D2-SOURCES-2026-10-17.md#9-待执行的验收细化) 在固定根/绑定下重复整锁离线恢复。覆盖本地/单文件/Git/link 的完整快照、缓存非必需但无内容不成功、远端移动、缺 settings/approval、旧 lock 1/新 lock 2、编译/绑定差异、owner 允许恢复与外来文件拒绝、native/replace 确认、多包/并发/失败回滚。全程零 Git/网络/缓存写入，锁 bytes/mode/mtime 不变；不宣称跨根/项目或真实信任生效。证据：[完整报告](review/M3-A5-2026-10-17.json)、[使用说明](STYLE-SOURCES.md#整锁离线恢复) 和 16 份新增 text/JSON golden。21 个新函数、209 个叶子用例（228 条含分组 pass，零新 fail/skip）覆盖完整目录/单文件/批准/Git 缓存、SHA-1/SHA-256 固定来源、零 Git/检测/缓存写入、精确 lock/settings/approval、link trust、全锁联合去重/门禁、15 类确认漂移及无漂移对照、每包与逐文件失败回滚及保留失败备份。双 module fresh test 3,163 条 pass/0 fail，24 条既有 opt-in skip 单列；vet、app/source race 1,299 pass、59 采集器通过。最终二进制 SHA256 `abb2ce7ba60b11cf51ae02d85e8b248b06d7c7e2e004aa5509054863774ece6f` 运行 30 frozen CLI、24 update CLI、40 smoke、16 真旧迁移、12 回退、17 Git 门禁均通过。三路线 owner 允许且摘要可重现的缺失配置恢复通过；部分配置缺失无法重现 rendered 变更集摘要、缺失 Claude 已登记 rules 和程序路径改变均保守失败，不扩大 owner 权限。早期 fixture/分类、CLI 对照绑定和误写采集器入口的失败保留并修正。仅关闭 A5；D3 项目部分现已另获确认，真实 B/C、Windows、CI 和官方两家各 0/40 保持待验。
- [x] **M3-A6 项目级路线**（M3-D3、M3-T30）：两家 fake/native 配置 fixture 覆盖用户/项目共存、不同项目同名包、嵌套目录、多 clone、范围显式选择与错误根。plan/install/list/sync/trust/doctor/uninstall 使用同一范围；index/state 不串用，现有文件字节/权限还原，native 归属与原生信任边界一致。按已确认项目部分的 [D3 §9](review/M3-D3-ROUTES-2026-10-17.md#9-待执行验收细化) 的 A6-01–A6-12 覆盖明确项目根/私有命名空间、跨范围同名冲突、新 clone 重现采用、Codex 静态动态降级、双根信任与逐文件恢复，见 [完整报告](review/M3-A6-2026-10-17.json)、[使用说明](STYLE-PROJECTS.md) 与 [built CLI 检查](../scripts/check_m3_project.py)。33 个新函数/166 个叶子用例全部通过（184 条含分组 pass、无新 fail/skip），44 份 text/JSON golden、18 项 CLI/真实 Git worktree 检查、逐包/逐文件/48 个取消检查点、9 类确认漂移及跨范围 guard 通过。最终双 module fresh test 3,347 pass/0 fail、24 项既有 opt-in skip，vet、相关 race、68 采集器、40 smoke、16 真旧迁移、12 回退、24 update、30 frozen、17 Git 门禁通过。最终二进制 SHA256 `31ed26797e2849a160ede978e43c108371fa31fc948039e8976a26e8509e0866`；首次 fixture/编译/分类失败、TLS 本地监听沙箱阻塞和修正重跑保留。仅关闭 A6，B2/C2 的原生加载/信任/真实投递不以离线结果替代。
- [x] **M3-A7 云端交付产物**（M3-D3、M3-T31）：按定稿对隔离 clone/项目产物验证必要文件、运行时路径与加载前提，不依赖本机用户配置或私有临时绝对路径。缺配置/运行时和不支持环境给出明确结果；本项仅验产物与可移植性，云端实际生效另见 C3。按待评审 [D3 §9](review/M3-D3-ROUTES-2026-10-17.md#9-待执行验收细化) 的 A7-01–A7-06 覆盖显式批准 bundle 导出、全新 clone/预置 runner、相对路径与 sensitive-reserved 拒绝/普通值披露、缺依赖及陈旧 generation、失败恢复；本轮调查不关闭本项。 已通过 [本轮报告](review/M3-A7-2026-10-17.json)：A7-01–A7-06 全部通过，54 条新 pass 与 race，根 module 2,989 pass/0 fail、vet、8 项 built CLI、40 smoke；新 clone、原生 fixture/缺依赖、私有状态、篡改/版本拒绝、确认漂移、多包和取消恢复均取证。实际 C3 另验。
- [x] **M3-A8 Claude 插件路线**（M3-D3、M3-T32）：生成产物与 agenthooks 原生配置契约一致；覆盖重复安装、用户/项目/插件路线切换、多包、登记前后状态、native 及静态规则。卸载恢复外部文件/权限，无重复 handler 或残留；生成文件不等同于已登记或被真实 harness 加载。按待评审 [D3 §9](review/M3-D3-ROUTES-2026-10-17.md#9-待执行验收细化) 的 A8-01–A8-06 覆盖本地目录 marketplace、手动原生登记/卸载、user/plugin 互斥切换、静态 user rules 与原生数据库残留分别报告；本轮调查不关闭本项。 已通过 [A8 报告](review/M3-A8-2026-10-17.json)：11 个新测试函数、56 条新 pass、root/agenthooks 全量 3,555 pass/0 fail，专项 race、vet、8 项插件 built CLI、8 项云端 CLI、40 smoke 与旧迁移/replace 回归通过。登记元数据 fixture 不代表真实 C4。

- [x] **M3-A9 refresh**（M3-D4、M3-T40）：按已确认 [D4 §9 的 A9-01–A9-12](review/M3-D4-REFRESH-2026-10-17.md#9-a9-待执行验收细化)覆盖 0/正整数/defaults/from、repeat 与 target 合法组合、system 动态与 Claude clock-only 订阅、同正文滚动间隔和无欠账。核对原生 ID/缺 ID/迟到事件、compact/resume/fork/子作用域、多进程时钟及 claim/释放、故障与快照，format 1/2 的真实旧程序门禁、原有 state/golden 和五路线生命周期。预算按单次全文，stats/explain 保持隐私和一致口径；设计报告中的旧行为回归不替代本项新功能验收。证据：[完整报告](review/M3-A9-2026-10-17.json)、[使用说明](STYLE-REFRESH.md) 和 [CLI 检查](../scripts/check_m3_refresh.py)。29 个新函数/85 个叶子用例、98 条含分组 pass；进程 helper 在父测试中以环境隔离启动，顶层非工作模式 skip 单列。双 module fresh test 3,445 pass/0 fail，24 条既有 opt-in skip（23 真实 e2e、1 性能）与 helper skip 单列；vet、state/engine/runtime/app race、68 采集器、50 refresh CLI、40 smoke、16 真旧迁移、12 回退、17 Git、24 update、30 frozen、18 项目 CLI 全部通过。实际 A6 旧二进制拒绝整份 format 2，旧 sync/trust/update/frozen 拒绝且 bytes/mode/mtime 不改；新源码冻结重建与规则变化审批/取消回滚通过。五路线各 50 次 hook 进程 p95 为 codex-user 13.962ms、codex-plugin 16.919ms、claude-user 9.269ms、codex-project 13.597ms、claude-project 13.623ms。最终二进制 SHA256 `5894f0ad248c3fe218ca91191ce6c5afdec48905fa837db0b680d63d79a8ef5c`；保留初期测试构造/编译、macOS 并发创建锁、采集器沙箱和修正重跑记录。macOS 本地运行与 Linux/Windows 交叉编译通过，后者不代表原生验收；仅关闭 A9，真实 B3/C5 和其他待验状态保持。
- [x] **M3-A10 事件内容围栏**（M3-D5、M3-T41）：覆盖提示、工具输入/输出中伪造围栏、嵌套标记、控制字符、Unicode、多行和超长内容。校验 reads/字段可用性、转义/截断与最终预算；缺字段和错误处理按设计一致。hook 日志、stats、doctor 中不泄露原始提示或工具正文，原生 stdout 仍符合白名单。 证据：[完整报告](review/M3-A10-2026-10-17.json)、[说明](STYLE-EVENT-CONTENT.md)、[围栏 golden](../testdata/event-content/fence.golden)和 [CLI 检查](../scripts/check_m3_event_content.py)。D5 的八组均有重复命令支撑：11 个新函数、最终 57 pass/race，根 module 3,079 pass/0 fail（2 既有 skip 单列），35 项五路线 built CLI、8 云端/8 插件 CLI、40 smoke 与 refresh 回归通过。最后新增 metadata/refresh 测试在全量编译后补充，由专项/race 单独重跑通过；生产源码与全量/二进制一致。

- [x] **M3-A11 provider 来源**（M3-D6、M3-T42）：按 adapter 声明的来源验证已知/缺失/未知服务值与切换行为，不以 harness 名、模型别名或 URL 猜测 provider。plan/explain/runtime 对可用性一致；旧 `provider-unimplemented` 行为的变更有明确 fixture 和诊断依据。[A11 报告](review/M3-A11-2026-10-17.json)固定最终源码、二进制与命令：15 新函数/95 pass，双 module 3,707 pass/0 fail（25 既有 skip 单列），根专项 race、双 vet、35 provider CLI/35 事件兼容/40 smoke 通过；D6 的 A11-01–06 关闭。受控祖先、配置版本 stub 和直接 hook 调用不代表真实投递。
- [x] **M3-A12 unknown 匹配**（M3-D6、M3-T43）：覆盖批准字段上的缺失、空字符串、未知原始值、显式 `unknown` 和与已有值组合；比较表达式、否定条件和不支持字段按定稿处理。未使用新语法的旧规则仍遵循既有缺值语义，plan/compile/runtime/explain 一致。 证据：[本轮报告](review/M3-A12-2026-10-17.json)；A12-01–04 均通过，六路线生命周期/frozen/标记清理、五路线 CLI 和实际 T42 旧运行器/管理器拒绝有独立证据。
- [x] **M3-A13 用户 effort 映射**（M3-D7、M3-T44）：验证用户配置与内置映射的定稿优先级、无效/冲突项、未知 raw、approx 与会话缓存更新。隔离配置下 doctor/plan 说明来源，fixture 逐行核对两家原生值；缺值时不沿用旧档位，不伪造原生未提供的思考开关。 证据：[A13 报告](review/M3-A13-2026-10-17.json)，A13-01–04 均通过；11 新函数/73 含分组 pass、同专项 race、全量 Go/vet、35 映射 CLI/35 unknown 兼容/40 smoke。
- [x] **M3-A14 规则级 fail-closed**（M3-D8、M3-T45）：覆盖受支持/不支持事件与路线、混合规则、模板/状态/超时故障、index 缺失/损坏及既有安装级开关。错误分类、原生输出与退出码按定稿，默认放行兼容；无法识别规则的故障不声称已实现规则级判断，正常未命中不误拦。 [验收报告](review/M3-A14-2026-10-18.json)覆盖 A14-01–04、六路线/云端、真实旧 T44 二进制拒绝/清标记恢复；9 新函数/74 pass、根 3,416 pass、race/vet/80 CLI/40 smoke 通过，2 既有 skip 单列。
- [x] **M3-A15 exact 模式**（M3-D9、M3-T50）：固定本地编码及计数接口 fixture，核对计数对象、模型/编码披露、文本/JSON 与确定性排序。覆盖缺凭据/未知模型/编码、取消、限流/网络/接口错误和计数失败；不把估算静默标为 exact，不泄露 key。默认估算模式与旧预算兼容；stub 不能关闭 TOKEN-01 或 CARRY-01。 [本轮报告](review/M3-A15-2026-10-18.json)与[指南](STYLE-COUNT.md)：两套完整词表和 137×2 独立 golden、无网络再生成、329 新专项/race pass；根 fresh test 3,745 pass/0 fail、vet、287 built CLI、45 故障兼容、50 refresh/五路线 p95 和 40 smoke 通过。首次帮助文案/fixture 失败保留，修正后复验；旧 testdata/agenthooks 不改。
- [x] **M3-A16 native 扩展**（M3-D10、M3-T60）：按评估结论覆盖获支持的 `async`/`onFailure`/非 command 类型及拒绝项、版本门槛、目录/单文件包和 from 覆盖。核对安装披露、sync/trust、owner/信任/卸载、原生执行约束及旧四字段兼容；native 仍不被虚构为 STYLE runtime stats/token 记录。 [A16 报告](review/M3-A16-2026-10-18.json)覆盖四字段保留与扩展拒绝结论：126 名称/格式/字段组合和 from 覆盖、六路线 108 次命令保全、冻结恢复；135 专项/race pass、根全量 3,880 pass/0 fail、24 agenthooks native pass、vet、40 五路线 CLI（旧 JSON/config 与外部 HTTP/MCP/prompt/agent/async command 保全）和 40 smoke 通过。初期 fixture 顺序/既有管理 gate 预期失败保留；源值不回显。
- [x] **M3-A17 Windows 平台**（M3-D11、M3-T70）：支持的两家 user 路线、盘符/空格/Unicode/长路径、安全引用/owner、stdin/退出码、DACL、状态并发/清理和文件事务均完成 Windows 原生 CI。[完整报告](review/M3-A17-2026-10-18.json)固定 run 39055736061 / `b481396`：三平台全过；Windows 根 3,380/agenthooks 521 pass、0 fail，280/35 skip 逐项列明，生产 CLI 25 项全过。POSIX 回归保持；W1 实际 harness 与最终 CI-01 独立。
- [x] **M3-A18 conformance 与升级**（M3-T80）：[报告](review/M3-A18-2026-10-18.json)固定双模块 fresh 4,412 pass/0 fail、25 既有 skip 单列；codec/stdout 白名单/golden、native/stats/跨包去重与旧包/锁/index/state 保持。六路线 36 项组合核对映射/provider/unknown/refresh/围栏/fail-closed、排序/零重复成本、frozen 状态保留与恢复；16 组真实旧布局迁移、12 组 replace 回退、40 smoke 和双 vet 通过，未重录旧 golden。

M3-A2 缓存补充：Codex 0.159.3 原生插件使用独立缓存，三次真实旧根关联失败保留；按 [D1 补充](review/M3-D1-CODEX-CACHE-2026-10-18.md) 核对登记/source/固定执行文件后绑定缓存根，[回归报告](review/M3-A2-CODEX-CACHE-2026-10-18.json)与新真实插件整轮通过。更换 native 正文并通过原生 add 刷新后，新会话收到新正文且旧正文缺席；probe 不执行登记/刷新命令。

## 准备验证

- [x] **M3-P1 两家 runbook/采集器**（M3-PREP-01）：[手册](STYLE-M3-MANUAL.md)与[报告](review/M3-P1-2026-10-18.json)固定私有源码/STYLE/harness/实际观察器副本摘要、原生时序/session/路线关联和完整正文判定；六路线干跑/bytes/mode 恢复、76 项全量和最终 10 项 M3 自测通过。拒绝漂移、缺原生/请求/时间证据、伪造回调单证及 symlink，验证短写入与原始观察器 subprocess。只关闭准备，不计真实 B/C。
- [x] **M3-P2 特殊环境 runbook**（M3-PREP-02）：[手册 P2](STYLE-M3-MANUAL.md#延期环境-p2)和[报告](review/M3-P2-2026-10-18.json)固定云端/独立计数 API/Windows 的环境、版本/账号依赖、采集白名单、失败保留和恢复。八项云端离线入口、两编码预检及缺 key 明确失败/零请求通过；Windows 自动化引用原生 A17。三类实际环境按用户决定延期，未计 C3/W1/API 通过。

## M3b：Codex 真实环境

既有 `codex-user`、`codex-plugin` 两路线独立记录，不拼接为一套通过；项目路线新增单独证据。记录固定版本、二进制和配置基线、原生 hook 来源/回调、完整上下文或请求摘要及卸载恢复；只声明实际观察到的信任范围。

- [x] **M3-B1 probe 与 explain**（M3-T10、M3-T11、M3-REAL-01）：真实会话触发可关联回调，user/plugin 路线分别执行；用独立原生 hook 来源/事件证据关联 probe 收据、时间与 session，不能只依赖进程祖先或手工调用。采集原生 payload、事件前 state、补充事实和固定 index 后按 D1 隔离解释，与实际命中/去重/输出及原生上下文/请求证据核对。未触发、退出、清理与卸载后新会话无残留分别记录；callback-observed 不单独证明模型投递。 用户路线已通过[真实运行](review/M3-B-CODEX-USER-2026-10-18.json)，插件路线也已通过[完整真实验收](review/M3-B-CODEX-PLUGIN-2026-10-18.json)，项目及用户最终复验均通过[三路线验收](review/M3-REAL-01-2026-10-18.json)。 [六路线未触发/中断检查](review/M3-PROBE-NEGATIVE-2026-10-18.json)已通过：有效 index、12 次失败状态/完整清理与显式配置恢复；无模型调用，不据此关闭真实投递。
- [x] **M3-B2 项目路线与来源生命周期**（M3-T20–T22、M3-T30、M3-REAL-01）：在隔离项目中用锁定来源安装，核对项目配置被真实 harness 接受、实际投递和用户/其他项目隔离；更新/frozen 与 native 变更信任沿定稿执行。多 clone 信任不外推，卸载后配置与原有用户规则还原。 [项目真实报告](review/M3-B-CODEX-PROJECT-2026-10-18.json)覆盖独立项目无投递、完整更新正文/frozen、受保护配置缺席恢复及卸载后新会话无 hook。
- [x] **M3-B3 运行时扩展**（M3-T40–T45、M3-REAL-01）：以固定真实事件序列核对 refresh、围栏、provider/unknown、effort 映射和受支持失败策略。原生回调/上下文、state、stats 与 explain 相符；不支持能力按批准设计证明边界，不把模型回复当投递证据。 用户路线已通过[真实运行](review/M3-B-CODEX-USER-2026-10-18.json)，插件路线也已通过[完整真实验收](review/M3-B-CODEX-PLUGIN-2026-10-18.json)，项目及用户最终复验均通过[三路线验收](review/M3-REAL-01-2026-10-18.json)。
- [x] **M3-B4 native 与既有路线回归**（M3-T60、M3-T80、M3-REAL-01）：真实触发仍受支持的 native 及 STYLE hook，核对来源、matcher/timeout、信任与共存。新 native 能力仅按 D10 的 Codex 支持范围取证；卸载恢复外部文件，后续新会话不再触发。 用户路线已通过[真实运行](review/M3-B-CODEX-USER-2026-10-18.json)，插件路线也已通过[完整真实验收](review/M3-B-CODEX-PLUGIN-2026-10-18.json)，项目及用户最终复验均通过[三路线验收](review/M3-REAL-01-2026-10-18.json)。

## M3b：Claude Code 真实环境

本地用户/项目/插件与云端分别记录。沿用隔离 runbook 和原生证据；交互操作沿既有授权范围执行，登录/钥匙串等需要用户交互时单列。固定版本、实际工作区信任范围和文件基线，安装/卸载分别启动新会话核对。

- [x] **M3-C1 probe 与 explain**（M3-T10、M3-T11、M3-REAL-02）：真实交互会话产生可关联原生回调，用独立原生 hook 来源/事件证据对应收据/时间/session。固定原 payload/index、事件前 state 和补充事实后隔离解释，对照命中/去重/codec 输出与实际投递；当前 user 路线单独记录，后续新路线另验。核对未触发/超时、清理及卸载后新会话；进程祖先、callback-observed 或采集器手工调用均不单独替代原生/投递证据。 [六路线未触发/中断检查](review/M3-PROBE-NEGATIVE-2026-10-18.json)已通过：有效 index、12 次失败状态/完整清理与显式配置恢复；无模型调用，不据此关闭真实投递。 用户路线已通过[实际认证模型验收](review/M3-C-CLAUDE-USER-2026-10-18.json)，[项目路线](review/M3-C-CLAUDE-PROJECT-2026-10-18.json)也通过完整验收；[插件路线](review/M3-C-CLAUDE-PLUGIN-2026-10-18.json)也通过完整验收；[本地总报告](review/M3-REAL-02-LOCAL-2026-10-18.json)固定全部三条路线。
- [x] **M3-C2 本地项目路线**（M3-T30、M3-REAL-02）：原生加载/指令附件或请求证据证明项目规则生效，用户规则与其他项目隔离；核对信任、动态/native 与静态位置、卸载恢复和新会话无残留。 [真实项目报告](review/M3-C-CLAUDE-PROJECT-2026-10-18.json)验证原生 UI 接受信任且正常退出、原生发现配置、其他项目无规则、native 新正文、frozen 与卸载/基线还原。
- [?] **M3-C3 云端项目交付**（M3-T31、M3-REAL-02）：在实际云端会话中验证定稿支持的交付路径、规则发现、hook 运行和完整投递；记录 clone/运行时条件及卸载或移除后的新会话。无真实环境保持待验，本地 fixture 不代替云端结果。
- [x] **M3-C4 插件路线**（M3-T32、M3-REAL-02）：[插件真实报告](review/M3-C-CLAUDE-PLUGIN-2026-10-18.json)已通过实际登记/启用插件、原生加载并触发目标规则；与已有用户/项目路线无重复投递，核对原生信任边界、禁用/卸载和外部文件恢复。
- [x] **M3-C5 运行时扩展**（M3-T40–T45、M3-REAL-02）：真实事件序列核对 refresh、事件围栏、provider/unknown 和用户 effort 映射；对规则级 fail-closed 注入定稿允许的故障并验证工具行为，恢复后新会话正常。用原生证据与 state/stats 对照，明确静态规则 compact 等未覆盖边界。 用户路线已通过[实际认证模型验收](review/M3-C-CLAUDE-USER-2026-10-18.json)，[项目路线](review/M3-C-CLAUDE-PROJECT-2026-10-18.json)也通过完整验收；[插件路线](review/M3-C-CLAUDE-PLUGIN-2026-10-18.json)也通过完整验收；[本地总报告](review/M3-REAL-02-LOCAL-2026-10-18.json)固定全部三条路线。
- [x] **M3-C6 native 扩展**（M3-T60、M3-REAL-02）：对 D10 获支持的能力验证实际原生事件、输出/失败行为、版本门槛及与 STYLE hook 共存；不支持项保留结论。核对 trust/卸载恢复及新会话不再触发，旧四字段命令继续可用。 用户路线已通过[实际认证模型验收](review/M3-C-CLAUDE-USER-2026-10-18.json)，[项目路线](review/M3-C-CLAUDE-PROJECT-2026-10-18.json)也通过完整验收；[插件路线](review/M3-C-CLAUDE-PLUGIN-2026-10-18.json)也通过完整验收；[本地总报告](review/M3-REAL-02-LOCAL-2026-10-18.json)固定全部三条路线。

## 计数与 Windows 真实验收

- [~] **M3-TOKEN-01 exact 能力**（M3-T50、M3-REAL-03）：对定稿支持的本地编码和真实计数接口运行生产 CLI，记录计数文本/请求摘要、模型/编码版本和结果；缺 key/模型/编码/网络时保留失败与待验，输出语义不扩大为全请求/账单精度。该产品验收与固定 40 条的官方估算校准 CARRY-01 分开。 [最终二进制本地证据](review/M3-TOKEN-LOCAL-2026-10-18.json)：137 文本 × 两编码的 274 计数、共 287 CLI 检查及完整词表摘要通过；本次必过的本地子项已完成，独立 API 子项按用户决定保持 `[?]`，不计官方 40 条校准。
- [?] **M3-W1 Windows 原生运行**（M3-T70、M3-REAL-04）：在支持的 Windows shell 与实际 harness 下执行安装、原生 hook、诊断、规则投递、状态并发与卸载恢复。记录 OS/shell/harness 版本和二进制摘要；原生 stdin/退出码、文件恢复及新会话无残留均需证据，交叉编译不计通过。

## CI 与最终验收

- [x] **M3-CI-01 跨平台 CI**（M3-CI-01）：最终 headSha 在 Ubuntu/macOS/Windows 的适用 format/vet、双 module fresh test/build、采集器、smoke 与旧迁移/回退检查全部通过；平台脚本使用原生临时缓存和环境清理。记录逐 job/步骤结果，跳过项不得算通过，适用时验证 `GOWORK=off` 发布形态。 [最终报告](review/M3-CI-01-2026-10-18.json)固定受验源码 `2c24e5f9`、run `39064157115`：三 job 全过；Windows fresh/双架构构建/25 CLI 完成，skip 与真实 W1 分列；后续文档提交有源码等价性核对。
- [x] **M3-FINAL 最终回归**（M3-FINAL）：M3-BASE-01、A0–A18、P1/P2、B1–B4、C1/C2/C4/C5/C6、本地 TOKEN-01 与 CI-01 按批准范围全部验收；关联同一最终源码/二进制或说明等价性，无未解释 golden/兼容性差异。C3、W1 与 Anthropic 独立 API 依用户明确决定延期，保持待验；CARRY-01 继续独立非阻塞，其他缺环境项不能自动豁免。 [最终报告](review/M3-FINAL-2026-10-18.json)固定同一生产源码/二进制的完整适用验收、三平台 CI、旧版兼容与延期边界；文档总验收已由 DOCS-01 独立完成。
- [x] **M3-DOCS-01 总文档验收**（M3-DOCS-01）：规范、命令帮助/示例、教程、adapter 总览和 M3 手册反映最终已验行为；任务/测试/日志证据互相对应，链接/围栏/摘要核对通过，历史归档未改。M3-FINAL 通过后关闭里程碑，并提示用户进行下一步规划。 [指南同步检查](review/M3-DOCS-GUIDES-2026-10-18.json)已覆盖当前实现、计数与失败策略/平台边界；本地真实、最终 CI 与 FINAL 已完成；[最终文档核对](review/M3-DOCS-01-2026-10-18.json)通过，关闭本项和本轮 M3。

## 2026-10-17 用户确认的延期真实验收

用户明确“这些等以后再测试，先排除这些，继续”，指环境请求中 Claude 托管云端、Windows 主机真实运行和 Anthropic 独立计数 API。C3、W1/REAL-04 和 TOKEN-01 的 Anthropic API 子项保持 `[?]`，从本次 FINAL 必过集合排除，后续另测；不以 fixture/交叉编译/替代编码标记通过。其余 A/P/CI 与本地真实 B/C，以及可本地执行的 exact 编码仍需完成。

六路线最终超时/中断复验使用缓存修复后二进制，见[最终报告](review/M3-PROBE-NEGATIVE-FINAL-2026-10-18.json)：12 次预期失败、有效 index 捕获、完整 cleanup 和配置/项目恢复均通过。
