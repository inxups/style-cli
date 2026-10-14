# STYLE 分发格式 v0.3：规则定义

规范总览和已定决定见 [STYLE-spec.md](STYLE-spec.md)；前置章节见 [包结构与顶层字段](STYLE-spec-package.md)，后续章节见 [运行时语义](STYLE-spec-runtime.md)。

## 3. 规则（`rules[]`）

### 3.1 公共字段

| 字段 | 默认 | 说明 |
|---|---|---|
| `id` | `STYLE.yaml` 里必填；规则文件默认取路径 | 包内唯一（1.2、1.7） |
| `target` | 必填 | `system` / `tool` / `mcp` / `skill` / `event` / `native`（3.3） |
| `file` 或 `text` | 二选一；规则文件里都不写，正文就是文件本身 | 正文，可用模板。`block` 时是拒绝理由；`warn` 时是给用户看的全文 |
| `model_text` | 正文的第一句 | 只对 `warn` 有意义：给模型的那一句（3.2）。可用模板。写在别的动作上给警告 `model-text-unused` |
| `action` | `inject` | `inject` / `warn` / `block`（3.2） |
| `at` | `after` | `before` / `after`，只对 `tool`、`mcp`、`skill` 有意义。`block` 一律在 `before` |
| `when` | 无 | 条件（第 4 节） |
| `repeat` | 按动作，见 3.5 | `every` / `turn` / `session` |
| `reinject_on_compact` | `system` 为 `true`，其余 `false` | 上下文压缩后这条规则算不算"已经丢了"（3.6） |
| `sources` | `[startup, clear, fork]` | 只对挂在 `session.start` 上的规则：在哪些会话来源上触发（3.6） |
| `fallback` | `degrade` | 当前 harness 做不到时：`degrade` 按第 5 节降级并在安装时提示；`skip` 跳过；`error` 拒绝安装 |
| `priority` | 继承包级 | |
| `max_tokens` | 包级 `budget.per_rule` | 这条规则给模型的文字的估算上限，超出触发 `T001`（7.3） |
| `reason` | 无 | 作者说明"为什么这条规则要重复注入"。只用来消掉 `T007`，不注入 |
| `from` | — | 只用在 `STYLE.yaml` 里：引入一个或一组规则文件，并覆盖其中的字段（1.3） |
| `enabled` | `true` | `false` 表示停用 |

### 3.2 三种动作：谁看得到，花不花 token

| 动作 | 进模型上下文（花 token） | 给用户看（不花模型 token） | 需要的能力 |
|---|---|---|---|
| `inject` | 正文（`additionalContext`） | — | `add-context` |
| `warn` | `model_text`，默认是正文第一句 | 正文全文（`systemMessage`） | `system-message` 和 `add-context`，有哪个发哪个 |
| `block`（工具调用） | 拒绝理由 | 拒绝理由 | `deny` |
| `block`（提示词、显式 skill） | 不进模型 | 拒绝理由 | `deny` |

- **`warn` 的两份**：只有 `add-context` 时只发 `model_text`；只有 `system-message` 时只发全文；两个都没有就按第 5 节处理（跳过并告警）。
- **"第一句"怎么取**：渲染后正文的第一个非空行；在这一行里找第一个 `。`、`！`、`？`，或后面跟空白或行尾的 `.`、`!`、`?`，截到它（含它）为止；一个都没有就取整行。
- **`block` 的理由**：工具调用每被拦一次，理由就进一次模型上下文，所以要短（lint `T005`：超过 40 token 告警）。Codex 要求理由非空【源码】，正文为空本来就是错误。
- **提示词被拦**：理由只给用户看【文档】。Claude 上 STYLE 一律带 `suppressOriginalPrompt: true`，拦截消息里不再附上原文【文档】。
- **同一次 hook 调用里有 `block` 命中时**，只输出拒绝，同一次调用里的 `inject` / `warn` 一律不发：调用不会执行，发了也是白花 token。

### 3.3 各 target

