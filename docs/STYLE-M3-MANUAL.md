# M3 真实验收手册

本手册对应 P1/P2；准备通过不等于 B/C、TOKEN 或 W1 已通过。范围与状态以[任务清单](STYLE-M3-TASKS.md)和[测试矩阵](STYLE-M3-TESTS.md)为准。用户已延期 Claude 托管云端 C3、Windows 实际 harness W1 和 Anthropic 独立计数 API；这些条目保留 `[?]`，本轮不申请账号或密钥。

## 构建与准备

从仓库根执行。记录 `git rev-parse HEAD`、工作树状态、Go/OS/原生 CLI 版本及二进制摘要。使用临时 GOPATH、GOMODCACHE、GOCACHE，清理 CODEX_HOME、CODEX_SANDBOX、STYLE_HOME、Claude 配置、effort 与 provider 选择变量；真实运行时再显式设置隔离配置。不可把真实配置目录当作 scratch。

```sh
umask 077
style_m3_cache=$(mktemp -d)
env -u CODEX_HOME -u CODEX_SANDBOX GOWORK=off GOPATH="$style_m3_cache/gopath" GOMODCACHE="$style_m3_cache/modcache" GOCACHE="$style_m3_cache/gocache" go build -trimpath -o /tmp/style-m3 ./cmd/style
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s testdata/manual -p 'test_m3_probe_support.py'
PYTHONDONTWRITEBYTECODE=1 python3 scripts/check_m3_acceptance_prep.py --binary /tmp/style-m3 --report /tmp/m3-preparation.json
```

构建使用三个独立临时缓存，不能使用日常缓存替代。准备脚本使用版本 stub，验证六条路线的 manifest、安装/sync/卸载、index 7、观察器副本摘要以及外部配置 bytes/mode 恢复；不启动模型。实际构建、完整环境与命令写入本轮证据报告。

采集器固定完整仓库源码、嵌入词表/schema/module lock、STYLE 二进制、原生 harness 和实际观察器副本的 SHA256。运行期间不修改这些文件；任何漂移均失败。独立 native hook 只保存原生 payload 和固定 fixture 正文，不包装 STYLE runner、不伪造 probe 回调。单元测试要求拒绝缺原生事件、缺完整请求正文、错 session/路线/二进制、清理失败及源码漂移。

## Codex 本地三路线

使用已配置且可用的真实 Responses provider，采集器将其复制到新的私有配置并经 TLS 校验转发到原有服务。不能更换为 stub 或关闭证书校验。`--codex` 指向实际原生二进制，不能是 Node/shell 启动脚本。每条路线使用全新空目录：

```sh
python3 testdata/manual/probe_m3_live_codex.py --style /tmp/style-m3 --codex /absolute/native/codex --route codex-user --scratch /tmp/m3-codex-user --output /tmp/m3-codex-user-report.json
```

分别以 `codex-plugin`、`codex-project` 重跑并更换 scratch/output。插件必须经原生 marketplace 命令登记；Codex 0.159.3 使用独立缓存，native 更新批准后需原生 `plugin add` 刷新，再开新会话核对新正文。项目采用规范根、原生 trust 与独立 STYLE home，另在其他工作目录运行检查隔离。`exec` 的成功不外推 TUI 信任提示或其他 clone。来源 update/frozen 要核对批准和锁的实际字节。

## Claude 本地三路线

使用已登录且专供验收的 CLAUDE_CONFIG_DIR；原生 Claude 自行消费登录状态和钥匙串，采集器不读取、复制或归档认证文件。真实 HOME 保持原生钥匙串可用，settings、rules、项目配置和 STYLE 状态全部隔离。日常 `~/.claude` 不参与。无可用隔离登录状态时保留失败，等待登录完成后重跑。

```sh
python3 testdata/manual/probe_m3_live_claude.py --style /tmp/style-m3 --claude /absolute/native/claude --auth-config /private/acceptance/claude --route claude-user --scratch /tmp/m3-claude-user --output /tmp/m3-claude-user-report.json
```

分别以 `claude-project`、`claude-plugin` 重跑并使用新的 scratch/output；同一验收 profile 串行使用。原生 UI 确认隔离工作目录 trust，保留私有 UI 记录，不使用 bypass；等待初始化稳定、实际选中 Yes 后再确认，要求 UI 正常退出。隔离 PATH 同时提供实际 Claude 与已固定的 STYLE 入口，项目 launcher 按该 PATH 执行。模型请求通过原生 transcript 的 content/shape hash 精确重建，不能凭回复包含 marker 判定。实际发现 user/project/plugin 配置，不能用显式 `--settings` 或临时 `--plugin-dir` 冒充路线加载。原生 stream 不提供 route 字段，来源按独占 fixture 配置、probe 绑定及独立观察器的 root/payload 关联，须在报告中说明归因方式。

