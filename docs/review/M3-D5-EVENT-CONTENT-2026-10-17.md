# M3-D5 事件内容模板定稿

日期：2026-10-17。状态：`[x]` 设计定稿。依赖 T40 已完成；用户授权全部后续评审通过，按本契约实施 T41/A10。

## 1. 显式引用与数据范围

新增 `{{event.prompt}}`、`{{event.tool.name}}`、`{{event.tool.input}}`、`{{event.tool.input.<路径>}}`、`{{event.tool.added}}`、`{{event.tool.output}}`、`{{event.mcp.server}}`、`{{event.mcp.tool}}`、`{{event.skill.name}}`。保持旧的 `{{prompt}}` 等裸字段禁止。必须声明完整字段，例如 `reads: {event: [prompt, tool.input.command]}`；包/规则/from 的声明取并集，未声明为 reads-missing，未使用为 reads-unused。条件仍使用既有原始字段及原有语义，不因事件模板声明而改变。

字段按现有 conditionFieldAvailable 的事件/目标约束检查；不适用的字段报 event-var-unavailable，可能缺失的字段可用既有 default 修饰。服务来源事实沿既有 adapter DataSource；模板字段可用性报告去掉 event. 后查询实际来源，不伪造可用数据。所有引用为动态模板；不允许进入静态槽位。

## 2. 唯一渲染形式

每次引用先取当前评估点的原始 condition field，不从其他评估点或会话缓存借正文。未提供时取 default 或空串；显式空串不触发 default。每个值最多保留 512 个 Unicode code point（无效 UTF-8 替换为 U+FFFD），超长保留前缀并标记 truncated=true。输出：

```text
<style-untrusted-event field="prompt" truncated="false">
"JSON string containing escaped data"
</style-untrusted-event>
```

数据采用单行 JSON string，控制字符、换行、引号、反斜线、尖括号、&、反引号和 Unicode 格式控制符均转义，不能制造真实换行或闭合 XML/Markdown 围栏。字段名来自受限语法，不取事件值。default 也执行同一截断/转义/围栏路径；没有 raw/关闭围栏选项。围栏是数据边界提示，不保证模型抵抗所有提示注入。不要把密码或私有信息放入事件引用。

warn 含事件引用时须显式提供 model_text，且其中事件引用仍完整渲染；禁止隐式第一句截取破坏围栏。普通非事件 warn 保持既有 first-sentence 语义。block/inject 使用完整围栏输出。

## 3. 编译、计数与兼容

使用事件引用的 index 为 format 3，整体门禁；旧格式 1/2 不允许 event. 变量，新 runner 仍读旧格式，纯旧规则输出不变。format 3 可包含既有 refresh，不能使 refresh 失效。原始事件正文不写入 index，只保留变量名/default（default 本来是包内容）。新编译默认表达仍用 {var, default}；语义由 event. 名称及 format 3 确定。

离线估算按每次完整围栏、512 个 code point 的最坏转义占位和更长 default（仍截断）计算，不按重复间隔摊薄。最终运行时硬上限按转义后的完整输出计算；超限整条丢弃并释放去重/refresh claim，不截断最终围栏。跨包去重比较原动态结构和完整策略，不按本次碰巧相同的事件内容合并。缓存 state、stats 与日志只保存既有摘要/安全字段；explain 的显式模型 wire 本来含输出，属于用户请求的诊断正文，trace 不含原文。

agenthooks 原有 codec/事件/renderer 不改；在 core 从现有字段读取接口取值并围栏。需要编译/schema/估算及 engine 的小幅扩展，不新增网络或文件正文读取。

## 4. A10 验收矩阵

| 编号 | 断言 |
|---|---|
| A10-01 | 显式命名、声明并集/缺失/多余/错误类型、未知字段、旧裸字段继续拒绝 |
| A10-02 | 目标/事件可用性、缺失/default/显式空值、动态规划、adapter 来源查询 |
| A10-03 | prompt/input 子路径/output/added 和 metadata；两家实际 codec payload 对照 |
| A10-04 | 假闭合标签/代码围栏/嵌套标记、控制字符/bidi/Unicode/无效 UTF-8/超长，固定 golden |
| A10-05 | warn 显式 model_text、完整围栏、运行时硬上限及 claim 释放、重复正文/refresh 组合 |
| A10-06 | format 3/schema/严格解码/真旧程序拒绝，旧格式与普通 golden 不变 |
| A10-07 | plan/token 估算包含完整最坏围栏，stats/state/trace/log 不含原文，explain wire 按用户明确请求返回 |
| A10-08 | 安装/sync/frozen/cloud 导出生命周期，全量相关回归及 built CLI；真实 B3/C5 后续另验 |
