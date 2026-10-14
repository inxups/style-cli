# STYLE 分发格式 v0.3：编译产物与未决项

规范总览和已定决定见 [STYLE-spec.md](STYLE-spec.md)；前置章节见 [运行时语义](STYLE-spec-runtime.md)。

## 9. 编译产物 `index.json`

`style install` / `sync` 把所有已装包编译成一份 `~/.style/build/index.json`。**运行时只读这份文件，从不读 md**。编译步骤见 CLI 设计第 7 节。

### 9.1 顶层

| 字段 | 类型 | 说明 |
|---|---|---|
| `format` | int | 当前是 `1`（v0.2 没发布过，不升号）。运行时发现格式号不对或文件损坏，**不输出任何内容**，在 stderr 和日志里提示运行 `style sync`。运行时从不自己重建 index |
| `style_version` | string | 只给 `doctor` 展示，不参与兼容判断 |
| `packages` | object[] | `{name, version, seq, lock, link}`；`seq` 是安装序号，`lock` 是锁文件里这个包的哈希，`link` 表示是 `--link` 开发包 |
| `static` | object | `{<harness>: StaticEntry[]}`：写进静态位置的规则，只给 `doctor` 和 `explain` 用，运行时不读 |
| `harnesses` | object | `{<harness>: {<评估点>: Entry[]}}`，评估点是 `session.start`、`prompt.submitted`、`skill.explicit`、`tool.pre`、`tool.post`、`tool.error`、`subagent.start` 之一。每个列表已按 `order` 排好 |

`StaticEntry`：`{rule, slot, src, body_sha256, est}`，`slot` 是 `developer_instructions`、`model_instructions_file`、`claude_md` 之一。

### 9.2 Entry

| 字段 | 类型 | 必有 | 说明 |
|---|---|---|---|
| `rule` | string | 是 | `<包名>/<规则 id>`。**去重键用它** |
| `entry` | string | 是 | `<rule>@<评估点>`，在同一个 harness 内唯一。只用于日志、`explain` 和报告 |
| `src` | string | 是 | 正文来源：规则文件或正文文件的包内路径；`text:` 内联时是 `STYLE.yaml#<规则 id>` |
| `src_sha256` | string | 是 | 来源**文件**的 sha256，给 `trust` 比对 |
| `body_sha256` | string | 是 | 渲染前的正文（模板原文）的 sha256，用来区分"这条规则变了"和"同一文件里别的东西变了" |
| `order` | [number, int, int] | 是 | `[priority, 包安装序号, 包内序号]`，`priority` 可为有限小数，包内序号是它在 `rules[]` 里的位置 |
| `target` | string | 是 | 规则的原 target：`system`、`event`、`tool`、`mcp`、`skill`。运行时可据此区分评估点上的 selector 匹配和事件级全局规则 |
| `action` | string | 是 | 降级之后的最终动作：`inject` / `warn` / `block` |
| `match` | object | `tool`、`mcp`、`skill` target 的匹配评估点必有 | 见 9.3；`target: event` 对 `tool.*` 事件没有 selector，故不输出此字段 |
| `when` | object | 是 | 规范化后的条件，见 9.4。至少有 `agent` |
| `repeat` | string | 是 | `every` / `turn` / `session`，已按 3.5 补上默认值 |
| `reinject_on_compact` | bool | 是 | |
| `sources` | string[] | `session.start` 上必有 | 允许的会话来源，已补上默认值；`reinject_on_compact` 为 true 时额外含 `compact` |
| `reeval` | bool | 否 | `true` 表示这是动态 system 规则在 `prompt.submitted` 上的重新评估条目（3.7） |
| `needs` | string[] | 是 | 正文变量 ∪ `when` 用到的运行时数据：`model`、`thinking_effort`、`harness.<键>` |
| `body` | object | 是 | `inject`：给模型的正文；`warn`：给用户的全文；`block`：拒绝理由。写法 `{"static": "…"}` 或 `{"parts": ["文本", {"var": "model", "default": "未知"}, …]}` |
| `model_body` | object | `warn` 上必有 | 给模型的那一句，写法同 `body` |
| `model_body_first_sentence` | bool | 否 | 仅隐式生成的动态 `warn` 正文可能为 `true`；运行时在模板渲染后提取首句。显式 `model_text` 不截断，静态正文在编译时已截断 |
| `est` | object | 是 | 给模型那份文字的估算 `{tokens, chars}`；动态部分按占位长度（CLI 设计第 11 节） |
| `degraded_from` | string \| null | 是 | 原本的评估点；没降级为 `null` |

