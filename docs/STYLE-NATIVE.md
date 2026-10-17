# Native hook 边界

M3 native 仍支持 `event`、`matcher`、`command`、`timeout` 四字段。作者命令直接交给 harness 执行；STYLE 负责安装披露、确认、归属标记和卸载，不经普通规则引擎，也不计入 stats 或 token 估算。

```yaml
rules:
  - id: native-notify
    target: native
    claude-code:
      event: PreToolUse
      matcher: '^Bash$'
      command: 'printf hook-ready'
      timeout: 10
```

事件和 matcher 按对应 adapter 校验。command 是单行 shell 文本，约束见[规则 §3.10](spec/STYLE-spec-rules.md#310-target-native)。安装和 native 变更需要确认；多条作者命令不会跨包去重。Windows 支持范围由 M3-D11 单独规定。

[D10 评估](review/M3-D10-NATIVE-EXTENSIONS-2026-10-18.md)后，以下字段仍明确拒绝：

| 声明 | 原因 |
|---|---|
| `async`、`asyncRewake` | 后台工作及后续回合投递需要独立生命周期；不能承诺当前事件输出或同步 timeout |
| `onFailure` | 原生策略有版本和事件边界；不能直接套用 STYLE runtime 的安装级或规则级故障保护 |
| `type`、`url`、`headers`、`prompt`、`model`、`server`、`tool`、`input` 等 | HTTP/MCP/模型验证需要新的类型化归属、秘密披露及授权 |
| `args`、`shell`、显式 `type: command` | 超出现有四字段契约，不能默默改变 shell/exec 形式 |

validate/plan 和新安装的形状校验给出 `native-field` 与具体原因，不输出字段值。update 保持既有安全的候选错误分类；sync 可能先以退出3要求 trust，此时仍零写入，随后 trust 的形状校验拒绝。`fallback: skip` 不能绕过这些校验。它们不会被忽略后当成功安装。

已经由其他工具安装的 HTTP、MCP、prompt、agent 或异步 command hook 属于外部配置，STYLE 的合并/卸载会保留。产品不开放某类声明，不表示会删除外部同类 hook。

[原生探针报告](review/M3-D10-NATIVE-PROBE-2026-10-18.json)只证明固定版本真实 CLI 到本地 stub 的协议行为；[A16 验收](review/M3-A16-2026-10-18.json)核对拒绝、四字段兼容和外部配置保全。真实模型和各路线投递由 B4/C6 单独记录。
