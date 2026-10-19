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
2. Run `style uninstall PACKAGE` in the original scope and answer `y`. If
   remaining old packages prevent rebuilding, use `style uninstall --all`.
3. Reinstall the migrated sources with `style install SOURCE` and the same
   harness, route and project options; review the disclosed native commands
   before answering `y`.

Old installed source or approval snapshots produce `native-format-removed` and
require reinstalling. `list` and `doctor` keep those packages visible. This change
continues to use `style/v0.3`; there is no migration command.

## Command line

`style --help` lists every command; `style help COMMAND` (or `--help` anywhere
on a command line) also shows advanced and project options. Boolean flags take
no value (`--json`, not `--json=true`).

`--harness` means the same everywhere: it is repeatable, takes comma lists, any
case and the `claude` alias, and `auto` (the default) selects detected harnesses
(`plan`, `install`) or every installed one (`probe`). `explain` reads a single
payload and requires exactly one named harness. Unknown names are usage errors.

Exit codes: `0` success, `1` the request was rejected or failed, `2` the command
line could not be parsed (unknown command or option, missing or malformed
argument), `3` `sync` found changes awaiting `trust`, `4` blocked by policy or
budget. Hook invocations never exit `2`, which harnesses treat as a block.

`install`, `restore`, `update`, `trust` and `uninstall` print a preview and ask
`[y/N]` on stderr before changing anything; only `y` or `yes` read from stdin
applies the change. No input, end of input or any other answer cancels without
writes, so scripts and CI answer explicitly, for example
`echo y | style install SOURCE --json`. With `--json` the preview and prompt go
to stderr and stdout carries only the final report.

The following forms were removed without compatibility entry points:

| Removed | Use instead |
| --- | --- |
| `style install --frozen` | `style restore` |
| `style adapters` | `style doctor` (text and JSON include the adapter inventory) |
| `--count=exact --count-provider=P --encoding=E` / `--count-model=M` | `--exact openai:E` / `--exact anthropic:M` |
| `install --fail-closed claude-code` | `install --fail-closed` |
| `--yes` on `install`, `restore`, `update`, `trust`, `uninstall` | answer `y` to the prompt (stdin) |

`explain --event` is optional; the event is read from the payload's
`hook_event_name`, and an explicit `--event` must match it.
