# Go 项目与 CLI 高维护性设计评估

日期：2026-10-13<br>
状态：**[x] 评估完成，作为后续实现基线**

本文评估当前 Go module、STYLE core、CLI 设计和 `agenthooks` 边界，并给出后续实现建议。目标是让 Go 包职责明确、命令可测试、运行时可预测、provider 事实不渗入通用规则语义。

## 1. 结论

保留一个 STYLE 根 module 和独立的 `agenthooks/` module。STYLE 继续直接在 `agenthooks/` 内维护 hook 协议相关实现；不另写 codec、事件解析或配置渲染副本。整体采用**模块化单体**：一个 `style` 二进制，内部按 domain、application、adapter、CLI 分层；不提前拆服务或引入动态 Go 插件。

```text
cmd/style                 进程入口和退出码
internal/cli              命令树、flags、提示、输出
internal/app              validate/plan/compile/install 等用例编排
core/spec                 STYLE 包解析、合并、校验和诊断
core/template             模板语法和渲染
core/when                 条件归一化、编译和匹配
core/plan                 能力需求、降级和报告
core/compile              稳定 index.json
core/tokens               预算和估算
core/engine               热路径规则评估
core/state                会话和去重状态
internal/adapter          Codex/Claude STYLE adapter 和注册表
internal/platform         文件、时钟、终端等系统端口
agenthooks/               独立 module：provider codec、事件、安装渲染和 quirks
```

依赖方向：`cli -> app -> core`；application 在组合根组装 adapter 和 platform 实现；`adapter -> core ports + agenthooks`。`core` 不依赖 CLI、具体 adapter、provider JSON 或操作系统路径实现。

## 2. 当前仓库评估

| 区域 | 现状 | 评估与建议 |
|---|---|---|
| Go modules | 根 module、嵌套的 `agenthooks` module、`go.work` | 分离 hook 与 CLI 合理；保持独立测试和发布边界。发布验证需检查无本地工作区时也能解析依赖。 |
| `core/spec` | loader、frontmatter、merge、validate 等已拆分 | 结构适当；按职责继续演进，不因文件行数单独重写。 |
| `core/template`、`core/when` | 独立 domain 包，无 provider 依赖 | 作为通用核心保留；导出 API 有文档、输入输出可复用。 |
| CLI 实现 | 还没有 `cmd/style` 或 application 层 | 先实现用例服务，再接命令树，避免业务和文件操作落入命令函数。 |
| Adapter | 设计基线的 `Adapter` 同时含检测、能力、安装、运行时职责 | 使用方依赖小接口；具体 provider adapter 可以组合实现多个接口。 |
| Hook 协议 | 由独立 `agenthooks/` module 承担 | 符合用户确定的边界；扩展 hook 行为时直接在该模块改。 |
| 设计文档 | CLI 设计位于被 `.gitignore` 忽略的 `STYLE_DOCS/` | 新 clone 看不到该基线；将有效设计迁入受版本控制的 `docs/`。 |
| README | 仍是占位内容 | CLI 有首个可运行命令后补充安装、使用、开发入口。 |

已完成的 T10–T14 与 R01–R08 不需要因此重写。T15 和后续 adapter/engine/CLI 工作应按本报告的依赖顺序推进。

## 3. Go module 与包边界

### 双 module

- 根 module 拥有 STYLE domain 和 `style` CLI；`agenthooks` module 拥有 provider 协议和 hook 运行能力。
- 根 module 的 `core/...` 不得 import `agenthooks/provider/...` 或具体 STYLE adapter；用依赖检查固定此边界。
- `go.work` 用于本地同时开发两个 module。发布路径还必须可在 `GOWORK=off` 下构建。
- 根 `go.mod` 的 `replace ... => ./agenthooks` 不应成为已发布 module 的长期依赖：嵌套 module 不属于根 module 的发布包内容，消费者拿到根 module 时本地 replace 路径可能不存在。发布前改用可解析的 agenthooks 版本；本地工作区负责覆盖开发版本。

验证入口应分别测两个 module，并增加发布形态检查：

```sh
go test ./...
go test -C agenthooks ./...
GOWORK=off go test ./...
GOWORK=off go vet ./...
go mod verify
```

### 包可见性与 API

- `cmd/style` 保持薄；`internal/cli`、`internal/app`、`internal/adapter`、`internal/platform` 属于可自由调整的程序内部实现。
- `core/spec`、`core/template`、`core/when` 及未来 domain 包只暴露有文档的领域 API。
- `pkg/` 不是必需目录；只有需由外部项目 import 的稳定 API 才公开。
- 不创建无明确领域归属的 `utils`、`common` 或空的 service/repository/controller 层。
- 动态 YAML 在解析边界转为受约束类型；避免 `map[string]any` 渗透整个 core。
- 编译后的 matcher/index 不受调用方后续修改输入 map 的影响；构造时复制或冻结输入。
- 需要取消、超时或外部 I/O 的用例接收 `context.Context`。

