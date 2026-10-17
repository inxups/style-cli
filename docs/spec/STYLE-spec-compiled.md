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

`StaticEntry`：`{rule, slot, src, body_sha256, est}`，`slot` 是 `developer_instructions`、`model_instructions_file`、`claude_rules` 或兼容旧产物的 `claude_md`。index format 不变。

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

[example/terse-reviewer](../../example/terse-reviewer/STYLE.yaml) 的 `rules[]`：0 `no-todo-comments`（`from:` 引入，`priority: 40`）、1 `style`、2 `session-info`、3 `low-effort-guard`、4 `review-skill`、5 `after-git-diff`、6 `before-create-pr`、7 `no-force-push`、8 `on-review-prompt`、9 `codex-only-notify`。包是第 1 个安装的。下面摘几个 Codex 上的条目（哈希省略）：

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
         "when": {"agent": ["*"], "conditions": [{"field": "tool.input.command", "op": "shell_regex",
                  "pattern": "^(?:(?:\\S*/)?git|'[^']*/git')(?:\\s+(?:-[cC]|--(?:git-dir|work-tree|namespace|attr-source))\\s+\\S+|\\s+-\\S+)*\\s+push(?:\\s+(?:-(?:[^\\s\\-o][^\\so]*)?o(?:\\s+|)\\S+|-(?:[^\\s\\-o][^\\so]*)?|--\\S*|[^\\s-]\\S*))*\\s+(?:--force|--mirror|-[^\\s\\-o]*f\\S*|\\+\\S+)(?:\\s|$)"}]},
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

### 9.6 M2-D3 编译去重契约

