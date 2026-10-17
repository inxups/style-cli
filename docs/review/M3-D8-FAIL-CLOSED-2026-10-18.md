# M3-D8 规则级失败策略定稿

日期 2026-10-18；依赖 T44/A13。按用户全部评审通过授权直接进入 T45，设计不替代 A14/B3/C5。

## 接口与范围

普通规则及 defaults 可声明 `fail: open|closed`，省略为 open；native 规则不接受。closed 支持 inject/warn/block，但必须直接落在具备 deny 的 tool.pre、prompt.submitted 或 skill.explicit；system、after、其他事件和缺 deny 的 adapter 直接 install-error，不能通过 fallback 降成静态/换事件/丢失失败策略。when.harness.name 排除的 adapter 可正常 skipped。计划披露 fail 与故障范围。

这是编译规则声明的**事件范围故障保护**：一个原生事件会评估的任一点存在 closed 规则，整个该次已解码回调的 handler error/panic/内部 timeout 使用 agenthooks FailClosed。同事件的其他规则故障也阻断，故障发生在 state/enrichment 前不能可靠计算 when/selector，因此故障保护不按正常谓词缩小；显式 skill 在 Codex 共用 prompt 原生回调，保护范围也包括普通 prompt。正常无故障的 target/when 未命中、缺事实、空 inject/warn、dedup 和 cap-drop 仍正常放行，不将它们当异常。空 block reason 是实际错误，阻断给固定安全原因，不回显原始异常或正文。

## 边界与安装级开关

仅成功读取/验证的 index 可识别规则策略。index 缺失/损坏、原生 JSON decode 失败、STYLE 未启动、被宿主杀死/外部超时、权限/信任未允许 hook、注册失败均不能由该规则保护保证。Claude 原有安装级 --fail-closed 保持独立：PreToolUse onFailure:block 需要 >=2.1.295，内部处理故障退出 1，且有效 index 不存在时仍可兜底。规则级策略只输出原生 deny/block JSON，正常 codec 退出码，不自动修改全局 onFailure 或安装级锁开关；不扩大为进程崩溃保护。

Codex 和 Claude 采用各自现有 agenthooks CapDeny/typed decision。解析、超时/panic捕获、原生错误响应及配置渲染仍由 agenthooks 负责。adapter 提供原生事件到统一故障评估点的保守映射，不在 core 写 harness 名。Claude 普通 prompt 的 suppressOriginalPrompt 由 adapter 提供点能力；engine 仅消费能力，codec 继续拒绝不支持的协议字段。

官方 Claude hooks 文档于 2026-10-18 获取，固定源 https://code.claude.com/docs/en/hooks.md；exit 0 的合法 deny、事件 deny 边界、onFailure 的版本/失败语义与上述一致。远程读取正文 SHA256：5599b94537beb359bfe35dfa7538503194ea78b8d824fcaccba068915d6a77d7。此文档与源码契约不替代真实 harness 行为验证。

## 产物与兼容

只有 closed 编译为 Entry.fail:closed，使用 index 7；显式 open 与省略均不写字段，旧产物/默认行为不变。schema、严格 JSON 和 engine constructor 同时门禁，拒绝旧格式携带 fail、非法值/不支持点。严格锁 min_index_format:7 防止旧管理器重编译降级；移除最后 closed 后按其他特性恢复适用格式和锁标记。frozen 从批准源重建并核对最低格式，不升级锁。去重必须保留不同失败策略。

## A14 矩阵

| 编号 | 状态 | 断言 |
|---|---|---|
| A14-01 | [x] | 字段/defaults/原生禁止、三动作、事件/能力拒绝、fallback 不弱化、计划披露 |
| A14-02 | [x] | 两家正常不匹配/混合规则、状态/空 block/内部 timeout/panic 的安全 deny 与恢复 |
| A14-03 | [x] | index/decode/进程失败边界、安装开关独立、prompt suppression 能力及旧默认行为 |
| A14-04 | [x] | index7/严格锁/旧程序拒绝、六路线管理/frozen/移除/云端、旧 golden 和兼容回归 |

T45/A14 已通过[离线验收](M3-A14-2026-10-18.json)，使用方式见[说明](../STYLE-FAIL-CLOSED.md)。原生 B3/C5 单独执行。
