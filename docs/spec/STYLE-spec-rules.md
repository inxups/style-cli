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
| `native` | 以 harness 名为键，每个 harness 一个原生 hook 映射（3.10） | 不经过规则引擎，原样写进 harness 的 hook 配置。M2 实现 |

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
| `tool.error` | 注入 | 只有 Claude（`PostToolUseFailure`）；Codex 无此入口。无条件规则按 fallback 为 skipped / install-error；缺失条件数据可先得到 never-trigger（运行时 §5） |
| `subagent.start` | 注入 | Codex F7 已实现并经真实 rollout/请求验证；Claude 仅观察，不声明输出能力 |

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
| `native` + 规则引擎字段 | `action`、`at`、`when`、`repeat` 等不能写在 native 规则上（`native-field`，3.10） |

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
| `resume` | 不变 | `sources` 含 `resume` 的。**默认不含**：两家专用startup-only探针在resume真实请求中均保留原注入文字且没有补发【FINAL-07】，不重复注入 |
| `clear` | gen +1：历史清空，之前的去重记录作废 | `sources` 含 `clear` 的 |
| `fork` | 不变（新 `session_id` 时本来就是空状态） | `sources` 含 `fork` 的。默认含；U-T1 如果证明 fork 出来的历史里保留了注入文字，就把 `fork` 从默认里去掉 |
| `compact` | epoch +1 | 只有 `reinject_on_compact: true` 的 |

- **压缩的意思**：对话太长时 harness 把历史换成摘要，之前注入的文字随之消失。epoch +1 之后，`reinject_on_compact: true` 的规则的去重记录全部作废，在 SessionStart(compact) 上补发一次；之后再命中时按新纪元去重。`reinject_on_compact: false` 的规则不受压缩影响（键里的 E 恒为 0），压缩后不会再出现，这是省 token 的默认。
- **两家都有这个信号**：Claude 在压缩后触发 SessionStart(`compact`)【文档】；Codex 在压缩完成时把 SessionStart(`compact`) 排进队列，**在下一个回合开始时**执行【源码】。
- **静态位置不走 hook 补发**：Claude M2 使用每包 `rules/style-<名字>.md`，兼容迁移旧 `CLAUDE.md`；C1/C3 已确认新会话中每包 rules 的加载和卸载还原；既有 `CLAUDE.md` 的 compact 加载事实不自动证明新 rules 的 compact 行为，本次不作该声明。Codex 的 `developer_instructions` 是配置，B3/U11 真实请求已确认 compact 后仍可见。
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
9. **依赖强度的规则**：Codex 上只有配置里写了 `model_reasoning_effort` 才有值，多数默认安装取不到；Claude FINAL-06的启动/提示/显式技能没有effort，工具前/后/失败有值；缺值保持unknown，后续按既定动态评估补齐。
10. **中文不一定更省**：中文大约一字一 token。用 `style plan` 看估算，别凭印象。

### 3.10 `target: native`

native 规则把作者给的命令原样写成 harness 的原生 hook。STYLE 不求值、不注入、不去重、不计 token，只负责校验、披露、写入和卸载。M2 只支持 `type: command` 一种 hook。

```yaml
---
id: notify-on-stop
target: native
codex:
  event: Stop
  command: "notify --title codex"
claude-code:
  event: PostToolUse
  matcher: "^Bash$"
  command: "notify --title claude"
  timeout: 10
---
可选的作者说明。native 规则的正文不注入、不估算，可以为空。
```

**字段**

| 位置 | 字段 | 规定 |
|---|---|---|
| 规则 | `id`、`target`、`enabled`、`priority`、`fallback`、`from` | 含义同 3.1；`priority` 只决定同一事件里 native 条目的先后（见下） |
| 规则 | 正文（`file`、`text` 或规则文件正文） | 可选，只作说明；不注入、不估算，不触发 `empty-body` |
| 规则 | harness 键（`codex`、`claude-code`，取自适配器注册表） | 至少写一个；每个值是一个映射，不接受列表。要多个 hook 就写多条规则 |
| 映射 | `event` | 必填，该 harness 的原生事件名，取自适配器的 `NativeEvents()`（CLI 设计 §3） |
| 映射 | `matcher` | 可选，原样写入；只能写在该适配器标为接受 matcher 的事件上（M2 两家都只开放工具类事件），按该 harness 的方言校验 |
| 映射 | `command` | 必填，一行 shell 命令，原样写入（加归属前缀，见下） |
| 映射 | `timeout` | 可选，整数秒，1–600；不写按适配器默认值（60 秒；Codex `SessionEnd` 仍按 agenthooks 压到 3 秒），报告显示实际值 |

规则引擎字段（`action`、`at`、`when`、`repeat`、`reinject_on_compact`、`sources`、`model_text`、`max_tokens`、`mode`、`reason`、`tool`、`names`、`mcp`、`skill`、`event`）显式写在 native 规则上就报错；从包级 `defaults` 继承来的值对 native 规则不生效，也不报错。harness 映射里的 `async`、`onFailure`、`type`、`args` 等其他键在 M2 一律报错，Claude 的 `async`/`onFailure` 和非 command 类 hook 留到 M3。`from:` 条目可以整体覆盖某个 harness 键的映射，不做逐字段合并。