| target | 字段 | 挂在哪个统一事件上 |
|---|---|---|
| `system` | `mode: append \| replace`（默认 `append`） | 没有 `when`、正文是静态的 → 写进 harness 的**静态位置**（第 10 节），不走 hook。否则是**动态 system**：挂在 `session.start`，同时在每条 `prompt.submitted` 上重新评估（3.7） |
| `tool` | `tool:` 工具分类，可写非空列表；或 `names:` 原生工具名，可写非空列表。selector 不得为空、只含空白或含首尾空白 | `at: before` → `tool.pre`；`after` → `tool.post` |
| `mcp` | `mcp: "server/tool"`，必须恰好一个 `/` 且两侧非空；各分量不得含首尾空白。支持 `server/*`、`*/tool`，可写非空列表 | 同 `tool` |
| `skill` | `skill: <名字>`，可写非空列表；selector 不得为空、只含空白或含首尾空白；名字不区分大小写，全等比较 | **隐式**（模型自己去用 skill）：按 `at` 挂 `tool.pre` / `tool.post`；**显式**（用户点名）：`skill.explicit` |
| `event` | `event:` 统一事件名，见下 | 原样 |
| `native` | 以 harness 名为键，写原生 hook | 不经过规则引擎。**M2 实现**：M1 的 validate 接受，plan 报"跳过（M2）" |

**工具分类**（以 agenthooks 的 `canonicalNames` 为准，下表是两家的摘要）：

| 分类 | Claude Code | Codex |
|---|---|---|
| `shell` | `Bash` | `Bash`（hook 里 shell 统一叫这个【源码】） |
| `file.edit` | `Edit` | `apply_patch`（新建文件也走它） |
| `file.write` | `Write` | 永不触发：新建文件也是 `apply_patch`，已归到 `file.edit` |
| `file.read` | `Read` | 永不触发：读文件走 `Bash` |
| `search` | `Grep`、`Glob` | 永不触发 |
| `fetch` | `WebFetch` | 永不触发 |
| `task` | `Agent` | 永不触发：订阅面不含 |

Codex 只拦截 `Bash`、`apply_patch` 和 MCP（agenthooks quirk #16）。规则同时写了 `file.edit` 和 `file.write` 时，plan 对 Codex 只提示"`file.write` 已由 `file.edit` 覆盖"；只写 `file.write` 时报"永不触发"。

**skill 的两种触发**：

| | Claude Code | Codex |
|---|---|---|
| 隐式 | 模型调用 `Skill` 工具【文档】 | 模型用 shell 读 `skills/<名字>/SKILL.md`【源码】；认 `cat`、`sed -n`、`head`、`tail`、`bat`、`cd … && cat SKILL.md`（fork 改动 F8） |
| 显式 | 用户直接打 `/<名字>`，触发 `UserPromptExpansion`（`command_name`）【文档】 | 用户在提示词里写 `$<名字>`，Codex 直接把 SKILL.md 注入，中间没有工具调用【源码】 |

`$<名字>` 的识别（RE2）：`(?:^|[^A-Za-z0-9_$])\$([A-Za-z0-9][A-Za-z0-9_-]*)`；一条提示里出现多个就各算一次，同名只算一次。

`skill.explicit` 是 STYLE 自己的统一事件，不在 agenthooks 里，由适配器产生；不能写在 `target: event` 里，用 `target: skill`。

**`target: event` 可用的事件**：

| 事件 | 能做什么 | 备注 |
|---|---|---|
| `session.start` | 注入 | 用 `sources` 选会话来源（3.6） |
| `prompt.submitted` | 注入、阻止 | Claude 上定时任务、后台子 agent 回报、别的会话发来的消息也会触发它【方案 N4】 |
| `tool.pre` | 注入、阻止 | |
| `tool.post` | 注入 | |
| `tool.error` | 注入 | 只有 Claude（`PostToolUseFailure`）；Codex 上永不触发 |
| `subagent.start` | 注入 | Codex 需要 fork 改动 F7 |

MCP 名字不在清单里拼。`mcp__<server>__<tool>` 的拆分由适配器做；Claude 上服务名取工具事件里的 `mcp_server.name`，因为插件自带的服务器工具名是 `mcp__plugin_<插件>_<服务>__<工具>`，按名字拆会拆错【文档】。

### 3.4 合法组合

违反就报错，错误码在括号里：

| 组合 | 规定 |
|---|---|
| `block` 的 target | 只能是 `tool`、`mcp`、`skill`，或 `event: tool.pre` / `prompt.submitted`（`action-target`） |
| `block` + `at: after` | 报错（`block-after`）。不写 `at` 时 `block` 自动按 `before` |
| `warn`、`inject` | 除 `native` 外都可以 |
| `mode: replace` | 只能用在 `target: system`（`replace-non-system`）；安装时要 `--allow-replace` |
| `sources` | 只对落在 `session.start` 上的规则（`system`、`event: session.start`）有意义，写在别处给警告 |
| `model_text` | 只对 `warn` 有意义，写在别处给警告 |
| `reinject_on_compact: true` | 只对 `repeat: session` 有意义，配别的给警告 |
| `block` + `repeat` | `block` 不去重，写了 `repeat` 也忽略，给警告 `block-repeat` |
| `from:` 条目覆盖 `target` 或 `id` | 报错（`override-target`） |

