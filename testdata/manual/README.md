# M1b 手动探针

配置步骤和验收标准见 [手动验收手册](../../docs/STYLE-M1B-MANUAL.md)。本目录只提供测试包，不注册 hook、原生 skill 或 MCP。

`m1b-probes` 为第二个包，包含四条规则：

- `session-info`：输出模型/effort，允许 compact 后补发，也可用于 model switch 观察。
- `warn-shell`：仅匹配命令中的 `STYLE_M1B_WARN`，分别输出用户/模型标记。
- `block-prompt`：仅阻断含 `STYLE_M1B_BLOCK_PROMPT` 的 prompt，提示不含原 prompt。
- `parallel-once`：仅匹配命令中的 `STYLE_M1B_PARALLEL`，按 session 去重。

从仓库根复现离线准备检查：

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off \
  go build -o /tmp/style-m1b-probe ./cmd/style
/tmp/style-m1b-probe validate testdata/manual/m1b-probes
/tmp/style-m1b-probe plan testdata/manual/m1b-probes --harness codex,claude-code
```

上述检查不能替代真实 harness 的配置接受、上下文投递、并行调用或性能验收。
