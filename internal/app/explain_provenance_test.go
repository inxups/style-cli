package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/speakeasy-api/agenthooks/privatefs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
)

func a1AssertProvenance(t *testing.T, home string) compile.Provenance {
	t.Helper()
	data := readSyncFile(t, filepath.Join(home, "build", "explain.json"))
	indexBytes := readSyncFile(t, filepath.Join(home, "build", "index.json"))
	var index compile.Index
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		t.Fatal(err)
	}
	provenance, err := compile.DecodeProvenance(data, indexBytes, index)
	if err != nil {
		t.Fatalf("provenance %s %v", data, err)
	}
	var document map[string]json.RawMessage
	if json.Unmarshal(data, &document) != nil || len(document) != 3 || document["harnesses"] == nil || document["rules"] != nil {
		t.Fatal("provenance differs from the D1 top-level contract")
	}
	info, err := os.Stat(filepath.Join(home, "build", "explain.json"))
	if err != nil || privatefs.CheckPath(filepath.Join(home, "build", "explain.json")) != nil {
		t.Fatalf("private provenance %v %v", info, err)
	}
	if strings.Contains(string(data), "Shared denial.") || strings.Contains(string(data), "command\"") || strings.Contains(string(data), "Shared prompt instructions.") {
		t.Fatal("provenance leaked rule content")
	}
	return provenance
}

type a1CancelAfterBuild struct {
	context.Context
	cancel  context.CancelFunc
	before  map[string][]byte
	reached bool
}

func (ctx *a1CancelAfterBuild) Err() error {
	if err := ctx.Context.Err(); err != nil {
		return err
	}
	for path, before := range ctx.before {
		after, err := os.ReadFile(path)
		if err != nil || bytes.Equal(before, after) {
			return nil
		}
	}
	ctx.reached = true
	ctx.cancel()
	return ctx.Context.Err()
}