## 4. Core、application、adapter

建议用例链路：

```text
source -> spec.Load -> Validate -> Merge
       -> plan.Requirements -> adapter.Capabilities
       -> compile.Index -> engine.Match/Render -> state/output
```

`core` 定义 STYLE 的规则、匹配、降级和编译语义。`core/when` 只通过 `FieldReader` 读取抽象字段；字段来源由 adapter 根据 agenthooks 事件提供。`core/plan` 查询能力 DTO，不 import Codex 或 Claude 的实现。`core/compile` 消费确定的 plan 结果，生成稳定 index；报告和运行时必须使用同一结果。

`internal/app` 提供 `Validate(ctx, request)`、`Plan(ctx, request)`、`Install(ctx, request)` 等用例。它负责步骤编排、事务和报告；不依赖 Cobra、TTY 控制或原始 provider payload。CLI、集成测试和未来的 explain 都通过相同用例调用。

设计中的大 `Adapter` 接口应按调用场景拆开，例如：

```go
type Detector interface {
	Detect(context.Context) (Installation, bool, error)
}

type CapabilityProvider interface {
	Capabilities(context.Context, Installation) (Capabilities, error)
}

type SurfaceProvider interface {
	Surface(Index) ([]HookSpec, error)
}

type RuntimeEnricher interface {
	Enrich(*agenthooks.Event, *state.Session) ([]Point, error)
}
```

Codex/Claude adapter 可组合实现多个小接口。`core/plan` 只依赖能力端口；安装用例只依赖检测、路线与变更端口；热路径只依赖 runtime enrichment。接口由使用方定义，避免所有 provider 被一个宽接口绑在一起。

## 5. CLI 设计

### 命令职责

命令层只负责解析参数、组装 request、调用用例、渲染结果。业务文件操作不直接写在 command handler 中；不在命令包解析 provider JSON；不通过错误文本决定退出码；`os.Exit` 只出现在最外层启动代码。

推荐命令保持按生命周期组织：`init`、`validate`、`plan`、`compile`、`install`、`uninstall`、`list`、`sync`、`trust`、`doctor`、`adapters`；`stats`、`explain`、`probe` 和更新功能在其数据契约完成后加入。`agenthooks run` 可作为隐藏/内部入口。

命令较多且需要 help/completion 时，Cobra 适合作为 `internal/cli` 的外壳。框架不能进入 `core` 或 `internal/app`，因此标准库与 Cobra 的选择不会影响领域层。

### 输出、错误和交互

| 通道 | 内容 |
|---|---|
| stdout | 成功结果、表格、JSON 或可复制内容 |
| stderr | 诊断、警告、日志、进度和交互提示 |
| exit code | 稳定结果类别；详细文本由诊断提供 |

JSON 输出应有版本化 envelope，例如 `{"version":1,"data":...,"diagnostics":[]}`；机器格式和人类格式消费同一 application result。hook stdout 仅用于 provider 协议响应。

退出码保持少且稳定：`0` 成功，`1` 参数/校验/一般失败，`3` 外部改动需 trust，`4` 用户中止或 `fallback:error` 阻止安装。用错误类型和稳定 code 映射退出码，不做字符串匹配。

配置优先级固定为 CLI flag > 明确环境变量 > 配置文件 > 默认值。统一提供可控的输出格式、静默/详细、颜色和非交互行为；只有跨命令一致的选项才放在 root。所有命令依赖从构造函数注入，测试传入 buffer、临时目录、固定时钟和 fake adapter；避免包级可变全局、init 时读配置和隐式 `Chdir`。

### 文件修改与事务

安装、sync、trust 和 uninstall 应先产生可审阅的 `Changeset`，再应用。每个文件通过同目录临时文件和原子 rename 更新，敏感 settings 使用明确的文件权限；变更带 STYLE 所有权标记，只能更新或删除自己拥有的内容。

多个文件不能依靠一次 rename 获得整体原子性。需要事务日志/备份和恢复步骤：任何阶段失败都恢复此前文件或留下可识别的恢复状态；下次启动/doctor 能检测未完成事务。`--dry-run` 可复用同一个 changeset，不另写一套预测逻辑。写操作还需进程级锁，避免两个 CLI 同时覆盖 lock/index/config。

## 6. agenthooks 与运行时

```text
agenthooks codec -> agenthooks.Event（保留 Raw）
 -> STYLE adapter.Enrich（Point、FieldReader、effort）
 -> core/engine（条件、模板、预算、去重）
 -> agenthooks/provider 输出编码
```

provider 事件、工具名、MCP 命名、退出语义、配置渲染和 quirks 继续由 `agenthooks/` 负责。STYLE adapter 补充 STYLE 所需的能力、评估点、静态位置、effort 来源和限制，不复制 hook 协议。core matcher 区分字段缺失与空字符串；缺失数据不能被误判为可匹配。

