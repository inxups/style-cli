# STYLE 分发格式 v0.3：运行时语义

规范总览和已定决定见 [STYLE-spec.md](STYLE-spec.md)；前置章节见 [规则定义](STYLE-spec-rules.md)，后续章节见 [编译产物与未决项](STYLE-spec-compiled.md)。

## 4. 条件（`when`）

### 4.1 写法

```yaml
when:
  model: ["gpt-5*", "**/claude-*"]
  thinking_effort: ">=high"        # 或单值 high，或列表 [minimal, low]
  thinking_effort_approx: false    # 可选
  harness:
    name: [codex, claude-code]
    permission_mode: [default]
  agent: any                       # 不写 = 只主 agent；any = 主 + 所有子 agent；或列表 [main, reviewer]
  conditions:
    - { field: tool.input.command, op: regex, pattern: '\bgit\s+diff\b' }
```

- **组合**：同一字段的多个值是"或"，不同字段之间是"且"。**所有条件都满足才注入**，没有"任一满足"的写法。
- **取不到就不满足**：某个条件需要的数据这次取不到（harness 没给、档位为空、`tool.added` 在这个工具上不存在），这个条件算不满足，规则不触发。安装时 plan 按第 6 节的表提前告警，例如"Codex 上没配置 `model_reasoning_effort` 时，规则 X 永远不会触发"。
- **`model`**：glob，不区分大小写，匹配 **harness 报告的模型字符串**（STYLE 分不清别名）。`*` 不跨 `/`；`**/` 前缀匹配零段或多段，所以 `**/claude-*` 既匹配 `claude-opus-5-5`，也匹配 `anthropic/claude-x`。
- **`provider`**：M1 没有数据来源。validate 接受并警告 `provider-unimplemented`，运行时永不满足。M3 再定。
- **`thinking_effort`**：写法只有四种：单值（`high`）、列表（`[minimal, low]`）、`>=档位`、`<=档位`，不区分大小写；其他写法报错 `bad-effort-expr`。别名（如 `ultra`）不能写在 `when` 里，只在映射层用。取值和比较规则见 4.2。
- **`thinking_effort_approx`**：`true` / `false`，要求档位是或不是近似值。M1 的两个适配器都只产出精确档位，所以永远是 `false`；字段留给以后的预算型 harness。
- **`harness.name`**：`claude-code`、`codex`（以 agenthooks 的 `Provider` 常量为准）。`harness.permission_mode`：harness 原生的取值。
- **`agent`**：不写时等于 `[main]`，只在主 agent 里生效；挂在 `subagent.start` 上的规则不写时等于 `any`。`any` 表示主 agent 和所有子 agent；列表里可以写 `main` 和子 agent 的类型名（`agent_type`）。**主 agent 的判断是"事件里没有 `agent_id`"**：Claude 用 `--agent` 启动时，主 agent 的事件里也有 `agent_type`【文档】。
- **`reads`**：`when` 里用到的 `model`、`thinking_effort`、`harness.<键>` 必须先在 `reads` 里声明（`reads-missing`）。`agent` 不需要声明，它是去重作用域本来就要读的。

### 4.2 thinking_effort：取值和映射

**档位**：七档 `none < minimal < low < medium < high < xhigh < max`。`none` 表示思考被关闭。

**`open`**：思考开着但强度不明（只有开关的模型）。它**不在档位顺序里**，只能用单值或列表匹配（`open`、`[none, open]`），`>=` / `<=` 一律不匹配它。M1 的两家都不会产出 `open`；只靠 `open` 才会触发的规则，plan 报"永不触发"。

**比较**：编译时把比较展开成档位列表：`">=high"` → `[high, xhigh, max]`；`"<=low"` → `[none, minimal, low]`（注意含 `none`；只想要"开着思考的低档"就写 `[minimal, low]`）。档位为空时任何写法都不满足。原始值用 `{{thinking_effort_raw}}` 读取。

**映射规则**（适配器按这些规则把原生值转成档位）：

