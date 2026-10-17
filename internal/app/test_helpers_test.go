package app

import (
	"context"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
)

type undetectedHarness struct{}

func (undetectedHarness) Detect(context.Context) (adapter.Installation, bool, error) {
	return adapter.Installation{}, false, nil
}

func testService(t *testing.T) *Service {
	t.Helper()
	service, err := New(codex.Definition(), claude.Definition())
	if err != nil {
		t.Fatal(err)
	}
	return service
}
