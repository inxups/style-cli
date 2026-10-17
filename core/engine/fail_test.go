package engine

import (
	"bytes"
	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/schema"
	"testing"
)

func TestM3A14FailConstructorAndCapabilityBoundary(t *testing.T) {
	entry := testEntry("pkg/guard", "prompt.submitted", "event", "block", "no", 1)
	entry.Fail = "closed"
	store, _ := state.New(t.TempDir())
	index := compile.Index{Format: 7, StyleVersion: "x", Packages: []compile.PackageInfo{}, Static: map[string][]compile.StaticEntry{}, Harnesses: map[string]map[string][]compile.Entry{"future-harness": {"prompt.submitted": {entry}}}}
	core, err := New(index, store)
	if err != nil {
		t.Fatal(err)
	}
	in := testInput("prompt.submitted")
	in.Harness = "future-harness"
	in.Points[0].Capabilities.SuppressOriginalPrompt = true
	out, err := core.Process(in, tokens.InputLimit{})
	if err != nil || !out.Block || !out.SuppressOriginalPrompt {
		t.Fatal(out, err)
	}
	if !core.HasFailClosed("future-harness", "prompt.submitted") || core.HasFailClosed("future-harness", "tool.pre") || core.HasFailClosed("codex", "prompt.submitted") {
		t.Fatal("guard buckets")
	}
	for _, point := range []string{"tool.post", "session.start", "subagent.start"} {
		index.Harnesses = map[string]map[string][]compile.Entry{"future-harness": {point: {entry}}}
		data, _ := compile.Encode(index)
		if schema.ValidateIndex(data) == nil {
			t.Fatal("schema accepted guard on", point)
		}
		if _, err := New(index, store); err == nil {
			t.Fatal("constructor accepted", point)
		}
		if _, err := Load(bytes.NewReader(data), store); err == nil {
			t.Fatal("loader accepted", point)
		}
	}
}