1. 原生值和某一档同名（不区分大小写），就是那一档。
2. 别名：`ultra` → `max`。
3. 关闭思考 → `none`。
4. 只有开关的模型：关 → `none`，开 → `open`（M1 用不到）。
5. token 预算按阈值近似分档（M1 用不到，阈值表见附录 A）。
6. 没列在映射表里的值（`persistent`、模型自定义值）：档位为空，`{{thinking_effort_raw}}` 仍是原值。
7. **用 harness 报告的值，不推断，不读会话记录**（例如 Codex 的 rollout）。两家含义不同：
   - **Codex** 报不了，STYLE 读配置里的**请求值**（hook 输入里没有强度【源码】）；
   - **Claude** 报的是**实际生效值**：设了模型不支持的档，报的是降级后的档【文档】。

**映射表**（规范性的，适配器的 `effort.yaml` 必须和它一致；要改先改这里）：

| harness | 原生值 → 档位 | 来源和备注 |
|---|---|---|
| Codex | `none` `minimal` `low` `medium` `high` `xhigh` `max` 同名；`ultra` → `max`；`persistent`、自定义值 → 取不到 | 配置里的 `model_reasoning_effort`，解析顺序见第 6 节。没写就取不到，**多数默认安装都是这样**。原生取值见 codex-rs `protocol/src/openai_models.rs` |
| Claude Code | `low` `medium` `high` `xhigh` `max` 同名；没有 `minimal` | 工具类事件的 `effort.level`，或环境变量 `$CLAUDE_EFFORT`【文档】。当前模型不支持 effort 参数时不报，取不到。思考被关闭能不能检测到待测（U9）；检测不到之前，Claude 上不会出现 `none` |

**映射表归谁**：规范拥有档位定义、映射规则和这张表；核心只认七档和 `open`，不认识任何原生值。每个适配器把自己那一行写成嵌入的数据文件 `adapter/<harness>/effort.yaml`（格式见 CLI 设计第 3 节），由共用的 `adapter/effortmap` 解析。一致性测试检查表里每一行都有 fixture。

### 4.3 conditions

`conditions` 是 hookify 式的字段判断：`{ field, op, pattern }`。

**字段**，以及哪些评估点有它（用在没有它的评估点上，validate 报错 `condition-field-wrong-event`）：

| 字段 | 内容 | 哪些评估点有 |
|---|---|---|
| `prompt` | 用户提示原文 | `prompt.submitted`、`skill.explicit` |
| `tool.name` | 原生工具名 | `tool.pre`、`tool.post`、`tool.error` |
| `tool.canonical` | 工具分类 | 同上 |
| `tool.input` | 整个输入的紧凑 JSON 字符串（输入本身是字符串时原样） | 同上 |
| `tool.input.<路径>` | 输入里的某个字段，用 agenthooks 的 `RawField` 解析 | 同上 |
| `tool.added` | 这次改动**新增**的文本。Codex `apply_patch`（输入是 `{"command": "<补丁>"}`【源码】）取以 `+` 开头的行，去掉 `+`，跳过 `+++`；Claude `Edit` 取 `new_string`，`Write` 取 `content`【文档】；其他工具取不到 | `tool.pre`、`tool.post`，只在 `file.edit` / `file.write` 上 |
| `tool.output` | 工具输出（字符串化） | `tool.post` |
| `mcp.server`、`mcp.tool` | MCP 的服务名和工具名 | MCP 调用上的 `tool.*` |
| `skill.name` | skill 名 | skill 规则的评估点 |
| `cwd` | 工作目录 | 全部 |

`tool.added` 的意义：删掉一行 TODO 的补丁里也有 "TODO"，用 `tool.input` 判断会误报；`tool.added` 只看新增的行。

**操作**：`regex`、`shell_regex`、`contains`、`equals`、`not_contains`、`starts_with`、`ends_with`。

