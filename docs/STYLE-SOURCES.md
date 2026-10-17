# Git 来源与锁定安装

M3-T20 支持显式公共 HTTPS Git 来源。本地目录、单文件和 `--link` 继续使用原有流程；包格式仍为 `style/v0.3`。本功能的离线与受控 TLS 验收见 [M3-A3](review/M3-A3-2026-10-17.json)，设计依据见 [D2](review/M3-D2-SOURCES-2026-10-17.md)。`update` 和 `install --frozen` 由后续 T21/T22 实现，当前不可用；两家真实投递、Windows 和远端 CI 单独验收。

## 使用

```sh
style validate git+https://example.invalid/team/styles.git --ref refs/tags/v1 --path packages/review --json
style plan git+https://example.invalid/team/styles.git --ref refs/heads/main --path review.style.md --harness codex --json
style install git+https://example.invalid/team/styles.git --ref refs/tags/v1 --path packages/review --harness codex --yes --json
```

示例域名是占位符，使用时换成实际公共仓库。先区分远程来源，再处理本地相对路径。三个命令共用 `--ref` 和 `--path`，支持空格或等号形式；重复选项、空值、Git 加 `--link`、本地来源加这两个选项均拒绝。

| 输入 | 规则 |
|---|---|
| URL | 必须 `git+https://host/repository`；去掉 `git+` 后持久化。scheme/host 小写，保留路径大小写、转义及尾斜线；无凭据、query、fragment |
| ref | 完整 `refs/heads/name`、`refs/tags/name`，或 40/64 位小写完整 commit OID；不猜测短名称、短 hash 或 revision 表达式 |
| 默认 ref | 将远端符号 HEAD 解析为规范 branch 并固定当次 commit；unborn/detached/歧义 HEAD 要求显式 ref |
| path | 仓库相对目录或 `.style.md` 单文件，默认 `.`；拒绝绝对路径、空段、dot/parent 段、反斜线、控制字符和展开表达式 |

生产取得仅开放 HTTPS，不读宿主 Git 配置、凭据 helper、askpass、proxy 或继承的 `GIT_*`。私有仓库、SSH、file/HTTP 和 helper 不开放；可先自行取得为本地来源。测试的本地仓库与 loopback TLS 使用内部依赖注入，没有 CLI 开关或环境变量绕过。

## 预览与批准

Git validate 的 JSON 是 `{source, diagnostics}`；原有本地 validate 仍输出诊断数组。plan/install 仅对 Git 增加可选 `source`。其中包含 kind、仓库包路径、规范 URL/ref、object format、固定 commit、selected tree/blob、内容 SHA256、Git 版本和 `cache_hit`。文本报告使用引用转义显示 URL/ref/path，不输出原始 Git stderr。失败诊断使用稳定 code；退出码沿用 0/1/4，3 仍仅属于 sync。

validate/plan 和未确认 install 可以建立完整来源缓存，不改包、锁、批准文件、settings、index 或 harness 配置。安装沿用能力/native 全量披露、`--yes` 和 `--allow-replace`，取得成功不表示包有效或已获执行授权。确认回调收到独立预览；后续应用同次捕获的字节，不再解析移动中的分支。

同名 Git 重装要求来源 URL/ref/commit/path/kind、文件、settings 和安装选择一致。版本相同但 revision 不同、换 ref、换路线或本地/Git 来源转换均不能被当作无变化。当前需要明确卸载后重装；按原来源更新由 T21 提供。

## 快照与缓存

每次取得使用私有空 bare、空模板/hooks 和明列的 Git 环境，按广告捕获的完整 OID fetch，再读取固定 tree/blob。没有 checkout、archive、filter、textconv、LFS、submodule 或安装脚本执行。分支在广告后移动时不会换取新 tip；旧 OID 不可达或服务端拒绝取得时失败。

只导出选中范围。普通文件全部参与摘要，忽略 `.git/.hg/.svn/.DS_Store`；`.gitattributes` 作为数据保留。拒绝选中 symlink/gitlink、非法路径、大小写折叠碰撞、非 UTF-8/NUL。限额为 1,024 文件、16 MiB 合计、每文件 256 KiB；文件写 0600、目录 0700，不保留 executable bit。本地来源没有新增总量限制。取得总时限 60 秒，并响应更早的取消；它不是远端 pack 下载字节上限。

