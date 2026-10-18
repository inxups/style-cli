package plan

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/spec"
)

// nativeFake is a fakeAdapter that can also resolve native mappings.
type nativeFake struct {
	fakeAdapter
	err  error
	seen *spec.NativeMapping
}

func (a nativeFake) NativeHook(rule spec.NativeMapping) (NativeEntry, error) {
	if a.seen != nil {
		*a.seen = rule
	}
	if a.err != nil {
		return NativeEntry{}, a.err
	}
	timeout := rule.Timeout
	if timeout == 0 {
		timeout = 60
	}
	return NativeEntry{Event: rule.Event, Matcher: rule.Matcher, Timeout: timeout}, nil
}

func nativeRule(id, fallback string, mappings map[string]any) spec.NativeHook {
	planned := rule(id, "native", "inject", "", fallback, "Author note with {{ unparsed")
	planned.Effective["action"] = "block" // inherited defaults must not leak into native plans
	for harness, mapping := range mappings {
		planned.Fields[harness] = mapping
		planned.Effective[harness] = mapping
	}
	return spec.NativeHook{Index: planned.Index, Source: planned.Source, Fields: planned.Fields, Effective: planned.Effective}
}

func TestNativeRulePlansToNativeHook(t *testing.T) {
	var seen spec.NativeMapping
	adapter := nativeFake{fakeAdapter: fakeAdapter{name: "codex"}, seen: &seen}
	native := nativeRule("notify", "degrade", map[string]any{
		"codex":       map[string]any{"event": "PreToolUse", "matcher": "^Bash$", "command": "notify"},
		"claude-code": map[string]any{"event": "Stop", "command": "other"},
	})
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), NativeHooks: []spec.NativeHook{native}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	got := report.NativeHooks[0]
	want := NativeHookPlan{ID: "notify", Source: "STYLE.yaml#notify", Status: NativeHook,
		Native: &NativeEntry{Event: "PreToolUse", Matcher: "^Bash$", Timeout: 60}}
	if !reflect.DeepEqual(got, want) || report.Blocked {
		t.Fatalf("native plan = %#v, want %#v", got, want)
	}
	if seen.Harness != "codex" || seen.HookID != "notify" || seen.Command != "notify" {
		t.Fatalf("adapter received %#v, want the codex mapping", seen)
	}
	text := report.String()
	if !strings.Contains(text, "- notify: native-hook (native)\n  -> native PreToolUse matcher ^Bash$; timeout 60s\n") {
		t.Fatalf("report text:\n%s", text)
	}
	if strings.Contains(text, "command") || strings.Contains(text, "Author note") {
		t.Fatalf("report leaks command or body:\n%s", text)
	}
}

func TestNativeRuleWithoutMappingIsSkippedRegardlessOfFallback(t *testing.T) {
	native := nativeRule("claude-only", "error", map[string]any{"claude-code": map[string]any{"event": "Stop", "command": "notify"}})
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), NativeHooks: []spec.NativeHook{native}}, nativeFake{fakeAdapter: fakeAdapter{name: "codex"}})
	if err != nil {
		t.Fatal(err)
	}
	got := report.NativeHooks[0]
	if got.Status != Skipped || got.Fatal || report.Blocked || !reflect.DeepEqual(got.Reasons, []string{"no native mapping for this harness"}) {
		t.Fatalf("plan = %#v", got)
	}
}

func TestUnwritableNativeMappingAppliesFallback(t *testing.T) {
	mapping := map[string]any{"codex": map[string]any{"event": "Stop", "command": "notify"}}
	unsupported := errors.New("codex cannot write Stop here")
	for _, tc := range []struct {
		name, fallback string
		adapter        Adapter
		status         Status
		fatal          bool
		reason         string
	}{
		{"degrade", "degrade", nativeFake{fakeAdapter: fakeAdapter{name: "codex"}, err: unsupported}, Skipped, false, "codex cannot write Stop here; native rules have no degradation ladder"},
		{"skip", "skip", nativeFake{fakeAdapter: fakeAdapter{name: "codex"}, err: unsupported}, Skipped, false, "codex cannot write Stop here; native rules have no degradation ladder"},
		{"error", "error", nativeFake{fakeAdapter: fakeAdapter{name: "codex"}, err: unsupported}, InstallError, true, "codex cannot write Stop here; fallback is error"},
		{"no native support", "error", fakeAdapter{name: "codex"}, InstallError, true, "adapter codex does not support native hooks; fallback is error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), NativeHooks: []spec.NativeHook{nativeRule("notify", tc.fallback, mapping)}}, tc.adapter)
			if err != nil {
				t.Fatal(err)
			}
			got := report.NativeHooks[0]
			if got.Status != tc.status || got.Fatal != tc.fatal || report.Blocked != tc.fatal || got.Native != nil ||
				!reflect.DeepEqual(got.Reasons, []string{tc.reason}) {
				t.Fatalf("plan = %#v blocked=%v", got, report.Blocked)
			}
		})
	}
}
