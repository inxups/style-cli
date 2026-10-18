package compile

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
)

type nativeCompileAdapter struct{ compileAdapter }

func (nativeCompileAdapter) NativeHook(mapping spec.NativeMapping) (plan.NativeEntry, error) {
	return plan.NativeEntry{Event: mapping.Event, Matcher: mapping.Matcher, Timeout: 60}, nil
}

func TestNativeHooksAreSeparateFromRuntimeAndProvenanceRules(t *testing.T) {
	pkg := loadResolved(t, "separate", "  - {id: base, target: system, text: Base instructions.}\n")
	input := inputFor(t, pkg, 1)
	baseline, err := Build(Request{StyleVersion: "test", Packages: []PackageInput{input}})
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{"id": "notify", "fallback": "skip", "priority": 50, "codex": map[string]any{"event": "Stop", "command": "PRIVATE_AUTHOR_COMMAND"}}
	pkg.NativeHooks = []spec.NativeHook{{Index: 0, Source: "STYLE.yaml#native_hooks/notify", Fields: fields, Effective: fields}}
	report, err := plan.Build(pkg, nativeCompileAdapter{compileAdapter{name: "codex"}})
	if err != nil {
		t.Fatal(err)
	}
	input.Plans = []plan.Report{report}
	request := Request{StyleVersion: "test", Packages: []PackageInput{input}}
	result, err := Build(request)
	if err != nil || !reflect.DeepEqual(result, baseline) {
		t.Fatalf("hook changed runtime compilation: %v", err)
	}
	raw, err := Encode(result.Index)
	if err != nil {
		t.Fatal(err)
	}
	data, err := BuildProvenance(request, result, raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE_AUTHOR_COMMAND") || strings.Contains(string(data), `"target": "native"`) {
		t.Fatal("native content leaked into rules")
	}
	provenance, err := DecodeProvenance(data, raw, result.Index)
	if err != nil || provenance.Format != 2 || len(provenance.Harnesses["codex"]) != 1 || len(provenance.NativeHooks["codex"]) != 1 {
		t.Fatalf("separate provenance: %+v %v", provenance, err)
	}
	for _, mutate := range []func(*plan.Report){
		func(p *plan.Report) { p.NativeHooks = nil },
		func(p *plan.Report) { p.NativeHooks[0].ID = "base" },
		func(p *plan.Report) { p.NativeHooks[0].Native.Event = "SessionStart" },
	} {
		cloned, _ := json.Marshal(report)
		var bad plan.Report
		if err := json.Unmarshal(cloned, &bad); err != nil {
			t.Fatal(err)
		}
		mutate(&bad)
		input.Plans = []plan.Report{bad}
		if _, err := Build(Request{StyleVersion: "test", Packages: []PackageInput{input}}); err == nil {
			t.Fatal("accepted mismatched native plan")
		}
	}
}

func TestProvenanceReadsRuleOnlyFormatOneAndRejectsLegacyNative(t *testing.T) {
	pkg := loadResolved(t, "legacy-provenance", "  - {id: base, target: system, text: Base.}\n")
	input := inputFor(t, pkg, 1)
	request := Request{StyleVersion: "test", Packages: []PackageInput{input}}
	result, err := Build(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Encode(result.Index)
	if err != nil {
		t.Fatal(err)
	}
	data, err := BuildProvenance(request, result, raw)
	if err != nil {
		t.Fatal(err)
	}
	var p Provenance
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	p.Format = 1
	p.NativeHooks = nil
	legacy, _ := json.Marshal(p)
	if _, err := DecodeProvenance(legacy, raw, result.Index); err != nil {
		t.Fatal(err)
	}
	p.Harnesses["codex"][0].Native = &plan.NativeEntry{Event: "Stop", Timeout: 60}
	legacy, _ = json.Marshal(p)
	if _, err := DecodeProvenance(legacy, raw, result.Index); err == nil || !strings.Contains(err.Error(), "native-format-removed") {
		t.Fatalf("legacy native accepted: %v", err)
	}
}

func TestEmptyProvenanceRoundTrips(t *testing.T) {
	request := Request{StyleVersion: "test"}
	result, err := Build(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Encode(result.Index)
	if err != nil {
		t.Fatal(err)
	}
	data, err := BuildProvenance(request, result, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeProvenance(data, raw, result.Index); err != nil {
		t.Fatal(err)
	}
}