- `regex` 用 Go RE2：**搜索语义**（不要求整串匹配），可以用 `(?i)`，不支持前后查找和反向引用，不会有回溯爆炸。在 YAML 里建议用单引号写 `pattern`。
- `shell_regex` 把字段值当作 shell 命令读，拆成 shell 会执行的各条简单命令，**任一条**匹配 RE2 `pattern` 就满足；`^`、`$` 锚定的是单条命令。每条命令去掉引号后按单个空格连接参数，含空白或特殊字符的参数重新加单引号，所以引号里的 `--force` 不会被当成独立参数。会继续读：`;`、`&&`、`||`、`|`、`&`、换行分隔的命令，子 shell，`$(...)` 和反引号，`sh`/`bash`/`zsh`/`dash`/`ksh` 的 `-c` 脚本和喂给它们的 heredoc、here-string（shell 自己取值的选项连同其值一起跳过：`--rcfile`、`--init-file`，以及短选项簇里的每个 `o`/`O`，如 `-o pipefail`、`-eo pipefail`、`-O extglob`、`+O nullglob`），没有脚本文件参数（或带 `-s`）的 shell 从管道读到的 `echo`/`printf` 输出和 `cat` 的 heredoc/here-string，`eval` 参数，未加引号 heredoc 里的命令替换，`find` 的 `-exec`/`-execdir`/`-ok`/`-okdir` 命令（到 `;` 或 `+` 为止）。当作数据：注释、重定向目标、加引号的 heredoc 正文、喂给非 shell 命令的 here-string。命令名前的变量赋值和保留字（`if`、`then`、`do`、`!` 等）会被去掉；不带选项的 `command`、`builtin`、`exec`、`nohup`、`env`、`time` 也会被去掉。`sudo`、`doas`、`env`、`nice`、`timeout`、`stdbuf`、`xargs`、`time`、`exec` 带选项时，整条命令照常参与匹配，跳过包装命令自己的选项（及其值、`timeout` 的时长）后的命令再作为一条命令读一遍，可以层层嵌套；`env -S` 的值按空白拆开放在前面。不展开：未列出的 shell（如 `fish`）、脚本文件和 `source` 的内容、`cat 文件 | sh`、`xargs` 从标准输入读到的参数、shell alias/函数、git alias 等由配置决定的展开。这是尽力而为的读取，不是完整 shell：输入不完整时不报错，读到结尾为止；嵌套最多 8 层。
- 其余操作区分大小写。
- 否定操作（`not_contains`）在字段取不到时同样算**不满足**。在可能取不到的字段上用否定操作，validate 给警告 `negated-optional-field`。

## 5. 能力和降级

每条规则先换算成"需要哪个能力、挂在哪个统一事件上"，再问适配器能不能做：

| 动作 | 需要的能力 |
|---|---|
| `inject` | `add-context` |
| `warn` | `system-message` 和 `add-context`，至少一个 |
| `block` | `deny` |

两家的能力（✓ 支持，— 不支持；斜杠前是 Codex，后是 Claude Code）：

| 统一事件 | Codex 原生 | Claude 原生 | add-context | system-message | deny |
|---|---|---|---|---|---|
| `session.start` | SessionStart | SessionStart | ✓ / ✓ | — / ✓（U-C5） | — / — |
| `prompt.submitted` | UserPromptSubmit | UserPromptSubmit | ✓ / ✓ | ✓ / ✓（U-C5） | ✓ / ✓ |
| `skill.explicit` | UserPromptSubmit（`$名字`） | UserPromptExpansion | ✓ / ✓ | ✓ / ✓（U-C5） | ✓ / ✓ |
| `tool.pre` | PreToolUse | PreToolUse | ✓ / ✓ | ✓ / ✓（U-C5） | ✓ / ✓ |
| `tool.post` | PostToolUse | PostToolUse | ✓ / ✓ | ✓ / ✓（U-C5） | — / — |
| `tool.error` | 无 | PostToolUseFailure | — / ✓ | — / ✓（U-C5） | — / — |
| `subagent.start` | SubagentStart | SubagentStart | ✓（Codex F7；真实 context 投递经 U7 验证） / — | — / — | — / — |

