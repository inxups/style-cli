# STYLE CLI M2 测试清单

更新时间：2026-10-16

本文件记录 M2 测试矩阵和每项当前状态。测试通过必须有命令输出、fixture/golden 文件或真实环境记录作为证据。任务清单见 [STYLE-M2-TASKS.md](STYLE-M2-TASKS.md)，M1 测试矩阵已只读归档（[STYLE-M1-TESTS.md](STYLE-M1-TESTS.md)）。

当前状态：M2-BASE-01 已通过，其余条目尚未执行。M2a 断言为初稿，标“D* 定稿后细化”的条目在对应设计项通过评审时补全具体 fixture、golden 和退出码。

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

- [ ] **M2-A1 native validate**（M2-T10，按规则 3.10）：
  - 验收清单：`testdata/invalid/` 新增 `native-mapping`、`native-field`、`native-event`、`native-matcher`、`native-command`、`native-timeout` 六个 fixture，纳入 `TestA1ValidateExampleAndEveryInvalidFixture` 的清单；每个在文本/JSON 两种模式下退出 1、stderr 为空、输出含对应码。
  - `core/spec` 表驱动用例覆盖各码的子情况：没有 harness 键、harness 值是列表或标量（`native-mapping`）；规则上写 `action`/`when`/`repeat`/`event`、顶层写 `gemini:`、映射里写 `async`/`onFailure`/`type`/`args`（`native-field`）；缺 `event`、非字符串、Codex 写 `UserPromptExpansion`、Claude 写 `PostModelSwitch`（`native-event`）；空串、首尾空白、写在 `Stop`/`UserPromptSubmit` 上、Codex 非法正则、Claude `*` 合法（`native-matcher`）；缺失、非字符串、含换行/回车/NUL、首尾空白、以 `;`/`&`/`|`/`)`/`#` 开头、未转义 `\` 结尾、引号不成对（`native-command`）；`0`、`601`、`1.5`、字符串（`native-timeout`）。
  - 合法用例退出 0：示例包（`codex-only-notify` 更新为 3.10 写法）；同时写两家映射、只写一家、带 matcher 与 timeout、正文为空的 native 规则；包级 `defaults.action: warn` 不使 native 报错；`from:` 整体覆盖某个 harness 映射。
  - 非 native 规则上写 `codex:` 仍报 `invalid-schema`。
  - 单文件包（`STYLE.md` frontmatter）与等价目录包对上述合法/非法用例给出相同的码和退出码。
  - harness 键与事件名来自注册表：用只注册一个假 harness 的注册表做单测，`codex` 键即报 `native-field`；`core/...` 不导入具体适配器的依赖测试仍通过。
- [ ] **M2-A2 native plan**（M2-T11，按规则 3.10）：
  - 两家 plan golden（文本与 JSON）：`codex-only-notify` 在 Codex 上为 `native-hook`，附事件 `Stop`、实际 timeout 60；在 Claude 上为 `skipped`，原因"没有该 harness 的 native 映射"。输出中不再出现 `skipped-m2` 或"跳过（M2）"。
  - token 一节单列"native hook 输出不估算"及规则/事件；常驻、单条、按原生事件合计与 M2-01 起点相比不含 native，`T001`/`T003` 不计 native。
  - `fallback`：用假适配器让 `Native()` 返回错误，`skip`/`degrade` 得 `skipped`，`error` 得 `install-error` 且 install 退出 4、不写任何文件。
  - plan 报告（文本/JSON）不含 native 规则正文，也不含命令；命令只在 install 披露、trust 预览和 doctor 里出现。
- [ ] **M2-A3 native 安装往返**（M2-T12/T60）：codex-plugin、codex-user、claude-user 三条路线 install → uninstall 后配置逐字节还原，他人条目保留，重复安装幂等，换路线无残留；native 条目带 STYLE owner 标记。
- [ ] **M2-A4 native 披露与信任**（M2-T13，按运行时 8.6）：
  - 安装报告逐条列出 harness、规则、事件、matcher、实际 timeout、作者命令原文（不含 `: --owner=style --native=…;` 前缀），并写明路线信任方式（`codex-user` 预置、`codex-plugin` 插件信任、`claude-user` 工作区信任）。
  - 非交互、无 `--yes`、本次会新增 native 条目时退出 1，harness 配置、锁文件、index 逐字节不变；带 `--yes` 成功。
  - 修改 native 的命令、事件、matcher、timeout 任一项，或增删 native 规则后，`sync` 退出 3；`--link` 包同样退出 3。`trust` 预览单列"native 条目差异"，给出改前改后四个字段；未确认时不写入，确认或 `--yes` 后配置与新渲染一致，`codex-user` 的 `trusted_hash` 同步更新。
  - `style list` 显示每个包的 native 条目数；`style doctor` 对一致、缺失、被手工改动三种状态分别输出正常、告警、告警，`codex-user` 另报 `trusted_hash` 是否匹配。
- [ ] **M2-A5 stats**（M2-T20，D2 定稿后细化）：固定 `stats.jsonl` fixture 按规则、按会话汇总的文本与 JSON 输出与 golden 逐字节一致；时间窗口过滤正确；坏行、超长行按 D2 处理；输出不含规则正文或提示词。
- [ ] **M2-A6 跨包去重**（M2-T30，D3 定稿后细化）：两个包含正文完全相同的规则时，index 只保留一份、plan 与 T003 只计一次；运行时只注入一次；卸载保留方后重编译使另一份生效；正文不同的规则不受影响。
- [ ] **M2-A7 Claude rules 静态位置**（M2-T40，仅 D4 成立时）：写入/删除 `~/.claude/rules/style-<名字>.md`；从 `CLAUDE.md` 标记块迁移后再卸载，两处文件逐字节还原。D4 不成立时本条以 D4 证据标注不适用并关闭。
- [ ] **M2-A8 output style replace**（M2-T50，仅 D5 可行时）：断言随 D5 定稿；不可行时本条以 D5 证据标注不适用并关闭。
- [ ] **M2-A9 M1 回归**：M1 的 A1–A22、adapter conformance（解析、effort、往返、安装、报告、输出白名单）在 M2 改动后全部仍通过；任何 golden 更新逐项说明原因。

## M2b：Codex 真实环境

Codex 真实配置由 agent 执行，步骤见 M2-PREP 建立的 `docs/STYLE-M2-MANUAL.md`。投递以 Codex rollout / OTel span 为证，不以模型自述为据。

- [?] **M2-B1 native hook 真实触发**：安装含 native 规则的包后，真实 Codex 会话中原生命令按配置的事件和 matcher 执行，rollout/OTel 可见；卸载后不再执行且配置字节还原。
- [?] **M2-B2 native 与 STYLE hook 共存**：同一事件上 native 条目与 STYLE 自身 hook 各执行一次，不重复、不互相覆盖；信任提示行为与 M1 B1 一致或按 D1 记录差异。
- [?] **M2-B3 stats 真实汇总**：真实会话产生的 `stats.jsonl` 经 `style stats` 汇总后，注入次数与 rollout 中 hook 上下文条数一致。
- [?] **M2-B4 跨包去重真实投递**：两个包含相同正文规则时，真实会话只注入一次。

## M2b：Claude Code 真实环境

Claude Code 界面项由用户执行，agent 提供手册与隔离 runbook。

- [?] **M2-C1 U-C4 实测**（M2-D4）：用户级 `~/.claude/rules/*.md` 是否被加载，以 `InstructionsLoaded` 或原生请求为证；记录结局。
- [?] **M2-C2 native hook 真实触发**：Claude Code 真实会话中 native 命令按事件和 matcher 执行；卸载后 `settings.json` 逐字节还原。
- [?] **M2-C3 rules 静态位置真实生效**（仅 D4 成立时）：安装后规则文件被加载，卸载后文件移除、`CLAUDE.md` 与安装前一致。
- [?] **M2-C4 output style 实测**（M2-D5）：output style 能否替代 system prompt、与用户已有 output style 的冲突行为；记录结局。

## 持续集成与文档核对

- [ ] **M2-CI-01**：M2 最终提交的 GitHub Actions 在 Ubuntu/macOS 上 format、vet、双 module test、采集器测试、build、smoke 全部 success。
- [ ] **M2-DOCS-01**：M2 修改过的当前文档本地链接目标全部存在、代码围栏成对，`git diff --check` 通过。
