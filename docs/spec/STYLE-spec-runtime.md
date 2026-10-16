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
- **`thinking_effort_approx`**：`true` / `false`，要求档位是或不是近似值。M1 两个适配器的已知映射均为 `false`；未知档位同时清除 approx，不沿用旧值。字段留给以后的预算型 harness。
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
| Claude Code | `low` `medium` `high` `xhigh` `max` 同名；没有 `minimal` | 工具类事件的 `effort.level`，或环境变量 `$CLAUDE_EFFORT`【文档】。当前模型不支持 effort 参数时不报，取不到。FINAL-06 U9：所测界面无off选项，控制配置的关闭尝试仍发adaptive请求，hook仍high；没有可用off信号，Claude上不产出`none`，档位仅反映原生effort参数，不能据此断言思考开关 |

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

Codex 一列以 agenthooks `capability.go` 为准；Claude 一列结合官方文档与FINAL-06真实验收：启动/提示/工具前后及失败/显式技能六类USER标记可见；clear来源的启动USER已输出但界面未显示，不保证所有来源均可见。`tool.post` 上 STYLE 不用 `decision: block`（工具已经执行，不是"阻止"）。

**降级阶梯**（`fallback: degrade` 时；每一步写进安装报告，`index.json` 的 `degraded_from` 记下原本的事件）：

1. **`tool` / `mcp` / `skill` 的 `inject` / `warn`**：先试 `at` 指定的事件，再试另一个（`after` ↔ `before`）。都不行时，只有 `inject`、没有任何 `when`、正文静态且有可用 append 槽位的规则可退到带目标限定前缀的常驻静态指令（例如 `When using file.edit: <正文>`），计入常驻预算；否则跳过并报告原因。skill 另有显式入口，保留可用部分时报告 partial。
2. **`system`**：无任何 `when` 的静态 inject 优先用静态槽位；槽位不可用时，degrade 先选 `session.start`，再选 `prompt.submitted` + `repeat: session`，只选一个。带运行时变量、条件或 warn 的动态 system 在两点均可用时双挂载，prompt 设 reevaluate，以正文哈希去重；仅一点可用时 degrade 保留该点。两点都缺条件数据时，非 error 为 never-trigger；无可用点的其他情况，skip 跳过，degrade/error 为 install-error。不将动态条件规则转成无条件静态文字。
3. **`warn`**：缺一个能力就只发另一个；两个都缺就跳过并告警。
4. **`block`**：没有 `deny` 时降级为 `warn`，安装报告里标红。`block` 不走第 1 条的静态回退。
5. **盲区**：工具在这个 harness 上根本看不到（Codex 的 `file.read` 等），报"永不触发"，不降级。
6. **`mode: replace`**：harness 没有替换位置（M1 的两家均无）时，`degrade` / `skip` = 跳过；`error` = 拒绝安装。
7. **`target: event`**：只评估指定事件，不自动换事件或静态回退。Codex 的无条件 `tool.error` 缺输出能力，degrade/skip 为 skipped、error 为 install-error；若先发现条件数据不可用，非 error 可为 never-trigger。

`degrade` 允许最终跳过，不保证保留规则。`skip` 在普通能力不足时移除规则，不尝试替代路径；`error` 在普通能力或数据失败时阻止安装。harness 过滤、M1 不求值的 provider、native 留到 M2、固有工具盲区等前置分支仍按各自状态报告，不承诺 error 拦住所有未生效规则。