### 3.5 去重（`repeat`）

先定义几个词：

- **作用域**：`(session_id, agent)`。`agent` 是 `main`，或者子 agent 的 `agent_id`。子 agent 有自己的上下文，单独计。
- **代（gen）**：每个作用域一个整数，`/clear` 时 +1（3.6）。
- **纪元（epoch）**：每个作用域一个整数，上下文压缩时 +1（3.6）。
- **回合**：Claude 用 `prompt_id`【文档】，Codex 用 `turn_id`【源码】。事件里没有（Claude 第一条提示之前、Codex 的 SessionStart）时，用这个作用域里 `prompt.submitted` 的计数。
- **正文哈希（bh）**：渲染后**给模型的**那份文字的 sha256 前 16 位（`warn` 取 `model_text`）。

| `repeat` | 同一个键只注入一次 |
|---|---|
| `every` | 不去重 |
| `turn` | `(作用域, gen, 回合, 规则, bh)` |
| `session` | `(作用域, gen, E, 规则, bh)`，其中 `E` = `reinject_on_compact` 为 true 时取 epoch，否则恒为 0 |

- **键里用的是规则，不是挂载点**。一条 skill 规则同时挂隐式和显式两处、一条动态 system 规则同时挂 `session.start` 和 `prompt.submitted`，它们共享去重记录，不会各注入一次。
- **正文变了算新内容**：动态正文里的变量（如模型名）变了，bh 就变，会再注入一次（3.7 靠的就是这一点）。
- **`block` 永不去重**。否则第二次 `git push --force` 就被放行了。
- **`subagent.start` 上的规则 STYLE 不去重**，交给 harness：Claude 只在子 agent 的上下文里已经没有这段文字时才重新注入【文档】；Codex 的 SubagentStart 只在子 agent 启动或 fork 时触发一次【源码】。
- **先占位再输出**：决定要输出时先抢占去重记录（CLI 设计 5.3 的 `O_EXCL` 文件），抢到才输出。harness 之后丢掉了输出（例如超过上限被落盘）也不补发，这是为了简单接受的取舍。

**默认值**（替换 v0.2 统一的 `turn`）：

| 动作 | 默认 `repeat` | 理由 |
|---|---|---|
| `inject` | `session` | 注入的文字留在历史里，再注入一次同样的内容就是重复付费 |
| `warn` | `every` | 每次命中都该提示；给模型的只有一句 |
| `block` | 不去重（固定） | |

`inject` 上写 `turn` 或 `every` 而没写 `reason:` 时，lint `T007` 告警："这条会在一个会话里重复注入"。

### 3.6 会话来源、压缩和清空

两家 SessionStart 的 `source` 都有五个取值：`startup`、`resume`、`clear`、`compact`、`fork`【文档】【源码】。

| 来源 | 状态怎么变 | 触发哪些 `session.start` 规则 |
|---|---|---|
| `startup` | 新作用域，gen = epoch = 0 | `sources` 含 `startup` 的 |
| `resume` | 不变 | `sources` 含 `resume` 的。**默认不含**：Claude 恢复会话时会回放保存的注入文字【文档】，再注入就是重复付费；Codex 是否回放待测（U-T1） |
| `clear` | gen +1：历史清空，之前的去重记录作废 | `sources` 含 `clear` 的 |
| `fork` | 不变（新 `session_id` 时本来就是空状态） | `sources` 含 `fork` 的。默认含；U-T1 如果证明 fork 出来的历史里保留了注入文字，就把 `fork` 从默认里去掉 |
| `compact` | epoch +1 | 只有 `reinject_on_compact: true` 的 |

