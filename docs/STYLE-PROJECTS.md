# STYLE 项目级安装

M3-T30 提供 `codex-project` 与 `claude-project`，通过 M3-A6 离线验收。两家真实加载、workspace trust 和模型实际投递仍分别归 M3-B2/C2；云端导出和 Claude 插件已分别由 T31/A7、T32/A8 实现，当前真实状态见[M3 测试矩阵](STYLE-M3-TESTS.md)。

## 选择项目

```sh
# 目录必须已存在；也支持没有 Git 的普通目录。
style plan ./style-pack --project /work/repo
style install ./style-pack --project /work/repo --harness codex,claude-code --yes
style list --project /work/repo
style update --all --project /work/repo --yes
style install --frozen --project /work/repo --yes
style sync --project /work/repo
style trust package-name --project /work/repo --yes
style doctor --project /work/repo
style stats --project /work/repo --json
style probe --project /work/repo --harness claude-code
style explain --project /work/repo --harness claude-code --event PreToolUse --payload event.json
style uninstall --all --project /work/repo --yes
```

`--project <dir>` 与 `--project=dir` 等价，相对目录以管理命令工作目录解析；不会从 cwd 自动选择项目。省略参数继续使用既有用户安装。项目范围只能选择项目路线；缺参数、重复范围、错误目录和 user/plugin 路线冲突都拒绝。`init`、`validate` 不接受项目范围。文本及 JSON 报告列出规范项目根、私有安装根、加载前提和同一用户基根下的用户包；各范围预算、排序、去重与统计独立，不代表整个会话的合并上限。

设用户管理基根为 U（`--style-home`、`STYLE_HOME` 或 `~/.style`），规范项目根为 R。私有安装位于 `U/projects/SHA256(R)`，使用完整路径摘要，并核对实际 R。lock/packages/settings/approved/build/state/cache/probes 均在这个安装根中；index 仍为 format 1，项目锁使用 format 3。旧程序拒绝项目锁，既有三条用户路线继续使用 lock 1/2。

用户与项目的不同名包可以共存；同一 harness 用户配置下的同名包双向拒绝，需显式卸载其中一处或更名。两个并列项目可安装同名包。U 的项目登记与用户/项目锁共同参与确认漂移和共享提交保护；发现登记缺失、陈旧或不认识的用户 STYLE owner 时，先恢复原登记或选择原 U，不能删除登记绕过冲突。

## 加载前提

两条路线要求已知版本：Codex >=0.159.3、Claude Code >=2.1.301；未知、低版本或 prerelease 不按已支持处理。这是本实现的版本下限，不能代替真实加载验收。

- Codex：hook 写入 `R/.codex/hooks.json`，对应定义哈希写入用户 `CODEX_HOME/config.toml`，每个项目的 trust 区域独立。STYLE 不写 `projects.trust_level`，仍需原生 workspace trust；从规范路径启动，symlink 别名的信任不自动扩展。项目静态 append 沿用 session.start 与 compact 补发降级，不覆盖用户 developer_instructions。
- Claude：hook 写入 `R/.claude/settings.json`，静态 append 写入 `R/.claude/rules/style-<包>.md`。从项目根启动新会话；子目录中能发现静态 rules 不足以证明项目 hook 已加载。用户 rules 和外来 CLAUDE.md 保留；replace 沿用既有不支持/回退行为。
- 启动 harness 的 PATH 必须能找到同一个已批准路径的 `style`，`STYLE_HOME` 必须解析到同一 U，harness 用户配置也须与绑定一致。`--style-home` 只为这次管理调用选 U，不会替以后启动的进程设置环境。

项目内生成 `.style/project.json` 和 `.style/hooks/style`。声明只保存入口版本、包顺序、编译效果与受管文件摘要，不携带私有安装路径、settings、批准快照或信任状态。Claude 入口使用 `${CLAUDE_PROJECT_DIR}`；Codex 从真实进程 cwd 向上发现唯一声明，忽略 payload 中的 cwd。私有 binding 同时核对项目根、U、harness 配置、入口和 runner 路径；失败不读取用户 index，沿 agenthooks 既有 fail-open/fail-closed 协议返回。

原生 native 命令继续由 harness 直接执行，保留原文、顺序和原生 cwd；它们不经过 STYLE engine 的项目绑定检查。审阅预览中的完整命令及其外部依赖，并按 harness 的信任机制管理仓库配置。配置文件存在、shell fixture 成功和 probe 观察到 callback 都不等于模型收到规则。

## clone、移动与恢复

新 clone/worktree 不继承另一根的本地安装、批准或 state。普通 install 必须用明确来源重新生成与仓库声明相同的整套效果，再确认采用；缺项、正文/settings 效果或 native 定义不同会拒绝。`--frozen` 只恢复原根、原 U、原绑定，不能激活未安装的 clone。

多包项目首次激活可以一次提供所有来源，确保确认和提交覆盖完整候选：

```sh
style install ./first-pack --project /work/clone \
  --project-source ./second-pack \
  --set voice=Concise \
  --project-set second-pack.voice=Friendly --yes
```

`--project-source` 可重复，整批最多 64 个包，只用于全新的项目复制安装；包名须唯一。第一个包使用原来的 `--set`，其余包使用 `--project-set 包名.设置名=值`；Git 的 `--ref`/`--path` 为整批共用。来源可先在本地准备为匹配的快照，激活过程不会从声明自动取源。按原包顺序提供候选；声明保留卸载后留下的 seq 间隙。

换 U、移动根或更换 runner 路径都需普通管理流程明确重建或重新批准；不能搬运私有 binding 来继承信任。嵌套 STYLE 项目暂不支持；普通子目录启动和并列项目分别处理。Claude 会话内切 worktree、桌面自动共享 worktree 的加载条件仍不作隔离承诺。

install/update/sync/trust/uninstall 共用现有捕获、预览、U/项目 guard、包 staging、agenthooks ApplyFiles 和逆序恢复。最后一个包卸载会移除本 clone 采用的 STYLE 效果、声明、入口和 binding，保留外部配置字节/权限、用户 native trust 与其他项目；运行历史仍在该项目的私有根，不自动移入其他范围。回滚失败会保留备份并报告恢复路径。

## 验证证据

[验收报告](review/M3-A6-2026-10-17.json) 映射 A6-01–A6-12，包含重放命令、失败及修正、golden、逐文件恢复、旧程序对照和独立项目状态。Go 验证使用临时 GOPATH/GOMODCACHE/GOCACHE 并清理外部变量；离线脚本为 [check_m3_project.py](../scripts/check_m3_project.py)。该脚本只调用版本 fixture 和已生成入口，不运行真实 harness 或模型请求。