Codex 一列以 agenthooks `capability.go` 为准；Claude 一列来自官方文档，其中 `systemMessage` 在各事件上是否真的显示给用户待测（U-C5）。`tool.post` 上 STYLE 不用 `decision: block`（工具已经执行，不是"阻止"）。

**降级阶梯**（`fallback: degrade` 时；每一步写进安装报告，`index.json` 的 `degraded_from` 记下原本的事件）：

1. **`tool` / `mcp` / `skill` 的 `inject` / `warn`**：先试 `at` 指定的事件，再试另一个（`after` ↔ `before`）。都不行时，只有"`inject`、没有 `when.conditions`、正文是静态的"规则可以改成常驻静态 system 一句："使用 X 时：<正文>"，这句算进常驻预算；其他情况丢弃并告警。
2. **动态 `system`**：先试 `session.start`；不行时改用 `prompt.submitted` + `repeat: session`；还不行时，没有 `when` 且正文静态的写进静态位置，否则报错。
3. **`warn`**：缺一个能力就只发另一个；两个都缺就跳过并告警。
4. **`block`**：没有 `deny` 时降级为 `warn`，安装报告里标红。`block` 不走第 1 条的静态回退。
5. **盲区**：工具在这个 harness 上根本看不到（Codex 的 `file.read` 等），报"永不触发"，不降级。
6. **`mode: replace`**：harness 没有替换位置（M1 的 Claude）时，`degrade` = 跳过并标红；`error` = 拒绝安装。

`fallback: skip` 直接跳过并告警；`fallback: error` 时任何一步降级都让安装整体中止。

在 Claude 和 Codex 上，绝大多数规则都是原生支持的。M1 里阶梯主要用在三处：Codex 的工具盲区、Codex 的 `subagent.start`（F7 之前；U7 已验证 F7 的真实投递）、Claude 的 `replace`。

## 6. 运行时数据（`reads`）

**`reads` 的写法**：

```yaml
reads:
  model: true
  thinking_effort: true
  harness: [name, version, cwd, permission_mode, agent, session_id, turn_id]
```

键只能是 `model`、`thinking_effort`、`harness`。模板或 `when` 用了没声明的数据，报错 `reads-missing`；声明了没用，给警告 `reads-unused`。规则文件 frontmatter 里的 `reads` 并入包级。

**两家的数据从哪来**：

| 数据 | Codex | Claude Code |
|---|---|---|
| `model` | 每个事件 stdin 的 `model`【源码】 | 只有 SessionStart 带，而且不一定有（例如 `/clear` 之后）；之后用 `PostModelSwitch` 的 `to_model` 更新【文档】 |
| `thinking_effort` | **hook 输入里没有**。读配置里的请求值 `model_reasoning_effort`，0.159.3 按第一个有值的来源解析：命令行 `-c model_reasoning_effort=…`（从祖先进程恢复）→ 明确受信项目 `.codex/config.toml` → `-p` 指定的 `CODEX_HOME/<name>.config.toml` → 用户 `config.toml`。**判断不了项目信任就不读项目层**；生产当前采用此回退。缺失 profile 文件可继承用户配置；新 CLI 拒绝 legacy profile selector/选中 table。旧/未知版本保留原表式 profile → 受信项目 → 用户的兼容解析。配置都没有就取不到，不推断模型默认强度。每个会话第一次需要时解析一次。会话中途 `/model` 改强度看不到；`plan_mode_reasoning_effort` 不读【M1-FINAL-02 配置实测；模型投递独立验收】 | 工具类事件（`PreToolUse`、`PostToolUse`、`Stop`…）里的 `effort.level`；其他事件读环境变量 `$CLAUDE_EFFORT`（所有 hook 进程里是否都有待测 U-C1）；都没有就用状态里最近一次的值【文档】 |
| 回合 | `turn_id`，SessionStart 上没有【源码】 | `prompt_id`，第一条提示之前没有【文档】 |
| 会话来源 | SessionStart 的 `source` | 同左 |
| agent | `agent_id`、`agent_type` | 同左；主 agent = 没有 `agent_id`【文档】 |
| `cwd`、`permission_mode`、`session_id` | stdin | stdin |
| harness 版本 | 适配器安装时检测，写进锁文件 | 同左 |

