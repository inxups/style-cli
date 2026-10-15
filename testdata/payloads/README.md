# Adapter conformance fixtures

46 payloads (19 Codex, 27 Claude Code) run through the real agenthooks codec,
adapter, persistent state, STYLE engine and provider encoder. Captured baseline
payloads come from `agenthooks/agenthookstest/fixtures`; additional synthetic
payloads exercise all mapped native events and explicit boundary cases.

`STYLE.yaml` is loaded, validated, merged, planned with each real adapter and
compiled before invocation. Each payload has an `.event.golden` projection
(excluding receive timestamps) and exact `.stdout.golden` bytes. Codex no-op
files are intentionally empty; Claude no-op is `{}` without a newline. Unknown
and observe-only events have no STYLE evaluation point. Claude SubagentStart
cannot emit context; Codex supports it. Codex apply_patch uses the documented
`tool_input.command` patch shape.

PermissionRequest is a deny-only capability probe outside the current M1
subscription surface. Its generic parse fixture uses an unknown tool so no
unsupported context injection is requested; `deny_permission.json` separately
checks that both codecs encode a matching shell block correctly. Runtime and
Runner ERROR logs fail conformance even when stdout is otherwise neutral.

`process_test.go` also builds the CLI and checks all 46 output goldens, index
failure policy and all three generated route commands. Package/lock/settings
files are deliberately malformed to detect accidental management-path reads.
Generated commands persist the installation's STYLE home and harness facts;
an incorrect parent STYLE_HOME must not change the loaded index.

`*-effort.json` contains 25 independent expectations covering every embedded
exact/alias mapping, uppercase spelling, missing values and unknown tiers.
Coverage checks reject an embedded mapping without a fixture. Runtime tests
assert both stored state and rendered template values.

Run from the repository root:

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off \
  go test -count=1 ./internal/adapter/conformance
```

For an intentional contract change only, add `UPDATE_GOLDEN=1`, inspect all
changed projections and wire output, then run again without that variable.
The whitelist checks run even during golden updates. Agent scope tests and
independent native-kind/evaluation-point assertions are never regenerated.