func TestM3A1ProvenanceTransactionRollback(t *testing.T) {
	service, home, _, source, _ := nativeInstallFixture(t, "codex-user")
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a6Manifest("first", 50))
	request := InstallRequest{Home: home, Source: source, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}, Yes: true}
	if _, err := service.Install(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(home, "build", "index.json"), filepath.Join(home, "build", "explain.json")}
	baseline := map[string][]byte{}
	for _, path := range paths {
		baseline[path] = readSyncFile(t, path)
	}
	request.Source = writeInstallPackage(t, filepath.Join(t.TempDir(), "second"), a6Manifest("second", 50))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fault := &a1CancelAfterBuild{Context: ctx, cancel: cancel, before: baseline}
	if _, err := service.Install(fault, request); !errors.Is(err, context.Canceled) || !fault.reached {
		t.Fatalf("failure did not occur after both build files were applied: %v reached=%t", err, fault.reached)
	}
	for _, path := range paths {
		if !bytes.Equal(readSyncFile(t, path), baseline[path]) {
			t.Fatalf("transaction did not restore %s", path)
		}
		if info, err := os.Stat(path); err != nil || privatefs.CheckPath(path) != nil {
			t.Fatalf("rollback changed private mode: %v %v", info, err)
		}
	}
	a1AssertProvenance(t, home)
	if _, err := os.Stat(filepath.Join(home, "packages", "second")); !os.IsNotExist(err) {
		t.Fatal("failed transaction retained the second package")
	}
}
func TestM3A1ProvenanceDedupAndLifecycle(t *testing.T) {
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user"} {
		t.Run(route, func(t *testing.T) {
			service, home, _, source, _ := nativeInstallFixture(t, route)
			harness := "codex"
			if route == "claude-user" {
				harness = "claude-code"
			}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a6Manifest("first", 50))
			request := InstallRequest{Home: home, Source: source, Harnesses: []string{harness}, Routes: map[string]string{harness: route}, Yes: true}
			if _, err := service.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			a1AssertProvenance(t, home)
			second := writeInstallPackage(t, filepath.Join(filepath.Dir(source), "second"), a6Manifest("second", 50))
			request.Source = second
			if _, err := service.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			p := a1AssertProvenance(t, home)
			folded := 0
			for _, row := range p.Harnesses[harness] {
				if row.Status == "compile-deduplicated" {
					folded++
					if !strings.HasPrefix(row.DeduplicatedInto, "first/") {
						t.Fatalf("wrong canonical %+v", row)
					}
				}
			}
			if folded == 0 {
				t.Fatal("folded declarations missing")
			}
			payload := filepath.Join(t.TempDir(), "payload.json")
			writeSyncFile(t, payload, `{"hook_event_name":"PreToolUse","session_id":"s","tool_name":"Bash","tool_input":{"command":"harmless"}}`)
			explain := ExplainRequest{Harness: harness, Event: "PreToolUse", Payload: payload, Index: filepath.Join(home, "build", "index.json")}
			before := snapshotStatsTree(t, home)
			result, err := service.Explain(context.Background(), explain)
			if err != nil || result.CompileProvenance.Status != "matched" || len(result.CompileProvenance.Static) == 0 {
				t.Fatalf("result %+v %v", result, err)
			}
			for _, row := range result.Entries {
				if strings.HasPrefix(row.Rule, "second/") {
					t.Fatal("folded runtime entry invented")
				}
			}
			if !reflect.DeepEqual(before, snapshotStatsTree(t, home)) {
				t.Fatal("read-only provenance changed installation")
			}
			if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, Name: "first", Yes: true}); err != nil {
				t.Fatal(err)
			}
			p = a1AssertProvenance(t, home)
			for _, row := range p.Harnesses[harness] {
				if row.Status == "compile-deduplicated" || strings.HasPrefix(row.Rule, "first/") {
					t.Fatalf("stale fold %+v", row)
				}
			}
			if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(home, "build", "explain.json")); !os.IsNotExist(err) {
				t.Fatal("last uninstall retained provenance")
			}
		})
	}
}
func TestM3A1ProvenanceSyncTrustAndLegacy(t *testing.T) {
	for _, link := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "linked"}[link], func(t *testing.T) {
			service, home, source, _ := syncFixture(t, link)
			if link {
				if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true}); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(home, "build", "explain.json")
			a1AssertProvenance(t, home)
			// A legacy installation has no sidecar. Read-only operations must not create it.
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			payload := filepath.Join(t.TempDir(), "payload.json")
			writeSyncFile(t, payload, `{"hook_event_name":"PreToolUse","session_id":"s","tool_name":"Bash"}`)
			request := ExplainRequest{Harness: "codex", Event: "PreToolUse", Payload: payload, Index: filepath.Join(home, "build", "index.json")}
			result, err := service.Explain(context.Background(), request)
			if err != nil || result.CompileProvenance.Coverage != "index-only" {
				t.Fatalf("legacy %+v %v", result, err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("explain backfilled sidecar")
			}
			writeSyncFile(t, filepath.Join(source, "rules", "shell.md"), strings.ReplaceAll(syncRule, "Original", "Changed"))
			if _, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil {
				t.Fatal(err)
			}
			if !link {
				if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true}); err != nil {
					t.Fatal(err)
				}
			}
			a1AssertProvenance(t, home)
		})
	}
}
func TestM3A1ProvenanceFallbackAndNativeMetadata(t *testing.T) {
	requirePOSIXFixture(t, "author native metadata fixture")
	service, home, _, source, _ := nativeInstallFixture(t, "claude-user")
	if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{"claude-code"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(t.TempDir(), "payload.json")
	writeSyncFile(t, payload, `{"hook_event_name":"PreToolUse","session_id":"s","tool_name":"Bash"}`)
	request := ExplainRequest{Harness: "claude-code", Event: "PreToolUse", Payload: payload, Index: filepath.Join(home, "build", "index.json")}
	result, err := service.Explain(context.Background(), request)
	if err != nil || result.CompileProvenance.Status != "matched" || len(result.CompileProvenance.Native) == 0 {
		t.Fatalf("native %+v %v", result, err)
	}
	for _, row := range result.CompileProvenance.Native {
		if row.Status != "native-not-executed" {
			t.Fatalf("invented native execution %+v", row)
		}
	}
	path := filepath.Join(home, "build", "explain.json")
	original := readSyncFile(t, path)
	for _, mode := range []string{"missing", "invalid", "stale", "symlink", "oversized", "missing-rules", "wrong-harness"} {
		t.Run(mode, func(t *testing.T) {
			_ = os.Remove(path)
			switch mode {
			case "missing":
			case "invalid":
				writeSyncFile(t, path, `{"format":1,"command":"secret command"}`)
			case "stale":
				var p compile.Provenance
				_ = json.Unmarshal(original, &p)
				p.IndexSHA256 = a1Hash
				data, _ := json.Marshal(p)
				writeSyncFile(t, path, string(data))
			case "missing-rules", "wrong-harness":
				var p compile.Provenance
				_ = json.Unmarshal(original, &p)
				if mode == "missing-rules" {
					p.Harnesses = map[string][]compile.ProvenanceRule{}
				} else {
					p.Harnesses["claude-code"][0].Harness = "codex"
				}
				data, _ := json.Marshal(p)
				writeSyncFile(t, path, string(data))
			case "symlink":
				target := filepath.Join(t.TempDir(), "target")
				writeSyncFile(t, target, string(original))
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				file, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := file.Truncate(16<<20 + 1); err != nil {
					t.Fatal(err)
				}
				_ = file.Close()
			}
			result, err := service.Explain(context.Background(), request)
			if err != nil || result.CompileProvenance.Coverage != "index-only" || len(result.CompileProvenance.Native) != 0 {
				t.Fatalf("fallback %+v %v", result, err)
			}
		})
	}
}

func TestM3A1ProvenanceRecapturesTransactionWindow(t *testing.T) {
	service, home, _, source, _ := nativeInstallFixture(t, "codex-user")
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a6Manifest("first", 50))
	install := InstallRequest{Home: home, Source: source, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}, Yes: true}
	if _, err := service.Install(context.Background(), install); err != nil {
		t.Fatal(err)
	}
	indexPath, provenancePath := filepath.Join(home, "build", "index.json"), filepath.Join(home, "build", "explain.json")
	oldIndex, oldProvenance := readSyncFile(t, indexPath), readSyncFile(t, provenancePath)
	install.Source = writeInstallPackage(t, filepath.Join(t.TempDir(), "second"), a6Manifest("second", 50))
	if _, err := service.Install(context.Background(), install); err != nil {
		t.Fatal(err)
	}
	newIndex, newProvenance := readSyncFile(t, indexPath), readSyncFile(t, provenancePath)
	writeSyncFile(t, indexPath, string(oldIndex))
	writeSyncFile(t, provenancePath, string(oldProvenance))
	payload := filepath.Join(t.TempDir(), "payload.json")
	writeSyncFile(t, payload, `{"hook_event_name":"PreToolUse","session_id":"s","tool_name":"Bash"}`)
	request := ExplainRequest{Harness: "codex", Event: "PreToolUse", Payload: payload, Index: indexPath, StateDir: t.TempDir()}
	ops := defaultExplainOps()
	calls := 0
	ops.snapshot = func(ctx context.Context, source, destination, session, agent string) (state.Snapshot, error) {
		calls++
		// A real transaction may replace both files after index capture and
		// before provenance capture. Force that exact interleaving once.
		writeSyncFile(t, indexPath, string(newIndex))
		writeSyncFile(t, provenancePath, string(newProvenance))
		return state.CopySnapshot(ctx, source, destination, session, agent)
	}
	result, err := service.explain(context.Background(), request, ops)
	if err != nil || calls != 1 || result.Inputs.IndexSHA256 != explainSHA(newIndex) || result.CompileProvenance.Status != "matched" || result.CompileProvenance.SHA256 != explainSHA(newProvenance) {
		t.Fatalf("mixed transaction generations: %+v calls=%d err=%v", result, calls, err)
	}
}
