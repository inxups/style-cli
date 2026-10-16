package codex

import (
	"errors"
	"os"
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

func TestFileProfilesMatchCodex159Configuration(t *testing.T) {
	root := t.TempDir()
	home, project := filepath.Join(root, "home"), filepath.Join(root, "project")
	for _, tc := range []struct {
		name, user, profile, override string
		trusted, ignore               bool
		want, source                  string
		bad                           bool
	}{
		{name: "selected file", user: `model_reasoning_effort="low"`, profile: `model_reasoning_effort="high"`, want: "high", source: "profile file"},
		{name: "project overrides file", user: `model_reasoning_effort="low"`, profile: `model_reasoning_effort="high"`, trusted: true, want: "xhigh", source: "project config"},
		{name: "launch overrides project and file", profile: `model_reasoning_effort="high"`, override: "medium", trusted: true, want: "medium", source: "launch -c"},
		{name: "empty file inherits user", user: `model_reasoning_effort="low"`, profile: "# empty", want: "low", source: "user config"},
		{name: "ignore user still loads selected file", user: `model_reasoning_effort="low"`, profile: `model_reasoning_effort="high"`, ignore: true, want: "high", source: "profile file"},
		{name: "raw unknown is preserved", profile: `model_reasoning_effort="future-tier"`, source: "profile file"},
		{name: "missing file inherits user", user: `model_reasoning_effort="low"`, want: "low", source: "user config"},
		{name: "malformed file is an error", profile: "= not toml", bad: true},
		{name: "legacy default rejected", user: `profile="probe"`, profile: `model_reasoning_effort="high"`, bad: true},
		{name: "legacy selected table rejected", user: "[profiles.probe]\nmodel_reasoning_effort=\"medium\"", profile: `model_reasoning_effort="high"`, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string][]byte{filepath.Join(home, "config.toml"): []byte(tc.user), filepath.Join(project, ".codex", "config.toml"): []byte(`model_reasoning_effort="xhigh"`)}
			if tc.profile != "" {
				files[filepath.Join(home, "probe.config.toml")] = []byte(tc.profile)
			}
			input := EffortInput{Home: home, CWD: project, HarnessVersion: "0.159.3", ProjectTrusted: tc.trusted,
				Launch: agenthooks.CodexLaunchSettings{Profile: "probe", IgnoreUserConfig: tc.ignore, ModelReasoningEffort: tc.override},
				ReadFile: func(path string) ([]byte, error) {
					value, ok := files[path]
					if !ok {
						return nil, os.ErrNotExist
					}
					return value, nil
				}}
			got, err := ResolveEffort(input)
			if tc.bad {
				if err == nil {
					t.Fatalf("invalid configuration accepted: %+v", got)
				}
				return
			}
			if err != nil || got.Effort != tc.want || got.Source != tc.source {
				t.Fatalf("resolution=%+v err=%v; want %s/%s", got, err, tc.want, tc.source)
			}
			if tc.name == "raw unknown is preserved" && (got.EffortRaw != "future-tier" || got.EffortApprox != nil) {
				t.Fatalf("unknown=%+v", got)
			}
		})
	}
	for _, profile := range []string{"../escape", "/absolute", `sub\profile`, ".", ".."} {
		if _, err := ResolveEffort(EffortInput{Home: home, HarnessVersion: "0.159.3", Launch: agenthooks.CodexLaunchSettings{Profile: profile}}); err == nil {
			t.Fatalf("accepted unsafe profile %q", profile)
		}
	}
}
