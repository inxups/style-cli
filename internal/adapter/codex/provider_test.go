package codex

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/state"
	"github.com/speakeasy-api/agenthooks"
)

func TestM3A11CodexProviderSourcesAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, user, profile, selector, override, version, want, source string
		known, ignore, explicit                                        bool
		readErr                                                        error
	}{
		{name: "default", known: true, want: "openai", source: "codex-default"},
		{name: "user", known: true, user: `model_provider="custom.ID"`, want: "custom.ID", source: "codex-user"},
		{name: "profile", known: true, user: `model_provider="user"`, profile: `model_provider="selected"`, selector: "work", want: "selected", source: "codex-profile"},
		{name: "missing profile inherits", known: true, user: `model_provider="user"`, selector: "work", want: "user", source: "codex-user"},
		{name: "launch overrides malformed config", known: true, user: "=bad", override: "Azure", want: "Azure", source: "codex-launch"},
		{name: "duplicate launch empty is invalid", known: true, explicit: true, source: "codex-provider-invalid-launch"},
		{name: "endpoint invalid", known: true, override: "https://private.invalid", source: "codex-provider-invalid-launch"},
		{name: "reserved unknown", known: true, override: "unknown", source: "codex-provider-invalid-launch"},
		{name: "missing ancestry", user: `model_provider="openai"`, source: "codex-provider-ancestry-unavailable"},
		{name: "ignored config", known: true, ignore: true, source: "codex-provider-unreplayable"},
		{name: "explicit override with ignore", known: true, ignore: true, override: "custom", want: "custom", source: "codex-launch"},
		{name: "older version", known: true, version: "0.150.0", source: "codex-provider-version-unavailable"},
		{name: "unknown version", known: true, version: "unknown", source: "codex-provider-version-unavailable"},
		{name: "launch known older version", known: true, version: "0.150.0", override: "local", want: "local", source: "codex-launch"},
		{name: "malformed", known: true, user: "[", source: "codex-provider-config-unavailable"},
		{name: "wrong field type", known: true, user: "model_provider=12", source: "codex-provider-config-unavailable"},
		{name: "empty ID", known: true, user: `model_provider=""`, source: "codex-provider-config-unavailable"},
		{name: "URL ID", known: true, user: `model_provider="https://secret.invalid"`, source: "codex-provider-config-unavailable"},
		{name: "long ID", known: true, user: `model_provider="` + strings.Repeat("x", 65) + `"`, source: "codex-provider-config-unavailable"},
		{name: "bad profile traversal", known: true, selector: "../secret", source: "codex-provider-config-unavailable"},
		{name: "bad profile absolute", known: true, selector: "/secret", source: "codex-provider-config-unavailable"},
		{name: "legacy selector", known: true, user: `profile="work"`, source: "codex-provider-config-unavailable"},
		{name: "legacy table", known: true, user: "[profiles.work]\nmodel_provider=\"old\"", selector: "work", source: "codex-provider-config-unavailable"},
		{name: "profile error cannot fall through", known: true, user: `model_provider="user"`, selector: "work", profile: "[", source: "codex-provider-config-unavailable"},
		{name: "oversize", known: true, user: strings.Repeat("#", (4<<20)+1), source: "codex-provider-config-unavailable"},
		{name: "permission", known: true, readErr: os.ErrPermission, source: "codex-provider-config-unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version := tc.version
			if version == "" {
				version = "0.159.3"
			}
			calls := 0
			reader := func(path string) ([]byte, error) {
				calls++
				if strings.Contains(path, ".codex/config.toml") || strings.Contains(path, "repo") {
					t.Fatal("provider read project config")
				}
				if tc.readErr != nil {
					return nil, tc.readErr
				}
				switch filepath.Base(path) {
				case "config.toml":
					if tc.user != "" {
						return []byte(tc.user), nil
					}
				case "work.config.toml":
					if tc.profile != "" {
						return []byte(tc.profile), nil
					}
				}
				return nil, os.ErrNotExist
			}
			got := ResolveProvider(ProviderInput{EffortInput: EffortInput{Home: "/provider-home", CWD: "/repo", ProjectTrusted: true, HarnessVersion: version, ReadFile: reader, Launch: agenthooks.CodexLaunchSettings{Profile: tc.selector, IgnoreUserConfig: tc.ignore, ModelProvider: tc.override, ModelProviderSet: tc.explicit}}, LaunchKnown: tc.known})
			if got.ID != tc.want || got.Source != tc.source {
				t.Fatalf("%+v", got)
			}
			if (!tc.known || tc.override != "" || tc.explicit || tc.ignore || tc.version != "") && calls != 0 {
				t.Fatal("unnecessary config read", calls)
			}
		})
	}
}
func TestM3A11CodexProviderCacheOptInAndOffline(t *testing.T) {
	e := NewEnricher()
	e.HarnessVersion = "0.159.3"
	calls := 0
	e.LaunchSettings = func(string) (agenthooks.CodexLaunchSettings, bool) {
		calls++
		return agenthooks.CodexLaunchSettings{ModelProvider: "local"}, true
	}
	e.ReadFile = func(string) ([]byte, error) { return nil, errors.New("must not read") }
	event := &agenthooks.Event{Provider: agenthooks.ProviderCodex, Kind: agenthooks.KindPromptSubmitted, Session: agenthooks.SessionInfo{ID: "session"}, Raw: json.RawMessage(`{"hook_event_name":"UserPromptSubmit","session_id":"session","prompt":"fixture"}`)}
	prior := state.Session{EffortResolved: true}
	if u := e.StateUpdate(event, nil, prior); u.ProviderResolved || calls != 0 {
		t.Fatal(u, calls)
	}
	e.ConfigureProvider(true)
	first := e.StateUpdate(event, nil, prior)
	if first.ModelProvider != "local" || !first.ProviderResolved || calls != 1 {
		t.Fatal(first, calls)
	}
	prior.ProviderResolved = true
	prior.ModelProvider = "local"
	prior.ProviderSource = "codex-launch"
	if u := e.StateUpdate(event, nil, prior); u.ProviderResolved || calls != 1 {
		t.Fatal(u, calls)
	}
	input, _, err := e.Enrich(event, &agenthooks.PromptEvent{Event: *event, Prompt: "fixture"}, prior, state.Scope{})
	if err != nil || input.Variables["provider"] != "local" || input.Points[0].When.ProviderSource != "codex-launch" {
		t.Fatal(input, err)
	}
	e.ProviderOffline = true
	if u := e.StateUpdate(event, nil, prior); u.ModelProvider != "" || u.ProviderSource != "offline-unavailable" || calls != 1 {
		t.Fatal(u, calls)
	}
	id := "offline"
	e.OfflineProvider = &id
	if u := e.StateUpdate(event, nil, prior); u.ModelProvider != "offline" || u.ProviderSource != "provided" {
		t.Fatal(u)
	}
	e.ProviderOffline = false
	_ = e.StateUpdate(event, nil, state.Session{EffortResolved: true})
	if calls != 2 {
		t.Fatal("new session did not resolve", calls)
	}
	event.Kind = agenthooks.KindSessionStart
	_ = e.StateUpdate(event, &agenthooks.SessionStartEvent{Event: *event, Source: "startup"}, prior)
	if calls != 3 {
		t.Fatal("startup reused stale provider", calls)
	}

}
func TestM3A11CodexProviderBoundedRegularConfig(t *testing.T) {
	home := t.TempDir()
	file := filepath.Join(home, "config.toml")
	run := func() ProviderResolution {
		return ResolveProvider(ProviderInput{EffortInput: EffortInput{Home: home, HarnessVersion: "0.159.3"}, LaunchKnown: true})
	}
	if err := os.WriteFile(file, []byte(`model_provider="local"`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := run(); got.ID != "local" {
		t.Fatal(got)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "other")
	if err := os.WriteFile(target, []byte(`model_provider="secret"`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, file); err != nil {
		t.Fatal(err)
	}
	if got := run(); got.ID != "" || got.Source != "codex-provider-config-unavailable" {
		t.Fatal(got)
	}
}
