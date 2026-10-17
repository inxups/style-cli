# Git 来源与锁定安装

M3-T20 支持显式公共 HTTPS Git 来源，M3-T21 支持明确选包更新。本地目录、单文件和 `--link` 继续使用原有流程；包格式仍为 `style/v0.3`。本功能的离线与受控 TLS 验收见 [M3-A3](review/M3-A3-2026-10-17.json)，设计依据见 [D2](review/M3-D2-SOURCES-2026-10-17.md)。`update` 已通过 [M3-A4](review/M3-A4-2026-10-17.json)，`install --frozen` 待 T22 实现；两家真实投递、Windows 和远端 CI 单独验收。

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

同名 Git 重装要求来源 URL/ref/commit/path/kind、文件、settings 和安装选择一致。版本相同但 revision 不同、换 ref、换路线或本地/Git 来源转换均不能被当作无变化。来源 URL/path/类型转换需要明确卸载后重装；按原来源更新使用下述 `update`。

## 快照与缓存

每次取得使用私有空 bare、空模板/hooks 和明列的 Git 环境，按广告捕获的完整 OID fetch，再读取固定 tree/blob。没有 checkout、archive、filter、textconv、LFS、submodule 或安装脚本执行。分支在广告后移动时不会换取新 tip；旧 OID 不可达或服务端拒绝取得时失败。

只导出选中范围。普通文件全部参与摘要，忽略 `.git/.hg/.svn/.DS_Store`；`.gitattributes` 作为数据保留。拒绝选中 symlink/gitlink、非法路径、大小写折叠碰撞、非 UTF-8/NUL。限额为 1,024 文件、16 MiB 合计、每文件 256 KiB；文件写 0600、目录 0700，不保留 executable bit。本地来源没有新增总量限制。取得总时限 60 秒，并响应更早的取消；它不是远端 pack 下载字节上限。

缓存位于 `STYLE_HOME/cache/sources/git/v1/<key>/`，包含严格 manifest 和 `files/`，不保存可变 bare。key 由 URL/object_format/commit/path/kind 的数组编码 SHA256 得出，ref 不参与。每次命中检查身份、文件全集、权限和摘要，拒绝额外文件、symlink、损坏或逃逸；普通命令可重取修复。只有完整暂存目录才原子发布，清理只处理本次暂存/隔离条目。

当前即使缓存命中，仍会解析远端并取得固定对象来核对选择；`cache_hit` 表示复用导出的源码字节，不表示离线或零 Git。取消可能留下完整可验证缓存，不会留下半安装。锁定离线恢复使用下述 `install --frozen`。

## 锁、漂移与提交保护

首次成功 Git 安装把整锁升到 format 2。新版读 1/2，纯本地旧锁保持 1，非空 format 2 不自动降级，最后卸载移除锁。旧管理二进制明确拒绝 format 2；普通 runtime 仍只读 format 1 index，不读来源或启动 Git。

`source.kind` 仍为 dir/file，`source.path` 为仓库包路径，`source.git` 保存 URL/ref/object_format/commit/object/content_sha256。逐文件摘要及 `path + NUL + hash + LF` 的排序聚合算法不变；Git OID 与内容 SHA256 分开。批准快照绑定 Git 元数据，不能只改锁里的 commit/object 文本并借其他管理写入重新批准。目录及单文件管理副本继续采用原存储布局。

Git 管理副本的编辑、丢失或 symlink 漂移报 `git-snapshot-changed`；sync/trust 不会把新内容标成原 commit。list/doctor 展示身份与漂移，不取得 Git。来源与安装文件保持只读；doctor 的既有 state 维护仍可能清理过期会话。恢复可先还原锁定字节；开发修改另用本地/link 来源，锁定离线恢复见下文。

install/update/sync/trust/uninstall 共用提交阶段的 `.management.lock`。网络和等待确认期间不持锁；busy 明确失败，陈旧回收核对 OS PID/出生身份，不按年龄删除。提交前重新检查捕获的 lock、包、批准文件、settings、index/说明和 renderer 输入，包括未改变的配置、文件身份/模式及父路径。确认期间发生本地复制包/link 或配置改动，现在会拒绝旧候选，要求重新预览。

app 协调包交换和文件事务，失败逆序恢复 bytes/mode/mtime 并报告恢复错误；恢复失败保留包或文件备份及路径。agenthooks 继续负责协议、渲染、owner/trust 合并与 ApplyFiles，提供 renderer 输入路径枚举供 app 捕获；ApplyFiles 现以类型化错误报告恢复失败，并保留恢复失败的备份及原 mode/mtime。保护范围是同版合作管理进程；它不承诺跨文件断电原子性，也不能排除旧版或同用户非合作写入。

## 明确选包更新

```sh
style update terse-reviewer --json
style update terse-reviewer --ref refs/tags/v2 --yes --json
style update --all --allow-replace --yes
```

