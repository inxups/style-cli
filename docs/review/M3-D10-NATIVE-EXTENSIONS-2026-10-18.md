# M3-D10 native 扩展评估定稿

日期：2026-10-18；基线 `5084a06a10295f93257954f93ebecbef52a7f017`。依赖 T50/A15；用户“所有评审全部通过”授权下定稿，随后落实 T60/A16。

## 结论与范围

M3 保留 native 的 `event`、`matcher`、`command`、`timeout` 四字段，不开放 async/onFailure/非 command 扩展。T60 明确拒绝并解释这些声明，不能忽略字段后降级安装。这里关闭的是逐项设计评估和结论落地；不宣称 STYLE 已提供原生全部能力。既有安装级 fail-closed 和普通规则 fail: closed 保持各自职责。

| 评估项 | 原生依据/实测 | M3 结论与原因 |
|---|---|---|
| `async` / `asyncRewake` | Claude 官方文档规定异步不能拦截、结果后续回合投递，async 不执行 timeout；2.1.301 的 2 秒任务在 timeout:1 下完成，初次请求先于结束且不含结果 | 不支持。四字段 timeout/当前事件输出不能表示后台任务生命期、会话结束取消和后续投递归属；不把此配置假装成受管理的同步 hook |
| `onFailure` | 官方要求 >=2.1.295，按事件有不同作用，对后台及若干结束事件无效；2.1.301 提示 hook exit7：unset/continue 各有一次模型请求，block 为零次，CLI 仍退出0 | 不支持 native 映射。现有 native 规划未承诺这一套版本/事件策略；不把安装级 STYLE runner 的保护套到作者命令上。明确拒绝而非忽略或承诺与规则故障保护等价 |
| `type: http` | 实测收到一次 UserPromptSubmit POST 且上下文进入本地 stub 请求 | 不支持。当前 agenthooks owner/恢复机制基于 command no-op 标记；URL/header/body/允许展开的环境变量需要新的归属与秘密披露策略，不能塞到 command 字段中 |
| `type: prompt/agent` | 官方说明会分别调用模型或带工具的验证 agent，事件集不同；agent 为实验能力。本轮未实测这两类 | 不支持。会新增模型/工具权限和费用，当前 native 的 shell 命令披露不能覆盖这些授权；不把文档结论记为模型调用成功 |
| `type: mcp_tool` | 当前官方文档列为独立类型、依赖服务和工具配置；本轮未实测 | 不支持。服务绑定、工具权限及归属没有四字段表达，不猜测调用结果 |
| 显式 `type: command`、`args` / `shell` 等 | 当前 STYLE 四字段由 renderer 生成固定 command 类型和归属前缀 | 本轮仍拒绝额外字段，保持旧包可重复渲染。作者使用既有单行 command；Windows shell 边界由 D11 单独定稿 |

“不支持”是本轮完整评估的产品结论，不是待验字段。后续若扩展，须另行设计后台生命周期、版本能力、类型化授权和不依赖 command 的 owner/卸载机制；本轮不创建空的成功状态。

## 接口、数据流与失败

形状校验保持 `native-field` 诊断码与退出1；目录、单文件、from/整体映射覆盖走同一校验。已知扩展字段增加固定安全原因，原值不回显；普通未知字段的旧消息保持。validate/plan/install/update/sync/trust 必须在任何执行或管理写入前拒绝不支持字段；既有 update 候选分类和 sync 先要求 trust 的退出3保持，后续 trust 仍须通过形状校验，不能以 fallback:skip 绕过形状校验。冻结重建只承认已经批准的四字段源。

四字段校验、计划状态/JSON、命令披露、NativeRule 指纹、timeout、owner 标记、锁/index/state 及真实协议不变。native 不经过 STYLE engine、不进 stats、不估算输出；与普通规则同时安装仍独立执行/授权，不去重作者命令。已有外部非 STYLE hooks，包括扩展类型，必须在合并和卸载中原样保留，不因本产品不开放而删除。

agenthooks 继续独占协议/config renderer/owner/文件事务；本任务不往 core 添加 harness 名判断，也不扩展 renderer 透传未知字段。

## 证据与验收

[原生报告](M3-D10-NATIVE-PROBE-2026-10-18.json)由[隔离探针](../../testdata/manual/probe_m3_native_extensions.py)实际执行；新 HOME/CLAUDE_CONFIG_DIR、本地 Messages/HTTP server 和固定占位 key，真实 CLI 2.1.301。六场景均通过，没有远端模型推理、订阅调用或官方计数。原始请求和日志留在私有 scratch，报告只记录计数、布尔值和摘要；CLI 退出0不替代故障是否阻断的请求证据。

[来源](M3-D10-SOURCES-2026-10-18.json)固定官方 hooks.md 和探针/命令摘要。设计证据不能替代 B4/C6 后续实际 STYLE native 安装验收。

| 编号 | 状态 | T60/A16 断言 |
|---|---|---|
| A16-01 | [x] | 全部扩展字段/类型、错类型、目录/单文件/from 覆盖与安全诊断；未知字段旧消息和命令四字段兼容 |
| A16-02 | [x] | validate/plan/install/update/sync/trust 拒绝且保全文件，不执行作者命令；frozen 使用批准四字段 |
| A16-03 | [x] | 四字段新旧 CLI 计划/配置逐字节对照、owner/授权/卸载与外部扩展 hooks 保全、stats/token 排除 |
| A16-04 | [x] | 相关 fresh test/vet/race、构建/CLI、文档/归档与追加日志检查 |

四组均已通过 [A16 验收](M3-A16-2026-10-18.json)；使用与管理 gate 说明见[指南](../STYLE-NATIVE.md)。B4/C6 真实模型投递仍待收尾执行。
