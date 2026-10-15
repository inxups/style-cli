package app

import (
	"testing"

	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
)

func testService(t *testing.T) *Service {
	t.Helper()
	service, err := New(codex.Definition(), claude.Definition())
	if err != nil {
		t.Fatal(err)
	}
	return service
}