当前实例与源码入口见 [adapter 总览 §6](../STYLE-ADAPTERS-CURRENT.md#6-fallback当前代码语义)。Codex `subagent.start` 已真实投递，不再作为缺能力的例子。Claude 各事件用户提示可见性仍待 FINAL-06 实测，表中声明不等同完整 UI 验收。

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
| `thinking_effort` | **hook 输入里没有**。读配置里的请求值 `model_reasoning_effort`，0.159.3 按第一个有值的来源解析：命令行 `-c model_reasoning_effort=…`（从祖先进程恢复）→ 明确受信项目 `.codex/config.toml` → `-p` 指定的 `CODEX_HOME/<name>.config.toml` → 用户 `config.toml`。**判断不了项目信任就不读项目层**；生产当前采用此回退。缺失 profile 文件可继承用户配置；新 CLI 拒绝 legacy profile selector/选中 table。旧/未知版本保留原表式 profile → 受信项目 → 用户的兼容解析。配置都没有就取不到，不推断模型默认强度。每个会话第一次需要时解析一次。会话中途 `/model` 改强度看不到；`plan_mode_reasoning_effort` 不读【M1-FINAL-02 配置实测；模型投递独立验收】 | 每个事件按 `effort.level` → `$CLAUDE_EFFORT` → 状态最近一次值解析；显式未知原值清除旧tier，不回退旧档位。FINAL-06实际工具前/后/失败两处均有值，启动/提示/显式技能均没有；所测关闭配置仍为adaptive，不产出none，未观测实际off原值 |
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

  超限后无法保证完整正文进入当前上下文，因此保留 T003 错误保护。FINAL-08 绕开 STYLE 的 Codex 12k/50k 字符原生探针证实完整落盘、模型收到约10,100字符头尾预览和路径；不能用首尾标记都出现推断完整投递。Claude 表中为文档依据，原生超限仍待隔离认证实测。上述字符测量不构成官方 token 计数，不能据此承诺语言间的具体 token 比例。

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

`T003` 是"全部已装包"的函数，所以在安装和同步时检查；其余按单个包检查。M2-D3 跨包去重（CLI 设计 T8）的实施契约见 §7.5；T30 已实现。

### 7.4 估算

默认离线估算，不联网，不依赖分词库：英文和数字约 4 字符 1 token，中日韩字符 1 字 1 token，标点和空白 0.5。结果明确标为估算，不提供百分比误差保证；40 条固定规则的两家官方计数校准待 API 认证（U-T3 / FINAL-09）。动态正文里的变量按固定占位长度估。具体算法、`style plan` 的 token 报告格式和运行时计数见 CLI 设计第 11 节。

### 7.5 M2-D3 跨包去重契约

本节为 2026-10-16 用户评审通过的 T8 实施契约，T30 已实现。**在编译阶段折叠跨包的等价规则**，先替换静态变量，再比较正文及完整执行语义。仅凭正文相同不足以删除另一条规则：例如相同拒绝理由分别用于 Bash 与 MCP，两条仍各自生效。

**候选与相等判定**：

- 每个 harness 单独分析完成 fallback、补默认值与静态变量/settings 替换的规则；只纳入实际有静态或 runtime placement 的普通规则。native 有独立副作用，即使命令相同也全部保留；skipped/never-trigger/disabled 不充当保留方。
- 静态规则比较最终写入的完整文本、slot 和 mode，含静态回退的目标前缀；字节完全相等才合并。不 trim、不折叠空白、不改换行、不做 Unicode 归一化。包名/设置渲染后不同，即使原模板相同也保留两份。
- 动态规则比较**整条规则的全部 placement 集合**：评估点、最终 action/输出能力、target、规范化 match/when、repeat、sources、reinject_on_compact、reevaluate、needs，以及静态变量替换后的 body/model_body/首句标志均须一致。条件列表只做现有规范化，不求解逻辑等价。warn 同时比较给用户与模型的两份正文。
- 动态 body 比较编译片段、变量名和默认值；渲染形状不同就保留。例如 `model={{model}}` 与固定 `model=X` 即使这一刻输出相同，仍是两条规则；`{{model}}` 与 `{{thinking_effort}}` 不因占位估算相同而合并。跨评估点的条目不单独摘除，避免破坏 system/skill 的多个入口。
- id、包名、来源路径/文件摘要、原始模板摘要、order、估算值、降级说明属于身份/报告信息，不单独阻止合并；其中包名等静态变量若进入正文，已按渲染结果比较。`body_sha256` 继续表示原模板，不能用它替代以上比较。摘要可用于分桶，最终须比较规范化结构与正文，不能只信哈希相同。
- M2 只折叠不同包之间的重复。若同一候选等价类内某个包有多条等价规则，整个类保守保留，报告 `multiple equivalent rules in one package`；避免一次跨包安装改变同包显式声明的重复次数。没有这个歧义时，两个或更多包的等价声明合成一份。

**保留顺序和运行时**：

1. 按现有 `[priority, 包安装序号, 包内序号]` 升序保留第一条；priority 可为小数。输入包遍历顺序不影响结果。被折叠规则的所有 placement 都指向同一个 `<包>/<id>` 保留方。
2. index、静态安装文本与动态订阅面只消费保留的 placement；包本身、锁文件、源文件与批准快照仍完整保留。两个包都在 list 中，未产生新输出的包也可单独卸载。
3. 运行时沿用现有 rule+渲染正文的 repeat 抢占，**不引入跨包共享的持久去重键**。条件匹配、block 每次拒绝、warn 默认 every、subagent.start 不抢占、compact/gen/epoch 与 cap-drop 释放语义均保持；等价声明在编译后只有一份，故每次调用最多执行这一份。
4. 卸载保留方、trust 或 link sync 改正文/priority 后，用剩余已批准规则重新计算；保留方可切换，新的静态块/条目来自接替包。接替规则有自己的 rule 身份，**已有会话中可能再次注入一次**；接受这一边界以保留现有状态键与 format 1，不把旧包的信任/占位迁给新包。旧 stats 不回写或重新归因。
5. 仅保留方的执行产生 output/blocked/dedup/cap-drop；被编译折叠的声明不产生 runtime 行，stats 不推测这些声明曾注入或为其虚构“去重次数”。

**plan 与预算**：

- `style plan <source>` 继续是独立、单包的离线报告，不读取 STYLE_HOME 中其他已装包；它描述这个包单独安装时的行为。跨包结果出现在 install/trust 的组合规划报告，sync/uninstall 重编译使用同一分析。
- 组合 install/trust 报告包含本次请求包与去重关系中的参与包，在被折叠规则的 plan/token 行增加可选 `deduplicated_into: "<包>/<id>"`，文本明确写出保留方；保留原 placement 能力状态和单条内容估算供检查。标记为空时 JSON 与已有 golden 保持相同。报告不得含正文或模板值。
- 各包的源级 T001/T002/T004–T007 先按自身声明校验，不能借装了别的包绕过 `budget.on: error`；其诊断明确是单包检查。组合报告的事件总计、always_on 与会话情景成本按有效 placement 计一次，折叠行不再贡献合计。原始单条估算仍可显示，标明无新增成本。
- 全部已装包的 T003 在去重结果确定后计算，保留 block 与非 block 分支取较大者的现有算法，并重算分隔符数量；不能简单把两个已拼接总数相减。真正不同的规则仍按现有最坏情况累加，不因运行时偶然输出相同降低预算。
- 源级校验、全局去重、预算与最终编译必须消费同一份已捕获输入；T003 超限仍退出 4，安装树无写入；静态 T001 继续在源级 validate 阶段退出 1，其他安装预算错误退出 4。没有去重候选时现有行为和 golden 不变。

执行语义一致限制、同包多条时保留、单包 plan/源级预算不依赖安装环境，以及保留方切换可再次注入，均已随 D3 评审通过。

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

**native 命令**（规则 3.10）是任意代码执行，披露和信任比别的规则更严：

- 安装报告逐条列出：harness、规则、原生事件、matcher、实际 timeout、作者命令原文（不含 STYLE 归属前缀），以及该路线的信任方式：`codex-user` 预置 `trusted_hash`，Codex 不再单独提示；`codex-plugin` 走插件信任；Claude 走工作区信任。
- Codex 用户路线预置 native 条目的信任，前提就是这一步的确认。所以只要本次安装、`sync` 或 `trust` 会新增或改变 native 条目，就必须交互确认或带 `--yes`；非交互又没有 `--yes` 时退出 1，什么都不写。
- native 条目变化（增、删、事件、matcher、timeout、命令任一改变）时，`sync` 退出 3，`--link` 开发包也不例外（不同于 [CLI 设计 §6](STYLE-cli-design.md) 对 `--link` 包改动只打印差异摘要的约定）。`trust` 的预览单列"native 条目差异"，按 harness/规则给出改前改后的四个字段，确认后才写入。
- native 执行顺序变化也要求重新信任；timeout 同时保留作者声明与实际生效值（例如 Codex `SessionEnd` 限制为 3 秒），声明值变化即使不改变实际值也要确认。配置中的 native 条目缺失、被改动或用户路线 `trusted_hash` 不匹配时，`sync` 只提示需要信任，以 `trust` 的预览和确认修复。普通 link 规则的自动同步约定不变。
- `style list` 显示每个包的 native 条目数；`style doctor` 逐条核对 harness 配置里的 native 条目：存在且一致、缺失、被改动（命令或字段与锁文件渲染指纹不符），后两种告警；`codex-user` 另核对对应的 `trusted_hash`。

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