**状态刷新**：事件里带了值就覆盖状态；没带就读状态；**从不用空值覆盖**已有的值。状态按会话保存（CLI 设计 5.3）。

## 7. token 预算

### 7.1 算什么，不算什么

- **算**：进模型上下文的文字，也就是 `inject` 的正文、`warn` 的 `model_text`、工具调用被拦的理由。
- **不算**：`warn` 给用户的全文、提示词被拦的理由（只给用户看）、没命中的规则（hook 什么都不输出，不花 token，只花时间）。前提是没命中时输出必须为空，不能输出"没有规则"之类的占位。
- **单次注入上限**（适配器声明）：

  | harness | 上限 | 单位 | 超了会怎样 |
  |---|---|---|---|
  | Claude Code | 10,000，每个字段单独算，不能调 | 字符 | 写进会话目录的文件，只给路径和前 2,000 字符预览，**不会叫模型去读**【文档】 |
  | Codex | 约 2,500（`additionalContextLimit` 可调，STYLE 不写这个字段，用默认值当保险） | token | 落盘，只留预览【源码】 |

  两家超限后规则都等于悄悄失效，所以 `T003` 是错误而不是警告。中文大约一字一 token，同一段中文在 Codex 上比在 Claude 上早得多触到上限。

### 7.2 预算字段

```yaml
budget:            # 包级，可省略；下面是默认值（未经实测，只是起点）
  per_rule: 400    # 单条规则给模型的文字
  per_event: 1200  # 本包在同一个原生 hook 事件上最坏情况的合计
  always_on: 600   # 本包的常驻部分
  on: warn         # T001、T002、T004 的严重程度：warn | error
```

- **`per_rule`**：每条规则给模型的那份文字的估算。规则上的 `max_tokens` 优先。
- **`per_event`**：本包在同一个原生 hook 事件（及同一个工具）上，**所有可能同时命中的条目**的估算合计。条件能不能同时成立在编译时不知道，所以按全部都命中算（最坏情况）。
- **`always_on`**：写进静态位置的规则，加上挂在 `session.start` 上、没有 `when` 的条目。它们每个会话都付；静态位置是否也进子 agent 由 harness 决定。
- **单位**：预算一律按 token 估算。适配器上限另按它自己的单位检查（Claude 数字符）。

### 7.3 lint 编码

| 编码 | 什么时候 | 严重程度 | 何时检查 |
|---|---|---|---|
| `T001` | 单条规则超 `per_rule`（或它的 `max_tokens`） | 跟 `budget.on` | validate、plan |
| `T002` | 本包同一事件合计超 `per_event` | 跟 `budget.on` | validate、plan |
| `T003` | **所有已装包**在同一事件上的最坏合计超过适配器上限的 80%（Claude 8,000 字符；Codex 2,000 token） | **永远是错误**，安装中止 | install、sync |
| `T004` | 本包常驻部分超 `always_on` | 跟 `budget.on` | validate、plan |
| `T005` | `block` 理由超过 40 token | 警告 | validate |
| `T006` | 模板用了每次都变的变量（`harness.session_id`、`harness.turn_id`） | 警告 | validate |
| `T007` | `inject` 用了 `repeat: every` 或 `turn`，又没写 `reason:` | 警告 | validate |

`T003` 是"全部已装包"的函数，所以在安装和同步时检查；其余按单个包检查。跨包去重（正文完全相同的规则只留一份）是 M2 的事（CLI 设计 T8）。

### 7.4 估算

默认离线估算，不联网，不依赖分词库：英文和数字约 4 字符 1 token，中日韩字符 1 字 1 token，标点和空白 0.5。结果标 `≈`，误差估计在 ±25% 左右，这是猜测，上线前要用真实规则校准（U-T3）。动态正文里的变量按固定占位长度估。具体算法、`style plan` 的 token 报告格式和运行时计数见 CLI 设计第 11 节。

