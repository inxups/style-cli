# M3-D0 注册表校验边界评审稿

日期：2026-10-17。状态：`[~]`，方案及调用链调查已完成，等待用户评审；尚未实现，不关闭 M3-D0、M3-T00、M3-A0 或 M3-RV-01。

调查起点是 clean `323acf81ad63b417ea27b29064f9473efbb7a12b`；其生产源码与建档提交 `7586cca` 相同。[M3-BASE-01](M3-BASE-01-2026-10-17.json) 已新执行双 module fresh test/vet、59 项采集器、40 项 smoke、16 组旧版迁移和 12 组回退。本文依据仓库源码定边界，不引入外部 harness 能力或新安装路线。

## 1. 现状与问题

| 位置 | 当前事实 | 本次处理 |
|---|---|---|
| [条件校验](../../core/spec/validate_conditions.go) | `validHarnessNames` 固定两家，检查 `when.harness.name` | 用注入的名称集合替代 |
| [包级校验](../../core/spec/validate_package.go) | `requires.harness`、`recommends` 各有两家名称判断 | 与条件校验使用同一集合 |
| [Registry](../../internal/adapter/registry.go) | `Providers()` 列出全部注册项，`NativeCatalog()` 只列有 native 能力的项 | 从同一注册表生成完整校验目录 |
| [Service](../../internal/app/service.go) | `validateSpec` 拼接 `spec.Validate` 和 `spec.ValidateNative` | 统一调用带目录的完整入口 |
| [validate](../../internal/app/validate.go)、[plan](../../internal/app/plan.go) | 前者预算遍历、后者 `auto` 检测固定两家；名称集合与检测结果是两种概念 | 遍历注册项，保留两家既有显示顺序及 CLI 别名 |
| [plan](../../internal/app/plan.go)、[安装编译](../../internal/app/install.go)、[去重后报告](../../internal/app/install_dedup.go) | 三处预算路径直接调用 `spec.Validate` | 传递完整入口产出的源诊断 |
| [tokens](../../core/tokens/analyze.go) | 未收到源诊断时自行调用 `spec.Validate`；只提取共享预算 lint | 保留无 adapter 依赖的结构/lint 回退，不能用它批准入包 |
| [原生指纹](../../internal/app/native.go) | `loadNativeSource` 只解析、合并以比较声明或重建批准基线 | 保留其用途；候选包在入包/重编译入口完整校验 |
| [core 依赖检查](../../internal/adapter/boundary_test.go) | 已禁止 agenthooks 根路径/子路径及 adapter 根路径，但未覆盖 adapter 子路径 | 补齐整个 adapter 路径前缀的约束 |

可重复调查命令（仓库根目录；输出须以固定起点复核）：

```sh
git rev-parse HEAD
rg -n 'validHarnessNames|unknown harness|unknown recommends harness' core/spec/validate_conditions.go core/spec/validate_package.go
rg -n 'spec.Validate\(|spec.ValidateNative\(' core internal --glob '*.go' --glob '!**/*_test.go'
rg -n '"codex"|"claude-code"' core --glob '*.go' --glob '!**/*_test.go'
```

最后一条还发现 [engine/process.go](../../core/engine/process.go) 有两处 Claude prompt suppression 的名称判断。这是既有原生输出安全边界，不参与名称合法性集合；本次 T00 聚焦 M3-RV-01 的三个校验点和相关入口，不删除这两个判断，也不声称所有 core 特例已清零。该额外发现保留到 M3-D8 的输出/失败策略边界设计中评估；任何能力化改动都需对应 engine/adapter/runtime 白名单回归。

## 2. 推荐接口

新增的 core 数据结构与入口如下；此处是拟议接口，不代表代码已存在：

```go
// core/spec：只有普通名称和既有 native 能力端口，不导入 adapter 或 agenthooks。
type ValidationCatalog struct {
    Harnesses []string
    Native    NativeCatalog
}

// 保留现有调用签名，执行结构、类型、版本表达式、reads、模板和 lint 校验。
func Validate(pkg *Package) []Diagnostic

// 完整入口：结构 + harness 名称成员关系 + native 事件/matcher 能力。
func ValidateWithCatalog(pkg *Package, catalog ValidationCatalog) []Diagnostic

// internal/adapter：从 Registry 的同一份定义生成快照。
func (r *Registry) ValidationCatalog() spec.ValidationCatalog
```

