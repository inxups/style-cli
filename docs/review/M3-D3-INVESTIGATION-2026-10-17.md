# M3-D3：安装范围与新路线隔离调查

日期：2026-10-17。状态：**调查稿，不是评审稿，未获用户批准**；M3-D3 保持 `[ ]`，不关闭 A6–A8、B2、C2–C4。依据 [M3 任务](../STYLE-M3-TASKS.md) “设计需要原生探针时可先做该设计范围的隔离调查”的约定，在 T21/T22 推进期间先固定原生事实，供 T22 关闭后的 D3 评审稿引用。

调查起点为 clean `fd88e8da66cd8409b6939e71875edd4bee3c9421`（分支 `claude/m3-d3-investigation`），本机 codex-cli 0.159.3、Claude Code 2.1.301、Git 2.54.0。生产源码、spec、任务/测试矩阵均未修改。证据见 [调查报告](M3-D3-INVESTIGATION-2026-10-17.json)。Codex 探针为 [probe_m3_d3_routes.py](../../testdata/manual/probe_m3_d3_routes.py)（自测 [test_probe_m3_d3_routes.py](../../testdata/manual/test_probe_m3_d3_routes.py)）；Claude 探针为 [probe_m3_d3_claude.py](../../testdata/manual/probe_m3_d3_claude.py)（自测 [test_probe_m3_d3_claude.py](../../testdata/manual/test_probe_m3_d3_claude.py)），由用户 2026-10-17 授权在隔离临时项目中运行，不改动真实 `~/.claude`。

## 1. 当前源码事实

| 当前源码 | 已有行为 | 对 D3 的影响 |
|---|---|---|
| [STYLE_HOME 解析](../../internal/app/lifecycle_store.go) `resolveStyleHome` | 只有一个根：`--style-home`、`STYLE_HOME` 或 `~/.style`；lock、packages、approved、settings、build/index.json、build/explain.json、state、cache 都挂在它下面 | 项目范围要么每个项目一个独立根，要么在同一根内按项目分命名空间；index/state 不能被用户级共用 |
| [Codex 路线 argv](../../internal/adapter/codex/route.go)、[Claude 路线 argv](../../internal/adapter/claude/route.go) | hook 命令里写死了 `--style-home=<绝对路径>`、`--harness-home=<绝对路径>`；[HookCommand](../../internal/adapter/command.go) 把可执行文件也解析成绝对路径 | 写进可提交的项目文件后，别的 clone、别的机器和云端都不能用；项目路线需要可移植的命令形式 |
| [agenthooks Scope](../../agenthooks/install/install.go) | 已有 `ScopeUser`、`ScopeProject`、`ScopePlugin` 三种范围 | 渲染层已有基础，不必另起一套 |
| [Claude 渲染](../../agenthooks/install/render_claude.go) | Project 写 `.claude/settings.json`；Plugin 写 `.claude-plugin/plugin.json` 和 `hooks/hooks.json`，命令仍是绝对路径，没有 launcher，也不生成 marketplace | 插件路线还要补：命令改用 `${CLAUDE_PLUGIN_ROOT}`、版本策略、marketplace 登记 |
| [Codex 渲染](../../agenthooks/install/render_codex.go)、[路径规范化](../../agenthooks/install/codex_path.go) | 非插件范围把 `hooks.json` 和含信任状态的 `config.toml` 写到**同一个** Dir；信任键 = `EvalSymlinks(Dir)/hooks.json` | 项目范围下这样不对：hooks.json 要放在 `<项目>/.codex/`，信任状态必须写进用户 `CODEX_HOME/config.toml`（见 §2 C-2）；另外信任键的规范化和实测的符号链接行为不一致（C-5）。这需要改 agenthooks 的边界，D3 要写明原因并配测试 |
| [Codex 路线表](../../internal/adapter/codex/route.go)、[Claude adapter](../../internal/adapter/claude/adapter.go) | Codex 有 `codex-plugin`、`codex-user`；Claude 只有 `claude-user`；Claude 静态规则落在 `<claude home>/rules` 和 `CLAUDE.md`；Codex 静态规则落在 `config.toml` 的 `developer_instructions` 托管区 | 项目级静态规则的落点（`<项目>/.claude/rules`、`<项目>/.codex/config.toml`）还要另行确认，本轮没有实测 Codex 项目级 `developer_instructions` |

## 2. Codex 项目级 `.codex/hooks.json` 实测

方法：每个场景都用全新的 HOME、CODEX_HOME 和临时 Git 项目；模型服务是本地回环 stub（不做真实推理，不读取也不复制凭据）；hook 是普通 shell 命令，每次触发写一个独立文件，便于计数。信任哈希由 Python 复刻 agenthooks 的 `DefinitionHash` 生成，已用两组 Go 向量核对一致。共 13 个场景，前后跑了 5 轮，结论一致（报告 `rounds`）。

