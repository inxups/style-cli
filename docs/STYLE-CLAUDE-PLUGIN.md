# Claude 本地 marketplace 插件

用户安装默认仍为 `claude-user`。显式选择插件载体：

```sh
style install ./my-style --harness claude-code --route claude-code=claude-plugin --yes --json
```

要求已知 Claude Code >= 2.1.301 和 POSIX shell。STYLE 生成文件后报告 `registration-required`，退出 1；这表示载体已生成、仍需要原生登记。JSON 的 `changed` 与 `registration` 分开记录。执行报告中给出的精确命令：添加本地 marketplace，再以 user scope 安装 `style-runtime@style-<规范 STYLE_HOME 的完整 SHA256>`，随后重启 Claude，用原生 UI/probe 检查加载与投递。报告的命令固定本次 `CLAUDE_CONFIG_DIR`，路径已做 shell 引用。

STYLE 不自动执行 `claude plugin`，不写 `known_marketplaces.json` 或 `installed_plugins.json`。登记状态来自这两份原生元数据及用户 `settings.json`：未登记、enabled、disabled 或 unknown 分别报告。仅支持本地目录 marketplace、相对插件来源和 user scope；远程 marketplace、project scope 登记、临时 `--plugin-dir` 及云端插件不属于本路线。原生状态无法核实时，管理变更停止；`enabled` 元数据仍不证明实际投递或工作区信任。

载体位于 `$STYLE_HOME/adapters/claude/marketplace/`：

- `.claude-plugin/marketplace.json` 指向相对 `./plugins/style-runtime`。
- `plugins/style-runtime/.claude-plugin/plugin.json` 使用固定载体版本 `0.1.0`。
- `plugins/style-runtime/hooks/hooks.json` 保存普通与 native 定义。
- `plugins/style-runtime/scripts/style` 绑定安装时的程序、STYLE_HOME、Claude 配置与版本；hooks 通过 `${CLAUDE_PLUGIN_ROOT}` 定位它，没有顶层 bin。

agenthooks 负责这些原生文件的渲染、owner 合并和文件事务；app 负责锁、批准、来源及路线选择。包版本和批准摘要独立于载体版本；本地相对来源原地加载，更新在下一会话检查。native 命令保留批准原文，更新仍需相应批准与披露。

静态规则继续由 STYLE 写入用户 `.claude/rules/style-<包名>.md`。禁用插件只停止插件 hook；静态 rules 到 STYLE 卸载才移除。doctor 会分别报告载体文件和原生登记状态，未启用时给出 `registration-required`。probe 还核对真实进程来源与 `CLAUDE_PLUGIN_ROOT`；直接手工调用不能据此算真实成功。

同一用户安装只能选择一个 STYLE hook 载体。切换 user/plugin 会在同一文件事务移除旧 hook、生成新 hook，并保留外部 handler、静态 rules 和原生数据库。移除或切换已启用的插件前，先运行报告中的原生 disable/uninstall 命令并核对状态，再执行 STYLE 管理命令。无法证明停用时不删除源目录。部分包卸载重编译共用载体；最后一包删除 STYLE 文件和静态效果。原生 CLI 的空键、数据库文件与缓存依原生生命周期保留并单列，不保证这些外部文件逐字节回到最早状态。

插件用户安装使用 lock format 3、`project` 为空，仅在锁定 `claude-plugin` 时允许；项目 lock 3 仍要求完整 project binding。真正旧 runner 在管理写入前拒绝这一新表示。切回普通 user 路线后恢复原 lock 1/2 规则；index 和普通输出语义保持。

离线复验：

```sh
python3 scripts/check_m3_claude_plugin.py --binary /path/to/style --old-binary /path/to/pre-plugin-style --report /tmp/style-plugin-report.json
```

脚本使用原生元数据 fixture 和配置中的真实启动器命令；不执行登记、不请求模型，不替代 C4 真实 marketplace 验收。
