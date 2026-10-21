# STYLE

English | [简体中文](README.zh_CN.md)

STYLE is a command-line tool that bundles coding-style rules and native hooks into a STYLE and installs it into coding agents (harnesses) such as Codex and Claude Code.

## About

STYLE uses one standardized configuration to quickly make a harness *yours*: it writes code the way you like, reminds you at the moments you care about, and guards your work with your own scripts.

Out of the box, a harness knows nothing about your preferences, and every harness puts its rule files, hook events and configuration in different places. Getting Codex and Claude Code to follow the same conventions usually means learning each one and maintaining several configurations by hand, then doing it all again on a new machine or project.

STYLE pulls those differences into a single `STYLE.yaml`:

- **Declare once**: write rules (when, for which tool, what to say) and native hooks (which command to run on which event) in one format
- **Adapt automatically**: STYLE compiles them into each harness's native configuration, and `plan` shows how every rule lands and how many tokens it costs before you install
- **Fully managed**: STYLE handles approval, installation, sync, updates and removal; after an edit, `sync` applies it, with no files to patch by hand
- **Reuse anywhere**: keep the configuration in a local directory or a Git repository, install it for your user or a single project, and stay consistent across machines and projects

Whether you want to carry your personal habits into every agent or share one set of coding conventions across a team, it starts with a single `STYLE.yaml`.

## Quick Start

### Prerequisites

- macOS (install with Homebrew), Windows (download a release zip), or Go 1.25 or later (build from source)
- At least one installed harness: Codex or Claude Code

### Installation

On macOS, use Homebrew:

```bash
brew install inxups/tap/style-cli
```

The formula installs the `style` binary and conflicts with homebrew-core's `style`; the two cannot be installed together.