`Registry.ValidationCatalog()` 的 `Harnesses` 取全部注册 ID，字典序排列；`Native` 取相同注册项中 `Definition.Native != nil` 的映射。返回新的 slice/map 容器，Service 构造时保存自己的快照；既有 NativeHarness 实例仍按不可变事实使用，不新增进程全局注册或可变默认目录。

保留 `Registry.NativeCatalog()`、`spec.NativeCatalog` 和 `spec.ValidateNative()` 的现有签名与底层用途。app 的入包门禁只用新完整入口，不再手工拼接或漏掉普通名称检查。

## 3. 名称和 native 的关系

1. 合法名称取决于这个 Service 注册了什么。既不是 agenthooks 所有 Provider 常量，也不是本机检测到了什么，更不是本次 `--harness` 选择了什么。
2. 包内名称按注册的 canonical ID 精确匹配，不加别名、不自动改大小写、不 trim 后放行。CLI 的既有 `claude` 别名仍在命令参数层处理，不成为包格式新取值。
3. 三处普通名称检查均使用 `Harnesses`：`when.harness.name` 的字符串/列表元素、`requires.harness` 的键、`recommends` 的键。既有类型、版本表达式和 reads 要求继续检查；recommends 内部仍是建议映射，不扩大为自动修改设置或新 schema。
4. 注册了普通 harness，未声明 Native，不妨碍该名称出现在前三处。若用作 native 映射则报 `native-field`，消息为 `harness %q does not support native hooks`；不能从支持普通规则推导支持 native。
5. native 映射先判断名称是否在 `Harnesses` 中，再检查对应 Native 能力。`Native` 中额外的键不能扩大合法名称集合；只取与 `Harnesses` 的交集。事件和 matcher 仍由该 adapter 的 NativeHarness 回答。
6. 完整入口的零值/空目录表示没有注册任何 harness。无 harness 引用的普通包仍能做结构校验；任何显式名称或 native 映射都拒绝。它不回落到两家内置名称，也不等同于“跳过检查”。
7. 包可引用已注册但当前未安装或未选中的 harness；安装版本过滤、plan 降级及 native 路线可写性仍由后续步骤处理。校验过程不为查询合法名称运行 detector 或 native 命令。

例如仅注册 `fixture-harness` 时，前三处均接受它并拒绝两家未注册名称；即使另传一个包含 `codex` 的 Native 映射，完整入口也不会接受未列在 Harnesses 的 `codex`。

## 4. 诊断和调用兼容

完整入口在现有 validator 遍历中检查成员关系，维持包字段、规则来源和 native 追加阶段的顺序。未知名称继续使用 `invalid-schema` 和现有消息：`unknown harness name %q`、`unknown harness %q`、`unknown recommends harness %q`。未知 requires 键继续在该处报错后跳过该键的版本检查；recommends 的未知键及值不是 mapping 时仍保留两条独立诊断。

native 未注册名称保留 `native-field` 及既有消息格式，registered 列表按完整目录字典序展示；两家默认目录的列表、事件/matcher 错误及来源路径应与旧结果一致。已注册但无 Native 的专用消息仅影响新增 fixture 场景。CLI JSON 字段、严重度、退出码和诊断排序不变。

**需要明确批准的兼容选择**：`spec.Validate(pkg)` 保持可编译的 Go 调用签名，但改为结构/lint 校验，不再在没有目录时猜测合法 harness。直接调用者若要判断完整有效性，必须迁移到 `ValidateWithCatalog`；仅继续调用 `Validate` 或 `Validate + ValidateNative` 都不代表完整名称验收。仓库内的普通未知名称测试改为显式传入 fixture 目录，另测试不带目录的结构行为。CLI/app 的完整有效性继续严格拒绝未注册名称。

这样不把默认两家名单移到另一处 core 常量，也不通过未传目录默认放行来构成 app 入包门禁。解析接口 `Load`、`LoadSnapshot` 保持只负责读取和构建 Package；它们不自行寻找注册表。

## 5. app 数据流与职责