| 场景 | 用户层项目信任 | 用户层 hook 哈希 | 结果 | 默认沙箱 |
|---|---|---|---|---|
| untrusted | 无 | 无 | 不运行 | read-only |
| project-trusted-only | 有 | 无 | 不运行 | workspace-write |
| hook-hash-only | 无 | 有 | 不运行 | read-only |
| project-and-hook-trusted | 有 | 有 | 两个事件各运行 1 次 | workspace-write |
| repo-self-declared-hook-trust | 有 | 仅写在仓库 `.codex/config.toml` | 不运行 | workspace-write |
| clone-with-original-trust | 仅原路径 | 仅原路径 | 不运行 | read-only |
| clone-project-trusted-only | 原路径 + clone 路径 | 仅原路径 | 不运行 | workspace-write |
| user-and-project-same-command | 有 | 用户级和项目级各一份 | 每个事件运行 **2 次** | workspace-write |
| run-from-subdir | 有 | 有 | 从子目录启动也会运行，payload 的 `cwd` 是子目录 | workspace-write |
| run-via-symlink | 规范路径 | 规范路径 | 不运行 | workspace-write |
| symlink-link-project-and-hooks | 规范 + 链接路径 | 规范 + 链接路径 | 运行 | workspace-write |
| symlink-link-project-only | 规范 + 链接路径 | 规范路径 | 不运行 | workspace-write |
| symlink-link-hooks-only | 规范路径 | 规范 + 链接路径 | 运行 | workspace-write |

结论（codex-cli 0.159.3，`codex exec`）：