相等判定、保留顺序、预算与运行时语义见 [运行时 §7.5](STYLE-spec-runtime.md#75-m2-d3-跨包去重契约)。T30 已实现，保持当前 schema。

- 在所有包完成 placement 构造、静态变量替换与排序后统一判定；只折叠不同包的完整等价规则，静态文本与 runtime 条目采用同一个保留方。动态规则的全部入口一起保留或折叠，不逐事件选择不同包。
- `compile.Result.Deduplicated` 额外返回不写入 index 的去重关系，包含 harness、被折叠规则、保留规则与对应 placement，交给 app 回填组合 plan/token 报告；`DeduplicationWarnings` 为同包多重声明类提供保留原因。`core` 不读取 lock/source、不导入具体 adapter；agenthooks 仍只负责最终 hook/静态块的 owner 合并、信任、写入与还原。
- `index.json` 继续 `format=1`，无新必填字段；`packages` 保留全部包，`static`/`harnesses` 仅保留最终有效条目，`StaticBodies` 同步折叠。保留条目的 rule/entry、order、src、src_sha256、body_sha256 与正文均来自原包，不合成来源或别名身份。
- 去重关系只用于冷路径报告；runtime 不解析别名、不重新选择保留方、不扫描包。不新增共享 state 键。升级后未 sync 的旧 index 继续保持旧行为；重新 install/sync/trust/uninstall 编译后开始去重。
- doctor 以同样的全包编译结果核对 index 和静态块；卸载任何包仍重建其他已批准包的产物，最终卸载所有包的外部文件字节与权限保证保持。

### 9.7 M3-D1 编译说明文件评审稿

本节是已获用户确认的 D1 设计，已由 M3-T10 实现并通过 [M3-A1](../review/M3-A1-2026-10-17.json)。用于 explain 解释已经被编译删除的跨包等价声明，详细契约见 [D1 方案](../review/M3-D1-DIAGNOSTICS-2026-10-17.md)。当前 format=1 index、lock、state 均不改；runtime 仍不读取这份冷路径文件。

新增 `build/explain.json`，权限 0600，严格 JSON、上限 16 MiB。顶层为 `format=1`、`index_sha256`（带 sha256: 前缀，对最终 index 文件的精确字节计算）、`harnesses`。每个 harness 保存本次冷编译实际得到的规则说明：canonical rule ID、来源包内路径、RulePlan 的 target/action/status、placement 的 event/static_slot、编译折叠的 deduplicated_into 和安全原因代码；native 可附 event/matcher/实际 timeout。

说明来自同一次已批准输入的 RulePlan、compile.Result.Deduplicated 与最终 index，不能重读新源推测。不得存正文、settings/vars 值、原生命令、工具/prompt 内容、凭据或原始配置；更早阶段已被移除且没有记录的声明不补造“未命中”原因。按 harness、规则稳定顺序输出，保留的 runtime/static placement 与折叠引用的保留方必须能与最终 index 对上；skipped/native 等说明不伪造 index entry。

所有成功的 install/sync/trust/保留包卸载重编译，把说明和 index 纳入现有同一文件事务，失败恢复两者及旧权限；最后一包卸载随 index 一同移除说明。不能单独增加信任，也不把说明当批准快照或运行时输入。

explain 只读捕获 index 和说明，精确摘要相同且结构/引用有效才使用；缺失、格式错误、超限、symlink 或摘要不符给 warning，按 index-only 解释，不编译源码也不补写文件。并发更新时可重新捕获一组一次，仍不相符就如实降为 index-only，不拼接两代元数据。旧安装缺说明继续正常运行，doctor 不因缺这份可选文件判损坏；下一次正常成功重建才生成。

static 仍只提供 index 的位置/摘要/估算，不证明原生已加载；native 说明只表示冷路径计划，不执行命令。compile-deduplicated 与运行时 repeat 的 dedup 分列，既有 stats 归因不变。A1-09/A1-10 已专项验收这些边界，包含三条安装路线、无说明的旧安装、实际应用后的取消回滚与 index/说明更新窗口；结果见上述 A1 报告。

### 9.8 M3-D4 refresh 产物评审稿

以下为已获用户确认的 [D4 §8](../review/M3-D4-REFRESH-2026-10-17.md#8-编译与旧安装兼容)，T40 已实现新版 engine 严格读取 1/2：只有最终有效 runtime entry 含正数 refresh 才生成 format 2 并附该字段；无有效 refresh 和显式 0 继续 format 1，旧 golden 保持。format 1 禁止夹带 refresh；旧 runner 必须拒绝整份新格式，不部分运行或忽略周期。

session/scope/旧 seen 与 lock 1/2/3 保持现有格式，新增 scope refresh 状态使用独立 format 1，snapshot 需要同时捕获。build/explain.json 可选说明加入安全周期元数据并和 index 同事务、绑定实际摘要；旧说明缺失回退与冻结重现边界保持。跨包等价比较必须包含完整周期/placement，项目 effects 与批准快照必须覆盖新效果。A9-09/A9-11/A9-12 的旧二进制门禁、只读重放与五路线事务见 [A9 报告](../review/M3-A9-2026-10-17.json)。先升级配置中实际调用的 runner，再重编译；doctor 提供 format 2 升级提示，旧程序错误信息及失败策略沿原行为。用法见 [周期补发说明](../STYLE-REFRESH.md)。

## 10. 安装后的状态和静态位置

这一节只写语义：装完之后应该是什么状态。命令、目录、路线选择、卸载步骤见 CLI 设计第 4–10 节。

1. 包被复制到 `~/.style/packages/<名字>/`；`~/.style/style.lock.yaml` 记录每个包的版本、来源、安装序号和逐文件 sha256。
2. 所有已装包编译成一份 `~/.style/build/index.json`。
3. 每个目标 harness 上装着普通规则的 hook，命令为 `style agenthooks run --provider=<harness>`；native 命令另按规则 3.10 写入，不经过运行时：
   - **Codex**：一组**固定**的 hook（订阅面不随规则变化），哪条规则命中由运行时读 index 判断。装、卸、更新包通常只改 index 和静态块，不改 hook 定义，也就不用重新信任。默认装成插件 `style-runtime`。
   - **Claude Code**：订阅面**跟着规则走**（Claude 没有逐个 hook 的哈希信任，只有工作区信任【文档】），装、卸包时重写 `settings.json` 里 STYLE 的条目。
4. 无条件、正文静态且未开启正数 refresh 的 `system` 规则写在静态位置：

   | harness | `append` | `replace` |
   |---|---|---|
   | Codex | `config.toml` 的 `developer_instructions`，每个包一段，用 `<!-- style:<名字> -->` … `<!-- /style:<名字> -->` 包住。**只在用户自己没设 `developer_instructions` 时由 STYLE 管理这个键**；用户已经设了就不碰，这些规则改走动态路径（`session.start` + `reinject_on_compact`），报告里说明 | 当前 adapter 无 replace 槽位；`--allow-replace` 仅授权，不会启用替换（第 5 节阶梯第 6 条） |
   | Claude Code | 配置目录下 `rules/style-<名字>.md`，每个有效静态包一份，槽位 `claude_rules` | 当前无槽位；M2-D5 已决定不提供 output style replace，保持回退（第 5 节阶梯第 6 条） |

   静态指令仍消耗输入上下文，只有满足模型缓存门槛、前缀一致和有效期等条件时才可能命中缓存；具体情景与费用边界见 CLI 设计 §11.3。只有"几乎每个请求都要生效、而且很短"的规则才适合放这里，常驻预算（7.2）管着它。
   M2-D4/D5 的 [迁移与回退契约](../review/M2-D4-D5-DESIGN-2026-10-17.md) 已于 2026-10-17 获用户批准；T40 已实现 `claude_rules`。旧产物 `claude_md` 继续按旧布局解释，首次成功重建路线时迁移，单纯搬迁不需额外 trust。锁文件的可选 `static_layout` / `static_files` 保存冷路径布局和生成文件摘要；批准快照仍保留源字节和静态编译表示。没有安装归属的同名文件不接管；缺失、摘要/格式漂移、symlink 或损坏旧标记阻止事务。C1/C3 真实交互已确认生产规则加载一次及卸载后的新会话还原，见 [C3 证据](../review/M2-C3-CLAUDE-2026-10-17.json)；记录顺序不构成更高优先级保证。
5. 改过的 md 不会自动生效：要 `style sync` 重新编译，并经 `style trust <名字>` 确认。这补上了 Codex 信任哈希不覆盖 md 内容的缺口。`--link` 开发包第一次 `trust` 之后，后续改动只在 `sync` 时显示差异摘要，不阻塞。

### 10.1 M3-D2 锁定重建评审稿

本节为已获用户确认的 [D2 契约](../review/M3-D2-SOURCES-2026-10-17.md)；Git 安装已由 T20/A3 完成，update 已由 T21/A4 完成，frozen 已由 T22/A5 完成。D1 的 index/state/explain format=1 继续有效；首次成功 Git 安装引入 lock format=2，新版读取 1/2，纯本地旧锁仍写 1，非空 format=2 不自动降级。Git URL/ref/object_format/commit/tree 或 blob 及内容摘要属于冷路径锁和缓存，不加入 runtime index，不使 hook 启动 Git。format=2 严格校验来源和字段；format=1 出现 git 字段（含 null）拒绝。approved 的现有 format=1 增加可选 Git 身份绑定，旧本地批准 JSON 不变；后续管理写入核对该绑定，不能仅改锁里的 commit/object 再批准。

index.packages 的 lock 仍按排序包路径、NUL、逐文件 SHA256 和换行聚合；相同字节不因 ref/commit 变化产生新内容身份，来源变化仍在管理锁与预览中披露。Git 单文件与目录来源通过 core 的既有不可变快照校验，与本地包共用 requires/settings/native/预算/跨包去重与解释说明生成。

update 将明确选择的候选和未选批准基线联合编译；未选包的批准渲染/native 效果变化先阻止，--all 才覆盖联合批准。包目录、approved/settings、index/explain、lock 和路线产物由 app 协调同一管理事务，复用 agenthooks 渲染/owner/Before 保护；失败逆序回滚 bytes/mode/mtime；包交换及 ApplyFiles 以类型化错误报告恢复失败，保留失败备份路径，不承诺多文件断电原子。

frozen 在当前整锁与绑定下离线重建，逐文件、Git 内容身份、名称/版本、批准表示、surface/rendered/static 布局均须相符；锁精确字节不变，不联网或以新 ref/default 重算旧批准。管理/批准字节和完整 Git 缓存可提供锁定内容，link 必须 live 精确匹配且已授权。settings 值不进入锁，现有值必须能重现批准效果；缺基线、要求迁移/重定位或输出差异给明确失败。

缺失可推导产物只在原 owner 计划允许时恢复，不接管外来同名文件或改过的托管块；现有 native/replace 确认与状态隔离继续有效。A3 的 12 组断言、旧锁与三路线回归已通过 [本轮验收](../review/M3-A3-2026-10-17.json)；A4 的 12 组已通过 [更新验收](../review/M3-A4-2026-10-17.json)，A5 的 12 组已通过 [离线恢复验收](../review/M3-A5-2026-10-17.json)。真实投递、Windows 和跨平台 CI 保持待验。

T22/A5 的恢复实现与 [使用说明](../STYLE-SOURCES.md#整锁离线恢复) 一致：显式非空锁、完整批准/精确 settings 键和逐包完整内容；共享管理事务保全锁 bytes/mode/mtime/身份。现有 rendered 为变更集摘要，无法重现的部分配置修复与缺失 Claude 已登记 rules 保守拒绝；程序路径变更须先由普通管理更新绑定，不扩大 owner 权限。项目/跨根仍由 D3 定义，开始前按用户要求通知。

### 10.2 M3-D3 范围与交付产物评审稿

本节项目部分已获用户确认并由 T30 实现，通过 [A6](../review/M3-A6-2026-10-17.json)；云端/插件已于 2026-10-17 获用户批准、待实现，完整定义见 [D3 §3–7](../review/M3-D3-ROUTES-2026-10-17.md#3-存储根身份与兼容)。用户布局继续有效；项目的 lock/packages/approved/settings/build/state/cache/probes 分别位于用户基根 projects/规范路径摘要，项目声明和 hook 文件不包含机器绑定参数或私有 settings。两个 clone 的安装/信任独立，新 clone 只有重现候选并重新确认才能采用既有受管产物。项目声明 format 1 包含 effects_sha256、package_order 和 artifacts/static 摘要；lock 3 的 project 字段保存规范 root/base_home/home、entry_version、runner、声明摘要与各 harness config_dir/version/route，私有 binding 核对同一身份。多包首次复制安装可显式提供全部来源和各包 settings，采用的 STYLE 效果在卸载时移除，见 [项目说明](../STYLE-PROJECTS.md)。

新路线使用 lock format=3 和显式 scope/binding，旧三路线保留 1/2，index format=1 不因项目增加字段；运行时不读取原锁/包。Codex 项目 append 静态槽位不可用，沿既有动态降级；Claude 项目写项目 rules。用户/项目同一 harness 同名包拒绝，不进行跨根编译去重或合并 state。

cloud manifest format=1 与管理锁分开：相对路径/生成器与 runner 兼容版本/逐文件与 native 摘要、逻辑来源名，导出已批准 index/provenance，不包含私有绑定；sensitive-reserved 继续拒绝，普通字符串不自动识别为 secret，须完整披露。云端 state/stats/cache/probe 放私有 HOME。完整性不等于与当前源码同代，旧 bundle 要显式重新 export。Claude plugin 仅为 hook 载体，静态规则仍在用户 rules；禁用与完整移除分别验收。A6 已通过离线验收；A7/A8 尚未执行，真实 B2/C2–C4 保持待验。

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
- **M3 再定**：`provider` 的数据来源；事件内容带围栏进模板；"取不到"能否在列表里写 `unknown` 匹配；用户自定义 effort 映射；规则级 `fail: closed`。

## 附录 A：推迟的 harness 备忘

当前不实现，留给以后写适配器时参考。历史设计材料 `style-gen/cross-harness.md` 和 `style-gen/thinking-effort.md` 未随本仓库分发；其中"六档"、`on_unknown` 等旧口径已被本文取代。这里的候选资料不作为已接入 adapter 的能力保证。

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

**能力提醒**：Cursor 的 `additional_context` 能否送到模型各方说法矛盾；Gemini 会转义 `<>`；Kimi 只有提示词事件的纯文本能进上下文；Windsurf 完全不能注入；OpenCode 是进程内函数，每次请求重建系统提示词，不需要压缩处理。加这些 harness 时，压缩信号可能需要"信号和动作分开、延后补发"的做法（历史材料 `style-review/solution-2026-10-12.md` A2 未随本仓库分发），适配器接口里的 `CompactSignal()` 就是为它留的。