**`command` 的约束**：非空；不含换行、回车、NUL；没有首尾空白；不以 `;`、`&`、`|`、`)`、`#` 开头；不以未转义的 `\` 结尾；单引号、双引号成对（按 POSIX shell 词法检查）。这些约束保证加上归属前缀后命令语义不变。

**诊断码**（`style validate` 退出 1）：

| 码 | 情况 |
|---|---|
| `native-mapping` | native 规则一个 harness 键都没有；harness 键的值不是映射 |
| `native-field` | native 规则写了规则引擎字段；顶层有既不是 STYLE 字段也不是已注册 harness 的键；映射里有 M2 不支持的键 |
| `native-event` | `event` 缺失、不是字符串，或不在该适配器的 `NativeEvents()` 里 |
| `native-matcher` | `matcher` 为空或有首尾空白；写在不接受 matcher 的事件上；不符合该 harness 的方言（Codex 按正则编译，Claude 接受 `*` 或可编译的正则） |
| `native-command` | `command` 缺失、不是字符串，或违反上面的约束 |
| `native-timeout` | `timeout` 不是整数，或不在 1–600 |

非 native 规则上出现 harness 键，仍按未知字段报 `invalid-schema`。

**plan**：每个目标 harness 一行。

| 情况 | 状态 | 说明 |
|---|---|---|
| 有该 harness 的映射 | `native-hook`（"原生 hook"） | 附事件、matcher、实际 timeout |
| 没有该 harness 的映射 | `skipped` | 原因"没有该 harness 的 native 映射"，与 `fallback` 无关 |
| 有映射但适配器在本版本或本路线上无法写入 | 按 `fallback`：`skip`、`degrade` → `skipped`（native 没有降级阶梯）；`error` → `install-error`（退出码 4） | |

M1 的 `skipped-m2` 状态取消。token 一节单列"native hook 输出不估算"，列出规则与事件；native 规则不计入 `always_on`、`per_rule`、`T001`、`T003`。命令输出如果被 harness 送进模型上下文，由作者自己负责。

**写入与归属**：

- 每条 native 规则在对应事件下单独占一个 matcher 组、一个 handler，排在 STYLE 自身订阅面的组之后；多条 native 规则按包结构 1.4 的顺序（`order`）排列。产出逐字节稳定。
- 写入的 `command` 为 `: --owner=style --native=<包名>/<规则 id>; <作者命令>`。`:` 是 POSIX 的空操作，不读 stdin、不改退出码；harness 的 hook 输入照常交给作者命令。`<包名>/<规则 id>` 含 shell 特殊字符时按 shell 规则加引号。卸载、换路线、重编译都沿用 agenthooks 按 `--owner=style` 识别归属的逻辑，不认这个前缀的条目一律保留。
- Codex 用户路线（`codex-user`）照常为 native 条目预置 `trusted_hash`；信任关口是 STYLE 的安装披露与确认（运行时 8.6）。
- Windows 的 cmd.exe 没有 `:` 空操作，native 规则在 Windows 上的写法随 M3 的 Windows 支持再定。
- native 条目不经过 STYLE 运行时：不写 `stats.jsonl`，`style stats` 不统计；M2-D3 确定 native 不参与跨包去重（命令即使相同也可能有副作用），见[运行时 §7.5](STYLE-spec-runtime.md#75-m2-d3-跨包去重契约)。

### 3.11 M3-D4 refresh 评审稿

以下是已获用户确认的 [D4 方案](../review/M3-D4-REFRESH-2026-10-17.md)，T40 已实现，在规则、defaults 和 from 覆盖中接受整数 `refresh: N`；0/省略关闭，正数不超过 1,000,000，只适用于 inject/warn 与 repeat session。block、repeat every/turn、subagent.start、system replace 不兼容；native 显式声明仍为 native-field。warn 的默认 every 不变，继承规则和 0 关闭详见方案 §2。

开启后同一作用域/代/按需纪元/规则/模型正文，首次正常命中就发，之后距离上次保留输出满 N 个实际提交回合才可再次发；不按命中次数计、不补欠账，未命中/空输出/block 优先/cap-drop 不消费机会。system 走动态双点，其他 target 保留原匹配入口；正文变更、A→B→A、clear/compact/resume/fork 与旧 seen 升级按方案 §3–6。每次仍发送完整正文；reason 继续只用于成本告警。

无有效 refresh 时，上文既有 repeat、sources、静态位置和 native 语义保持。用法与升级边界见 [周期补发说明](../STYLE-REFRESH.md)，实现验收见 [M3-A9](../review/M3-A9-2026-10-17.json)。

## M3-D8 规则级失败策略

已定稿[规则级失败契约](../review/M3-D8-FAIL-CLOSED-2026-10-18.md)：fail: closed 仅在可 deny 的直接运行时评估点使用；故障保护按整个原生事件范围保守选择，正常未命中不拦。index 7 与严格最低格式锁保护语义；index 不可读/宿主终止需安装级机制，agenthooks 保留协议职责。T45/A14 后续独立验收。
