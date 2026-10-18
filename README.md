# STYLE

STYLE manages rules and native hooks for registered coding harnesses.

Ordinary `rules` describe behavior compiled by STYLE. Native commands are declared
separately, inline in the top-level `native_hooks` list in `STYLE.yaml`:

```yaml
spec: style/v0.3
name: example
version: 1.0.0
description: Example package

rules:
  - id: shell-reminder
    target: tool
    tool: shell
    at: before
    text: 执行命令前检查风险。

native_hooks:
  - id: command-check
    description: 使用本地脚本检查命令。
    fallback: skip
    claude-code:
      event: PreToolUse
      matcher: Bash
      command: /absolute/path/check-command.sh
      timeout: 10
```

A directory package may contain rules, hooks, or both. IDs must be unique across
both lists. Hooks accept `id`, `description`, `enabled`, `priority`, `fallback`,
and registered harness mappings. Each mapping accepts only `event`, `matcher`,
`command`, and `timeout`. Hooks default to enabled, priority 50 (or the package
priority), and `fallback: skip`; `fallback: error` blocks installation when a
mapping cannot be installed. A missing harness mapping is always skipped.
Ordinary rule `defaults` do not apply to hooks. Single-file `.style.md` packages
only support ordinary rules.

STYLE manages hook approval, installation, checks, sync, update and removal.
Harnesses execute native commands; hooks do not enter STYLE's rule engine,
templates, deduplication, runtime statistics or token budgets. `description` is
plain explanatory text. Development links require renewed approval when their
executable hook declarations change; editing only `description` does not require
execution approval. The ordinary rule plan status `native` still means the
harness supports that rule directly.

`target: native` has been removed without a compatibility entry point. To migrate
an existing installation:

1. Move each declaration into `STYLE.yaml` under `native_hooks`, remove `target`,
   and move any Markdown notes into `description`.
2. Run `style uninstall PACKAGE --yes` in the original scope. If remaining old
   packages prevent rebuilding, use `style uninstall --all --yes`.
3. Reinstall the migrated sources with `style install SOURCE --yes` and the same
   harness, route and project options; review the disclosed native commands.

Old installed source or approval snapshots produce `native-format-removed` and
require reinstalling. `list` and `doctor` keep those packages visible. This change
continues to use `style/v0.3`; there is no migration command.
