# Claude 云端功能已移除

2026-10-18，用户决定删除 Claude 托管云端功能，并确认没有安装过，无需保留迁移或清理逻辑。

当前版本已移除 `style export`、云端 bundle 加载、云端专用状态以及相关 doctor/卸载分支。项目 hook 在 `CLAUDE_CODE_REMOTE=true` 时报告不支持，并沿既有原生失败策略返回。

Claude Code 的本地[用户路线](STYLE-TUTORIAL.md)、[项目路线](STYLE-PROJECTS.md)和[插件路线](STYLE-CLAUDE-PLUGIN.md)继续支持。M3-C3 已取消，未计为验收通过；历史设计和验收报告保留原样，当前范围见[M3 任务清单](STYLE-M3-TASKS.md#2026-10-18-删除-claude-云端功能)。
