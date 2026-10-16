# STYLE CLI 设计 v0.3（实现基线：通用核心 + 薄适配器）

对应规范：[STYLE-spec.md](STYLE-spec.md)（v0.3）。规范定义包格式、运行时语义、token 预算、安全约束和 `index.json`；本文定义命令、本地布局、编译和运行时流程、Claude Code 与 Codex 两个适配器、token 估算器、fork 改动、里程碑、验收和未决项。冲突时，格式和语义以规范为准，实现细节以本文为准。

状态（2026-10-16）：M1a 离线验收已完成，M1b B/C 历史条目按各自记录范围通过，全部 M1 补验收仍进行中。FINAL-01–05、10 已闭合，Claude 界面/上下文/原生超限、官方计数及最终回归尚未全部闭合；逐项证据以 [任务清单](../docs/STYLE-M1-TASKS.md) 和 [测试矩阵](../docs/STYLE-M1-TESTS.md) 为准。M1 只做 **Claude Code** 和 **Codex**。本文接口代码为设计示意；当前接口与职责见 [adapter 总览](../docs/STYLE-ADAPTERS-CURRENT.md)。

**依据**：
- agenthooks `3f734c7` 源码，历史整理资料为 `agenthooks-codex.md`（外部资料，本仓库未收录）；
- Codex 源码：`a9b0815`（外部资料 `codex-hook-install-routes.md`、`agenthooks-codex.md`、`codex-injection-points.md`）和 2026-10-12 的 `main`（外部资料 `solution-claude-codex-2026-10-12.md`）；这些资料未收录于本仓库，当前路线和信任行为以第 4 节的实现链接及验收记录复核；
- Claude Code 官方 hooks 参考（`code.claude.com/docs/en/hooks.md`，2026-10-12 读，本文涉及的字段已逐一核对）；
- token 成本分析：`STYLE-token-design.md`（外部资料，本仓库未收录）；本仓库的 token 约束见 [规范第 7 节](STYLE-spec-runtime.md#7-token-预算)。

出处标记和规范相同：【文档】【源码】【推断】【待测 Uxx】。

**给实现者的阅读顺序**：
1. 规范第 0–4 节（决定、包格式、规则、条件）；
2. 本文第 1–8 节（原则、分层、接口、命令、布局、安装、编译、运行时）；
3. 规范第 7–9 节（token 预算、安全、`index.json`）；
4. 本文第 9–10 节（两个适配器）、第 11 节（token 估算）、第 12–15 节（测试、fork、里程碑、未决项）。

## 1. 原则

- **一套通用核心，每个 harness 一个薄适配器。** 核心（规则、条件、模板、优先级、去重、降级阶梯、token 估算、报告）里不出现任何 harness 的名字。用依赖检查强制：`core/...` 不允许 import `adapter/codex`、`adapter/claude`，CI 里跑一个测试。harness 名的合法取值（`native` 的键、`when.harness.name`）由适配器注册表提供，核心不硬编码字符串。
- **适配器只回答事实**：支持哪些能力、数据从哪来、装到哪、怎么信任、静态文字写到哪、单次注入上限多少。"做不到怎么办"由核心按规范第 5 节的阶梯决定。
- **适配器分两层**：协议层（事件名、stdin/stdout、工具名）用 agenthooks（fork）的 codec；STYLE 适配器只写协议层以外的部分：安装路线、信任、静态位置、数据来源、skill 识别补充、上限。
- **热路径只读 `index.json`**，不读 md、不算哈希；没命中时输出为空。
- **出错放行**（规范 8.8）。
- **省 token 是一等需求**：`validate` 出 lint，`plan` 出 token 报告，运行时记计数。

## 2. 分层

```
┌─────────────────────────────── style（一个 Go 二进制）───────────────────────────────┐
│ cmd/style     init validate plan install uninstall list sync trust doctor adapters   │
│               （M2 stats；M3 explain probe update）；隐藏的 agenthooks run           │
├────────────────────────────────────────────────────────────────────────────────┤
│ core/         与 harness 无关                                                   │
│   spec     解析 STYLE.yaml、from: 引入的规则文件；schema；validate 和错误码            │
│   source   本地目录 / 单文件 / --link（git 来源在 M3）                               │
│   lock     style.lock.yaml                                                       │
│   plan     规则 → 需求 → 问适配器 → 降级阶梯 → 报告                                   │
│   compile  生成 build/index.json（规范第 9 节）                                      │
│   tokens   估算器、预算、lint T001–T007、plan 的 token 报告                           │
│   engine   运行时：评估点、when、模板、repeat、合并、上限保护、输出白名单                  │
│   state    按会话的状态：session.json、scope.json、去重占位、计数                      │
├────────────────────────────────────────────────────────────────────────────────┤
│ adapter/      接口、注册表、effortmap、一致性测试、共用工具（JSON/TOML/MD 托管区合并）     │
│   codex/   claude/                    （以后的 harness 各加一个目录）                  │
├────────────────────────────────────────────────────────────────────────────────┤
│ agenthooks（fork）  codec、事件归一、工具分类、MCP 名、能力矩阵、quirk、install.Render     │
└────────────────────────────────────────────────────────────────────────────────┘
```

## 3. 适配器接口

```go
package adapter

// Adapter 描述一个 harness。除了 Enrich，其他方法只在 install、plan、sync、doctor 时调用。
type Adapter interface {
    Provider() agenthooks.Provider                    // "codex"、"claude-code"
    Detect(ctx context.Context) (Installation, bool)  // 装了没有、版本、配置目录

    // 事实，供 core/plan 和 core/tokens 使用
    Supports(req Requirement, v Version) Support      // 原生 / 换个事件可行 / 不支持，附原因
    DataSource(field string, point string) DataAvail  // model、effort 等在这个评估点上拿不拿得到、从哪来
    EffortMap() effortmap.Table                       // 嵌入的 effort.yaml
    BlindSpots() []BlindSpot                          // 例如 Codex 的 file.read 永不触发
    StaticSlots() []Slot                              // append、replace 能写到哪里
    Limits() Limits                                   // 单次注入上限：值、单位（chars|tokens）、超限行为
    CompactSignal() CompactSignal                     // M1 两家都返回 {SessionStartSource: "compact"}；给以后的 harness 留的口

    // 安装
    Surface(idx *compile.Index, inst Installation) []HookSpec // 订阅面；Codex 返回固定集合
    Routes() []Route                                          // 按优先顺序排列
    Native(rule spec.NativeRule) (HookSpec, error)            // M2

    // 运行时（热路径）
    Enrich(ev *agenthooks.Event, st *state.Session) []Point
}

// HookSpec 是一个原生 hook 条目，同时说明它服务哪些评估点（token 估算按原生事件分组要用）。
type HookSpec struct {
    NativeEvent string   // "PreToolUse"
    Matcher     string   // 没有就空
    Points      []string // 例如 Codex UserPromptSubmit → ["prompt.submitted", "skill.explicit"]
    Extra       map[string]any // 例如 Claude 的 onFailure
}

// Point 是一次 hook 调用里的一个评估点。
type Point struct {
    Name  string // session.start、prompt.submitted、skill.explicit、tool.pre、tool.post、tool.error、subagent.start
    Skill string // skill.explicit 和隐式 skill 时是 skill 名
}

// Route 是一种安装位置，例如 Codex 的插件或用户级 hooks.json。
type Route interface {
    Name() string                                   // "codex-plugin"、"codex-user"、"claude-user"
    Available(inst Installation) (ok bool, why string)
    Trust() TrustInfo                               // 不需要 / 用户确认 / 可预置 / 工作区信任；键稳不稳定
    Plan(specs []HookSpec, static []StaticBlock) (Changeset, error)
    Apply(cs Changeset) error
    Remove(owner Owner) (Changeset, error)          // 只删带 STYLE 归属标记的内容
    Status(inst Installation) RouteStatus           // 给 doctor：装了吗、信任了吗、最近一次收到事件的时间
}
```

v0.2 的 `Wrap` 去掉了：规范 3.8 规定不加外壳。

**`effort.yaml` 的格式**（每个适配器一个，必须和规范 4.2 的表一致）：

```yaml
# adapter/codex/effort.yaml
source: config.model_reasoning_effort   # 说明用：请求值
map: { none: none, minimal: minimal, low: low, medium: medium, high: high, xhigh: xhigh, max: max, ultra: max }
unknown: [persistent]                   # 其余没列出的值也是取不到
```

```yaml
# adapter/claude/effort.yaml
source: effort.level | $CLAUDE_EFFORT   # 说明用：实际生效值
map: { low: low, medium: medium, high: high, xhigh: xhigh, max: max }
```

以后的预算型（`kind: budget`，带阈值表和 `approx: true`）和开关型（`kind: switch`，`off: none`、`on: open`）格式见 v0.2 CLI 设计第 3 节，M1 不用。`effortmap` 的比较展开不能包含 `open`。

**plan 怎么得出报告**：

1. 核心把每条规则换算成 `Requirement{能力, 评估点, 需要的数据}`，例如"`inject` @ `tool.post`，需要 `model`"。
2. 对每个目标适配器：用 `Supports` 和 `DataSource` 查能不能做；按规范第 5 节的阶梯换事件或改成静态 system 再查；用 `BlindSpots` 给出"永不触发"。
3. 每条规则一行："原生 / 降级到 X / 丢弃 / 永不触发 / 跳过（M2）"，附原因；再加 token 一节（第 11 节）。

阶梯只在核心里有一份，所以两家的报告格式完全一样。编译用的是同一个 plan 结果：报告说降级到 X，运行时就真的走 X。

**以后加一个 harness**：agenthooks 已支持的，写一个适配器目录（路线、静态位置、数据来源、`effort.yaml`、上限），跑一致性测试（第 12 节）；agenthooks 不支持的，先在 fork 里加 codec 和 install 渲染。压缩信号和这两家不一样时，在 `CompactSignal` 上扩展（规范附录 A）。

## 4. 命令

涉及具体 harness 的选项值一律写成 `harness=值` 或 harness 名（例如 `--route codex=codex-user`），核心不认识具体 harness。路线 ID 使用适配器返回的完整名称：Codex 为 `codex-plugin`、`codex-user`，Claude Code 为 `claude-user`；CLI、报告与锁文件使用同一组名称，不接受旧示意名 `plugin`、`user`、`user-settings`。可用路线用 `style adapters --json` 核对。

| 命令 | 作用 | 里程碑 |
|---|---|---|
| `style init [name] [--single]` | 生成包骨架：`STYLE.yaml` + `prompts/` + `rules/`；`--single` 生成 `<name>.style.md` | M1 |
| `style validate [path]` | schema、`from:` 展开和互斥、id 重复、未声明的规则文件、路径和名字限制、模板变量和 `reads`、正则能否编译、合法组合（规范 3.4），以及 lint `T001`、`T002`、`T004`–`T007` | M1 |
| `style plan <源> [--harness auto\|a,b] [--assume-requests 60] [--assume-triggers 3]` | 只出报告（每条规则的落点和状态 + token 一节），不改任何文件 | M1 |
| `style install <源> [--harness …] [--route h=r] [--set k=v] [--link] [--allow-replace] [--fail-closed h] [--yes] [--json]` | 安装（第 6 节）。`<源>` 是目录或 `*.style.md` | M1 |
| `style uninstall <name>` / `--all` | 卸载（第 6 节） | M1 |
| `style list` | 已装包：版本、来源、安装序号、是否 link、哈希是否和锁文件一致 | M1 |
| `style sync` | 从 `packages/`（或 link 的源目录）重新编译 index、重写静态块和 Claude 的订阅面；有文件的 sha256 和锁文件不同就列出改动并停下，等 `trust`（link 包除外，见第 6 节） | M1 |
| `style trust <name>` | 显示该包改动的文件和差异（含渲染后的差异），确认后写新哈希并完成编译 | M1 |
| `style doctor [--reset-state]` | 逐个适配器调 `Detect` 和每条路线的 `Status`；检查 index 和锁文件是否一致、Codex 登录 shell 是否有输出、effort 来源、花得最多的 3 条规则；`--reset-state` 清空所有会话状态 | M1 |
| `style adapters` | 适配器、检测结果、可用路线、信任方式、能力摘要、上限 | M1 |
| `style agenthooks run --provider=<h> [--owner=style] [--fail-closed]` | 隐藏，hook 入口（第 8 节） | M1 |
| `style stats` | 按规则和会话汇总注入次数和估算 token | M2 |
| `style explain --harness h --event e --payload f.json` | 离线模拟一次 hook 调用，打印命中的规则、去重判断和最终输出 | M3 |
| `style probe [--harness h]` | 自检：确认 harness 真的在调用 hook | M3 |
| `style update`、git 来源、`install --frozen`、`install --project`、`--count=exact` | | M3 |

**选项说明**：
- `--route h=r`：指定路线，例如 `codex=codex-user`、`codex=codex-plugin`、`claude-code=claude-user`。
- 当前 CLI **没有 `--preseed-trust` 选项**，传入会报 unknown option。Codex 两条路线均由现有 agenthooks 渲染器生成 `hooks.state` 信任摘要，随正常安装确认（或 `--yes`）一起写入；没有额外的预置确认开关。生成摘要不代表插件已注册或真实 harness 已接受信任，详见第 9 节。
- `--allow-replace`：允许 `mode: replace`（规范 8.5）。
- `--fail-closed h`：只有声明支持的适配器接受（M1 只有 `claude-code`），其他报错。
- `--yes`：跳过安装前的能力披露确认（规范 8.6）。非交互环境没有 `--yes` 就退出码 1。
- `--set k=v`：给 settings 赋值；非交互环境缺必填且没有 `default` 时退出码 1。

实现对照：[CLI 选项解析](../internal/app/lifecycle_command.go)、[Codex 路线 ID](../internal/adapter/codex/adapter.go)、[Claude 路线 ID](../internal/adapter/claude/adapter.go)、[Codex 信任摘要渲染](../agenthooks/install/render_codex.go)。路线 ID 是安装参数；`adapters` 的信任种类与锁文件持久值是不同表示，映射见 [trustState](../internal/app/install.go)：`user-confirmation` → `user`，`preseed` → `preseeded`，`workspace` → `workspace`，其余 → `managed`。这些值描述路线策略，不能当作真实 UI 信任确认记录。

**退出码**：`0` 成功；`1` 校验失败或用法错误；`3` `sync` 发现改动，需要 `trust`；`4` 安装被中止（`fallback: error` 命中，或 `T003`）。`agenthooks run` 永远退出 0，只有 `--fail-closed` 模式下内部出错时退出 1。

## 5. 本地布局、锁文件和状态

### 5.1 目录

根目录是 `~/.style/`，可以用环境变量 `STYLE_HOME` 改。

```
~/.style/
├── style.lock.yaml                  # 锁文件（5.2）
├── packages/<name>/                 # 安装时复制的包；--link 的包只记录源路径
├── settings/<name>.yaml             # settings 的值，权限 0600
├── build/index.json                 # 编译产物，权限 0600（规范第 9 节）
├── build/codex-instructions.md      # 只在 Codex 用 replace 时生成
├── state/<sid>/                     # 按会话的状态（5.3）
├── log/hook.log                     # 运行时日志，滚动 1 MiB × 3，不记正文
└── adapters/codex/plugin/           # Codex 插件路线生成的插件（9.2）
```

### 5.2 锁文件

```yaml
format: 1
packages:
  - name: terse-reviewer
    version: 0.3.0
    seq: 1                       # 安装序号，参与排序（规范 1.4）
    source: { kind: dir, path: /home/u/styles/terse-reviewer, link: false }
    files:                       # 包内每个文件的 sha256
      STYLE.yaml: sha256:...
      prompts/style.md: sha256:...
      rules/no-todo-comments.md: sha256:...
    settings: [max_findings]     # 只记名字，值在 settings/<name>.yaml
    replace: false
harnesses:
  codex:
    route: codex-plugin          # codex-plugin | codex-user
    version: 0.159.3
    config_dir: /home/u/.codex
    surface: sha256:...          # 订阅面指纹；变了就要重新信任
    rendered: sha256:...         # 生成的 hook 配置和静态块的指纹
    trust: user                  # 对应 user-confirmation；codex-user 为 preseeded，对应 preseed
  claude-code:
    route: claude-user
    version: 2.1.300
    config_dir: /home/u/.claude
    surface: sha256:...
    rendered: sha256:...
    trust: workspace             # Claude 只有工作区信任
    fail_closed: false
```

### 5.3 状态目录

```
state/<sid>/
├── session.json                 # model、effort、effort_raw、harness_version、updated_at
└── <agent>/                     # main，或子 agent 的 agent_id
    ├── scope.json               # gen、epoch、prompt_count
    ├── seen/                    # 去重占位，每条记录一个空文件
    │   ├── s.<gen>.<E>.<rule16>.<bh16>       # repeat: session；E 见规范 3.5
    │   └── t.<gen>.<turn16>.<rule16>.<bh16>  # repeat: turn
    └── stats.jsonl              # 运行时计数（11.5）
```

- `<sid>` 和 `<agent>` 进路径前过白名单 `[A-Za-z0-9_-]{1,64}`，不合法就用 sha256 前 16 位；`<rule16>`、`<turn16>` 是对应字符串的 sha256 前 16 位。
- **去重占位**用 `open(O_CREAT|O_EXCL)`：创建成功 = 本进程赢，注入；`EEXIST` = 已经注入过，跳过。并行的工具调用（两家都会并行跑 hook【文档】）不会丢更新，也不需要锁。
- `session.json`、`scope.json` 用"写临时文件再改名"。`scope.json` 的 gen、epoch 只在 SessionStart 里改（不会并行），`prompt_count` 只在 `prompt.submitted` 里改。`session.json` 可能被并行的工具事件同时写，后写的覆盖先写的；写入的都是同一时刻的 model / effort，丢一次无害。
- `stats.jsonl` 用 `O_APPEND` 单次写一行（每行小于 512 字节，追加是原子的）。
- **清理**：`sync` 和 `doctor` 删除 7 天没更新的 `state/<sid>/`。**热路径上不清理**。

## 6. 安装和卸载流程（与 harness 无关）

1. 解析来源 → validate（规范 1.1–1.7、3.4）→ 收集 settings，写进 `settings/<name>.yaml`。
2. 检查 `requires.runtime`；按 `requires.harness` 过滤目标 harness，版本不满足的跳过并告警。
3. **plan**：对每个目标适配器出报告；`fallback: error` 命中就中止（退出码 4）。
4. 冲突检查：`replace` 是否唯一、有没有 `--allow-replace`；`recommends` 是否和当前设置冲突（只提示）。
5. **能力披露**（规范 8.6），等用户确认或 `--yes`。
6. 写包、写锁文件，编译 index（第 7 节），其中 `T003` 按全部已装包检查，超了就中止（退出码 4），什么都不改。
7. 对每个适配器：
   - 用 `Surface(index)` 算订阅面；
   - 选路线：`--route` 指定的；没指定就取锁文件里已有的；都没有就取 `Routes()` 里第一个 `Available` 的，报告里写明选了哪条、为什么跳过前面的；
   - **换路线**（和锁文件里记的不同）时，先对旧路线 `Remove`，再对新路线 `Apply`，在同一个事务里完成，失败就回滚，避免两处都触发造成重复注入；
   - 依次 `Plan` → `Apply`，同时写静态块。
8. 打印报告，路线 ID 与锁文件一致。Codex 用户路线报告 preseed，插件路线报告 user-confirmation；固定订阅面避免包变动改写 hook 定义，但实际信任提示及插件注册仍需真实环境确认。Claude 需要工作区信任；新增订阅按实际 UI 重新加载或新开会话。
9. **卸载**：从锁文件删包，重新执行第 6、7 步。一个包都不剩时，对每条路线执行 `Remove`，并删掉所有静态块。卸载后用户的配置文件要和安装前逐字节相同（除了用户自己期间做的改动）。

**检测不到任何 harness**：仍然编译 index，报告"未安装到任何 harness"，退出码 0。

**`--link` 开发包**：第一次 `trust` 之后，后续改动只在 `sync` 时打印差异摘要，不阻塞；`style list` 标 `link=dev`，`doctor` 提示。非 link 包改动一律阻塞等 `trust`。

**hook 配置是"全部已装包"的函数**：Codex 的订阅面固定，装卸包通常只改 index 和静态块；Claude 的订阅面跟着规则走，装卸包会重写 `settings.json` 里 STYLE 的条目。

## 7. 编译

`core/spec` + `core/compile` + `core/tokens`，产物结构见规范第 9 节：

1. **解析**：读 `STYLE.yaml`；展开 `rules[]` 里的 `from:`（含 glob），读规则文件，拆出 frontmatter 和正文；读 `file:` 引用的正文文件。`rules/` 下没被引入的 md 只给警告，不读。
2. **合并**：按规范 1.3 的优先级逐层合并，补默认 `id`，去掉 `enabled: false` 的规则；重复 `id` 报错。
3. **校验**：schema、`reads` 并集、模板变量（规范 1.5）、正则能否编译（RE2）、路径和名字（规范 1.7）、合法组合（规范 3.4）。
4. **渲染**：静态正文直接渲染成文本；动态正文解析成"文本片段 + 变量"列表。`warn` 的 `model_text`：写了就照样处理；没写时，静态正文在编译时取第一句，动态正文在运行时渲染后再取。
5. **规划**：对每个目标 harness 跑能力查询和降级阶梯（规范第 5 节），决定每条规则落在静态位置、哪个评估点，还是丢弃。静态 `system` 交给适配器的 `StaticSlots` 检查（例如 Codex 上用户自己设了 `developer_instructions` 时改走动态路径，9.5）。
6. **展开挂载点并补默认值**：
   - 动态 system → `session.start` 条目 + `prompt.submitted` 条目（`reeval: true`）；
   - skill → 隐式条目（按 `at`，`tool.pre` 或 `tool.post`）+ `skill.explicit` 条目；
   - `repeat` 按动作补默认值；`session.start` 条目补 `sources`，`reinject_on_compact` 为 true 时加上 `compact`；`when.agent` 补默认值（`subagent.start` 上是 `*`，其他是 `main`）；
   - 算 `needs` = 正文变量 ∪ `when` 用到的运行时数据。
7. **估算和 lint**：每个条目算 `est`（第 11 节）；本包的 `T001`、`T002`、`T004`–`T007`；全部已装包的 `T003`。
8. **排序和输出**：按 harness → 评估点分组，组内按 `order` 排好；写 `static` 列表；"写临时文件再改名"。

## 8. 运行时（hook 热路径）

`style agenthooks run --provider=<h>` 每次 hook 调用起一个进程：

1. 读 stdin。
2. 读 `index.json`；文件缺失、损坏或 `format` 不对 → 输出"没意见"（`--fail-closed` 模式下退出码 1），stderr 提示 `style sync`，写日志。
3. agenthooks codec 解码成统一事件。
4. 适配器 `Enrich`：补 effort、从状态补 model、算回合号和 agent、算 `tool.added`、识别 skill，返回这次调用的**评估点**列表。
5. 更新状态：事件里带了 model / effort 就写 `session.json`（不用空值覆盖）；SessionStart 按 `source` 改 gen / epoch（规范 3.6）；`prompt.submitted` 上 `prompt_count` +1。
6. 对每个评估点，从 `index[h][评估点]` 取候选条目，按 `match` 过滤（工具分类、原生名、MCP、skill 名），再判断 `when`（agent、model、effort、harness、conditions）。`session.start` 条目还要过 `sources`。
7. 渲染正文（`warn` 另渲染 `model_text`），算 bh。
8. 所有评估点的命中条目合在一起按 `order` 排序：
   - 有 `block` → 只输出拒绝，理由按序拼接，不占任何去重记录；
   - 否则对 `repeat` 不是 `every` 的条目，按规范 3.5 的键抢占 `O_EXCL` 占位，抢到的才保留；`subagent.start` 条目不抢占。
9. 拼接（规范 3.8）；超过适配器上限时从 `order` 大的一端丢弃，释放被丢条目的占位，日志记 `cap-drop`。
10. 编码；**输出白名单断言**（规范 8.1），断言失败就改成输出"没意见"并记日志。写 stdout，退出码 0。
11. 有输出的条目、被去重挡掉的条目、`cap-drop` 各追加一行到 `stats.jsonl`。

**时间预算**：exec 形式（Claude）整个进程 p95 < 50 ms；Codex 加上登录 shell 启动，p95 < 150 ms（验收 a20、b11、c9）。SessionStart 要尤其快：Claude 的第一次回复要等 SessionStart hook 跑完【文档】。热路径不做网络请求、不扫描目录、不清理状态。

**日志**：`~/.style/log/hook.log`，每行：时间、provider、原生事件、评估点、条目 `entry`、动作或错误码。**不记正文、不记提示词和工具参数**。

## 9. Codex 适配器

### 9.1 路线

| 顺序 | 路线 | 什么时候可用 | 信任 |
|---|---|---|---|
| 1（默认） | **`codex-plugin`**，插件 `style-runtime` | 当前 Available 检查配置根和路线目录；不验证 Codex 已注册/启用插件，注册由用户执行 | 信任种类为 `user-confirmation`，Status 不因生成文件存在而声称 Trusted。渲染器仍生成长格式 `<plugin_id>:hooks/hooks.json:<事件蛇形>:<组序号>:<handler 序号>` 的 trusted_hash，STYLE 的 plugin_id 为 `style-runtime@style`；真实信任是否接受由 M1b 确认 |
| 2（兜底） | **`codex-user`**，用户级 `CODEX_HOME/hooks.json` | 当前 Available 检查路线目录；真实 harness 接受配置由 M1b 验证 | 键为 `<hooks.json 规范化绝对路径>:<事件蛇形>:<组>:<handler>`，安装自动生成 trusted_hash，无独立 CLI 选项；路线信任种类为 `preseed` |
| — | 项目级 `.codex/` | M3（`--project`） | 项目要受信任，每个 clone 还要单独信任每个 hook |
| — | 系统级或托管 | 不由 STYLE 安装 | 只在文档里说明（托管来源不需要信任） |

信任状态只从用户层 `CODEX_HOME/config.toml` 和 `-c` 参数里读【源码】，仓库里的配置没法替自己声明"已信任"。

### 9.2 插件路线的做法

```
~/.style/adapters/codex/plugin/
├── .codex-plugin/plugin.json    # name: style-runtime，当前 manifest version 固定为 0.1.0
├── hooks/hooks.json
└── bin/style                    # POSIX sh 启动器
```

`bin/style`：

```sh
#!/bin/sh
exec '/abs/path/to/style' "$@"
```

`hooks/hooks.json`（`<CMD>` 是 `exec "$PLUGIN_ROOT/bin/style" agenthooks run --provider=codex --owner=style`）：

```json
{
  "hooks": {
    "SessionStart":     [{"hooks": [{"type": "command", "command": "<CMD>", "timeout": 10}]}],
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "<CMD>", "timeout": 10}]}],
    "PreToolUse":       [{"matcher": "^(Bash|apply_patch|mcp__.*)$",
                          "hooks": [{"type": "command", "command": "<CMD>", "timeout": 10}]}],
    "PostToolUse":      [{"matcher": "^(Bash|apply_patch|mcp__.*)$",
                          "hooks": [{"type": "command", "command": "<CMD>", "timeout": 10}]}],
    "SubagentStart":    [{"hooks": [{"type": "command", "command": "<CMD>", "timeout": 10}]}]
  }
}
```

- Codex 把 hook 命令交给 shell 的 `-lc` 执行，并设置 `PLUGIN_ROOT`【源码】，所以 `$PLUGIN_ROOT` 会被展开。命令字符串固定不变；`style` 二进制换位置时只重写 `bin/style`，它不在信任哈希里，不用重新信任。
- 当前 STYLE 仅生成插件目录，不自动登记 marketplace 或启用插件。手动注册、缓存副本以及切回用户路线前的取消步骤见 [M1b 手册第 6 节](../docs/STYLE-M1B-MANUAL.md#6-codex-插件路线与手动注册)；本机 Codex CLI 0.159.3 的完整手动登记、第二包变更、真实 plugin hook、取消登记及双向切换已通过（FINAL-04/05）。
- **不写 `additionalContextLimit`**：2,500 token 的默认值正好当保险，STYLE 自己的预算更严；不写也避开了"这个字段是否参与信任哈希"的问题（规范 7.1）。
- 不写 `async`（Codex 解析但不执行，quirk #10）。

### 9.3 固定订阅面

`Surface()` 不看规则，永远返回上面这组，规则在进程内过滤：

| Codex 事件 | matcher | 评估点 |
|---|---|---|
| SessionStart | 不写 | `session.start` |
| UserPromptSubmit | 不写（不支持 matcher） | `prompt.submitted`，以及提示里每个 `$名字` 一个 `skill.explicit` |
| PreToolUse | `^(Bash\|apply_patch\|mcp__.*)$` | `tool.pre` |
| PostToolUse | 同上 | `tool.post` |
| SubagentStart | 不写 | `subagent.start`（F7） |

- 每个事件只有一组、一个 handler，所以信任键里的序号永远是 `0:0`；订阅面不变，键就不变，用户基本只需要信任一次。
- **当前不按事件版本裁剪**：Surface 固定返回上述五类，未实现 events.yaml 版本表。示例通过既有 `requires.harness` 要求 Codex >=0.159.3，低/未知版本按包过滤并保持配置不变（FINAL-01）；不能把这项过滤写成所有自定义包都有自动事件兼容。
- 指纹 = 规范化后（键排序、紧凑）的 `hooks.json` 内容的 sha256。
- 这组 matcher 是适配器写死的，不经过 agenthooks 的 `CompileMatcher`，所以 F1 的 matcher bug 不影响这里，但 fork 里仍要修。
- 代价：每次 Bash、apply_patch、MCP 调用都多起一个进程（经过登录 shell），没命中时不输出、不花 token，只花时间（U5）。

### 9.4 数据和 `Enrich`

| 数据 | 做法 |
|---|---|
| model | 每个事件 stdin 的 `model` |
| effort | 只看配置请求值。实测 0.159.3：祖先进程 `-c` → 明确受信项目配置 → `-p` / `--profile` 指定的 `CODEX_HOME/<name>.config.toml` → 用户配置。缺失 profile 文件继承用户配置；legacy selector/table 与新 profile 同选时原生拒绝。旧/未知版本保留原有表式解析。生产 `runHook` 尚无可靠信任回调，明确跳过项目层，doctor 报告原因；不从模型默认值伪造 effort。结果映射并缓存到 session，一会话一次；未知 raw 不沿用旧 tier。配置及两条路线真实投递证据：M1-FINAL-02/04 |
| 回合 | `turn_id`；SessionStart 上没有，用 `prompt_count` |
| agent | `agent_id`、`agent_type` |
| `tool.added` | `apply_patch` 的输入是 `{"command": "<补丁文本>"}`【源码】：取以 `+` 开头的行，去掉 `+`，跳过 `+++` |
| MCP | agenthooks `ParseMCPName` 拆 `mcp__<server>__<tool>` |
| 隐式 skill | agenthooks `SkillActivationOf` 的 shell 分支，F8 放宽到 `cat`、`sed -n`、`head`、`tail`、`bat`、`cd <dir> && cat SKILL.md`；路径必须以 `skills/<名字>/SKILL.md` 或 `skills/.system/<名字>/SKILL.md` 结尾（`cd` 的写法要求目录以 `skills/<名字>` 结尾）。云端 skill 不在当前路线支持范围，plan 明确提示；真实原生工具名尚未采集（U6 按边界关闭） |
| 显式 skill | UserPromptSubmit 的提示文字里按规范 3.3 的正则找 `$名字`，每个不同的名字一个 `skill.explicit` 评估点 |
| harness 版本 | `Detect` 时取得，写进锁文件和 `session.json` |

### 9.5 静态位置

- **append**：`config.toml` 的 `developer_instructions`。这是根级键，必须出现在第一个表头之前，所以 STYLE 的托管区放在**文件开头**：

  ```toml
  # >>> style (managed; edit with `style`) >>>
  developer_instructions = """
  <!-- style:terse-reviewer -->
  本仓库的代码评审先给结论（能否合并），……
  <!-- /style:terse-reviewer -->"""
  # <<< style <<<
  ```

  - 值用 TOML 编码器生成（多行基本字符串，正确转义），不手拼。
  - **只在用户自己没设 `developer_instructions` 时**由 STYLE 管理这个键，通过托管标记和当前配置检测归属，锁文件不含 `owns_developer_instructions` 字段。用户已经设了，就不碰它，这些静态规则改走动态路径：`session.start` + `reinject_on_compact: true`，报告里说明。
  - 没有静态规则时删掉整个托管区。
- **replace**：`model_instructions_file = "<~/.style/build/codex-instructions.md 的绝对路径>"`，同样写在开头的托管区里。要 `--allow-replace`；用户已经设了这个键就报错。
- **信任摘要**：两条 Codex 路线均沿用 agenthooks 的 `[hooks.state."<键>"]` 托管区，随安装写入，不需要额外 CLI 选项。用户路线使用规范化 hooks.json 路径，插件路线使用 plugin_id 长键。插件 Status 仍等待用户确认，不从生成摘要推断真实信任。
- 卸载后，托管区之外的字节完全不变。

### 9.6 输出方言

- 没意见：**stdout 为空**（未知 JSON 会被 Codex 拒绝）。
- 注入：`hookSpecificOutput.additionalContext`，`hookEventName` 填原生事件名。
- 拒绝工具调用：`permissionDecision: "deny"` + **非空**理由【源码】。
- 拒绝提示词：`decision: "block"` + `reason`。
- `warn`：顶层 `systemMessage`（给用户）+ `additionalContext`（给模型）。
- 不支持、STYLE 也绝不输出：`allow`、`ask`、单独的 `updatedInput`、`continue: false`、`stopReason`、`suppressOutput`【源码】。

### 9.7 登录 shell

Codex 用 `shell -lc` 跑 hook 命令，登录 shell 会读用户的 profile：启动变慢；profile 里如果有 `echo`，输出会混进 hook 的 stdout，SessionStart 上变成垃圾上下文，其他事件上让 JSON 解析失败。对策：

- 命令尽量短，用 `exec` 直接换成 `style` 进程；
- `style doctor` 在登录 shell 里跑一次 `true`，stdout 不为空就标红，提示把输出挪到交互式 shell 的配置里；
- 性能预算把 shell 启动算进去（p95 < 150 ms）。

### 9.8 已知限制

- 压缩后的 SessionStart(`compact`) 在**下一个回合开始时**才执行【源码】，测试时要注意时机。
- 子 agent 被压缩后不会再触发 SubagentStart，子 agent 里的注入不会补发【源码】。
- 会话中途用 `/model` 改强度看不到。
- 多数用户没在配置里写 `model_reasoning_effort`，这时 effort 取不到。`doctor` 显示"effort 来源"：解析到的值、来自哪一层、原始值、映射结果。
- 盲区：`file.read`、`file.write`（已归 `file.edit`）、`search`、`fetch`、`task` 永不触发。
- `recommends.codex.personality` 和用户设置不同时只提示，不改。

## 10. Claude Code 适配器

### 10.1 路线

| 顺序 | 路线 | 说明 |
|---|---|---|
| 1（默认） | **用户级 `~/.claude/settings.json`** | 合并写入，保留别人的条目；STYLE 的条目靠 `args` 里的 `--owner=style` 识别（F3）。同一个 handler 写在多个设置文件里只跑一次【文档】 |
| — | 插件（`hooks/hooks.json`） | M3：卸载更干净，代价是要走 marketplace |
| — | 项目级 `.claude/settings.json` | M3（`--project`） |

**信任**：Claude 没有逐个 hook 的哈希信任。交互式会话里，所有设置文件里的 hook（包括用户自己的 `~/.claude/settings.json`）都要等用户接受该目录的**工作区信任**之后才运行【文档】。所以订阅面可以跟着规则走，不需要像 Codex 那样固定。

### 10.2 条目格式

用 **exec 形式**：设了 `args` 时，Claude 把 `command` 当成可执行文件直接启动，不经过 shell【文档】，避开 shell 启动开销和 profile 污染。

```json
{
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "command": "/abs/path/to/style",
                  "args": ["agenthooks", "run", "--provider=claude-code", "--owner=style"],
                  "timeout": 10}]}
    ],
    "PreToolUse": [
      {"matcher": "^(Bash|Edit|Write|mcp__.*)$",
       "hooks": [{"type": "command", "command": "/abs/path/to/style",
                  "args": ["agenthooks", "run", "--provider=claude-code", "--owner=style"],
                  "timeout": 10}]}
    ]
  }
}
```

`style` 二进制换位置时，`sync` 重写这些条目即可（没有哈希信任）。

### 10.3 订阅面（跟着规则走）

`Surface(index)` 按 index 里实际有的条目生成：

| Claude 事件 | 什么时候订阅 | matcher | 评估点 |
|---|---|---|---|
| SessionStart | 只要装了任何规则（要记模型、处理 clear / compact） | 不写 | `session.start` |
| UserPromptSubmit | 有 `prompt.submitted` 条目（含动态 system 的重新评估条目） | 不支持 matcher | `prompt.submitted` |
| UserPromptExpansion | 有 `skill.explicit` 条目 | `^(skill名1\|skill名2)$`（按 `command_name` 匹配【文档】） | `skill.explicit` |
| PreToolUse | 有 `tool.pre` 条目 | 条目 `match` 的原生名并集（分类用 agenthooks `CompileMatcher` 转成 Claude 原生名）；有隐式 skill 加 `Skill`；有 MCP 条目加 `mcp__.*`（服务名在进程内按 `mcp_server.name` 过滤，插件自带的 MCP 工具名不能按名字拆） | `tool.pre` |
| PostToolUse | 有 `tool.post` 条目 | 同上 | `tool.post` |
| PostToolUseFailure | 有 `tool.error` 条目 | 同上 | `tool.error` |
| SubagentStart | 有 `subagent.start` 条目 | 不写 | `subagent.start` |
| PostModelSwitch | 有条目的 `needs` 含 `model`（v2.1.251 起【文档】） | 不写 | 无，只同步更新状态；等写入完成再处理下一条提示 |

订阅面变了就重写 `settings.json` 里 STYLE 的条目。FINAL-06实测在未重启当前进程时新增Read matcher，实际调用收到新USER/MODEL标记，热更新成立；此结论限于该2.1.300环境。

### 10.4 静态位置

- **append**：`~/.claude/CLAUDE.md` 末尾，每个包一段标记块：

  ```markdown
  <!-- style:terse-reviewer -->
  本仓库的代码评审先给结论（能否合并），……
  <!-- /style:terse-reviewer -->
  ```

  文件不存在就创建；卸载后如果文件是 STYLE 创建的且只剩空白，就删掉，保证和安装前一致。`CLAUDE.md` 在会话开始时加载，压缩后会重新加载【文档】，所以静态规则不需要 hook 补发。
  U-C4 证实用户级 `~/.claude/rules/*.md` 生效后，M2 改成每包一个文件 `~/.claude/rules/style-<名字>.md`。
- **replace**：M1 不支持（规范第 5 节阶梯第 6 条）。output style 能不能当替换位置，M2 评估。

### 10.5 数据和 `Enrich`

| 数据 | 做法 |
|---|---|
| model | SessionStart 的 `model`（可能没有）；`PostModelSwitch` 的 `to_model`。都写进 `session.json`，不用空值覆盖 |
| effort | 事件里的 `effort.level`（工具类事件才有）→ 环境变量 `$CLAUDE_EFFORT`（U-C1）→ `session.json` 里最近一次的值 |
| 回合 | `prompt_id`；第一条提示之前没有，用 `prompt_count` |
| agent | `agent_id`、`agent_type`；没有 `agent_id` 就是主 agent |
| `tool.added` | `Edit` 取 `new_string`，`Write` 取 `content`；`MultiEdit`、`NotebookEdit` 字段没核实，M1 取不到 |
| MCP | 事件里的 `mcp_server.name`（v2.1.274 起【文档】）；没有时退回按 `mcp__<server>__<tool>` 拆，`mcp__plugin_…` 开头的拆不了，算取不到 |
| 隐式 skill | `Skill` 工具（agenthooks `SkillActivationOf` 的 Claude 分支） |
| 显式 skill | `UserPromptExpansion`，`expansion_type` 为 `slash_command` 时 `command_name` 就是 skill 名 |
| harness 版本 | `Detect` 时取得 |

### 10.6 输出

- 没意见：退出码 0、stdout 为空。
- 注入：`hookSpecificOutput.additionalContext`（带 `hookEventName`），包成 system-reminder 放在事件发生的位置【文档】。每个字段上限 10,000 字符。
- `warn`：顶层 `systemMessage` + `additionalContext`。FINAL-06六事件的USER标记可见；clear虽然输出SessionStart标记，界面未显示，保留来源边界。
- 拒绝工具调用：`hookSpecificOutput.permissionDecision: "deny"` + `permissionDecisionReason`。
- 拒绝提示词：顶层 `decision: "block"` + `reason`，并在 `hookSpecificOutput` 里带 `suppressOriginalPrompt: true`【文档】。拒绝显式 skill：UserPromptExpansion 的 `decision: "block"` + `reason`。
- 绝不输出 `permissionDecision: "allow"`（会跳过权限提示）、`ask`、`updatedInput`、`updatedToolOutput`、`continue`、`initialUserMessage` 等（规范 8.1）。

### 10.7 `--fail-closed`

- 只给 **PreToolUse** 那一条加 `"onFailure": "block"`（需要 Claude ≥ v2.1.295【文档】，版本不够就拒绝这个选项），并在 `args` 里加 `--fail-closed`。
- 这个模式下，运行时内部出错（index 缺失或损坏等）时退出码改成 1，Claude 就会拦下这次工具调用。正常退出 0 加空输出不算失败，所以这一步是必须的。
- **默认关闭**：开了之后 index 一坏，所有工具调用都被拦。锁文件记 `fail_closed: true`，`doctor` 提示。

### 10.8 已知限制

- `UserPromptSubmit` 不只在用户打字时触发，定时任务、后台子 agent 回报、别的会话发来的消息也会触发它【方案 N4】。`prompt.submitted` 规则会命中这些；默认 `repeat: session` 让它们不会反复注入。
- Claude 的云端会话不读本机的 `~/.claude/settings.json`，用户级安装在那里不生效。项目级路线放 M3。
- 未接受工作区信任的目录里，hook 不跑。
- SessionStart 要快：第一次回复要等它跑完；`/clear` 时没跑完的结果会被丢弃【方案 N10】。

## 11. token 估算和报告

### 11.1 估算器（`core/tokens`）

```
tokens ≈ ceil( A / 4 + C + 0.5 × P )
  A = ASCII 字母和数字的个数
  C = 中日韩字符（CJK 统一表意文字、假名、谚文）的个数
  P = 其余字符（标点、符号、空白）的个数
chars = Unicode 字符（rune）个数，精确值
```

- 动态正文里的每个变量按占位长度算：一般变量 16 token / 64 字符；`harness.cwd` 32 token / 128 字符。
- 结果明确标为估算，不提供百分比误差保证；U-T3 官方计数校准尚未完成，不调整现有系数。
- `--count=exact`（调用 Anthropic 的 token 计数接口、本地 OpenAI 分词器）留到 M3。

### 11.2 按原生事件分组

最坏情况的合计按 `(harness, 原生 hook 事件, 工具)` 分组，因为一次原生 hook 调用的所有评估点会合成一个输出（规范 3.8）。适配器的 `HookSpec.Points` 说明每个原生事件对应哪些评估点。

- 工具维度：Codex 是 `Bash`、`apply_patch`、`mcp`（所有 MCP 算一组）；Claude 是条目里出现过的每个原生工具名，加 `Skill`、`mcp`。一个条目属于它的 `match` 可能匹配到的每个组。隐式 skill 条目：Codex 归 `Bash`，Claude 归 `Skill`。
- Codex 的 UserPromptSubmit = 全部 `prompt.submitted` 条目 + 全部 `skill.explicit` 条目（一条提示里可以有多个 `$名字`）。Claude 的 UserPromptExpansion 一次只有一个命令，取"同一个 skill 名的条目合计"的最大值。
- `warn` 按 `model_body` 算；有 `block` 的组取"全部 `block` 理由之和"和"全部 `inject` / `warn` 之和"中较大的那个（有 `block` 命中时只输出拒绝）。
- `T002` 按单个包算；`T003` 按全部已装包算，和适配器上限的 80% 比（Claude 用字符，Codex 用 token）。
- 常驻（`T004`）= 静态条目 + 没有 `when` 的 `session.start` 条目，按 harness 分别算，取最大值。

### 11.3 `style plan` 的报告

```
terse-reviewer 0.3.0 → codex（路线：plugin）
  规则               落点                                             状态
  no-todo-comments   tool.post [file.edit, file.write] warn           原生；file.write 已由 file.edit 覆盖
  style              静态 developer_instructions                       原生
  session-info       session.start + prompt.submitted                 原生
  low-effort-guard   session.start + prompt.submitted                 原生；⚠ effort 只来自配置的 model_reasoning_effort，没配置时永不触发
  review-skill       tool.post（Bash 读 SKILL.md）+ skill.explicit       原生
  after-git-diff     tool.post [shell]                                原生
  before-create-pr   tool.pre mcp github/create_pull_request          原生
  no-force-push      tool.pre [shell] block                           原生
  on-review-prompt   prompt.submitted                                 原生
  codex-only-notify  native                                           跳过（M2）

  Token（离线估算，校准待验；单位 token，数字为示意）
  常驻            style ≈ 45 · session-info ≈ 45（每个会话都付）     合计 ≈ 90 / always_on 600
  单条            最大 review-skill ≈ 30 / per_rule 400
  按原生事件的最坏合计（上限 2,500，报错线 2,000）
    SessionStart              ≈ 90
    UserPromptSubmit          ≈ 150   （含 skill.explicit）
    PreToolUse  Bash          ≈ 18    （block 理由）
    PreToolUse  mcp           ≈ 25
    PostToolUse Bash          ≈ 55
    PostToolUse apply_patch   ≈ 26
  会话成本模型（占位假设：60 次请求，每条触发型规则触发 3 次）
    常驻 ≈ 90 × (1.25 + 0.1 × 59) ≈ 640；触发 ≈ …；合计 ≈ … 单位
  告警
    （无）
```

- 每个目标 harness 各出一份，单位和上限不同。
- 前几块都是确定性的计算，不联网。
- **会话成本情景**：一段 N token 的文字在共 K 次请求的会话里、于第 k 次请求注入，保留公式 `N × (1.25 + 0.1 × (K − k))`（单位：基础输入 token 价）。它假设符合缓存门槛的前缀始终不变且未过期，首次写入 1.25 倍、后续每次命中 0.1 倍；对应 Claude 5 分钟缓存情景，不代表所有模型、Codex 或真实账单。Claude 1 小时写入及 OpenAI 模型的条件/倍率另见 [FINAL-10 官方资料核对](../docs/review/M1-FINAL-10-CACHE-2026-10-16.md)。常驻和静态的 k = 1；触发型规则的 T 次触发均匀分布在 `k_i = round(i × K / (T + 1))`；`repeat: session` 只算第一次，`turn` / `every` 每次都算。公式不包括基础上下文、工具与模型输出；默认 60 次请求、3 次触发及预算 400/1200/600 保留，尚无代表性工作负载支持调整。真实统计分列规则估算和会话总 usage，不能逐规则归因缓存费用。

### 11.4 lint 输出

`validate` 和 `plan` 用同一套编码（规范 7.3），每行格式 `<编码> <包>/<规则>: <说明>`，例如 `T007 terse-reviewer/after-git-diff: repeat 是 turn，一个会话里可能重复注入；没有特别理由就改成 session，或写 reason`。

### 11.5 运行时计数

- `stats.jsonl` 每行：`{"ts": …, "entry": "…", "kind": "emit|dedup|cap-drop", "est": N}`。没命中的条件不记（每次工具调用都记会太多）。
- `style doctor` 列出最近 7 天估算 token 最多的 3 条规则，并标出"注入次数多但正文哈希一直相同"的规则（说明 `repeat` 设错了）。
- `style stats`（M2）按规则和会话汇总。
- **只存估算值和计数，不存正文**，避免把规则文字和提示词长期留在磁盘上。

## 12. 一致性测试（每个适配器都要过）

放在 `adapter/conformance`，每个适配器提供 fixture 后复用：

1. **解析**：每个原生事件的样例 payload 解析成正确的统一事件和评估点，model、cwd、回合、agent、`tool.added`、skill 识别都对。
   - **effort 映射**：`effort.yaml` 每一行（以及规范 4.2 表里属于这个 harness 的每一行）都有 fixture，档位、`EffortRaw`、`EffortApprox` 和预期一致；表里没有的值得到空档位。
2. **往返**：样例规则走 engine 后编码出来的 stdout 和黄金文件一致；"没意见"的输出符合这家的方言。
3. **安装**：在临时目录里 `Plan` → `Apply` → `Status` → `Remove`：别人的条目原样保留；没有 STYLE 残留；再装一遍内容完全相同（幂等）；换路线后旧路线无残留。
4. **报告**：同一个包在这个适配器上的 plan 报告（含 token 一节）和黄金文件一致。
5. **输出白名单**：对所有黄金输出做静态检查，出现规范 8.1 禁止的字段就失败。
6. **e2e**（默认跳过，`-tags e2e` 才跑）：用真实 CLI 跑一回合，确认注入到达模型、`block` 生效。

## 13. 要在 agenthooks fork 里补的东西

编号 F1–F10 是规范和本文共用的唯一编号。

| # | 改动 | 原因 | 里程碑 |
|---|---|---|---|
| F1 | Codex 的 shell matcher 改成 `Bash`（`matcher.go:117`），加一个带 matcher 安装的 e2e 用例 | 现在生成 `local_shell\|shell`，而 Codex hook 里 shell 叫 `Bash`【源码】。固定订阅面不受影响，但要修 | M1b |
| F2 | 新增 `install.Uninstall` | 现在没有卸载 | M1a |
| F3 | 条目归属改成按 `--owner=style` 标记 | `mergeManagedJSON` 把任何含 `agenthooks` 字样的条目都当成自己的（`install.go:291`），会覆盖别的 agenthooks 程序装的条目 | M1a |
| F4 | Codex 插件形式的渲染；插件信任键（长格式）的 `DefinitionHash` 和预置 | 现在只会写 `CODEX_HOME`（`render_codex.go:27-95`） | M1b |
| F5 | `SessionInfo` 加 `Effort`、`EffortRaw`、`EffortApprox`、`HarnessVersion`；`adapter/effortmap`；Codex 的配置解析（9.4）；Claude 的 `effort.level` / `$CLAUDE_EFFORT` | 规范 4.2、第 6 节 | M1a |
| F6 | 按会话保存的状态（5.3） | 规范 3.5–3.7 | M1a |
| F7 | Codex `subagent.start` 的 add-context：新增 `SubagentStartDecision.WithContext`，改能力矩阵和编码 | 现在 `OnSubagentStart` 只返回 `error`（`agenthooks.go:187`），而 Codex 的 SubagentStart 支持 `additionalContext`【源码】 | M1b（U7） |
| F8 | Codex skill 识别放宽（`head`、`tail`、`bat`、`cd … && cat SKILL.md`、`skills.read`），以及提示里的 `$名字` → `skill.explicit` | 现在只认 `cat` 和 `sed -n`（`claude_skill.go:153-170`），不认 `$名字` | 本地解析和真实投递已完成；云端读取按 U6 不支持边界关闭 |
| F9 | Claude codec 补齐：`prompt_id` → `TurnID`；`effort.level`；`mcp_server{name, source}`；`UserPromptExpansion`；`PostModelSwitch`（`to_model`）；SessionStart 的 `fork`；编码 `suppressOriginalPrompt` | 这些都是 v0.3 要用的；agenthooks `3f734c7` 里已有哪些没核对（U-F9），先核对再补 | M1a |
| F10 | Claude 安装渲染支持 exec 形式（`command` + `args`）、`timeout`、可选 `onFailure: "block"` | agenthooks 的 `hookCommand` 生成的是一条 shell 命令字符串（`install.go:338-352`） | M1a |

F1–F4、F10 是安装和协议层的问题，最好也提给上游。F5–F9 是 STYLE 需要的扩展。

## 14. 里程碑和验收

### 14.1 M1a：核心 + 两家，离线，全自动

不需要真实的 Claude Code 或 Codex，全部在 CI 里自动判定。

**范围**：包格式（目录包、单文件包）；`init`、`validate`、`plan`、`install`、`uninstall`、`list`、`sync`、`trust`、`doctor`、`adapters`（安装目标是临时目录）；compile、engine、state、tokens；两个适配器的 codec 往返、订阅面、路线的 `Plan` / `Apply` / `Remove`（临时目录）、静态块；一致性测试 1–5；fork 的 F2、F3、F5、F6、F8（解析）、F9、F10。

**验收**（每条都是可执行的断言）：

| # | 断言 |
|---|---|
| a1 | `style validate` 对示例包退出 0；对 `testdata/invalid/<错误码>.*` 每个文件退出 1，输出含预期的错误码。至少覆盖：`dup-id`、`from-and-inline`、`inline-missing-id`、`file-and-text`、`file-into-rules`、`body-has-frontmatter`、`rule-file-pkg-field`、`empty-body`、`override-target`、`defaults-field`、`undeclared-var`、`reads-missing`、`event-var-in-template`、`sensitive-reserved`、`condition-field-wrong-event`、`bad-regex`、`bad-effort-expr`、`action-target`、`block-after`、`replace-non-system`、`bad-name`、`bad-id`、`path-escape`、`symlink`、`file-too-large`、`bad-encoding`、`marker-in-body` |
| a2 | `style plan` 对示例包的输出和 `testdata/golden/plan-codex.txt`、`plan-claude-code.txt` 逐字相同（含 token 一节） |
| a3 | 编译示例包得到的 `index.json` 和 `testdata/golden/index.json` 逐字相同，并通过 `schema/index.schema.json` |
| a4 | 对 `testdata/payloads/{codex,claude-code}/*.json` 每个 fixture，`style agenthooks run` 的 stdout 和 `*.golden` 相同（含"没意见"时为空） |
| a5 | 合并：同一次调用命中 2 条 `inject` + 1 条 `warn` → 一个 `additionalContext`（按 order、空行拼接、`warn` 只带第一句，没有外壳）和一个 `systemMessage`；再加 2 条 `block` → 只有拒绝，理由按序拼接 |
| a6 | repeat：`session` 相同输入第二次无输出；改变 `{{model}}` 后再次输出；`turn` 换回合再次输出；`block` 连续两次都拦；`warn` 每次都发 |
| a7 | 并发：20 个进程同时对同一个 `(作用域, 规则, bh)` 触发，恰好 1 个有输出 |
| a8 | 会话来源：`startup` 触发；`resume` 默认不触发；`clear` 后 gen +1 重新触发；`compact` 只触发 `reinject_on_compact` 的规则，之后 `reinject_on_compact: false` 的 `session` 规则不再出现 |
| a9 | 重新评估：动态 system 规则在模型变化后（Codex 事件里的 `model` / Claude 的 `PostModelSwitch`）的下一条 `prompt.submitted` 上补发一次；没变化就不发；`when` 后来才满足的在那时注入 |
| a10 | effort：每行映射有 fixture；未知值为空；`">=high"` 对空档位不触发；`open` 只被单值或列表匹配；Codex 0.159.3 按 `-c` > 受信任项目 > 独立 profile 文件 > 用户的顺序（旧版表式解析保留兼容），未受信任的项目层被忽略；Claude 按事件 > 环境变量 > 状态的顺序 |
| a11 | agent：不写 `agent` 的规则在子 agent 事件上不触发；`any` 触发；`subagent.start` 规则默认触发且不去重 |
| a12 | 信任：改动包内 md 后 `sync` 退出码 3 并列出差异，index 不变；`trust` 后 index 更新；`trust` 过程中源文件又被改动，结果仍基于确认时的字节；link 包首次 trust 后改动不阻塞 |
| a13 | 安装隔离：对含他人条目的 Codex `hooks.json` / `config.toml`、Claude `settings.json` / `CLAUDE.md`，`install` → `uninstall` 后逐字节还原；重复 `install` 幂等；换路线后无残留 |
| a14 | 静态位置：Codex 托管区在第一个表头之前；用户自己设了 `developer_instructions` 时改走动态路径并在报告里说明；Claude `CLAUDE.md` 标记块正确增删 |
| a15 | 输出白名单：所有黄金输出不含禁止字段；Claude 的提示词拦截带 `suppressOriginalPrompt: true` |
| a16 | 依赖检查：`core/...` 不 import `adapter/codex`、`adapter/claude` |
| a17 | 损坏的 `index.json`：stdout 为空、退出码 0、日志有记录、`doctor` 标红；`--fail-closed` 模式下 PreToolUse 退出码 1 |
| a18 | token：估算器对固定字符串的结果和黄金数字一致；`T001`–`T007` 各有一个 testdata 触发；两个包合计超过 80% 时第二个包的安装以退出码 4 中止，且什么都没改 |
| a19 | 运行时上限保护：动态变量把合计推过上限时，从 order 大的一端丢弃，占位被释放，日志有 `cap-drop` |
| a20 | 性能：只读 index 的热路径，进程 p95 < 50 ms（基准测试，不含 shell 启动） |
| a21 | `warn` 第一句：中文、英文、没有句末标点、多行正文、首行为空等情况的提取结果和预期一致 |
| a22 | 能力披露：非交互环境没有 `--yes` 时安装退出 1；`replace` 没有 `--allow-replace` 时被拒绝 |

### 14.2 M1b：真实环境

**范围**：两家的真实安装路线、Codex 固定订阅面、静态块、e2e、第 15 节的实测项；fork 的 F1、F4、F7、F8（实测）。记录实测的 Codex 和 Claude Code 版本号，`requires.harness` 的示例值改成它们。

**Codex**（前置：`config.toml` 里设 `model_reasoning_effort = "low"`）：

| # | 步骤 → 通过条件 |
|---|---|
| b1 | 全新环境 `style install` 示例包 → 用户信任一次 → 再装第二个包、卸掉它、`sync` → **不再出现任何信任提示**，`doctor` 全绿 |
| b2 | 新会话里让模型复述收到的上下文 → 含实际模型名和"推理强度 low"；去掉 `model_reasoning_effort` 重开 → 显示"未知"，`low-effort-guard` 不触发；设成 `high` → 不触发；设成 `minimal` → 触发 |
| b3 | `/compact` → 下一个回合开始时 `session-info` 重新出现且只出现一次；模型仍能复述 `developer_instructions` 里的静态规则（U11） |
| b4 | 用 `cat`、`sed -n 1,20p`、`head` 读 `code-review` 的 SKILL.md → skill 规则只在第一次出现；提示里写 `$code-review` → 不再注入 |
| b5 | `git push --force` 被拦并显示理由；`git push --force-with-lease` 放行 |
| b6 | `apply_patch` 新增含 TODO 的行 → 用户看到全文，模型能复述那一句；只删除 TODO 行 → 没有提示 |
| b7 | 调用 GitHub MCP 的 `create_pull_request` 前出现注入（没有这个 MCP 时用测试用的假 MCP 服务器） |
| b8 | 静态规则出现在 `config.toml` 开头的托管区；卸载后托管区消失，其余字节不变 |
| b9 | 改了包里的 md 不 `sync` → 行为不变；`sync` → 提示需要 `trust` → `trust` 后行为改变 |
| b10 | 插件路线和用户级路线各完整通过 b1–b9 一次；切换路线后旧路线无残留，不重复注入 |
| b11 | 固定订阅面下 PreToolUse 多出的延迟 p95 < 150 ms（含登录 shell）；`doctor` 的登录 shell 检查通过 |
| b12 | `codex resume` 后 `session.start` 规则不重新注入；记录之前注入的文字是否还在（U-T1） |

**Claude Code**：

| # | 步骤 → 通过条件 |
|---|---|
| c1 | 安装后新开会话（接受工作区信任）→ SessionStart 注入出现；模型名正确；强度在 SessionStart 时就有（U-C1 成立）或在第一次工具调用后的下一条提示上补上 |
| c2 | `/compact` → `reinject_on_compact` 的规则重新出现且只出现一次 |
| c3 | `/model` 切换 → 下一条提示上 `session-info` 补发一次，值是新的 |
| c4 | 模型调用 `Skill` 工具和用户直接打 `/code-review` 各触发一次 skill 规则；同一会话里不重复 |
| c5 | PreToolUse 上的 `warn`：界面出现提示（U-C5），模型能复述那一句 |
| c6 | `git push --force` 被拒，模型看得到理由；提示词被拦时，拦截消息里没有原文 |
| c7 | 并行工具调用时，同一条 `session` 规则只出现一次 |
| c8 | 卸载后 `settings.json` 和 `CLAUDE.md` 和安装前逐字节相同（别人的条目保留） |
| c9 | hook 进程 p95 < 50 ms（exec 形式）；`claude --resume` 后 `session.start` 规则不重新注入 |

### 14.3 实测项的结局

每个实测项记录**符合预期**、**回退方案生效**或**阻塞**，并保留实际范围与失败证据。原 M1b 门槛不替代后来批准的全部补验收；任何尚缺证据的 FINAL 条目继续保留 `[~]` / `[!]` / `[?]`，不以脚本准备或历史程序结果标记当前完成。整批 C9 需最终程序重新采集，见 [隔离 runbook](../docs/review/M1-FINAL-06-CLAUDE-RUNBOOK.md)。全部 M1 补项闭合后再规划 M2。

### 14.4 之后

| 里程碑 | 内容 |
|---|---|
| M2 | `target: native`；`style stats`；跨包去重（T8）；Claude 静态位置改用 `~/.claude/rules/`（U-C4）；评估 Claude 的 output style 能否做 `replace` |
| M3 | `explain`、`probe`、`update`、git 来源、`--frozen`、项目级路线（含 Claude 云端会话）、Claude 插件路线、`--count=exact`、`refresh`（token 设计 T4）、事件内容带围栏进模板、`provider`、规则级 `fail: closed`、用户自定义 effort 映射、`unknown` 匹配、Windows |
| M4 | 其他 harness（Gemini、OpenCode、Cursor、Copilot、Kimi…），每家一个适配器；资料见规范附录 A |

## 15. 未决项

### 15.1 要实测的

保留设计阶段的待测项；已做的准备验证写在对应行，完整验收状态以 [测试矩阵](../docs/STYLE-M1-TESTS.md) 为准。"测之前"一栏记录当前采用的做法；"M1b 结果"一栏按 M1b 真实验收证据回填，B/C 编号指测试矩阵条目，"未测"表示 M1b 没有覆盖、原做法不变。

Claude U9、U-C1/C2/C3/C5 的隔离补验收已准备在 [FINAL-06 runbook](../docs/review/M1-FINAL-06-CLAUDE-RUNBOOK.md)，用户选择稍后操作。准备成功不改变下表真实结果；发现无效能力后须修正声明与回归，再采最终 C9。

| # | 问题 | 为什么重要 | 怎么测 | 测之前 | 结局对 M1 的影响 | M1b 结果（2026-10-16） |
|---|---|---|---|---|---|---|
| U2 | Codex 个人 marketplace 登记和启用插件能否自动化 | `install` 能不能全自动 | 装一次，看 Codex 是否加载插件 | 当前由用户按 M1b 手册手动登记/启用；0.159.3 本地管理命令准备验证已通过，STYLE 尚未实现自动化 | 不能自动化不算失败，b1 按手动步骤做 | 未自动化：B10 由 agent 手动执行 `codex plugin marketplace add` 与 `codex plugin add`，Codex 0.159.3 加载插件，只新增 marketplaces/plugins 两段；取消注册后 config.toml 逐字节还原 |
| U3 | 已生成的插件 hook `trusted_hash` 是否被真实 Codex 接受 | 生成摘要与实际信任是否一致 | 新会话记录是否仍有信任提示 | 渲染器默认生成摘要；插件路线仍报告 user-confirmation，不假定已信任，无额外 CLI 开关 | 不影响通过 | 接受：B10 插件路线新会话四类 hook 全部 `hook.source=plugin`，无 hook 信任提示（`codex exec`）；B1 用户级路线的预置摘要在 exec 与 TUI 下也无 hook 信任提示。报告仍写 user-confirmation，未改 |
| U5 | 每次工具调用多起一个进程的开销（Codex 含登录 shell） | 固定订阅面是否可接受 | 基准：连续 1000 次 PreToolUse | 先按固定订阅面实现 | 超出预算 → 改成按规则生成 matcher（每次变化要重新信任），属于设计变更，M1b 暂停评审 | 成立（用户决定以 200 次为准）：先测连续 731 次真实 PreToolUse（用户叫停，未满 1000），p95=41.6ms、max=44.9ms；CR-05 后用 d7438ec 复测 200 次，p95=28.6ms、max=74.2ms；两轮均 0 错误，远低于 150ms，不需改设计，见 B11 |
| U6 | Codex 云端 skill 的读取工具在 hook 里叫什么 | 云端 skill 能否识别 | 核查固定订阅、runtime 与 plan；实际云端工具名需未来环境另测 | 不支持，plan 里说明 | 不影响通过 | FINAL-05 按不支持边界关闭：plan 明确说明，仅支持本地显式/隐式路径；原生云端工具名和投递未实测 |
| U7 | Codex SubagentStart 返回的 context 是否真的进入子 agent | F7、`event: subagent.start` | 让子 agent 复述 | 按源码实现 | 不成立 → Codex 上 `subagent.start` 规则降级为丢弃，F7 移出 M1 | 成立：CDEX-U7 中 spawn_agent 启动的子线程 rollout 有逐字相同的 `hooks.additional_context`，stats 记在子 agent scope，父线程没有；F7 保留在 M1。FINAL-07 补充 fresh、fork=all 和 depth2 嵌套三个子 agent，context/tool inject/block 均在独立 scope，父 scope 无子事件；真实请求与 rollout 交叉核对通过 |
| U9 | Claude 的 hook 里能否知道思考被关闭 | Claude 能否产出 `none` | 关掉思考后看 payload 和环境变量 | 不产出 `none` | 不影响通过 | FINAL-06按既定边界关闭：用户所见Opus5.5无off档；真实Haiku5.5控制配置alwaysThinkingEnabled=false/MAX_THINKING_TOKENS=0仍发送adaptive、hook与CLAUDE_EFFORT仍high。没有观测到实际off，不伪造none，保留原回退 |
| U10 | Codex 配置层的优先级、项目信任的判断方法、`-c` 能否从祖先进程恢复 | Codex effort 准不准 | 多种组合启动，对比 `doctor` 的 effort 来源和 Codex 实际用的值 | 按 9.4 的顺序实现；判断不了项目信任就不读项目层 | 不影响通过，结论写回 9.4 | 配置解析已实测：M1-FINAL-02 用真实 0.159.3 与本地请求 sink 核对 11 组配置，修复独立 profile 和项目优先级；legacy 新版拒绝、缺失 profile 继承用户。生产未知信任仍跳过项目层；M1-FINAL-04 两条路线各自用 9 种配置核对原生真实请求、STYLE state/stats、rollout 与正文摘要。受信项目原生 high / STYLE 用户 low 的差异按既定回退保留，doctor 明确报告原因；unknown 不伪造档位。两条路线独立各完成 B1–B9，非跨路线拼接 |
| U11 | Codex `/compact` 之后 `developer_instructions` 是否仍对模型可见；SessionStart(`compact`) 的时机 | 静态规则压缩后是否安全 | `/compact` 后让模型复述 | 认为可见 | 不可见 → Codex 上静态 system 规则同时挂一个只在 `compact` 来源触发的 `session.start` 条目补发 | 可见：B3 经 app-server compact 后 developer_instructions 仍在模型输入；SessionStart(compact) 排到下一回合开始时执行，compact 后进程退出则排队的事件丢失 |
| U-C1 | `$CLAUDE_EFFORT` 在 SessionStart、UserPromptSubmit 的 hook 进程里是否存在 | 第一次工具调用之前能否拿到强度 | 探针 hook 打印环境变量 | 认为存在；不存在就等工具事件，靠 3.7 的重新评估补上 | 不影响通过 | FINAL-06逐事件采集：启动/提示/显式技能无effort及环境值，工具前/后/失败含effort.level与CLAUDE_EFFORT；观察high→medium，启动缺值保持unknown，后续按既定动态重新评估补齐 |
| U-C2 | 改了 `settings.json` 的 hooks 后，运行中的会话是否马上生效 | 报告怎么写 | 会话运行中装一个包 | 报告写"新会话生效" | 不影响通过 | FINAL-06当前进程新增Read matcher，实际Read调用、原生hook输出、stats和用户HOT_READ可见性一致；原Glob尝试实际Bash，未计通过 |
| U-C3 | `/clear` 后 `session_id` 是否变化、`model` 是否缺失 | 状态和去重 | 探针 | gen +1 兼容两种情况；model 缺失时保留旧值 | 不影响通过 | FINAL-06：clear创建新session，原生无model、effort和环境值，新state不沿用旧model；后续工具补medium。SessionStart USER已输出但用户未见 |
| U-C4 | 用户级 `~/.claude/rules/*.md` 是否生效 | 静态位置选哪个 | 放一个文件，看 `InstructionsLoaded` | 用 `CLAUDE.md` | 生效 → M2 切换 | 未测（M2） |
| U-C5 | Claude 各事件上的 `systemMessage` 是否显示给用户 | `warn` 给用户的那份 | 每个事件返回一条 | 认为 PreToolUse、PostToolUse、UserPromptSubmit、UserPromptExpansion 都显示 | 不显示的事件在适配器里去掉 `system-message` 能力，`warn` 只剩模型那句 | FINAL-06六类事件USER标记均可见；clear来源的SessionStart标记虽输出但用户未见，文档明确边界，不宣称所有来源均可见 |
| U-T1 | resume、fork 之后，之前注入的文字是否还在上下文里（两家） | `sources` 的默认值 | 恢复后让模型复述 | resume 不注入、fork 注入 | fork 也保留 → 默认去掉 `fork`；Codex resume 不保留 → Codex 适配器把 `resume` 当 `startup` 处理 | Codex FINAL-07：resume/fork 的真实请求保留原 context；startup-only 探针没有补发，默认 sources 的 fork 条目按既定策略仍投递一次。Claude FINAL-07真实认证CLI也保留startup-only标记，resume无重注、fork默认规则投递一次；两家sources默认值均不改 |
| U-T2 | 压缩后 hook 注入的文字是否被摘要保留 | `reinject_on_compact` 的默认值 | `/compact` 后复述 | 认为丢失（`system` 默认补发） | 保留 → 默认值是否改成 false 由用户决定 | Codex FINAL-07 实测：关闭 reinject 且 startup-only 的专用探针，compact 后 epoch=1、输出仍各一次，真实请求中两条动态标记均消失；支持保留补发默认值。Claude FINAL-07专用关闭补发探针也测到两标记消失：native手动compact边界3307→775、epoch1、历史输出仍各一次；不以模型复述替代请求核对 |
| U-T3 | 估算器校准 | 误差范围、系数 | 固定 40 条中英各半规则，两家官方接口相同结构基线增量 | 不承诺误差范围 | 保留现有系数，真实校准后再议 | FINAL-09：语料/正文哈希、模型和请求摘要已固定，生产 CLI 估算已记录；两家官方 API key 缺失，真实计数均 0/40，保持待验；删除 CLI ±25% 未验证保证 |
| U-T4 | 真实会话请求、规则注入/去重与总 usage | 成本模型参数、预算默认值 | stats 与原生 usage 分层统计 | 60 次请求、3 次触发；预算 400 / 1,200 / 600 | 有代表性数据再调默认值 | 已统计：FINAL-10 两条 Codex 路线各 17 会话/31 请求，Claude 历史会话 108 个带 usage 响应；累计快照及重复消息已去重。规则 token 仍为估算，验收负载不代表典型账单，默认值不变 |
| U-T5 | 超限时两家如何处理 | `T003` 的严重程度 | 原生 raw hook 绕开 STYLE，分别记录完整/预览/丢弃/报错 | 保留 STYLE 保护 | 不影响通过 | FINAL-08：两家 STYLE T003 写入前拒绝；Codex SessionStart 12k/50k 与 PreToolUse 12k 实际落盘并传头尾预览及路径，文件逐字节等于 raw body；不等于静默丢弃。Claude2.1.300向本地协议stub的8组实际请求确认：9,999/10,000字符完整，10,001/12k/50k和prompt/pre/post12k均落盘预览，文件逐字节相等；无远程推理或官方token计数结论 |
| U-T6 | OpenAI / Codex 的提示词缓存条件与 usage | 成本公式的适用范围 | 核对官方文档及真实 usage 字段 | 公式只作明确假设的比较 | 不影响通过 | 已核对：FINAL-10 记录官方资料、模型门槛/生命周期及 usage 字段；CLI 标注情景，不把自定义服务 usage 或规则估算当实际账单 |
| U-V | 目标 Codex 版本和 `main` 在 hook 上的差异；Claude 的最低版本（`PostModelSwitch` v2.1.251、`mcp_server` v2.1.274、`onFailure` v2.1.295） | `requires.harness` 的取值 | 对目标版本重读源码并实测 | Claude ≥ 2.1.274；Codex 写实测版本 | 写回示例包 | 已回填：示例包要求 Codex >=0.159.3、Claude Code >=2.1.300；M1-FINAL-01 同时修复安装/重编译未执行版本过滤的问题，旧版及未知版本跳过且不修改配置 |
| U-F9 | agenthooks `3f734c7` 的 Claude codec 已经有哪些字段和事件 | F9 的工作量 | 读 `codec_claudecode.go` | 按缺失处理 | 不影响通过 | 已关闭：T42 读取并补齐 Claude codec 字段与事件 |

### 15.2 已由文档或源码关闭（各留一个回归测试，不再算实测项）

- **Codex**：压缩后有 SessionStart(`compact`)，排队到下一个回合开始时执行；`$PLUGIN_ROOT` 经 `shell -lc` 展开（原 U1）；`apply_patch` 的输入是 `{"command": "<补丁>"}`（原 U12）；插件信任键是长格式；hook 输入里没有强度；hook 里 shell 叫 `Bash`（原 U4，F1 照修）。
- **Claude Code**：SessionStart 的五种来源；`prompt_id`；`effort.level` 报实际生效值；`model` 只在 SessionStart；exec 形式；`onFailure`；`suppressOriginalPrompt`；单字段 10,000 字符上限；`UserPromptExpansion`；`mcp_server`；注入文字保存在 transcript、resume 时回放；子 agent 压缩后重新注入。

### 15.3 设计层面留到以后的

- `refresh`（隔 N 个回合补一次，token 设计 T4）、事件内容带围栏进模板、`--count=exact`、用户自定义 effort 映射、"取不到"能否用 `unknown` 匹配、`provider` 的数据来源、规则级 `fail: closed`：M3。
- Codex 会话中途 `/model` 改强度看不到，这是"只读配置"的代价；要不要补办法，M3 之后再说。
- 只有开关的模型（Kimi、GLM）的开关从哪读：跟着这些 harness 一起推迟到 M4。
