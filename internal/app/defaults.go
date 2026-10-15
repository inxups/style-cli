package app

import (
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
)

// DefaultService registers the two M1 provider adapters.
func DefaultService() (*Service, error) {
	return New(codex.Definition(), claude.Definition())
}
