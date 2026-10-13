.PHONY: test-go vet-go

test-go:
	env -u CODEX_HOME -u CODEX_SANDBOX \
		GOPATH=/tmp/style-cli-gopath \
		GOMODCACHE=/tmp/style-cli-gomodcache \
		GOCACHE=/tmp/style-cli-gocache \
		go test ./...
	env -u CODEX_HOME -u CODEX_SANDBOX \
		GOPATH=/tmp/style-cli-gopath \
		GOMODCACHE=/tmp/style-cli-gomodcache \
		GOCACHE=/tmp/style-cli-gocache \
		go test -C agenthooks ./...

vet-go:
	env -u CODEX_HOME -u CODEX_SANDBOX \
		GOPATH=/tmp/style-cli-gopath \
		GOMODCACHE=/tmp/style-cli-gomodcache \
		GOCACHE=/tmp/style-cli-gocache \
		go vet ./...
	env -u CODEX_HOME -u CODEX_SANDBOX \
		GOPATH=/tmp/style-cli-gopath \
		GOMODCACHE=/tmp/style-cli-gomodcache \
		GOCACHE=/tmp/style-cli-gocache \
		go vet -C agenthooks ./...
