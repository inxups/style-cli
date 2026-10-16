# STYLE 分发格式 v0.3（实现基线）

状态（2026-10-16）：M1a 离线验收已完成，Codex 0.159.3 和 Claude Code 2.1.300 已有真实验收证据；M1 全部补验收仍进行中。Claude 界面/上下文/原生超限已补验，两家官方 token 计数及最终远端CI尚未闭合，逐项状态以 [任务清单](../docs/STYLE-M1-TASKS.md) 和 [测试矩阵](../docs/STYLE-M1-TESTS.md) 为准。读文档或源码得出的结论保留出处；`Uxx` 的当前结果见 [CLI 设计第 15 节](STYLE-cli-design.md#15-未决项)。

**范围**：M1 只支持 **Claude Code** 和 **Codex**。其他 harness 推迟，但"通用核心 + 每家一个薄适配器"的边界保留，以后加一家只写一个适配器。已经调研过的资料放在附录 A。

**设计原则**（有冲突时按这个顺序取舍）：

1. **少花 token。** 注入文字可能随历史进入后续请求，缓存、压缩及上下文管理会改变保留范围和费用。默认只注入一次、条件要窄、正文要短，安装前显示离线估算（第 7 节）。规则成本公式只比较明确假设的情景，不保证每次命中缓存，也不代表账单（CLI 设计 §11.3）。
2. **只微调行为。** 规则是追加在 harness 自带指令后面的短句，写成事实陈述。替换内置指令（`mode: replace`）要用户显式同意。
3. **做不到就说。** 某个 harness 做不到的规则按固定阶梯降级，每一步写进安装报告，不假装生效。
4. **核心不认识 harness。** 格式和语义只有一份；各家的差异全部由适配器回答。

**和 CLI 设计的分工**：本规范定义包格式、运行时语义、token 预算、安全约束和编译产物 `index.json`。命令、本地布局、两个适配器的具体做法、fork 改动、里程碑、验收和未决项在 [STYLE-cli-design.md](STYLE-cli-design.md)。两份冲突时，格式和语义以本规范为准，实现细节以 CLI 设计为准。

**完整示例**：[example/terse-reviewer/STYLE.yaml](../example/terse-reviewer/STYLE.yaml)

**出处标记**：

| 标记 | 含义 |
|---|---|
| 【文档】 | Claude Code 官方 hooks 参考（`code.claude.com/docs/en/hooks.md`，2026-10-12 读） |
| 【源码】 | 读源码，没有运行：Codex `main`（2026-10-12）、agenthooks `3f734c7` |
| 【推断】 | 推理得出，没有直接证据 |
| 【待测 Uxx】 | 要在真实环境里测，测法和测之前的默认做法见 CLI 设计第 15 节 |

## 0. 已定的决定

| 问题 | 决定 | 来源 |
|---|---|---|
| 支持哪些 harness | M1 只做 Claude Code 和 Codex；其他推迟，保留适配器边界 | 用户，2026-10-12 |
| 运行时 | agenthooks（Go）的 fork；`style` 一个二进制同时是 CLI 和 hook 入口，入口是 `style agenthooks run --provider=<harness>` | v0.2 |
| 架构 | 通用核心 + 每家一个薄适配器；降级阶梯只在核心 | v0.2 |
| 规则清单 | 只有 `STYLE.yaml` 的 `rules[]` 一份；规则文件必须用 `from:` 声明 | v0.2 |
| `when` | 所有条件都满足才注入；取不到数据的条件算不满足 | v0.2 |
| 推理强度 | 七档 + `open`；用 harness 报告的值，不推断（4.2） | v0.2；本版修正了 Claude 那一侧的含义 |
| `inject` 默认去重 | `repeat: session`：同一会话、同一内容只注入一次 | token 设计 T3 |
| `warn` | 用户看全文，模型只看一句（`model_text`） | 评审 A1 + token 设计 T5 |
| 子 agent | 规则默认只在主 agent 生效 | token 设计 T6 |
| 恢复会话（resume） | 默认不重新注入 | token 设计 T7 |
| 预算 | 单条 400、单事件 1,200、常驻 600 token，超出默认只告警；超过 harness 上限的 80% 直接报错 | token 设计 T1/T2 |
| 事件内容进模板 | M1 禁止，只能在条件里用 | 评审 C1 |
| 压缩后重注入 | 两家都走 SessionStart(`source=compact`) 这一条路径 | Claude+Codex 方案 A2 |
| 模型、强度中途变化 | 动态 system 规则在每条用户提示上重新渲染，正文变了才补发 | Claude+Codex 方案 A4 |
| Codex 安装 | 插件优先，用户级 `hooks.json` 兜底；固定订阅面；不写 `additionalContextLimit` | v0.2 + token 设计 |
| Claude 安装 | 用户级 `settings.json`；exec 形式（不经过 shell）；订阅面跟着规则走 | Claude+Codex 方案 |
| 出错时 | 默认放行；Claude 可以选 `--fail-closed` | Claude+Codex 方案 |
| 多个包同时命中 | 按 `priority` 从小到大拼接（默认 50），同值按安装顺序 | v0.2 |
| `replace` | 允许但不是默认，要 `--allow-replace`；同一 harness 只允许一个包用 | v0.2 + 评审 C1 |

## 规范正文导航

规范正文保留原章节编号，按维护职责拆分：

- [包结构与顶层字段（第 1–2 节）](STYLE-spec-package.md)
- [规则定义（第 3 节）](STYLE-spec-rules.md)
- [条件、运行时数据、预算与安全（第 4–8 节）](STYLE-spec-runtime.md)
- [编译产物、安装状态与未决项（第 9–12 节及附录 A）](STYLE-spec-compiled.md)
