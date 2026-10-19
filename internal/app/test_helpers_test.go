package app

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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

// decodeReport checks a --json envelope and decodes its result into v with the
// hoisted diagnostics merged back, so report assertions stay direct. A slice
// target receives the diagnostics alone.
func decodeReport(data []byte, v any) error {
	var envelope jsonEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if envelope.Format != envelopeFormat || envelope.Command == "" || envelope.Status == "" || envelope.Scope == nil || envelope.Diagnostics == nil || envelope.Result == nil {
		return fmt.Errorf("not a --json envelope: %s", data)
	}
	diagnostics, err := json.Marshal(envelope.Diagnostics)
	if err != nil {
		return err
	}
	if reflect.TypeOf(v).Elem().Kind() == reflect.Slice {
		return json.Unmarshal(diagnostics, v)
	}
	if string(envelope.Result) != "null" {
		if err := json.Unmarshal(envelope.Result, v); err != nil {
			return err
		}
	}
	return json.Unmarshal([]byte(`{"diagnostics":`+string(diagnostics)+`}`), v)
}