- **C-1 双重门槛**：项目级 hook 只有在“用户层项目信任 + 用户层按 `<项目路径>/.codex/hooks.json:<事件>:<组>:<handler>` 预置的 trusted_hash”同时满足时才运行。
- **C-2 仓库不能自证信任**：仓库自己 `.codex/config.toml` 里的 `hooks.state` 被忽略，和 [CLI §9.1](../spec/STYLE-cli-design.md#91-路线) “仓库里的配置没法替自己声明已信任”一致。
- **C-3 每个 clone 单独信任**：信任键带路径，换个 clone 路径就要重新信任项目和每个 hook。
- **C-4 从子目录启动**：Codex 会向上找到项目的 `.codex/`，hook 的 `cwd` 是实际启动目录。
- **C-5 符号链接拼写**：项目信任按规范路径匹配；但 **hook 信任键使用启动时的路径拼写**，不做符号链接解析。agenthooks 现在只为规范路径预置，所以通过符号链接进入项目时，hook 会被静默跳过。
- **C-6 不去重**：用户级和项目级定义完全相同的命令时，每个事件运行两次。STYLE 的用户路线和项目路线同时存在时，会重复投递。
- **C-7 静默跳过**：未受信的 hook 在 exec 模式下没有任何 stderr 提示，doctor/probe 需要能报告这种情况。
- **C-8 信任会放宽沙箱**：写入 `projects."<路径>".trust_level = "trusted"` 后，默认沙箱从 read-only 变为 workspace-write。所以**项目信任是用户的安全决定，STYLE 不能替用户写**，最多只预置 hook 哈希，并说明还缺项目信任。
- 所有场景中 Codex 都没有改写用户 `config.toml`（M1 的 app-server 会自己写项目信任，exec 不会）。hook payload 字段：SessionStart 有 `cwd/hook_event_name/model/permission_mode/session_id/source/transcript_path`，UserPromptSubmit 多了 `prompt/turn_id`、没有 `source`。

## 3. Claude Code 项目级实测

方法：每个场景都用全新的 HOME、`CLAUDE_CONFIG_DIR` 和临时 Git 项目；模型服务是本地回环 Anthropic Messages stub，`ANTHROPIC_API_KEY` 是只发给 stub 的占位值，不做真实推理。`-p` 和伪终端交互两种方式分别测试；预置的 `.claude.json` 只跳过首次引导和 API key 确认，“已信任”场景另外写入该项目的 `hasTrustDialogAccepted`。运行前后核对过真实 `~/.claude/settings.json`、`plugins/known_marketplaces.json`、`~/.config/git/ignore`、`~/.gitconfig` 的摘要、权限和 mtime，均未变化。

| 场景 | 结果 |
|---|---|
| project-print-untrusted：项目 `settings.json`，`-p`，未信任 | 两个事件各运行 1 次 |
| project-tty-untrusted：交互，未信任 | 出现工作区信任对话框；不接受时 hook 不运行 |
| project-tty-trusted：交互，已信任 | 两个事件各运行 1 次 |
| dup-user-project-shell / -exec：用户级与项目级定义相同的 handler | 各运行 **1 次**（会去重） |
| dup-user-project-exec-args-differ：同一命令，只有 `--style-home=` 参数不同 | 各运行 **2 次** |
| dup-project-and-local-shell：项目 `settings.json` 与 `settings.local.json` 相同 | 各运行 1 次 |
| projdir-root：exec 形式，args 中使用 `${CLAUDE_PROJECT_DIR}` | 替换生效，值为项目根 |
| projdir-symlink：通过符号链接路径启动 | 运行；`CLAUDE_PROJECT_DIR` 是**规范路径** |
| projdir-subdir / subdir-discovery：从项目子目录启动 | 项目 hook **不运行**，只运行用户级 hook；`CLAUDE_PROJECT_DIR` 等于启动目录 |

结论（Claude Code 2.1.301）：

- **K-1 信任只拦交互模式**：`-p` 在未受信目录里也会运行项目 `settings.json` 中的 hook；交互模式在用户接受工作区信任之前不运行。STYLE 的 doctor/probe 不能用 `-p` 的结果代替交互会话的信任状态。
- **K-2 去重看整个 handler**：用户级、项目级、本地级之间完全相同的 handler 只运行一次，shell 和 exec 形式都一样；参数只要有一处不同就各自运行。现在 STYLE 命令里带 `--style-home=<绝对路径>`，用户路线和项目路线同时安装就会**重复投递**。
- **K-3 项目根就是启动目录**：Claude 不会向上查找父目录的 `.claude/settings.json`（Codex 会，见 C-4），`CLAUDE_PROJECT_DIR` 等于启动目录；通过符号链接启动时会解析成规范路径。从子目录打开会话时，项目路线完全不生效。
- **K-4 可移植命令可行**：exec 形式 args 里的 `${CLAUDE_PROJECT_DIR}` 会被替换，也会导出为环境变量，可以用来去掉写进可提交文件里的绝对路径。
- 文档另外说明：`.claude/settings.local.json` 是个人文件（Claude 首次写入时会加进全局 git excludes），优先级为 managed > `--settings` > local > project > user，hooks 跨层合并；未跟踪的 local 文件里的 allow 规则不需要等信任。

## 4. Claude 云端会话（仅文档依据，C3 待验）

- 云端会话从仓库的全新 clone 启动，单仓库会话读取：仓库 `.claude/settings.json` 里的 hooks 和权限规则、`CLAUDE.md`、`.claude/rules/`、`.claude/skills|agents|commands`。**不读取** `~/.claude/settings.json` 和 `.claude/settings.local.json`；仓库 `enabledPlugins` 声明的**插件不会安装**。
- 环境变量 `CLAUDE_CODE_REMOTE=true` 可以用来判断是否在云端；云端默认预装 Go，`proxy.golang.org` 在默认网络白名单里；setup 脚本在 claude.ai 的环境设置里配置，大约 5 分钟内完成才会被缓存；SessionStart command hook 默认 600 秒超时。
- 对 STYLE 的影响：云端没有 `style` 二进制，也没有 `~/.style`。可选方案：
  1. SessionStart hook 用 `${CLAUDE_PROJECT_DIR}` 下的引导脚本，按锁定版本 `go install` 或构建 style，再生成 index/state；
  2. 让用户在环境的 setup 脚本里安装 style（配置不在仓库内）；
  3. 只交付静态规则（`.claude/rules`），云端不运行动态 hook。

  不论哪种，可提交的配置里都不能出现本机绝对路径；index 要么提交到仓库，要么在会话启动时重建，state 必须放在仓库之外。
- 插件路线不是云端的交付方式。

## 5. Claude 插件路线实测

在同一个隔离配置中依次执行：`claude plugin validate` → `marketplace add <本地目录>` → `install style-runtime@style-m3d3`（user 范围）→ `-p` 运行 → 不改版本号直接修改源目录的 hooks → 运行 → 在用户设置里加入与插件相同的 handler → 运行 → `uninstall` → 运行 → `install --scope project` → 运行 → `uninstall --scope project` → `marketplace remove`。每一步的 CLI 都返回 0，每一步前后都对配置目录和项目 `.claude/` 做了快照对比（报告 `steps`）。

- **P-1 登记写入的位置**：`marketplace add` 新建 `plugins/known_marketplaces.json`，并在用户 `settings.json` 写入 `extraKnownMarketplaces`；`install` 再写入 `enabledPlugins`，新建 `plugins/installed_plugins.json`，并把插件复制到 `plugins/cache/style-m3d3/style-runtime/0.1.0/`。
- **P-2 实际原地加载**：尽管复制了一份到缓存，运行时 `CLAUDE_PLUGIN_ROOT` 指向**源目录**；不改版本号直接修改源目录，下一个会话就会用新 hook。这和文档“本地路径 marketplace 中相对路径声明的插件原地加载”一致，也就不受固定 `0.1.0` 版本号的影响。
- **P-3 插件与设置文件不去重**：插件和用户设置里完全相同的 handler 运行 2 次。所以用户路线和插件路线必须互斥，切换路线时要移除另一边。
- **P-4 项目范围**：`install --scope project` 会**新建**项目的 `.claude/settings.json`，写入 `{"enabledPlugins": {"style-runtime@style-m3d3": true}}`；`-p` 下即使未受信也会运行。卸载后这个文件仍然留着，内容是 `{"enabledPlugins": {}}`，也就是说**仓库里多出一个文件**。
- **P-5 卸载有残留**：全部卸载并移除 marketplace 后，相对基线多出：用户 `settings.json`（内容为空的 `enabledPlugins` 和 `extraKnownMarketplaces`）、内容为空的 `installed_plugins.json` 和 `known_marketplaces.json`，以及带 `.orphaned_at` 标记的缓存副本（文档称 14 天后后台清理）。卸载后的新会话里 hook 不再运行。STYLE 如果要做到“逐字节还原”，就要自己清理这些空键和文件；否则就只能宣称“功能上卸载干净”。
- 文档补充：顶层 `bin/` 会进 Bash PATH，而且带 `bin/` 的插件 claude.ai/Cowork 不会安装，不能照搬 Codex 插件的 `bin/style`；项目范围的插件在交互模式下要等工作区信任；云端会话不加载插件。

## 6. 留给 D3 评审稿的待决问题

1. **项目状态放在哪里**：`~/.style/projects/<规范路径摘要>/` 这样的独立根（不入库），还是仓库内 `.style/`（index 是否提交）？
2. **项目根怎么定**：两家行为不同：Codex 会向上发现项目（C-4），Claude 只认启动目录（K-3）。默认用 git toplevel，还是显式 `--project <dir>`？对 Claude，是否要求从项目根启动并在 doctor 中提示？符号链接拼写怎么处理（C-5：预置多个拼写，还是只支持规范路径并提示）？
3. **用户级与项目级共存**（C-6、K-2、P-3）：Codex 不去重；Claude 只在 handler 完全相同时去重，而 STYLE 命令带 `--style-home`。是禁止同一个包同时装两处，跨两个根做运行时去重，还是让两条路线的 handler 完全一致、由 Claude 去重（Codex 仍会重复）？
4. **Codex 项目信任**（C-1、C-8）：确认 STYLE 只预置 hook 哈希，不写项目信任；install/doctor 报告“项目未受信”。多个 clone 各自安装、各自信任（C-3）。
5. **Claude 项目文件**：写共享的 `settings.json`（可提交，云端可用），还是个人的 `settings.local.json`（不入库，云端不可用）？还是两种都提供？
6. **云端引导**：§4 的三种方案选哪种，或者 M3 只承诺静态规则交付？
7. **Claude 插件**：本地目录 marketplace 已证明会原地加载（P-2），可以和 Codex 插件路线对称。是否由 STYLE 自动执行 `claude plugin` 登记和卸载？卸载残留（P-4、P-5）是自己清理到逐字节还原，还是只承诺功能上卸载干净？
8. **agenthooks 边界改动**：Codex ScopeProject 把信任写入位置拆开、信任键的路径拼写、Claude 插件和项目命令改用 `${CLAUDE_PLUGIN_ROOT}`/`${CLAUDE_PROJECT_DIR}`。每一项都要写明原因、影响和双 module 测试。

## 7. 未覆盖与下一步

- 未覆盖：Codex TUI 下的项目 hook 信任提示、项目级 `developer_instructions`、项目 hook 与 codex-plugin 路线并存；Claude 交互模式下的插件信任、`/reload-plugins`；Claude 静态规则（`.claude/rules`、`CLAUDE.md`）的项目级加载；云端会话（C3）；Windows；STYLE 二进制本身（探针只用普通 shell hook）。
- Claude 的去重、信任和插件结论来自 stub 模型下的隔离运行，没有验证真实模型请求里的上下文投递；真实投递仍归 C2/C4。
- 下一步：T22 关闭后，把本稿事实并入 D3 评审稿，按 AGENTS 约定更新任务状态并追加 PROGRESS。
