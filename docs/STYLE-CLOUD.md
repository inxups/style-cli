# Claude 云端交付

`style export` 将本地项目已批准的 Claude 规则导出为可提交的预编译 bundle。首先在项目根安装；云端环境须预置兼容的 STYLE runner 和 native 命令依赖。托管云端实际投递由 M3-C3 单独验收，本地检查不代表云端已加载。

```sh
style install ./my-style --project . --harness claude-code --yes
style export --target claude-cloud --project . --json
style export --target claude-cloud --project . --yes --json
```

不带 `--yes` 时输出完整预览，存在待确认改动时退出 1，且不写文件。预览包含所有将交付的正文、普通设置替换结果、项目 Claude 配置与 native 命令。检查其中是否含有不应提交的信息；普通字符串没有自动脱敏保证。`sensitive` 设置仍不受支持。已有 source、settings、批准快照、index、配置或项目绑定漂移时，先明确执行相应 update/sync/trust，再重新 export；导出本身不会批准、修复、下载、编译或取得来源。

导出生成 `.style/cloud/manifest.json`、`index.json` 和 `explain.json`。manifest format 1 绑定逐文件摘要、编译说明、项目声明、入口、Claude 配置和静态 rules，记录 cloud entry v1、STYLE runtime 版本及外部依赖。来源元数据使用包逻辑相对名，不复制本机锁、批准仓库、私有根、缓存或 settings 文件。正文和 native 命令保留已批准的内容，可能包含作者自己的路径或普通字符串，须审阅。

将 bundle、`.style/project.json`、`.style/hooks/style`、`.claude/settings.json` 和所需 `.claude/rules/style-*.md` 一起提交。单仓库云端会话从仓库根启动；环境中 `CLAUDE_CODE_REMOTE=true` 时，项目入口核验 bundle 后运行；本地仍检查本机私有项目 binding。缺 bundle、摘要不符、版本不兼容不会回落到用户安装。PATH 缺少 runner 时，由原生进程报告启动失败。

云端 HOME 必须位于共享仓库之外。state、stats、codec cache 和 probe 收据位于 HOME 下 `.style/cloud-state/<规范仓库路径摘要>/`，不同会话保留独立 session 状态；不写仓库或本机私有安装。native 命令仍由 Claude 原生执行，其 cwd、依赖和错误行为不经过 STYLE engine；完整性清单不能替代原生工作区信任。

bundle 是显式快照。更改或移除包后，旧 bundle 不自动更新；doctor 报告陈旧，重新 export 才交付新版本。云端会话不自动刷新已加载的规则。最后一包卸载会在同一事务移除三个 bundle 文件和私有 export receipt；带 export 的项目可先运行 `style uninstall --all --project . --json` 查看删除预览，再加 `--yes`。修改过的导出文件不会静默删除。

离线复验：

```sh
python3 scripts/check_m3_cloud.py --binary /path/to/style --report /tmp/style-cloud-report.json
```

该脚本使用固定版本 stub、全新隔离 clone 和真实配置中的 hook 命令，覆盖普通规则、portable native、缺依赖、缺 runner、篡改拒绝与卸载；不请求模型，不把静态文件存在当作真实投递。
