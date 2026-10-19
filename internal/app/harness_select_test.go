package app

import (
	"errors"
	"reflect"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
)

func TestParseHarnessesIsTheOnlySyntax(t *testing.T) {
	service := testService(t)
	for _, test := range []struct {
		values []string
		auto   bool
		named  []adapter.ProviderID
	}{
		{nil, true, nil},
		{[]string{"auto"}, true, nil},
		{[]string{" Claude ,codex", "CODEX", "claude-code"}, false, []adapter.ProviderID{"claude-code", "codex"}},
		{[]string{"codex,auto"}, true, []adapter.ProviderID{"codex"}},
	} {
		got, err := service.parseHarnesses(test.values)
		if err != nil || got.Auto != test.auto || !reflect.DeepEqual(got.Named, test.named) {
			t.Fatalf("%q = %+v, %v", test.values, got, err)
		}
	}
	for _, values := range [][]string{{""}, {"codex,"}, {"bogus"}, {"codex", "claude,bogus"}} {
		var usage *UsageError
		if _, err := service.parseHarnesses(values); !errors.As(err, &usage) {
			t.Fatalf("%q error = %v, want usage error", values, err)
		}
	}
	for values, want := range map[string]string{"Claude": "claude-code", "codex,CODEX": "codex"} {
		if got, err := service.singleHarness([]string{values}); err != nil || got != want {
			t.Fatalf("single %q = %q, %v", values, got, err)
		}
	}
	for _, values := range []string{"auto", "codex,claude", "codex,auto"} {
		if _, err := service.singleHarness([]string{values}); err == nil {
			t.Fatalf("single %q accepted", values)
		}
	}
}