必须指定一个已安装包名或 `--all`，不默认更新全部；`--ref` 只适用于单个 Git 包。默认生成完整预览，有变化且未确认时退出 1；`--yes` 应用本次捕获的候选。两次 CLI 调用各自重新捕获，后一次报告才对应实际应用内容。无变化退出 0，不改安装文件的字节、mode、mtime 或 inode；Git 取得仍可能读写完整来源缓存，报告将 acquisition 与安装 changed 分开。

Git 重新解析锁定 branch/tag，固定 OID 不跟随分支，不可达时也不回退到缓存中的其他 revision。可显式换 ref，不能换 URL/path/存储类型。本地复制包读取原 source.path，link 读取当前链接源；名称和 dir/file 类型必须保持。选中复制/Git 管理副本先与旧锁比对，已编辑、丢失或 symlink 漂移先失败，不覆盖未批准改动。版本允许不变或降低，来源、版本、内容及 native 变化分别披露。

保留 seq 和仍声明的旧 settings 值，删除已移除键；新键仅用包中 default，即使私有文件已有同名未声明值也不采用。缺新必填值明确失败，提示先按现有 install/配置流程补齐；update 不接收 `--set`、环境或其他包的值。新增 replace 要求 `--allow-replace`，两 replace、requires、预算和 fallback:error 沿用既有门禁。

未选包只取经 lock.approval 校验的批准字节；普通 link 已有同步授权也不带入新内容。选中候选与未选基线联合编译、去重和预算检查；未选的渲染/native/settings 效果或配置信任修复发生变化时，报 `update-unselected-effect` 并列出包名，须显式 `--all` 或先处理相关包。未选包缺可验证批准基线时保守失败。旧普通批准可兼容；旧 native 批准缺有序表示时，须明确选中或 `--all` 重新披露批准。选中旧包缺基线会在预览中标明，不将未知旧效果伪装成相同。

路线、版本、config_dir、fail_closed 和静态布局沿锁绑定，不 Detect 或自动迁移旧 Claude 布局。需要迁移时先按既有 sync/管理流程处理，再运行 update。预览完整披露 native 增删改及顺序、首次 link 信任、source/rendered 差异、settings 键与预算/路线报告。一次调用只捕获一次，确认回调收到独立副本；确认后不重新取分支或读取新源码。提交前核对捕获输入及身份、mode、mtime、父路径，变化报 `installation-changed`；私有 lock/approved/settings/index/explain 或 build 父路径为 symlink 时拒绝。

多包先全部校验、暂存，再交换包并调用同一次 agenthooks 文件事务。任一失败逆序恢复，恢复失败报 `rollback-failed` 并保留备份路径；不会清理 runtime state、stats 或 probe 数据。共享 guard 与 owner 保护仍适用，`--yes` 不扩大归属权限。

JSON 始终输出一份 format=1、command=update 文档，含 status、selected、changes、reports、diagnostics、awaiting_confirmation；changes 按原 seq 排序，before/after 含安全来源、版本、内容摘要和 settings 键。native 命令和正文仅出现在显式批准披露，不进入常规错误摘要。status 为 unchanged/confirmation-required/applied/error；用法解析失败 stdout 为空。无变化/应用成功退出 0，未确认/普通失败退出 1，既有预算或 fallback:error 阻止退出 4；3 仍仅属于 sync。

## 整锁离线恢复

```sh
style install --frozen --json
style install --frozen --allow-replace --yes --json
```

T22/A5 实现当前 STYLE_HOME 的整锁恢复。入口不接位置来源，也不接 `--harness`、`--route`、`--set`、`--link`、`--ref`、`--path`、`--fail-closed`、`--project` 或锁路径覆盖。要求显式 format=1/2 的非空完整锁，以及每个包可认证的 approval；旧锁缺元数据时先通过现有 install/sync/trust 流程补齐。只复制 lock 无法建立安装或批准。

复制目录/单文件依次使用精确匹配的管理副本、原路径或批准快照；Git 使用管理副本、完整校验的已有导出缓存或批准快照。每包必须是一份完整匹配的快照，不拼接缺失文件。Git 身份与逐文件摘要均核对；缓存可以缺失或损坏，只要另有完整批准内容，且不会修复或写缓存。link 仅允许当前路径的精确匹配且已 trust 的 live 内容，不能用旧批准掩盖变更。全过程不启动 Git、不解析 ref、不联网；远端分支移动或不可达不影响已有锁定内容。

私有 settings 的键必须与锁完全相同，值必须能重现批准后的渲染/native 表示，不采用 default 或环境补值。现有锁没有 settings 值摘要，因此未影响批准效果的值变化不能被判为内容漂移；这不允许缺键或额外键。源、schema、runtime/harness requires、联合预算/去重、native 与 replace 门禁仍执行，replace 即使无变化也要求 `--allow-replace`。按原 seq 处理整锁，任一包失败不开始安装。

