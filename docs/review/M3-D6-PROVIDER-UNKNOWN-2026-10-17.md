# M3-D6 provider 与 unknown 定稿

日期：2026-10-17。状态：`[x]` 设计定稿。依赖 T41 已完成；按用户全部评审批准的授权实施 T42/A11，再实施 T43/A12。设计通过不代表功能或真实投递通过。

## 1. provider 的含义与接口

`provider` 表示从原生启动配置或 hook 进程环境观测到的模型服务选择请求。它不是 harness 名，不按模型别名、URL 或密钥推断，也不保证组织托管策略、网关上游、账单主体或会话中途的实际网络路由。`harness.name` 仍是注册表中的 CLI 标识；隐藏入口 `--provider=<harness>`、stats 与既有日志分组的 provider 字段保持原义。

新增 `reads: {provider: true}`、动态 `{{provider}}` 及 `when.provider` 单值/列表。服务 ID 按大小写精确匹配，允许 `[A-Za-z0-9][A-Za-z0-9_.-]{0,63}` 的自定义 ID；`unknown` 保留给缺值语义。拒绝 URL、控制字符及过长值，不回显被拒绝原文。未读取到服务时模板按原 default/空串规则处理。计划显示条件来源可解析但可能缺失，取消旧 provider-unimplemented/永不满足分支；条件和模板都要求 reads，包/规则/from 并集与现有布尔声明一致。

## 2. Codex 来源

只恢复 agenthooks allowlist 的 `-c model_provider=…`（最后一次覆盖生效）、profile 与配置忽略标记，不向 STYLE 暴露任意启动参数。明确启动覆盖优先；没有可确认的 Codex 祖先进程，或启动上下文不可重放时，返回缺值及固定原因码。

对已观测的 0.159.3 起文件式 profile：启动覆盖 → 选中的 `$CODEX_HOME/<profile>.config.toml` → 用户 config.toml → 文档定义的 openai 默认。**provider 与 effort 的层级不同**：官方配置参考明确项目层忽略 model_provider/model_providers，不能把 effort 的受信项目覆盖规则复制过来。旧/未知版本仅接受明确启动覆盖，其他配置路径返回缺值，避免猜测旧版本 provider 层级。忽略用户配置但无明确覆盖时返回缺值；不把不可重放状态当默认安装。

配置采用有上限的普通文件读取；缺文件可继续继承，坏 TOML、错误字段类型、非法 ID、坏 profile 名、legacy selector/选中 table 或无法读取均返回缺值；不吞掉高层错误后回退较低层。默认值只在可恢复启动上下文且允许的配置层均已确认后使用。解析结果是本地配置请求，不声称恢复全部机器/组织托管策略。每个需要 provider 的会话首次解析一次；即便结果缺失也记录已解析，避免每个工具 hook 读取进程和配置。新会话重新解析；会话中途原生设置切换不在本轮保证范围。

## 3. Claude 来源

读取 hook 进程环境白名单：单独有效的 `CLAUDE_CODE_USE_BEDROCK=1`、`CLAUDE_CODE_USE_VERTEX=1`、`CLAUDE_CODE_USE_FOUNDRY=1` 分别得到 bedrock、vertex、foundry；0/false/空为关闭，1/true 为开启，其他值返回缺值。无覆盖时为原生 anthropic 默认选择请求。多个服务开关同时开启返回缺值，不猜优先级。

`CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST`（及旧 host-managed 标记）、自定义端点变量的非空存在、尚未支持的 Mantle/Anthropic AWS 路线标记均返回缺值。只检查端点变量是否非空，不解析或保存 URL；不读取任何 key/token。环境事实每个事件重新评估，缺值明确清除旧服务，不借用旧会话已知值。选择开关不是实际请求或云端投递证明。

## 4. 状态、重放和兼容

只有 index 需要 provider 才开启额外来源解析及可选 session 字段 model_provider/provider_resolved/provider_source；纯旧规则的热路径和 state JSON 保持。来源使用固定码，trace 只报告可用性及来源，不含配置、环境原文或 URL。explain 新增绑定 payload SHA256 的可选 model_provider 事实；缺该事实按缺值重放，绝不读取本机 provider 配置/环境。snapshot 保留可选安全字段，真实运行和 explain 使用同一匹配路径。

provider 条件/变量整体采用 index format 4；unknown 在独立 T43 交付时采用 **format 5**，保护已经支持 format 4 的 T42 runner。新 runner 继续读合法 1/2/3/4，旧格式不能承载新语义；所有入口严格拒绝旧格式的 provider/unknown 规则。doctor 提示升级，refresh/event/provider/unknown 同时使用取最高格式。

