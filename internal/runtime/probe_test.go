package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/internal/probe"
	"github.com/speakeasy-api/agenthooks"
)

type a2RuntimeFile struct {
	Data string
	Mode fs.FileMode
}

func a2RuntimeState(t *testing.T, root string) map[string]a2RuntimeFile {
	t.Helper()
	result := map[string]a2RuntimeFile{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// State's last-change wall clock is the only intentionally variable
		// value. Stats timestamps are fixed by Runtime.Now and compared exactly.
		if strings.HasSuffix(path, ".json") {
			var object map[string]any
			if err = json.Unmarshal(data, &object); err != nil {
				return err
			}
			delete(object, "updated_at")
			data, err = json.Marshal(object)
			if err != nil {
				return err
			}
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[name] = a2RuntimeFile{string(data), info.Mode()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestM3A2OrdinaryRunnerObservationParity(t *testing.T) {
	for _, provider := range []agenthooks.Provider{agenthooks.ProviderCodex, agenthooks.ProviderClaudeCode} {
		for _, mode := range []string{"inject", "block", "warn", "no-match", "repeat", "load-error", "decode-error", "fail-closed"} {
			if mode == "fail-closed" && provider != agenthooks.ProviderClaudeCode {
				continue
			}
			t.Run(string(provider)+"/"+mode, func(t *testing.T) {
				var expected []string
				var expectedState map[string]a2RuntimeFile
				for _, observation := range []string{"none", "absent", "active", "malformed"} {
					home := t.TempDir()
					canonicalHome, err := filepath.EvalSymlinks(home)
					if err != nil {
						t.Fatal(err)
					}
					root := filepath.Join(home, "state")
					if err = os.Mkdir(root, 0o700); err != nil {
						t.Fatal(err)
					}
					store, err := state.New(root)
					if err != nil {
						t.Fatal(err)
					}
					point, event, fixture, action := "tool.pre", "PreToolUse", "pre_tool_use.json", "inject"
					if mode == "block" {
						action = "block"
					}
					if mode == "warn" {
						point, event, fixture, action = "tool.post", "PostToolUse", "post_tool_use.json", "warn"
					}
					if mode == "repeat" {
						point, event, fixture = "prompt.submitted", "UserPromptSubmit", "user_prompt_submit.json"
					}
					entry := runtimeTestEntry("pkg/observed", point, "event", action, staticBody("Probe parity body."))
					if mode == "warn" {
						body := staticBody("Probe model body.")
						entry.ModelBody = &body
					}
					entry.When = when.Rule{Agent: []string{"main"}}
					entries := map[string][]compile.Entry{point: {entry}}
					if mode == "no-match" {
						entries = map[string][]compile.Entry{}
					}
					core, err := engine.New(runtimeTestIndex(entries), store)
					if err != nil {
						t.Fatal(err)
					}
					runtime, err := New(core, store, testEnricher{}, provider, mode == "fail-closed")
					if err != nil {
						t.Fatal(err)
					}
					if mode == "load-error" || mode == "fail-closed" {
						runtime.Engine = nil
					}
					runtime.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
					runtime.Now = func() time.Time { return time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC) }
					if observation == "active" {
						value := probe.HashString("offline-binding")
						binding := probe.Binding{Harness: string(provider), Route: "fixture-user", HomeSHA256: probe.HashString(canonicalHome), HarnessHomeSHA256: probe.HashString(canonicalHome), RouteRootSHA256: value, RuntimeID: value, RuntimeSHA256: value, HarnessID: value, HarnessSHA256: value, LockSHA256: value, ConfigSHA256: value, Events: []string{event}}
						session, beginErr := probe.Begin(home, []probe.Binding{binding}, time.Minute, probe.Options{Lookup: func(pid int) (agenthooks.ProcessIdentity, error) {
							return agenthooks.ProcessIdentity{PID: pid, Birth: "parity-owner"}, nil
						}})
						if beginErr != nil {
							t.Fatal(beginErr)
						}
						defer session.Close()
						defer func() {
							if cleanup := session.Cleanup(); !cleanup.Complete {
								t.Errorf("cleanup: %+v", cleanup)
							}
						}()
					} else if observation == "malformed" {
						if err = os.Mkdir(filepath.Join(home, "probes"), 0o700); err != nil {
							t.Fatal(err)
						}
						if err = os.WriteFile(filepath.Join(home, "probes", string(provider)+".lease.json"), []byte(`{"bad":"lease"}`), 0o600); err != nil {
							t.Fatal(err)
						}
					}
					if observation != "none" {
						runtime.Observer = probe.NewObserver(home, home, string(provider), "")
					}
					folder := "codex"
					if provider == agenthooks.ProviderClaudeCode {
						folder = "claude"
					}
					payload, err := os.ReadFile(filepath.Join("../../agenthooks/agenthookstest/fixtures", folder, fixture))
					if err != nil {
						t.Fatal(err)
					}
					if mode == "decode-error" {
						payload = []byte(`{"broken"`)
					}
					actual := []string{}
					for invocation := 0; invocation < 2; invocation++ {
						var out, errOut bytes.Buffer
						code := runtime.Run(context.Background(), []string{"agenthooks", "run", "--provider=" + string(provider)}, bytes.NewReader(payload), &out, &errOut, agenthooks.WithoutMCPResolution(), agenthooks.WithoutDedup())
						actual = append(actual, string(rune('0'+code)), out.String(), errOut.String())
					}
					current := a2RuntimeState(t, root)
					if observation == "none" {
						expected, expectedState = actual, current
					} else if !reflect.DeepEqual(actual, expected) || !reflect.DeepEqual(current, expectedState) {
						t.Fatalf("%s altered wire/exit/state/stats: actual=%q expected=%q\nstate=%v expected=%v", observation, actual, expected, current, expectedState)
					}
				}
			})
		}
	}
}
