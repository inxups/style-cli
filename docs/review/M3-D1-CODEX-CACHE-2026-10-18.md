# M3 D1 补充：Codex 原生插件缓存绑定

2026-10-18，适用 Codex 0.159.3 的本地 marketplace。完整 M3 授权内的真实验收修正；不改变 probe 的只读、有限读取、来源关联而非认证边界。

三次隔离真实运行已收到原生 SessionStart/UserPromptSubmit 和完整模型规则正文，但 probe 因根不匹配保持 unverified。独立作者观察器的两项原生 root 变量及原始收据证明：实际根为 CODEX_HOME/plugins/cache/style/style-runtime/0.1.0，生成源根为 STYLE_HOME/adapters/codex/plugin。原生 plugin list 与配置/本地 marketplace 核对同一来源；二者是独立文件。新实现前不把这些失败记为通过。

## 只读绑定

1. 保留现有 owner 命令、订阅、信任、源 launcher/descriptor 核对。
2. 有对应本地 marketplace 登记时，由已捕获的 config.toml 读取 source_type/source；读取固定 `.agents/plugins/marketplace.json`，核对 marketplace 名、唯一插件名与 local source 的规范路径等于已安装生成根。不得执行原生 CLI、扫描目录、猜测任意缓存根或读取包源码。
3. 插件 ID/name/version 必须是安全的单一路径组件；固定派生缓存目录 `ConfigDir/plugins/cache/<marketplace>/<name>/<version>`。只读缓存 hooks/hooks.json、bin/style、.codex-plugin/plugin.json，分别逐字节核对已捕获的对应源文件，包含 native/foreign hook 的实际形状。缺失、陈旧或错来源拒绝。
4. agenthooks 返回通过核对的运行根，adapter 用其建立当前 probe 路线根；原生收据仍须满足运行根、可执行文件身份、存活祖先、时间/session 和不可变安装绑定。等待期间源、登记或缓存改变仍失败。
5. 没有对应 marketplace 登记时沿用原来的源根关联规则，不扩大可接受 root 集合；这保留已有直接加载/离线进程 fixture 的契约。已登记但无效的缓存不得退回源根。

agenthooks 保留原生注册格式、渲染、owner 和文件检查职责；adapter 只传递已检查运行根。probe 不写原生缓存或登记文件，不通过包装 STYLE runner 或设置假 root 变量绕过校验。

## 更新与真实验收

原生 CLI 的无模型隔离检查已证明：STYLE update 改生成源文件，缓存不会同步；重新执行 `codex plugin add style-runtime@style --json` 会刷新同版本缓存。此原生步骤继续由操作者执行，STYLE 不自动登记或改原生数据库。

真实采集器在批准 native 更新后运行该命令，并将 SessionStart 的 native 正文改为独立 canary；下一真实新会话必须收到新正文且不含旧正文，不能只凭 timeout 字段或不变 marker 推断缓存已更新。Claude 本地插件仍按其原地加载契约另验。

## 回归要求

覆盖有效缓存、每个缓存执行文件缺失/漂移、错 marketplace/source、重复项、remote 来源、不安全路径组件、已登记缓存不可退回、原源根冒充缓存根及等待中缓存变化。旧 source-only、user/project 路线保持既有行为。六条本地未触发/中断检查及原生正向验收使用新二进制重跑；旧失败、旧三平台成功结果保留，最终 CI 另跑。