热路径只读取编译产物和有限状态，不读规则 markdown、不做目录扫描、网络访问或状态清理。session、scope、去重、统计分别由小 store 操作；并发更新用文件锁、原子 rename 或明确的 append-only 记录。

## 7. 测试与 CI

| 层 | 覆盖内容 | harness 依赖 |
|---|---|---|
| Domain unit | spec/template/when/plan/compile/tokens/state | 无 |
| Contract | 诊断 code、index JSON、能力、route changeset | fixture |
| CLI integration | 参数、输出、退出码、临时目录、冲突和回滚 | 无 |
| agenthooks conformance | provider 往返、未知字段、响应和配置渲染 | fixture |
| Runtime integration | index + FieldReader + engine 完整评估 | fake event |
| Real environment | Claude/Codex 安装、信任、触发 | 有，单独记录 `[?]` |

CI 对根 module 和 agenthooks module 分别运行 tests/vet；再验证 `GOWORK=off` 发布形态。对 provider 协议和稳定 index 使用 golden fixtures；对 malformed YAML/template/payload 做边界测试或 fuzz；固定 clock 和 ID 生成器避免快照包含随机值。新增诊断 code 要保留稳定排序测试。

## 8. 实施顺序

### P0：定住设计和发布边界

1. 把当前 CLI 设计基线放到受版本控制的 `docs/`，更新过时的“尚未实现”状态。
2. 确认根 module 对 agenthooks 的发布依赖，并建立 `GOWORK=off` 检查。
3. 建立 CLI/app/adapter/platform 依赖边界及 core 不引用 provider 的检查。
4. 定义稳定 Diagnostic、错误 code、退出码和 JSON envelope。

### P1：离线核心

1. 做 `core/plan`、token 预算和确定性 `core/compile`。
2. `plan` 报告与 index 使用同一个规划结果。
3. 提供可离线运行的 validate/plan/compile application 用例。
4. 用 fake adapter 和 conformance suite 固定 provider 能力契约。

### P2：运行时和安装

1. 实现 state/engine，并接入 agenthooks runtime event。
2. 按任务顺序做 Codex、Claude adapter，不把 provider 事实移入 core。
3. install/sync/trust/uninstall 使用 changeset、所有权标记、锁、回滚及恢复检查。
4. 用临时 HOME 覆盖重复执行、用户修改冲突和中途失败恢复。

### P3：真实环境和扩展

完成 M1a 离线契约后再做真实环境、probe、explain、stats、update 和额外 harness。M1 期间不引入远程决策服务或动态脚本插件。

## 9. 应避免的结构

- 不把所有逻辑塞进 `cmd/style` 或单个 root command 文件。
- 不让 core import Cobra、agenthooks、TTY 或具体 adapter。
- 不用一个大 `Adapter` 接口强迫每个 provider 同时实现 detect、plan、install 和 runtime。
- 不通过全局 registry/config 隐藏依赖，不用错误文本做程序判断。
- 不向 hook stdout 写日志或调试信息。
- 不为目录外观创建空的 `pkg/`、`utils/` 或多层 CRUD 架构。
- 不为尚未证明的扩展提前拆服务、引入反射或动态插件系统。

## 10. 参考资料

以下资料提供 Go module/包组织和成熟 CLI 设计的进一步阅读入口；仓库特定结论以本地源码、STYLE 规范、agenthooks 设计和已确定的职责边界为依据。

- [Organizing a Go module](https://go.dev/doc/modules/layout)
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Managing dependencies](https://go.dev/doc/modules/managing-dependencies)
- [Go command reference](https://pkg.go.dev/cmd/go)
- [Cobra user guide](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md)
- [GitHub CLI command packages](https://github.com/cli/cli/tree/trunk/pkg/cmd)
- [agenthooks design](../agenthooks/DESIGN.md)

仓库当前 CLI 设计基线位于被 `.gitignore` 忽略的 `STYLE_DOCS/STYLE-cli-design.md`，本地评估时已阅读；由于它不会随正常 clone 提交，P0 中把有效基线迁入 `docs/` 是必要的可维护性工作。

外部站点在本次执行环境的 DNS/TLS 访问失败，因此未对链接页面进行实时抓取；代码与仓库文档评估基于本地内容，外部链接作为后续阅读入口。

## 11. Review 完成标记

- [x] 已阅读当前 module/workspace、STYLE core、CLI 设计和 agenthooks 设计。
- [x] 已评估 Go 包边界、CLI 分层、adapter、运行时、错误/输出、测试与发布策略。
- [x] 已记录目标目录、依赖方向、优先级和可执行顺序。
- [x] 已同步更新 `docs/STYLE-M1-REVIEW-2026-10-13.md` 和 `docs/PROGRESS.md`。
- [x] 本次只完成评估文档，没有开始 T15，也没有修改 agenthooks 源码。