插件首次 STYLE install 退出 1 且 changed=true、registration-required 属预期；按返回命令原生登记并启用后再运行。删除前先原生 uninstall/移除 marketplace，再 STYLE uninstall。原生元数据允许保留空容器；采集器在该私有 profile 中恢复明确列出的配置基线，另验证新会话没有 STYLE/native 回调或正文。

## 未触发与中断

```sh
python3 testdata/manual/probe_m3_negative.py --style /tmp/style-m3 --harness-binary /absolute/native/codex --harness codex --route codex-user --scratch /tmp/m3-negative-codex-user --output /tmp/m3-negative-codex-user-report.json
```

Codex 的 user/plugin/project 和 Claude 的 user/plugin/project 分别使用新空目录；Claude 指定 `--harness claude-code` 与实际原生 Claude 二进制。该采集器只允许原生 version/plugin 管理命令，不读取登录 profile、不启动 HTTP server 或模型。每条路线安装真实 CLI 支持的配置，核对有效 index 为 captured，未触发时 1s 超时与 SIGINT 中断均为失败状态且完整清理；再卸载并恢复显式配置基线。此检查不计原生回调或模型投递；最终六路线证据见[报告](review/M3-PROBE-NEGATIVE-2026-10-18.json)。

## 每条路线的判定与恢复

1. 首次 SessionStart 前状态为空，固定原 payload、index、绑定 facts；explain 的完整 wire 正文必须出现在实际原生请求。probe 收据还须与独立原生事件、session、路线、时间及两份二进制摘要对应。仅 observed 不代表模型投递。
2. 同一 session 连续三次提交核对 refresh 的输出累计、state/stats 与实际请求历史。事件模板以原始 UserPromptSubmit 单独重放，仅比较该规则完整围栏正文；不把空状态中额外出现的其他规则误当 prompt 前状态。
3. 核对 provider、raw/canonical/approx 映射及显式 unknown。原生未提供的事实必须如实保留缺失，不能从模型行为猜测。
4. 真实 Bash/对应 shell 工具触发 native matcher 和普通 guard，未匹配 Read 正文不得出现。保持 index 有效，仅损坏已存在 session state，验证真实工具未写入目标文件且有原生 PreToolUse 尝试；恢复原文件后再次成功。
5. update 修改批准 native timeout 和独立正文标记，下一新会话须收到新正文且无旧正文；frozen 恢复损坏的托管来源并保留锁字节。卸载核对外部配置 bytes/mode 和新会话无残留。

原始 payload、请求、transcript、UI/CLI 日志、配置备份与失败详情只能写入 0700 scratch 中的 0600 文件。仓库报告白名单仅含版本、摘要、固定 fixture 事实、计数/退出码与断言。不得归档 prompt、工具正文、headers、令牌、配置内容或端点。失败报告保持 passed=false；保留原因摘要和复现命令。恢复只操作显式基线文件，遇到 symlink 或来源漂移停止，不能覆盖日常配置。

## 延期环境 P2

| 验收 | 所需环境与入口 | 证据及恢复 |
|---|---|---|
| C3 托管云端 | 实际托管 Claude 会话、兼容 runner、私有仓库外 HOME、native 依赖；按[云端指南](STYLE-CLOUD.md)从已批准项目 export 并审阅后提交交付文件 | 记录云端 clone、版本、bundle 摘要、原生 payload/请求与私有状态位置；移除 bundle/规则后新会话验证。当前缺实际会话，仍待验 |
| TOKEN-01 API | 独立 ANTHROPIC_API_KEY 由本地安全环境注入、可计数模型 ID、官方 HTTPS 接口；按[计数指南](STYLE-COUNT.md)运行生产 CLI | 仅记录最小请求/文本摘要、模型、版本、计数和 scope；缺 key/网络或 API 错误失败，不退回估算。不使用 Claude 订阅凭据 |
| W1 | 实际 Windows 主机、原生 STYLE.exe 和受支持的 Codex/Claude.exe、系统 PowerShell、私有配置与真实账号 | 两家 user 路线安装、原生 stdin/退出码、实际模型正文、DACL/并发状态及卸载；Windows 项目/插件和作者 POSIX native 明确不支持。CI 的版本 stub 不能关闭 W1 |

本地 exact 可立即运行：`style count text.txt --count=exact --count-provider=openai --encoding=o200k_base --json`，再用 cl100k_base 复验；记录固定文本与编码版本，不宣称请求或账单精度。Anthropic API、C3、W1 后续另测，固定 40 条官方校准仍两家各 0/40、非阻塞。
