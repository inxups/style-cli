# A1 invalid fixtures

Each YAML file describes one intentionally invalid directory package. The test
materializes `manifest` as `STYLE.yaml` and copies `files`, then runs the CLI in
both text and JSON modes. The filename is the required error code.

`generate` creates a real symbolic link, a 256 KiB + 1 byte file, or invalid UTF-8
inside a temporary package. These filesystem cases are reproducible without
checking a large file or a platform-specific symlink into Git.

Run from the repository root:

```sh
env -u CODEX_HOME -u CODEX_SANDBOX \
  GOPATH=/tmp/style-cli-gopath GOMODCACHE=/tmp/style-cli-gomodcache \
  GOCACHE=/tmp/style-cli-gocache GOPROXY=off GOSUMDB=off \
  go test -count=1 ./internal/app -run TestA1ValidateExampleAndEveryInvalidFixture -v
```