缓存位于 `STYLE_HOME/cache/sources/git/v1/<key>/`，包含严格 manifest 和 `files/`，不保存可变 bare。key 由 URL/object_format/commit/path/kind 的数组编码 SHA256 得出，ref 不参与。每次命中检查身份、文件全集、权限和摘要，拒绝额外文件、symlink、损坏或逃逸；普通命令可重取修复。只有完整暂存目录才原子发布，清理只处理本次暂存/隔离条目。

当前即使缓存命中，仍会解析远端并取得固定对象来核对选择；`cache_hit` 表示复用导出的源码字节，不表示离线或零 Git。取消可能留下完整可验证缓存，不会留下半安装。真正的锁定离线恢复由 T22 提供。

## 锁、漂移与提交保护

首次成功 Git 安装把整锁升到 format 2。新版读 1/2，纯本地旧锁保持 1，非空 format 2 不自动降级，最后卸载移除锁。旧管理二进制明确拒绝 format 2；普通 runtime 仍只读 format 1 index，不读来源或启动 Git。

`source.kind` 仍为 dir/file，`source.path` 为仓库包路径，`source.git` 保存 URL/ref/object_format/commit/object/content_sha256。逐文件摘要及 `path + NUL + hash + LF` 的排序聚合算法不变；Git OID 与内容 SHA256 分开。批准快照绑定 Git 元数据，不能只改锁里的 commit/object 文本并借其他管理写入重新批准。目录及单文件管理副本继续采用原存储布局。

Git 管理副本的编辑、丢失或 symlink 漂移报 `git-snapshot-changed`；sync/trust 不会把新内容标成原 commit。list/doctor 展示身份与漂移，不取得 Git。来源与安装文件保持只读；doctor 的既有 state 维护仍可能清理过期会话。恢复可先还原锁定字节；开发修改另用本地/link 来源，frozen 恢复待 T22。

install/sync/trust/uninstall 共用提交阶段的 `.management.lock`。网络和等待确认期间不持锁；busy 明确失败，陈旧回收核对 OS PID/出生身份，不按年龄删除。提交前重新检查捕获的 lock、包、批准文件、settings、index/说明和 renderer 输入，包括未改变的配置、文件身份/模式及父路径。确认期间发生本地复制包/link 或配置改动，现在会拒绝旧候选，要求重新预览。

app 协调包交换和文件事务，失败逆序恢复并报告恢复错误；恢复失败保留包备份。agenthooks 继续负责协议、渲染、owner/trust 合并与 ApplyFiles，仅新增 renderer 输入路径枚举供 app 捕获。保护范围是同版合作管理进程；它不承诺跨文件断电原子性，也不能排除旧版或同用户非合作写入。

## 重复验收

双 module fresh test/vet、采集器、smoke、旧迁移和 replace 命令见 [测试环境约定](STYLE-M3-TESTS.md#测试环境约定)。新测试使用真实本地 SHA-1/SHA-256 仓库和 `127.0.0.1` TLS fixture；受限环境须允许本机回环监听。30 个 A3 测试函数的 202 个叶子用例均不依赖真实 harness 或远端服务。

在测试矩阵的同一 shell 中完成 `$m3_build_dir/style` 和固定 `74a473d` 的 `$m3_build_dir/style-legacy` 构建后，导出真实安装 fixture 并运行旧版门禁：

```sh
m3_a3_fixture="$m3_build_dir/format-two-fixture"
env -u CODEX_HOME -u CODEX_SANDBOX -u CLAUDE_CONFIG_DIR -u STYLE_HOME \
  GOPATH=/tmp/style-cli-gopath \
  GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache \
  GOFLAGS=-mod=readonly GOWORK=off \
  STYLE_M3_A3_EXPORT="$m3_a3_fixture" \
  go test -count=1 ./internal/app -run '^TestM3A3ExportFormatTwoFixture$'
PYTHONDONTWRITEBYTECODE=1 python3 scripts/check_m3_git_source.py \
  --old-binary "$m3_build_dir/style-legacy" --binary "$m3_build_dir/style" \
  --fixture "$m3_a3_fixture" --report "$m3_build_dir/git-source-gate.json"
```

`STYLE_M3_A3_EXPORT` 仅由测试读取，生产命令没有对应入口。脚本的 17 项检查覆盖六条旧管理命令拒绝 format 2 且保全目录身份/字节/模式/mtime、新版 list、真实新旧 hook wire 相同，以及三条 CLI 的九组非法来源在启动 Git 前被拒绝。生成的安装仅供门禁 fixture 使用，不能当作真实 harness 投递结果。