- **压缩的意思**：对话太长时 harness 把历史换成摘要，之前注入的文字随之消失。epoch +1 之后，`reinject_on_compact: true` 的规则的去重记录全部作废，在 SessionStart(compact) 上补发一次；之后再命中时按新纪元去重。`reinject_on_compact: false` 的规则不受压缩影响（键里的 E 恒为 0），压缩后不会再出现，这是省 token 的默认。
- **两家都有这个信号**：Claude 在压缩后触发 SessionStart(`compact`)【文档】；Codex 在压缩完成时把 SessionStart(`compact`) 排进队列，**在下一个回合开始时**执行【源码】。
- **静态位置不需要补发**：Claude 的 `CLAUDE.md` 压缩后会重新加载（`InstructionsLoaded` 的 `compact` 原因）【文档】；Codex 的 `developer_instructions` 是配置，不在对话历史里【推断，待测 U11】。
- **子 agent**：Claude 的子 agent 在自动压缩后会重新注入 SubagentStart 的内容【文档】。Codex 的子 agent 被压缩后不会再触发 SubagentStart【源码】，这是已知限制。
- 回合级（`turn`）的记录压缩时不用清，它本来按回合失效。

### 3.7 模型或强度中途变化

动态 system 规则除了在 `session.start` 上评估，还在**每条 `prompt.submitted`** 上重新评估一次：重新判断 `when`、重新渲染正文，再按 3.5 的键去重。效果：

- 内容没变：键已存在，什么都不发。
- 模型或强度变了，正文里的值跟着变：bh 变了，补发一次。
- `when` 当初不满足、现在满足了（例如 Claude 上第一次工具调用之后才知道强度）：这时注入。

数据来源：Claude 切模型会触发 `PostModelSwitch`，适配器只用它更新状态【文档】；Codex 每个事件都带 `model`【源码】。Codex 的强度来自配置，会话中途用 `/model` 改强度看不到（第 6 节）。

两点限制：

- **撤不回**：注入是追加，已经进了历史的文字删不掉。`when` 后来不满足了，之前注入的那段仍在。规则要写成"过期了也无害"。
- **成本**：补发正好发生在模型或强度变化时，而这时提示词缓存本来就要重建【文档】，所以几乎没有额外的缓存代价。

### 3.8 同一次 hook 调用命中多条规则

一次 hook 调用可能对应几个评估点（例如 Codex 的 UserPromptSubmit 同时是 `prompt.submitted` 和 `skill.explicit`）。所有评估点命中的条目合在一起处理：

1. 有 `block` 命中：只输出拒绝。多条理由按 `order` 用换行拼接。
2. 否则：给模型的文字按 `order` 排好，**用一个空行拼成一段**，放进一个 `additionalContext`（Codex 上是一条 developer 消息【源码】，Claude 上是一个 system-reminder【文档】）。`warn` 给用户的全文同样拼成一个 `systemMessage`。
3. **不加任何外壳或标题行**（没有 `<style …>`、没有 `[STYLE: …]`）：重复注入时这些会累积成本。来源靠日志和 `style explain` 查。
4. **逐字节稳定**：同样的输入必须产出完全相同的字节，否则会破坏提示词缓存，也会让去重失效。
5. **运行时上限保护**：拼好的文字如果超过适配器上限（Claude 10,000 字符，Codex 约 2,500 token），从 `order` 最大的条目开始丢，直到不超；丢掉的条目释放去重占位，日志记 `cap-drop`。正常情况下 `T003`（7.3）在安装时就拦住了，只有动态变量才会把它推过线。

### 3.9 规则作者指南

1. **写事实或约定，不写命令式的系统指令。** 像"带外系统命令"的文字会触发 Claude 的提示词注入防御，Claude 会把它当可疑内容报告给用户，而不是照做【文档】。好："本仓库评审要求每条问题带 `file:line`。" 不好："你必须……"、"SYSTEM: ……"。
2. **一条规则说一件事**，控制在 100–300 token；不放示例（实在要放只放一个）；不写模型本来就知道的泛泛之谈（"写好代码"）。
3. **挂得越窄越省**：能挂在具体工具、MCP、skill 上，就别挂在 `session.start`。
4. **长规范用"指针规则"**：注入一句话，说明详细规范在哪个文件、什么时候去读。适合偶尔才需要的长规范，不适合必须遵守的硬规则。
5. **正文逐字节稳定**：不放时间、随机数、会话 id、计数。
6. **运行时变量写 `default`**，例如 `{{thinking_effort | default: "未知"}}`。
7. **`block` 理由不超过 40 token**。
8. **正文里不要出现 `<system-reminder>` 标签**，Claude 会把它转义【文档】。
9. **依赖强度的规则**：Codex 上只有配置里写了 `model_reasoning_effort` 才有值，多数默认安装取不到；Claude 上第一次工具调用之前可能没有（待测 U-C1）。
10. **中文不一定更省**：中文大约一字一 token。用 `style plan` 看估算，别凭印象。
