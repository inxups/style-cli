package codex

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/inxups/style-cli/core/state"
	"github.com/speakeasy-api/agenthooks"
)

func TestResolveEffortPrecedenceAndTrust(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "repo")
	home := filepath.Join(root, "codex-home")
	files := map[string][]byte{
		filepath.Join(home, "config.toml"): []byte(`profile = "selected"
model_reasoning_effort = "low"

[profiles.selected]
model_reasoning_effort = "medium"

[profiles.explicit]
model_reasoning_effort = "high"
`),
		filepath.Join(project, ".codex", "config.toml"): []byte(`model_reasoning_effort = "xhigh"
`),
	}
	read := func(path string) ([]byte, error) {
		if value, ok := files[path]; ok {
			return value, nil
		}
		return nil, errors.New("not found")
	}
	for _, tc := range []struct {
		name   string
		input  EffortInput
		want   string
		source string
	}{
		{name: "launch override", input: EffortInput{Home: home, CWD: project, ProjectTrusted: true, Launch: agenthooks.CodexLaunchSettings{ModelReasoningEffort: "ultra"}, ReadFile: read}, want: "max", source: "launch -c"},
		{name: "explicit selected profile", input: EffortInput{Home: home, CWD: project, ProjectTrusted: true, Launch: agenthooks.CodexLaunchSettings{Profile: "explicit"}, ReadFile: read}, want: "high", source: "profile"},
		{name: "top level profile", input: EffortInput{Home: home, CWD: project, ProjectTrusted: true, ReadFile: read}, want: "medium", source: "profile"},
		{name: "project is skipped when a profile is selected but absent", input: EffortInput{Home: home, CWD: project, ProjectTrusted: true, Launch: agenthooks.CodexLaunchSettings{Profile: "missing"}, ReadFile: read}, want: "xhigh", source: "project config"},
		{name: "user fallback", input: EffortInput{Home: home, Launch: agenthooks.CodexLaunchSettings{Profile: "missing"}, ReadFile: read}, want: "low", source: "user config"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveEffort(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if got.Effort != tc.want || got.EffortRaw == "" || got.Source != tc.source || got.EffortApprox == nil || *got.EffortApprox {
				t.Fatalf("ResolveEffort() = %#v; want tier %q from %q", got, tc.want, tc.source)
			}
		})
	}
}

func TestResolveEffortSkipsUserConfigForIgnoreUserConfig(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "repo")
	home := filepath.Join(root, "codex-home")
	files := map[string][]byte{
		filepath.Join(home, "config.toml"):              []byte(`model_reasoning_effort = "high"`),
		filepath.Join(project, ".codex", "config.toml"): []byte(`model_reasoning_effort = "low"`),
	}
	resolution, err := ResolveEffort(EffortInput{
		Home: home, CWD: project, ProjectTrusted: true,
		Launch: agenthooks.CodexLaunchSettings{IgnoreUserConfig: true},
		ReadFile: func(path string) ([]byte, error) {
			value, ok := files[path]
			if !ok {
				return nil, errors.New("not found")
			}
			return value, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Effort != "low" || resolution.EffortRaw != "low" || resolution.Source != "project config" {
		t.Fatalf("ignore-user-config resolution = %#v", resolution)
	}
}

func TestCodexStateUpdateResolvesOnceIncludingUnknown(t *testing.T) {
	first := StateUpdate(EffortInput{Launch: agenthooks.CodexLaunchSettings{ModelReasoningEffort: "future-tier"}}, state.Session{})
	if !first.EffortResolved || first.ThinkingEffort != "" || first.EffortRaw != "future-tier" || first.EffortApprox != nil {
		t.Fatalf("unknown effort update = %#v", first)
	}
	second := StateUpdate(EffortInput{Launch: agenthooks.CodexLaunchSettings{ModelReasoningEffort: "high"}}, state.Session{EffortResolved: true})
	if second.EffortResolved || second.EffortRaw != "" || second.ThinkingEffort != "" {
		t.Fatalf("resolved session repeated effort lookup: %#v", second)
	}
}
