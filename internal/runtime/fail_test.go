package runtime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/internal/adapter"
	claudeadapter "github.com/inxups/style-cli/internal/adapter/claude"
	codexadapter "github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/speakeasy-api/agenthooks"
)

// Handler timeouts can return while an uncooperative handler finishes logging.
type a14LogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *a14LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}
func (b *a14LogBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buffer.String() }
func (b *a14LogBuffer) Reset()         { b.mu.Lock(); defer b.mu.Unlock(); b.buffer.Reset() }

type a14FaultAdapter struct {
	adapter.RuntimeEnricher
	fault string
}

func (a a14FaultAdapter) FailurePoints(event *agenthooks.Event) []string {
	return a.RuntimeEnricher.(interface {
		FailurePoints(*agenthooks.Event) []string
	}).FailurePoints(event)
}
func (a a14FaultAdapter) Enrich(event *agenthooks.Event, typed any, session state.Session, scope state.Scope) (engine.Input, tokens.InputLimit, error) {
	switch a.fault {
	case "panic":
		panic("controlled handler failure")
	case "enrich":
		return engine.Input{}, tokens.InputLimit{}, errors.New("sensitive-enrichment-canary")
	case "timeout":
		time.Sleep(30 * time.Millisecond)
		return engine.Input{}, tokens.InputLimit{}, errors.New("completed after internal deadline")
	}
	return a.RuntimeEnricher.Enrich(event, typed, session, scope)
}
func a14Runtime(t *testing.T, provider string, closed bool, point, fault string) (*Runtime, string, string) {
	t.Helper()
	root := t.TempDir()
	store, err := state.New(root)
	if err != nil {
		t.Fatal(err)
	}
	entry := runtimeTestEntry("pkg/guard", point, "event", "inject", staticBody("guarded-context"))
	entry.When = when.Rule{Agent: []string{"main"}, Event: point}
	if point == "skill.explicit" {
		entry.Target = "skill"
		entry.Match = &compile.Match{Skill: []string{"fixture"}}
	}
	if closed {
		entry.Fail = "closed"
	}
	if fault == "empty-block" {
		entry.Action = "block"
		entry.Body = staticBody("")
	}
	if fault == "normal-miss" {
		entry.When = when.Rule{Agent: []string{"main"}, Model: []string{"never-matches"}, Event: point}
	}
	index := runtimeTestIndex(map[string][]compile.Entry{point: {entry}})
	index.Format = 7
	index.Harnesses = map[string]map[string][]compile.Entry{provider: {point: {entry}}}
	core, err := engine.New(index, store)
	if err != nil {
		t.Fatal(err)
	}
	var enrich adapter.RuntimeEnricher
	if provider == "codex" {
		e := codexadapter.NewEnricher()
		e.Offline = true
		enrich = e
	} else {
		enrich = claudeadapter.NewEnricher()
	}
	r, err := New(core, store, a14FaultAdapter{enrich, fault}, agenthooks.Provider(provider), false)
	if err != nil {
		t.Fatal(err)
	}
	payload := claudeToolPrePayload
	if provider == "codex" {
		b, err := os.ReadFile("../../agenthooks/agenthookstest/fixtures/codex/pre_tool_use.json")
		if err != nil {
			t.Fatal(err)
		}
		payload = string(b)
	}
	return r, root, payload
}
func TestM3A14RuleFailuresAndRecovery(t *testing.T) {
	for _, provider := range []string{"codex", "claude-code"} {
		for _, closed := range []bool{false, true} {
			for _, fault := range []string{"state", "enrich", "panic", "timeout", "empty-block", "normal-miss"} {
				name := provider + "/" + map[bool]string{false: "open", true: "closed"}[closed] + "/" + fault
				t.Run(name, func(t *testing.T) {
					r, root, payload := a14Runtime(t, provider, closed, "tool.pre", fault)
					if fault == "state" {
						if err := os.RemoveAll(root); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(root, []byte("poison state root"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					ctx := context.Background()
					if fault == "timeout" {
						var cancel context.CancelFunc
						ctx, cancel = context.WithTimeout(ctx, 5*time.Millisecond)
						defer cancel()
					}
					var out bytes.Buffer
					var logs a14LogBuffer
					code := r.Run(ctx, []string{"agenthooks", "run", "--provider=" + provider}, strings.NewReader(payload), &out, &logs)
					if fault == "timeout" {
						time.Sleep(40 * time.Millisecond)
					}
					wantDeny := closed && fault != "normal-miss"
					if code != 0 || strings.Contains(out.String(), `"deny"`) != wantDeny || strings.Contains(out.String(), "sensitive-enrichment-canary") {
						t.Fatal(code, out.String(), logs.String())
					}
					if fault == "state" {
						if err := os.Remove(root); err != nil {
							t.Fatal(err)
						}
						if err := os.MkdirAll(root, 0700); err != nil {
							t.Fatal(err)
						}
					}
					r.Adapter = a14FaultAdapter{r.Adapter.(a14FaultAdapter).RuntimeEnricher, ""}
					if fault == "empty-block" || fault == "normal-miss" {
						return
					}
					out.Reset()
					logs.Reset()
					if code := r.Run(context.Background(), []string{"agenthooks", "run", "--provider=" + provider}, strings.NewReader(payload), &out, &logs); code != 0 || !strings.Contains(out.String(), "guarded-context") || strings.Contains(out.String(), `"deny"`) {
						t.Fatal("recovery", code, out.String(), logs.String())
					}
				})
			}
		}
	}
}
func TestM3A14EventScopeAndUnreadableIndexBoundary(t *testing.T) {
	for _, provider := range []string{"codex", "claude-code"} {
		for _, scenario := range []string{"unrelated-point", "mixed-miss-fault", "missing-index", "corrupt-index", "decode-failure", "installation-closed"} {
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				point := "tool.pre"
				if scenario == "unrelated-point" {
					point = "prompt.submitted"
				}
				r, root, payload := a14Runtime(t, provider, true, point, "state")
				if scenario == "mixed-miss-fault" {
					r, root, payload = a14Runtime(t, provider, true, point, "normal-miss")
				}
				if scenario == "decode-failure" {
					payload = "{"
				} else if scenario == "missing-index" || scenario == "corrupt-index" || scenario == "installation-closed" {
					path := filepath.Join(t.TempDir(), "index.json")
					if scenario == "corrupt-index" {
						os.WriteFile(path, []byte("{"), 0600)
					}
					closed := scenario == "installation-closed"
					if closed && provider != "claude-code" {
						return
					}
					var err error
					r, err = Load(path, root, r.Adapter, agenthooks.Provider(provider), closed)
					if err != nil {
						t.Fatal(err)
					}
				} else {
					os.RemoveAll(root)
					os.WriteFile(root, []byte("state failure"), 0600)
				}
				var out bytes.Buffer
				var logs a14LogBuffer
				code := r.Run(context.Background(), []string{"agenthooks", "run", "--provider=" + provider}, strings.NewReader(payload), &out, &logs)
				want := scenario == "mixed-miss-fault" || scenario == "installation-closed"
				if strings.Contains(out.String(), `"deny"`) != want {
					t.Fatal(code, out.String(), logs.String())
				}
				if scenario == "installation-closed" && code != 1 {
					t.Fatal(code)
				}
			})
		}
	}
}

func TestM3A14PromptFailureCodecsAndMixedRules(t *testing.T) {
	for _, tc := range []struct{ provider, native, point string }{
		{"codex", "UserPromptSubmit", "prompt.submitted"},
		{"codex", "UserPromptSubmit", "skill.explicit"},
		{"claude-code", "UserPromptSubmit", "prompt.submitted"},
		{"claude-code", "UserPromptExpansion", "skill.explicit"},
	} {
		t.Run(tc.provider+"/"+tc.point, func(t *testing.T) {
			r, _, _ := a14Runtime(t, tc.provider, true, tc.point, "enrich")
			payload := `{"session_id":"prompt-failure","hook_event_name":"` + tc.native + `","prompt":"hello","command_name":"fixture","expansion_type":"slash_command"}`
			var out, logs bytes.Buffer
			code := r.Run(context.Background(), []string{"agenthooks", "run", "--provider=" + tc.provider}, strings.NewReader(payload), &out, &logs)
			if code != 0 || !strings.Contains(out.String(), `"block"`) {
				t.Fatal(code, out.String(), logs.String())
			}
			wantSuppress := tc.provider == "claude-code" && tc.point == "prompt.submitted"
			if strings.Contains(out.String(), `"suppressOriginalPrompt":true`) != wantSuppress {
				t.Fatal(out.String())
			}
		})
	}
	for _, provider := range []string{"codex", "claude-code"} {
		t.Run(provider+"/mixed-open-error", func(t *testing.T) {
			r, _, payload := a14Runtime(t, provider, true, "tool.pre", "normal-miss")
			closed := runtimeTestEntry("pkg/guard", "tool.pre", "event", "inject", staticBody("unused"))
			closed.When = when.Rule{Agent: []string{"main"}, Event: "tool.pre", Model: []string{"never-matches"}}
			closed.Fail = "closed"
			broken := closed
			broken.Rule = "pkg/broken"
			broken.Entry = "pkg/broken@tool.pre"
			broken.Fail = ""
			broken.Action = "block"
			broken.When = when.Rule{Agent: []string{"main"}, Event: "tool.pre"}
			broken.Body = staticBody("")
			index := runtimeTestIndex(nil)
			index.Format = 7
			index.Harnesses = map[string]map[string][]compile.Entry{provider: {"tool.pre": {closed, broken}}}
			var err error
			r.Engine, err = engine.New(index, r.State)
			if err != nil {
				t.Fatal(err)
			}
			r.Adapter = a14FaultAdapter{r.Adapter.(a14FaultAdapter).RuntimeEnricher, ""}
			var out, logs bytes.Buffer
			code := r.Run(context.Background(), []string{"agenthooks", "run", "--provider=" + provider}, strings.NewReader(payload), &out, &logs)
			if code != 0 || !strings.Contains(out.String(), `"deny"`) {
				t.Fatal(code, out.String(), logs.String())
			}
		})
	}
}