## 8. 安全

### 8.1 输出白名单

STYLE 的 hook 输出**只允许**这几样：`additionalContext`、`systemMessage`、拒绝（PreToolUse 的 `permissionDecision: "deny"` + 理由；提示词和显式 skill 的 `decision: "block"` + 理由）、Claude 的 `suppressOriginalPrompt`。

**绝不输出** `allow`、`ask`、`updatedInput`、`updatedToolOutput`、`continue: false`、`stopReason`、`initialUserMessage` 等其他字段。原因：Claude 里 `permissionDecision: "allow"` 会跳过权限提示，`updatedInput` 能改写工具参数，误输出就等于绕过了用户确认【文档】；Codex 会把 `allow`、`ask`、单独的 `updatedInput` 当成无效输出【源码】。编码层强制这条规则，测试对所有黄金输出做静态检查。

### 8.2 事件内容不进模板

用户提示、工具参数、工具输出可能被人操控。M1 里它们只能用在 `when.conditions`，不能出现在注入正文里（1.5）。M3 再考虑开放，开放时要包一层"不可信内容"的围栏、转义围栏字符、限制长度。

### 8.3 正文和标记

没有外壳，所以不存在"伪造别的包的外壳"的问题。正文里不允许出现 STYLE 的静态块标记（1.7 `marker-in-body`），否则卸载时会定位错。写进 TOML 时用 TOML 编码器转义，不手拼字符串。

### 8.4 敏感 settings

`settings[].sensitive` 是保留字段，M1 写了就报错。原因：模板里引用的值会被渲染进 `index.json` 和静态位置，最终送进模型上下文。

### 8.5 `replace`

替换内置指令是风险最高的操作。必须 `style install --allow-replace`（或交互确认），锁文件记 `replace: true`，`style doctor` 一直提示。同一个 harness 上只允许一个包用 `replace`。

### 8.6 安装前的能力披露

第一次安装本身就是信任行为。安装报告列出：有哪些 `block` 规则、有没有 `replace`、有没有 `native` 命令（M2）、读哪些运行时数据、写哪些静态位置、常驻部分的 token 估算。用户确认后才安装（`--yes` 跳过；非交互环境没有 `--yes` 就报错退出）。`settings` 或 `vars` 变化导致渲染结果变化时，`sync` 显示"渲染后的差异"。

### 8.7 信任到底防什么

> `trust` 防的是"包内容在你不知情时变了"：更新、`--link` 的源目录被改、拷贝来的包被换掉。它不防本机同一用户权限下能写 `~/.style/` 的程序，那种程序可以同时改锁文件和 index。运行时不校验 index 里的哈希，是有意的取舍：热路径不读 md、不算哈希。

实现要求：

- `trust` 读入文件字节后，**对同一份内存里的字节**算哈希、显示差异、编译，确认之后不重新读磁盘。
- `index.json`、锁文件、状态文件、静态块都用"写临时文件再改名"，读的一方永远看到完整的旧版或新版。

### 8.8 出错时

- **默认放行**：运行时任何错误（index 缺失、损坏、格式号不对，状态目录不可写，模板错误，超时）一律退出码 0、输出为空（各家"没意见"的写法），错误写进日志。
- **代价**：index 损坏时 `block` 规则会静默失效。`style doctor` 必须能发现"index 和锁文件不一致或已损坏"并标红。
- **Claude 可选 `--fail-closed`**：只给 PreToolUse 那一条加 `onFailure: "block"`（Claude ≥ v2.1.295【文档】），这时运行时在内部出错时退出码改为 1，Claude 就会拦下这次工具调用。**默认关闭**，否则 index 一坏，所有工具调用都被拦，Claude 完全不能用。Codex 不提供这个选项（行为没核实）。
- 规则级的 `fail: closed` 留到 M3。