不进 index 的：`enabled: false` 的规则（编译时已剔除）、`max_tokens`、`reason`、`fallback`（都在编译时用完了）。

### 9.3 `match`

```json
{
  "canonical": ["file.edit", "file.write"],
  "names": ["apply_patch"],
  "mcp": [{"server": "github", "tool": "create_pull_request"}, {"server": "github", "tool": "*"}],
  "skill": ["code-review"]
}
```

- `canonical`、`names` 和 agenthooks 的 `ToolMatcher` 同义；`mcp` 里 `"*"` 是通配。
- 对应的 selector 列表必须非空；selector 不得为空、只含空白或含首尾空白；MCP `server` 和 `tool` 分量各自非空且不得再含 `/`。
- `target: event` 是评估点级规则，在 `tool.pre`、`tool.post`、`tool.error` 上覆盖该事件的全部工具，不带 `match`；`target: tool`、`mcp`、`skill` 则必须带对应 selector。
- `tool.pre` / `tool.post` 上的 `skill` 表示隐式调用；`skill.explicit` 上的 `skill` 表示显式调用。
- MCP 的服务名：Claude 取事件里的 `mcp_server.name`，Codex 从 `mcp__<server>__<tool>` 拆出来。

### 9.4 `when`（规范化后）

```json
{
  "model": ["gpt-5*", "**/claude-*"],
  "provider": [],
  "thinking_effort": ["high", "xhigh", "max"],
  "thinking_effort_approx": null,
  "harness": {"name": ["codex"], "permission_mode": ["default"]},
  "agent": ["main"],
  "conditions": [{"field": "tool.added", "op": "contains", "pattern": "TODO"}]
}
```

- 没写的字段省略；`agent` 永远存在，`any` 写成 `["*"]`。
- `thinking_effort` 是展开后的档位列表（可以含 `open`）；`thinking_effort_approx` 是 `true` / `false` / `null`（不限制）。
- `model` 统一转小写。

### 9.5 示例（和示例包一致）

[example/terse-reviewer](../example/terse-reviewer/STYLE.yaml) 的 `rules[]`：0 `no-todo-comments`（`from:` 引入，`priority: 40`）、1 `style`、2 `session-info`、3 `low-effort-guard`、4 `review-skill`、5 `after-git-diff`、6 `before-create-pr`、7 `no-force-push`、8 `on-review-prompt`、9 `codex-only-notify`。包是第 1 个安装的。下面摘几个 Codex 上的条目（哈希省略）：

```json
{
  "format": 1,
  "style_version": "0.3.0",
  "packages": [{"name": "terse-reviewer", "version": "0.3.0", "seq": 1, "lock": "sha256:…", "link": false}],
  "static": {
    "codex": [{"rule": "terse-reviewer/style", "slot": "developer_instructions", "src": "prompts/style.md",
               "body_sha256": "…", "est": {"tokens": 45, "chars": 62}}]
  },
  "harnesses": {
    "codex": {
      "session.start": [
        {"rule": "terse-reviewer/session-info", "entry": "terse-reviewer/session-info@session.start",
         "src": "prompts/session-info.md", "order": [50, 1, 2], "action": "inject",
         "when": {"agent": ["main"]}, "repeat": "session", "reinject_on_compact": true,
         "sources": ["startup", "clear", "fork", "compact"], "needs": ["model", "thinking_effort"],
         "body": {"parts": ["当前会话：模型 ", {"var": "model", "default": "未知"},
                            "，推理强度 ", {"var": "thinking_effort", "default": "未知"}, "。"]},
         "est": {"tokens": 45, "chars": 60}, "degraded_from": null}
      ],
      "prompt.submitted": [
        {"rule": "terse-reviewer/session-info", "entry": "terse-reviewer/session-info@prompt.submitted",
         "src": "prompts/session-info.md", "order": [50, 1, 2], "action": "inject", "reeval": true, "when": {"agent": ["main"]},
         "repeat": "session", "reinject_on_compact": true, "needs": ["model", "thinking_effort"],
         "body": {"parts": ["…同上…"]}, "est": {"tokens": 45, "chars": 60}, "degraded_from": null}
      ],
      "skill.explicit": [
        {"rule": "terse-reviewer/review-skill", "entry": "terse-reviewer/review-skill@skill.explicit",
         "src": "prompts/skill-code-review.md", "order": [50, 1, 4], "action": "inject", "match": {"skill": ["code-review"]},
         "when": {"agent": ["main"]}, "repeat": "session", "reinject_on_compact": false, "needs": [],
         "body": {"static": "使用 code-review skill 时，本仓库要求每条问题带 file:line，最多 10 条。"},
         "est": {"tokens": 30, "chars": 44}, "degraded_from": null}
      ],
      "tool.pre": [
        {"rule": "terse-reviewer/no-force-push", "entry": "terse-reviewer/no-force-push@tool.pre",
         "src": "STYLE.yaml#no-force-push", "order": [50, 1, 7], "action": "block", "match": {"canonical": ["shell"]},
         "when": {"agent": ["*"], "conditions": [{"field": "tool.input.command", "op": "regex",
                  "pattern": "git\\s+(?:-C\\s+\\S+\\s+)?push\\b.*(?:\\s--force(?:\\s|$)|\\s-f(?:\\s|$)|\\s\\+\\S)"}]},
         "repeat": "every", "reinject_on_compact": false, "needs": [],
         "body": {"static": "评审风格下不允许 force push，请改用新提交。"},
         "est": {"tokens": 18, "chars": 22}, "degraded_from": null}
      ],
      "tool.post": [
        {"rule": "terse-reviewer/no-todo-comments", "entry": "terse-reviewer/no-todo-comments@tool.post",
         "src": "rules/no-todo-comments.md", "order": [40, 1, 0], "action": "warn",
         "match": {"canonical": ["file.edit", "file.write"]},
         "when": {"agent": ["*"], "conditions": [{"field": "tool.added", "op": "contains", "pattern": "TODO"}]},
         "repeat": "every", "reinject_on_compact": false, "needs": [],
         "body": {"static": "本仓库不在代码里留 TODO 注释，没做完的事列在回复里。新增的代码里出现了 TODO。"},
         "model_body": {"static": "本仓库不在代码里留 TODO 注释，没做完的事列在回复里。"},
         "est": {"tokens": 26, "chars": 27}, "degraded_from": null}
      ]
    }
  }
}
```