| 场景 | 采用的校验与诊断来源 |
|---|---|
| `validate` / `plan` | Load 后由 Service 完整校验；plan 与 token 分析使用同一源诊断。validate 的预算遍历来自注册项；plan 的 auto 检测同样遍历注册项 |
| 新装、同包重装、`--link`、单文件包 | Install 在写入/编译候选包前调用同一完整入口；不以版本检测或 lock 命中替代它 |
| `sync` / `trust` | 实际待编译的当前或已批准 snapshot 经 `parseInstalledSnapshot` 完整校验，记录其源诊断 |
| 单包卸载后的重编译 | 只完整校验保留包；移除包仍按既有 lock/owner 清理，不要求重新接纳它 |
| 去重前预算与去重后有效报告 | 使用 installPackage 保存的完整源诊断，避免自由函数重新选择默认名单；诊断为空时 tokens 的结构回退不改变名称集合 |
| `list` / `stats` / 指纹与批准基线检查 | 维持既有只读或声明比较职责，不因新增完整入口把旧安装升级成写操作；候选写入仍受前述门禁约束 |

注册表名称清单用字典序保证稳定，CLI 目标报告仍保持 Codex、Claude、其余 ID 字典序的既有顺序。新增假 harness 只用于测试；本轮不注册新的真实 harness。事件 payload/响应 codec、原生配置渲染、owner 合并和卸载恢复继续归 agenthooks；本任务无需改 agenthooks 协议、锁/index/state 格式、runtime 行为或 native 授权。

## 6. M3-A0 验收细化

以下均是 T00 的待执行断言，当前不算通过：

| 子项 | 必须证明的行为 |
|---|---|
| A0-01 | 两家、仅假 harness、移除某家、空目录四种集合分别用于三处普通名称检查；字符串/列表、目录包/单文件包和 from 覆盖均取同一集合 |
| A0-02 | Native 能力是普通名称集合的子集；普通已注册但无 Native 可用于三处普通字段，native 则拒绝；额外 Native 键不能扩大名称集合 |
| A0-03 | 错类型、非法版本、缺 reads、未知名、未知名叠加坏值、nil Package 保留既有 code/path/severity、消息及稳定顺序；两家 native 错误保持兼容 |
| A0-04 | `Validate` 是结构检查，`ValidateWithCatalog` 是完整检查；零目录不回落内置两家，不从包自己推导名单；Load/LoadSnapshot 无注册表依赖 |
| A0-05 | 两个不同 Service/Registry 的快照互不影响；修改返回的容器不能污染 Registry 或已构造 Service；校验不调用 detector/native 命令 |
| A0-06 | validate/plan（显式与 auto）纳入注册的 fake；全部入包/重编译路径拒绝同一未注册名称，在各命令已有诊断封装内保留相同错误事实 |
| A0-07 | plan、安装前后去重及 tokens 空源诊断路径不恢复旧名单；fake 预算 lint 实际执行，两家原有预算及 golden 不变 |
| A0-08 | 无效候选在 install/sync/trust/保留包重编译时不改变 lock/index/state/外部配置的 bytes/mode/mtime；更换目录不破坏既有 trust 与卸载恢复 |
| A0-09 | core 不导入整个 adapter/agenthooks 路径前缀；三个校验点没有内置名称 allowlist；不以搜索整个 core 零名称替代行为验证 |
| A0-10 | 双 module fresh test/vet、既有 conformance/stdout golden、40 项 smoke、16 组旧迁移及 12 组回退通过；新 case 数和所用二进制摘要完整记录 |

Go 命令遵循 [M3 测试环境约定](../STYLE-M3-TESTS.md#测试环境约定)，清理宿主变量并使用临时三缓存。T00 对目录快照并发行为运行相关 core/spec、app、adapter 的 race 回归；本轮只写设计，M3-BASE-01 已通过的 Go 检查不冒充 A0 的新结果。

## 7. 评审与完成条件

用户确认本稿的完整目录接口、`Validate` 的结构校验兼容选择和上述实现范围后，记录批准并关闭 D0；随后执行 T00 和 M3-A0。只有 A0 的真实回归通过才能关闭 M3-RV-01、开始 D1。当前仅完成调查与文档检查，设计状态保持 `[~]`，实现/测试状态保持 `[ ]`。