2026-10-17 [实际兼容探针](M3-D6-UNKNOWN-COMPAT-2026-10-17.json)证明 T42 二进制接受 format 4 的 model: unknown，但只匹配模型字面量 unknown，且旧 sync 能把未标记的新 index 重新编译降为 format 1。仅 bump index 不足以保护管理冷路径。因此新增锁的可选 **min_index_format: 5**：有有效 unknown 产物才写入，用户普通锁至少采用既有严格 format 2，项目/Claude 插件仍 format 3；旧程序在任何管理写入前因未知锁字段拒绝。没有 unknown 的锁不加字段；移除最后一个 unknown 后清除标记（可保留合法 format 2）。sync/update/install 按本次完整编译重新计算，frozen 只允许锁标记与批准编译一致，不自动升级锁。此前“不改锁”的提议据实际降级风险修订；owner 与原生 codec/event/renderer 职责不变。

## 5. unknown 的唯一含义

显式选择器 `unknown` 仅支持 when.model、when.provider、when.thinking_effort、when.harness.permission_mode；匹配规范化值缺失或空串。模型名和 permission_mode 的其他非空值、有效自定义 provider 都是已知；无法映射的 effort 原值保留在 thinking_effort_raw，但其空 canonical tier 可匹配 unknown。unknown 不自动替代模板空串，也不是可输出的 provider 值。

同字段列表可混合已知值和 unknown（OR）；跨字段仍 AND。模型 glob 的普通通配符不匹配缺值；unknown 不参与 glob。effort 的 >=/<= 不含 unknown/open，>=unknown/<=unknown 拒绝。harness.name 继续注册表成员校验；agent、approx、conditions 不新增 unknown/否定运算，conditions 中字符串 unknown 仍是普通字面量，缺字段的 not_contains 仍不满足。

包含 unknown 的字段不是 plan 的硬性可用数据依赖，避免因 adapter 当前没有数据而移除特意匹配缺值的规则；needs 仍完整记录。其他字段与无新语法旧规则保持“缺值不满足”。保证模板有值的 when 推断不得把含 unknown 的条件视为保证。

## 6. 官方依据与边界

2026-10-17 读取官方 [Codex 配置参考](https://developers.openai.com/codex/config-reference)（跳转 learn.chatgpt.com）：model_provider 为 provider ID，默认 openai；profile 为相邻文件；项目配置不能覆盖 provider。与仓库 D3 的实际 0.159.3 文件 profile 证据相符。旧/未知版本不外推来源顺序；不更改已有 effort 解析。

同日读取官方 [Claude 环境变量](https://code.claude.com/docs/en/env-vars)与[网关说明](https://code.claude.com/docs/en/llm-gateway)：三个服务开关、托管标记及自定义端点是独立事实，网关可在客户端不变时切换上游。使用 openai-docs 技能查询 OpenAI 官方文档；本次未登录、调用模型或计数服务，也未读取密钥。

## 7. 待执行验收

| 编号 | 状态 | 断言 |
|---|---|---|
| A11-01 | [x] | reads/模板/条件完整校验、大小写/自定义 ID/非法值、plan 来源声明 |
| A11-02 | [x] | Codex 启动/profile/user/default、未知版本/祖先/忽略配置、坏/超限/类型/缺文件与项目层排除 |
| A11-03 | [x] | Claude 三服务/default、冲突/非法/托管/端点/其他路线与缺值清除，不读取凭据 |
| A11-04 | [x] | 按需解析、会话缓存及新会话、state/snapshot、两家实际 codec 和来源 trace |
| A11-05 | [x] | 离线 explain facts 绑定、无环境/配置泄漏、缺事实不偷读、普通输出一致 |
| A11-06 | [x] | index 4/schema/严格旧格式门禁、真旧 runner、生命周期/refresh/event/旧 golden 与 CLI |
| A12-01 | [x] | 四字段缺失/空/非空、未知 effort 原值、列表 OR/跨字段 AND |
| A12-02 | [x] | 模型 glob/effort 比较、其他字段和否定条件不扩展 |
| A12-03 | [x] | plan 不错误移除缺值规则、reads 与 var-no-default、compile/engine/trace/explain 一致 |
| A12-04 | [x] | format 5/锁标记和旧缺值兼容、fixture/golden 与 built CLI；真实 B3/C5 后续独立 |

T42/A11 六组均通过[验收报告](M3-A11-2026-10-17.json)，具体接口见[使用说明](../STYLE-PROVIDER.md)。A12 四组也通过[离线验收](M3-A12-2026-10-17.json)，见[缺值说明](../STYLE-UNKNOWN.md)；真实投递未据此关闭。