（`est` 的数字是示意，实际以估算器为准。）

## 10. 安装后的状态和静态位置

这一节只写语义：装完之后应该是什么状态。命令、目录、路线选择、卸载步骤见 CLI 设计第 4–10 节。

1. 包被复制到 `~/.style/packages/<名字>/`；`~/.style/style.lock.yaml` 记录每个包的版本、来源、安装序号和逐文件 sha256。
2. 所有已装包编译成一份 `~/.style/build/index.json`。
3. 每个目标 harness 上装着 hook，命令都是 `style agenthooks run --provider=<harness>`：
   - **Codex**：一组**固定**的 hook（订阅面不随规则变化），哪条规则命中由运行时读 index 判断。装、卸、更新包通常只改 index 和静态块，不改 hook 定义，也就不用重新信任。默认装成插件 `style-runtime`。
   - **Claude Code**：订阅面**跟着规则走**（Claude 没有逐个 hook 的哈希信任，只有工作区信任【文档】），装、卸包时重写 `settings.json` 里 STYLE 的条目。
4. 无条件、正文静态的 `system` 规则写在静态位置：

   | harness | `append` | `replace` |
   |---|---|---|
   | Codex | `config.toml` 的 `developer_instructions`，每个包一段，用 `<!-- style:<名字> -->` … `<!-- /style:<名字> -->` 包住。**只在用户自己没设 `developer_instructions` 时由 STYLE 管理这个键**；用户已经设了就不碰，这些规则改走动态路径（`session.start` + `reinject_on_compact`），报告里说明 | `model_instructions_file` 指向 `~/.style/build/codex-instructions.md`。要 `--allow-replace`；用户已设这个键时报错 |
   | Claude Code | `~/.claude/CLAUDE.md` 里每个包一段标记块（U-C4 证明用户级 `~/.claude/rules/` 生效后，M2 改成每包一个文件 `~/.claude/rules/style-<名字>.md`，卸载更干净） | M1 不支持（第 5 节阶梯第 6 条） |

   静态位置在前缀里：不改就一直被缓存，每次请求只付命中价；改了会让之后新会话的缓存重写一次。只有"几乎每个请求都要生效、而且很短"的规则才适合放这里，常驻预算（7.2）管着它。
5. 改过的 md 不会自动生效：要 `style sync` 重新编译，并经 `style trust <名字>` 确认。这补上了 Codex 信任哈希不覆盖 md 内容的缺口。`--link` 开发包第一次 `trust` 之后，后续改动只在 `sync` 时显示差异摘要，不阻塞。

## 11. 依赖的 fork 改动

本文的语义依赖下面几项 fork 改动。完整清单（原因、源码位置、里程碑）见 CLI 设计第 13 节，编号以那里为准。