On Windows, download `style-cli_<version>_windows_amd64.zip` (or `_arm64.zip` on ARM devices) from [GitHub Releases](https://github.com/inxups/style-cli/releases), extract it, and add the folder containing `style.exe` to `PATH`. Each release also publishes `style-cli_<version>_windows_checksums.txt` with SHA-256 checksums. The Windows builds have not been tested on real Windows machines: amd64 only runs in CI on GitHub-hosted Windows runners, and arm64 is cross-compiled without being run.

Build from source:

```bash
git clone https://github.com/inxups/style-cli.git
cd style-cli
go build -o style ./cmd/style
```

### Usage

Create a STYLE, check it, and preview how it lands in each harness:

```bash
style init demo
style validate demo
style plan demo
```

`style init demo` creates `demo/STYLE.yaml`, `demo/prompts/default.md` and `demo/rules/review.md`; `validate` prints `valid`; `plan` lists, for each detected harness, how every rule is placed and its estimated token cost, for example:

```text
Harness: claude-code
Preferred route: claude-user (availability checked during install)
Package: demo
Adapter: claude-code
- style: native (inject system)
  ...
```

When it looks right, install it. STYLE prints a preview first and asks `[y/N]` on stderr; it writes only when you answer `y` or `yes`:

```bash
style install demo
```

## Features

- **Native hook management**: declare harness-native commands under `native_hooks`; STYLE discloses, approves and installs them, while the harness still runs them
- **Development links**: `style link` links a local directory as a development package, and `sync` applies your edits
- **Diagnostics**: `list`, `doctor` (including live callback checks with `--live`), `stats` and `explain`
- **Machine-readable output**: every command except `init` supports `--json` and emits one common JSON envelope

## Examples

### Writing a STYLE

A directory package may contain ordinary rules (`rules`), native hooks (`native_hooks`), or both; IDs must be unique across both lists:

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

- Hooks accept `id`, `description`, `enabled`, `priority`, `fallback` and per-harness mappings; each mapping accepts only `event`, `matcher`, `command` and `timeout`
- Hooks default to enabled, priority 50 (or the package priority) and `fallback: skip`; `fallback: error` blocks installation when a mapping cannot be installed; a missing harness mapping is always skipped
- Ordinary rule `defaults` do not apply to hooks, and hooks do not enter STYLE's rule engine, templates, deduplication, runtime statistics or token budgets
- Single-file `.style.md` packages (`style init NAME --single`) support only ordinary rules

### Installing from Git

Git sources use the go-getter form; the `//path` and `?ref=` selectors are optional (defaulting to the repository root and the remote HEAD):

```bash
style install 'git+https://host/org/repo.git//package/path?ref=refs/tags/v1'
```

Refs must be canonical: `refs/heads/…`, `refs/tags/…` or a full commit OID. `style update NAME --ref REF` switches the ref of an installed Git package.

### Installing into a project

Global options go before the command:

```bash
style --project . install SOURCE_A SOURCE_B --set SOURCE_A_NAME.KEY=VALUE
```

Several sources (at most 64) are accepted only for a fresh project installation, and they are reviewed and installed as one transaction. With one source use `--set KEY=VALUE`; with several, every setting names its package: `--set PACKAGE.KEY=VALUE`.

### Local development

```bash
style link ./my-package
style sync
```

The first `sync` after linking needs approval. After that, edits to ordinary rules take effect on `sync`, while changes to executable hook declarations need renewed approval (editing only `description` does not). `sync` shows the source, rendered and native diffs and asks one package at a time; declining one stops `sync` with exit code `1`. `sync NAME` approves only that package. `style uninstall NAME` removes a link.

### Diagnostics

```bash
style list
style doctor
style doctor --live --timeout 60s
style explain --harness claude-code --payload payload.json
```

`doctor --live` runs the static checks and then waits for a callback from each selected harness; once stderr says the check is ready, trigger a subscribed event in a harness session. `explain` reads a single payload and requires exactly one harness; the event defaults to the payload's `hook_event_name`.

### Scripts and CI

Commands that change state cancel without writing when there is no input, input ends, or the answer is anything other than `y`/`yes`, so scripts must answer explicitly:

```bash
echo y | style install SOURCE --json
```

With `--json`, the preview and prompt go to stderr and stdout carries only the final report, always wrapped in one envelope:

```json
{"format": 2, "command": "list", "status": "ok", "scope": {"kind": "user", "home": "…"},
 "diagnostics": [], "result": {"packages": []}}
```

`status` mirrors the exit code (`ok`, `error`, `blocked`). Exit codes:

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | The request was rejected or failed, including a missing `--set`, an undetected harness, a package that is not installed, or an unconfirmed preview (`confirmation-declined`) |
| `2` | The command line could not be parsed: unknown command, option, harness or route; missing, malformed or conflicting argument (including a malformed Git address or `--ref`); nothing is written to stdout |
| `4` | Blocked by policy or budget |

Hook invocations never exit `2`, which harnesses treat as a block.

## Configuration

| Name | Description | Default |
|---|---|---|
| `STYLE_HOME` | STYLE home for the user scope, and the home a project is bound under | `~/.style` |
| `--style-home DIR` | Global option that replaces `STYLE_HOME` | None |
| `--project DIR` | Global option that scopes plan, installation and diagnostic commands to a project | None (user scope) |
| `--harness` | Selects harnesses; repeatable, takes comma lists, case-insensitive, `claude` is an alias for `claude-code` | `auto`: detected harnesses for `plan` and `install`, every installed one for `doctor --live` |
| `CODEX_HOME` | Codex configuration directory; affects harness detection | Codex's own default |
| `CLAUDE_CONFIG_DIR` | Claude Code configuration directory; affects harness detection | Claude Code's own default |

`style --help` lists every command; `style help COMMAND` or `COMMAND --help` also shows advanced and project options. Boolean flags take no value (`--json`, not `--json=true`).

## Project Structure

| Directory | Description |
|---|---|
| `cmd/style` | CLI entry point |
| `core` | Core logic: package spec parsing, compilation, planning, templates, token counting |
| `internal/app` | Command implementations and lifecycle management such as install and sync |
| `internal/adapter` | Harness adapters (`codex`, `claude`) |
| `internal/source` | Resolution and caching of local and Git sources |
| `agenthooks` | Vendored harness hook codec and routing library (a separate Go module) |
| `schema` | Index JSON Schema |
| `scripts` | Smoke tests and acceptance check scripts |
| `testdata` | Test fixtures and golden files |
| `third_party` | Third-party license notes |

## Development

Run tests and vet (these use temporary Go caches and unset `CODEX_HOME` and `CODEX_SANDBOX`, which would otherwise affect harness detection):

```bash
make test-go
make vet-go
```

Smoke-test a built binary:

```bash
go build -o /tmp/style ./cmd/style
python3 scripts/smoke.py /tmp/style
```

CI is defined in [.github/workflows/ci.yml](.github/workflows/ci.yml) and covers Linux, macOS and Windows.

## FAQ

### `brew install` reports a conflict with `style`

homebrew-core has a different formula named `style`. Both install a `style` binary, so uninstall one before installing the other.

## License

This project is licensed under the [MIT](LICENSE) License. Licenses for bundled third-party dependencies are listed in [third_party/README.md](third_party/README.md).
