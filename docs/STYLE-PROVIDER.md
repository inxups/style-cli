# 模型服务 provider

`provider` 是从原生启动配置或 hook 环境读到的服务选择请求。`harness.name` 是 `codex` 或 `claude-code` 等 CLI 名；服务可以是 `openai`、`anthropic`、`bedrock`、`vertex`、`foundry` 或明确配置的自定义 ID。不会根据模型别名、URL 或密钥猜测服务，也不保证网关实际使用的上游、账单主体或组织托管策略。

```yaml
spec: style/v0.3
name: service-guidance
version: 1.0.0
description: 按已知服务请求选择提示
reads: {provider: true}
rules:
  - id: hosted
    target: system
    when: {provider: [anthropic, bedrock]}
    text: '当前服务配置请求为 {{provider}}。'
```

条件支持单值或列表，按大小写精确匹配。ID 最多 64 个 ASCII 字符，首位为字母/数字，其余还可用 `_`、`.`、`-`。URL 不合法。模板可写 `{{provider | default: "服务未知"}}`；无 default 且没有已知服务条件保证时，会警告可能为空。条件和模板都须声明 reads.provider；包、规则和 from 声明取并集。

Codex 的明确 `-c model_provider=…` 优先，最后一次覆盖生效。可确认的 0.159.3 起文件式配置按选中 profile 文件、用户 config.toml、文档默认 openai 读取；provider 不采用项目配置层，和 effort 的优先级不同。无法恢复祖先启动上下文、版本未知/较旧（且无明确覆盖）、配置被忽略、配置损坏/无法读取或 ID 非法时，结果为空。每会话首次需要时解析并缓存，startup 重新解析；中途修改配置不会保证即时反映。

Claude 每个 hook 事件读取服务环境白名单，分别识别三个服务选择开关；无覆盖时表示 anthropic 默认请求。冲突、非法开关、托管标记、自定义端点或尚未支持的 AWS/Mantle 路线使结果为空，明确清除旧服务。只检查端点是否非空，不解析/保存地址，不读取任何凭据。

来源与可用性进入诊断，值不写入 trace；session 仅保存受限 ID 和固定来源码。`style explain` 只接受绑定 payload SHA256 的 `model_provider` 补充事实，省略或空串时按缺值重放，不读取本机环境/配置。即使提供快照，也由这个显式事实决定重放 provider。格式示例见 [explain](STYLE-EXPLAIN.md#补充事实)。

使用这些条件或模板会生成 index 4，doctor 提醒升级所有配置的 STYLE runner。旧 runner 拒绝整份新产物；新 runner 仍支持合法 index 1/2/3，但这些旧格式不再接受 provider 条件，须从原包重编译。没有新语法的旧规则、预算及热路径保持；provider 的估算包括标识符最坏标点宽度和显式 default，运行时仍按实际完整输出执行上限。

T42 的离线证据见 [A11 报告](review/M3-A11-2026-10-17.json)与 [D6 契约](review/M3-D6-PROVIDER-UNKNOWN-2026-10-17.md)。四字段 `unknown` 选择器已由 T43 实施，组合使用会升级为 index 5 并增加严格锁标记，见[说明](STYLE-UNKNOWN.md)；真实 B3/C5 仍独立验收。隐藏 hook 入口、旧 stats/log 中的 provider 字段仍表示 harness，保持兼容。
