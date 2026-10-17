# M3-D9 精确计数定稿

日期 2026-10-18；依赖 T45/A14。按全部评审通过授权定稿并直接推进 T50。Anthropic 独立 API 真实验收已由用户延期，不阻止实现和离线 API 测试。

## 命令与文本范围

新增 `style count <UTF-8普通文件> [--count=estimate|exact] [--count-provider=openai|anthropic] [--encoding=name] [--count-model=id] [--json]`。默认 estimate 使用现有生产估算；文件保留全部字节/换行，不自动加前缀，最多 256 KiB，拒绝 symlink/非普通文件/无效 UTF-8。只读，不加载安装或检测 harness，不输出原文。

`plan` 和 `explain` 也接受以上四个计数选项（没有文件参数，沿原命令）。默认/显式 estimate 保持原结果、预算和阈值。exact 是附加报告，不替换预算估算、session_cost、运行时硬上限或 stats。count-provider 必须显式提供，不能把 harness 名、provider 条件或模型别名猜成计数器。

plan 在正常 compile 解析静态 vars/settings、warn model_text/首句、fallback 和去重后，分别计各 placement 的模型文本；静态槽只计正文，不计宿主管理标记。任何被保留的运行时模板都不能在无事件时称 exact：在任何计数联网前拒绝整次 exact 请求，提示用 explain。native 输出不可确定，排除并在口径披露；prompt/skill block 只面向用户，记不属于模型输入，不能把拒绝原因加到输入数。

explain 在原有隔离状态/显式 facts 求值与 cap/dedup/合并之后，计最终实际 additional context；工具 block 计 reason，prompt block 不计模型输入。其既有 wire 保持不变。输出无模型文本时计 0 并披露 none，不调用远端。计数失败让管理命令失败，不改变实际 hook 行为或把原生退出 0 当成功。

## 本地具名编码

OpenAI 路径只支持显式 `o200k_base` / `cl100k_base`，用固定纯 Go 实现 github.com/tiktoken-go/tokenizer@v0.7.0（Go 1.23/MIT），词表随二进制嵌入；无需 Python、key 或运行时下载。v0.8.1 要 Go 1.26，故不为本功能提高项目 Go 1.25 基线。报告实现版本、encoding、UTF-8 文本摘要、字符数和整数 token 数。特殊 token 拼写按普通文字计，不添加模型控制 token。

不提供隐式 model→encoding 推断；本地指定 model 会拒绝，防止把编码精确偷换为某个模型/全请求的精确。独立 Python tiktoken0.12.0 的固定文本/随机 Unicode 对照用于验收，公开语料和词表摘要固定。CARRY-01 官方模型校准仍两家各 0/40，不被本地对照关闭。

## Anthropic 请求与失败

显式 anthropic + count-model，将所选完整文本封成一个 user message，POST 到固定 HTTPS api.anthropic.com/v1/messages/count_tokens，使用官方 anthropic-version:2023-06-01 和调用时读取的 ANTHROPIC_API_KEY。encoding 不能提供；不读取 Claude 订阅配置/代理令牌，不提供重定向或自定义 endpoint。只在 exact 冷路径且有待计文本时联网；命令本身是明确的文本发送选择。

结果的 scope 为 anthropic-messages-request，input_tokens 包含这份最小请求的消息封装，不是纯正文 token 增量，也不是 STYLE 与无 STYLE 的差值。报告请求摘要/模型/接口版本，不能把分条请求相加当实际完整模型请求。纯文件空文本的 Anthropic 请求拒绝；无模型输入的 explain/placement 用 none/0。

15 秒网络上限，响应最多 1 MiB；缺 key/model、非法参数、取消/超时、传输失败、非 2xx、坏 JSON/缺值/负数/非整数/溢出均失败，禁止静默估算或缓存旧结果。错误只含安全类别/HTTP 状态，不打印 key、服务器回显正文或已计原文。自动重试关闭。

## 验证

| 编号 | 状态 | 断言 |
|---|---|---|
| A15-01 | [ ] | 两编码独立 Python golden/中文/英文/控制/emoji/组合/特殊 token 拼写，离线运行和版本披露 |
| A15-02 | [ ] | Anthropic 固定请求/header/摘要、成功和缺配置/HTTP/JSON/超时/取消/重定向/泄漏保护，全部 transport fixture |
| A15-03 | [ ] | count/plan/explain CLI、静态变量/warn/去重/动态拒绝、实际最终文本、无输入/失败恢复和旧 JSON/预算兼容 |
| A15-04 | [ ] | 三缓存 fresh test/vet/race、无运行时网络/副作用、发布构建与文档 |

TOKEN-01 的本地编码需实际运行生产 CLI；API 真运行保持用户延期的 [?]。实现完成不自动关闭 TOKEN-01。

## 调查来源

固定版本上游 tokenizer.go/go.mod/LICENSE 和 Anthropic 官方 Count tokens in a Message 参考（2026-10-18 获取），摘要见[来源记录](M3-D9-SOURCES-2026-10-18.json)。前者为 Go port，不冒充 OpenAI 官方 SDK。官方 API 描述 input_tokens 为已提供 messages/system/tools 的总数，本设计只构造明确的单 message。