路线、版本、配置根、surface/rendered、trust、fail_closed、静态布局和程序路径必须保持原绑定，不 Detect、不迁移或重定位。当前可执行文件路径改变也会失败，应先用普通安装明确更新绑定。可恢复缺失/损坏的 index/explain 和复制包；配置只在 agenthooks 既有 owner 计划允许且能重现锁中摘要时恢复，不接管外来文件或编辑过的托管块。

现有 lock.rendered 保存的是上次变更集摘要，并非完整安装清单：部分配置缺失时可能不能重现该摘要，frozen 将保守失败；Claude 已登记但缺失的 rules 文件也沿用 owner 拒绝规则。先用 doctor 检查并还原批准文件，或经现有管理流程重新建立完整基线。`--yes` 不放宽这些边界。

默认对实际恢复要求确认，预览完整披露 native 与路线/预算报告；无变化直接成功。JSON 为单份 format=1、command=`install --frozen`，在共享 selected/changes/reports/diagnostics/status 字段外含 `locked_source: true`、原始 `lock_sha256` 和每包 `contents`（snapshot/storage_repair）。状态 unchanged/confirmation-required/applied/error；无变化或成功退出 0，未确认或普通失败 1，预算/fallback:error 阻止 4；3 仅属于 sync。用法错误 stdout 为空。

锁、settings、approval 保持只读；锁 bytes/mode/mtime/文件身份均保全。提交阶段复用共享 guard、捕获输入复核、多包全部暂存后交换及同一次 agenthooks 文件事务。stale 输入、busy、取消或应用失败明确报告，逆序恢复 bytes/mode/mtime；恢复失败保留备份并报 `rollback-failed`。不清理 runtime state、stats 或 probe。固定根下的离线 fixture 不证明真实 harness 信任/加载或跨平台可移植性，项目与云端边界由 D3 定义。

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

A4 的 31 个新函数/244 个叶子用例及 16 份新增 text/JSON golden 覆盖更新、联合授权、失败恢复和旧安装；旧 golden 未改。新增脚本还对三条路线运行 24 项 built CLI 检查，比较本轮开始前的 `fd88e8d` 与新二进制的普通 runtime wire，并用固定 M1 `74a473d` 验证旧安装生命周期和 Claude 旧布局迁移边界。两种对照各有用途，不能把 M1 对较新 index 的既有不兼容计为更新回归。

在上述构建目录中额外构建固定 T20 二进制，再运行脚本：

```sh
m3_update_baseline="$m3_build_dir/update-baseline"
mkdir -p "$m3_update_baseline"
git archive fd88e8da66cd8409b6939e71875edd4bee3c9421 | tar -x -C "$m3_update_baseline"
env -u CODEX_HOME -u CODEX_SANDBOX -u CLAUDE_CONFIG_DIR -u STYLE_HOME \
  GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache GOFLAGS=-mod=readonly GOWORK=off \
  go -C "$m3_update_baseline" build -trimpath -buildvcs=false \
  -o "$m3_build_dir/style-update-baseline" ./cmd/style
PYTHONDONTWRITEBYTECODE=1 python3 scripts/check_m3_update.py \
  --binary "$m3_build_dir/style" --old-binary "$m3_build_dir/style-legacy" \
  --baseline-binary "$m3_build_dir/style-update-baseline" \
  --report "$m3_build_dir/update-gate.json"
```

脚本使用私有配置和仅报告版本的 harness stub；安装后以 canary 证明 update/普通 runtime 不调用检测或 Git，native 只披露不执行。真实投递、Windows、远端 CI 和官方计数继续单独验收。

A5 使用 16 份独立 text/JSON golden 记录恢复预览、应用、无变化和错误；已有 golden 保持不变。built CLI 脚本复用版本 fixture，用 canary 核对零检测/Git 和缓存只读，比较 T21 与当前普通 runtime wire，并用真实 M1 安装验证缺基线拒绝、普通管理补齐和恢复/卸载。

```sh
m3_frozen_baseline="$m3_build_dir/frozen-baseline"
mkdir -p "$m3_frozen_baseline"
git archive b55e4c8acabe52499e5d88ed8e2364a82c311c72 | tar -x -C "$m3_frozen_baseline"
env -u CODEX_HOME -u CODEX_SANDBOX -u CLAUDE_CONFIG_DIR -u STYLE_HOME \
  GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache GOFLAGS=-mod=readonly GOWORK=off \
  go -C "$m3_frozen_baseline" build -trimpath -buildvcs=false \
  -o "$m3_build_dir/style-frozen-baseline" ./cmd/style
PYTHONDONTWRITEBYTECODE=1 python3 scripts/check_m3_frozen.py \
  --binary "$m3_build_dir/style" --old-binary "$m3_build_dir/style-legacy" \
  --baseline-binary "$m3_build_dir/style-frozen-baseline" \
  --report "$m3_build_dir/frozen-gate.json"
```

A5 的命令、源码/二进制摘要、失败修正及边界记录见 [完整报告](review/M3-A5-2026-10-17.json)。真实 B2/C2、Windows、远端 CI 和官方两家各 0/40 仍待验；D3 开始前按用户要求通知。