| 编号 | 改动 | 本文哪里用到 |
|---|---|---|
| F5 | `SessionInfo` 加 `Effort`、`EffortRaw`、`EffortApprox`、`HarnessVersion`；映射由适配器的 `effort.yaml` 提供 | 4.2、第 6 节 |
| F6 | 按会话保存的状态（model、effort、gen、epoch、去重占位） | 3.5–3.7 |
| F7 | Codex 的 `subagent.start` 能返回 context | 3.3、第 5 节 |
| F8 | Codex skill 识别放宽，加上显式 `$名字` | 3.3 |
| F9 | Claude codec 补字段和事件：`prompt_id`、`effort.level`、`mcp_server`、`UserPromptExpansion`、`PostModelSwitch`，编码 `suppressOriginalPrompt` | 3.2、3.3、3.5、3.7 |

## 12. 和格式语义有关的未决项

所有要实测的项、测法和测之前的默认做法统一在 CLI 设计第 15 节。和本文语义直接相关的是：

- **U-T1**：resume、fork 之后，之前注入的文字是否还在上下文里。决定 `sources` 的默认值（3.6）。
- **U-T2**：压缩之后，hook 注入的文字是否被摘要保留。如果保留，`system` 的 `reinject_on_compact` 默认 `true` 就偏保守，要不要改由用户定。
- **U11**：Codex `/compact` 之后 `developer_instructions` 是否仍对模型可见（3.6、第 10 节）。
- **U-C1**：Claude 的 `$CLAUDE_EFFORT` 在 SessionStart / UserPromptSubmit 的 hook 进程里是否存在（第 6 节）。
- **U-C5**：Claude 各事件上的 `systemMessage` 是否真的显示给用户（3.2、第 5 节）。
- **U9**：Claude 的 hook 能否知道思考被关闭（4.2）。
- **U-T3**：估算器误差；预算默认值（7.2）都是起点，要用真实规则校准。
- **M3 再定**：`provider` 的数据来源；事件内容带围栏进模板；"取不到"能否在列表里写 `unknown` 匹配；用户自定义 effort 映射；`refresh`（隔 N 回合补一次，token 设计 T4）；规则级 `fail: closed`。

## 附录 A：推迟的 harness 备忘

M1 不实现，留给以后写适配器时参考。详细资料在 [cross-harness.md](../style-gen/cross-harness.md) 和 [thinking-effort.md](../style-gen/thinking-effort.md)（这两份里"六档"、`on_unknown` 等旧口径已被本文取代）。

**effort 映射**（以后的适配器照 4.2 的规则写自己的 `effort.yaml`）：

| 来源 | 原生值 → 档位 | 备注 |
|---|---|---|
| Gemini 3 / Gemini CLI `thinkingLevel` | `minimal` `low` `medium` `high` 同名 | 没有 `xhigh`、`max`；Gemini CLI 的值来自配置文件 |
| DeepSeek | `low` `medium` `high` `xhigh` `max` 同名；`minimal` 同名；`ultra` → `max`；`thinking: disabled` → `none` | 服务端会折叠成 low / high / max，只管请求值 |
| Grok | `low` `medium` `high` 同名 | 第三方文档，待核实 |
| Cursor | `model_params` 里 `effort` 的值和七档同名时对应；其他写法取不到 | 实际格式待实测 |
| OpenCode | variant 名和七档同名时对应；其他取不到 | variant 名可自定义 |
| Kimi、GLM（只有开关） | 关 → `none`；开 → `open` | 开关从哪读还没定，没定之前一律取不到 |
| Gemini 2.5、Qwen、Claude 旧 `budget_tokens`（预算） | 按下表近似分档，`EffortApprox` = true | |

**预算阈值表**（STYLE 自己定的近似值，不是厂商定义；预算永远不映射到 `max`）：

| 预算（token） | 0 | 1–1024 | 1025–4096 | 4097–16384 | 16385–32768 | 32769 以上 | -1（动态） |
|---|---|---|---|---|---|---|---|
| 档位 | `none` | `minimal` | `low` | `medium` | `high` | `xhigh` | 取不到 |

**能力提醒**：Cursor 的 `additional_context` 能否送到模型各方说法矛盾；Gemini 会转义 `<>`；Kimi 只有提示词事件的纯文本能进上下文；Windsurf 完全不能注入；OpenCode 是进程内函数，每次请求重建系统提示词，不需要压缩处理。加这些 harness 时，压缩信号可能需要"信号和动作分开、延后补发"的做法（见 [solution-2026-10-12.md](../style-review/solution-2026-10-12.md) A2），适配器接口里的 `CompactSignal()` 就是为它留的。
