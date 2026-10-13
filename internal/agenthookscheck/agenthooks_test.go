package agenthookscheck

import (
	"testing"

	"github.com/speakeasy-api/agenthooks"
)

func TestLocalAgenthooksDependency(t *testing.T) {
	if runner := agenthooks.New(); runner == nil {
		t.Fatal("agenthooks.New() returned nil")
	}
}
