# A18 token diagnostics

T001, T002, T004–T007 each contain a valid package whose plan emits that warning.
T003 contains a 4,008-character ASCII tool instruction, estimated at 1,002 tokens.
It fits within 80% of Codex's 2,500-token input limit (the 80% threshold is 2,000 tokens) by itself. Installing a
second renamed copy exceeds the combined limit and must exit 4 without changing
any installed files. No fixture requires real provider configuration.

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off \
  go test -count=1 ./internal/app -run TestA18 -v
```
