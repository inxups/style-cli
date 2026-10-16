package state

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestUpdateMaintainsSessionAndScopeLifecycle(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	approx := false
	started, main, err := store.Update("session-1", "", EventUpdate{
		Source: "startup", Model: "model-a", ThinkingEffort: "high", EffortRaw: "ultra", EffortApprox: &approx, EffortResolved: true, HarnessVersion: "1.2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if main.AgentID != "main" || main.Generation != 0 || main.Epoch != 0 || main.PromptCount != 0 {
		t.Fatalf("startup scope = %#v", main)
	}
	if started.Model != "model-a" || started.ThinkingEffort != "high" || started.EffortRaw != "ultra" || started.EffortApprox == nil || *started.EffortApprox || !started.EffortResolved || started.HarnessVersion != "1.2" {
		t.Fatalf("startup session = %#v", started)
	}

	updated, child, err := store.Update("session-1", "agent_2", EventUpdate{PromptSubmitted: true, Model: "model-b"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Model != "model-b" || updated.ThinkingEffort != "high" || updated.EffortRaw != "ultra" {
		t.Fatalf("empty fields overwrote session facts: %#v", updated)
	}
	if child.AgentID != "agent_2" || child.PromptCount != 1 || child.Generation != 0 {
		t.Fatalf("child prompt scope = %#v", child)
	}
	_, main, err = store.Update("session-1", "main", EventUpdate{Source: "clear"})
	if err != nil {
		t.Fatal(err)
	}
	if main.Generation != 1 || main.Epoch != 0 || main.PromptCount != 0 {
		t.Fatalf("clear scope = %#v", main)
	}
	_, main, err = store.Update("session-1", "main", EventUpdate{Source: "compact"})
	if err != nil {
		t.Fatal(err)
	}
	if main.Generation != 1 || main.Epoch != 1 {
		t.Fatalf("compact scope = %#v", main)
	}
	_, main, err = store.Update("session-1", "main", EventUpdate{Source: "resume"})
	if err != nil {
		t.Fatal(err)
	}
	if main.Generation != 1 || main.Epoch != 1 {
		t.Fatalf("resume changed counters: %#v", main)
	}
}

func TestSessionEffortApproximationRemainsUnknownUntilProvided(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	started, _, err := store.Update("session", "main", EventUpdate{Source: "startup", EffortRaw: "custom", EffortResolved: true})
	if err != nil {
		t.Fatal(err)
	}
	if started.ThinkingEffort != "" || started.EffortApprox != nil || !started.EffortResolved || started.EffortRaw != "custom" {
		t.Fatalf("unknown effort state = %#v", started)
	}
}

func TestReportedUnknownEffortClearsPreviousTier(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	approx := false
	if _, _, err := store.Update("session", "main", EventUpdate{ThinkingEffort: "high", EffortRaw: "high", EffortApprox: &approx, EffortResolved: true}); err != nil {
		t.Fatal(err)
	}
	unknown, _, err := store.Update("session", "main", EventUpdate{EffortRaw: "future-tier", EffortResolved: true})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.ThinkingEffort != "" || unknown.EffortRaw != "future-tier" || unknown.EffortApprox != nil || !unknown.EffortResolved {
		t.Fatalf("unknown raw kept stale tier: %#v", unknown)
	}

	// An update without any effort reading never clears the stored value.
	kept, _, err := store.Update("session", "main", EventUpdate{Model: "model-b"})
	if err != nil {
		t.Fatal(err)
	}
	if kept.EffortRaw != "future-tier" || kept.ThinkingEffort != "" {
		t.Fatalf("empty update overwrote effort: %#v", kept)
	}
	known, _, err := store.Update("session", "main", EventUpdate{ThinkingEffort: "max", EffortRaw: "ultra", EffortApprox: &approx})
	if err != nil {
		t.Fatal(err)
	}
	if known.ThinkingEffort != "max" || known.EffortRaw != "ultra" || known.EffortApprox == nil || *known.EffortApprox {
		t.Fatalf("known raw after unknown = %#v", known)
	}
}

func TestStartupResetsScopeAndReservations(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	_, scope, err := store.Update("s", "main", EventUpdate{PromptSubmitted: true, Source: "compact"})
	if err != nil {
		t.Fatal(err)
	}
	claim, ok, err := store.Claim(ClaimInput{Scope: scope, Repeat: "session", Action: "inject", Event: "prompt.submitted", Rule: "p/r", Body: "text"})
	if err != nil || !ok {
		t.Fatalf("claim before startup = %v, %v", ok, err)
	}
	_ = claim // Startup must clear the marker, not depend on an explicit release.
	_, scope, err = store.Update("s", "main", EventUpdate{Source: "startup"})
	if err != nil {
		t.Fatal(err)
	}
	if scope.Generation != 0 || scope.Epoch != 0 || scope.PromptCount != 0 {
		t.Fatalf("startup did not reset scope: %#v", scope)
	}
	if _, allowed, err := store.Claim(ClaimInput{Scope: scope, Repeat: "session", Action: "inject", Event: "prompt.submitted", Rule: "p/r", Body: "text"}); err != nil || !allowed {
		t.Fatalf("startup left old reservation: allowed=%v, err=%v", allowed, err)
	}
}

func TestStateFilesAreAtomicPrivateAndStrict(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Update("session", "main", EventUpdate{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "session", "session.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("session mode = %o, want 600", info.Mode().Perm())
	}
	for _, directory := range []string{root, filepath.Join(root, "session"), filepath.Join(root, "session", "main")} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("state directory %s mode = %o, want 700", directory, info.Mode().Perm())
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "session"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("atomic write left temporary file %q", entry.Name())
		}
	}
	if err := os.WriteFile(path, []byte(`{"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Update("session", "main", EventUpdate{}); err == nil {
		t.Fatal("malformed state unexpectedly accepted")
	}
}

func TestUnsafeIDsAreHashedBeforePathUse(t *testing.T) {
	for _, value := range []string{"../../escape", "has space", strings.Repeat("x", 65)} {
		path := component(value)
		if path == value || strings.Contains(path, "/") || len(path) > 65 {
			t.Errorf("unsafe component %q became %q", value, path)
		}
	}
	if got := component("safe-ID_123"); got != "safe-ID_123" {
		t.Fatalf("safe component = %q", got)
	}
}

func TestCleanupUsesLastStateModificationAndNeverRunsImplicitly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Update("old", "main", EventUpdate{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Update("new", "main", EventUpdate{}); err != nil {
		t.Fatal(err)
	}
	oldPath, newPath := filepath.Join(root, "old", "session.json"), filepath.Join(root, "new", "session.json")
	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "old", "main", "scope.json"), oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "old", "main"), oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "old"), oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatal(err)
	}
	removed, err := store.Cleanup(DefaultExpiry)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed sessions = %d, want 1", removed)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old state still exists, stat error = %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("recent state was removed: %v", err)
	}
}

func TestRepeatClaimSemanticsAndRelease(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	_, scope, err := store.Update("s", "main", EventUpdate{PromptSubmitted: true})
	if err != nil {
		t.Fatal(err)
	}
	input := ClaimInput{Scope: scope, Repeat: "session", Action: "inject", Event: "prompt.submitted", Rule: "pkg/rule", Body: "same"}
	first, allowed, err := store.Claim(input)
	if err != nil || !allowed || first == nil {
		t.Fatalf("first claim = %#v, %v, %v", first, allowed, err)
	}
	if duplicate, allowed, err := store.Claim(input); err != nil || allowed || duplicate != nil {
		t.Fatalf("duplicate claim = %#v, %v, %v", duplicate, allowed, err)
	}
	if changed, allowed, err := store.Claim(ClaimInput{Scope: scope, Repeat: "session", Action: "inject", Event: "prompt.submitted", Rule: "pkg/rule", Body: "changed"}); err != nil || !allowed || changed == nil {
		t.Fatalf("changed body claim = %#v, %v, %v", changed, allowed, err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("second release = %v", err)
	}
	if _, allowed, err := store.Claim(input); err != nil || !allowed {
		t.Fatalf("claim after release = %v, %v", allowed, err)
	}
	for _, special := range []ClaimInput{
		{Repeat: "every", Action: "warn"},
		{Repeat: "session", Action: "block"},
		{Repeat: "session", Action: "inject", Event: "subagent.start"},
	} {
		if reservation, allowed, err := store.Claim(special); err != nil || !allowed || reservation != nil {
			t.Errorf("non-deduped claim %#v = %#v, %v, %v", special, reservation, allowed, err)
		}
	}
}

func TestTurnAndCompactKeys(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	_, scope, err := store.Update("s", "main", EventUpdate{PromptSubmitted: true})
	if err != nil {
		t.Fatal(err)
	}
	turn := ClaimInput{Scope: scope, Repeat: "turn", Action: "inject", Event: "prompt.submitted", Rule: "r", Body: "b"}
	if _, ok, err := store.Claim(turn); err != nil || !ok {
		t.Fatalf("first fallback turn claim = %v, %v", ok, err)
	}
	if _, ok, err := store.Claim(turn); err != nil || ok {
		t.Fatalf("duplicate fallback turn claim = %v, %v", ok, err)
	}
	turn.Scope.PromptCount++
	if _, ok, err := store.Claim(turn); err != nil || !ok {
		t.Fatalf("next fallback turn claim = %v, %v", ok, err)
	}
	base := ClaimInput{Scope: Scope{SessionID: "s", AgentID: "main", Generation: 2, Epoch: 1}, Repeat: "session", Action: "inject", Event: "session.start", Rule: "r", Body: "b"}
	if _, ok, err := store.Claim(base); err != nil || !ok {
		t.Fatalf("session claim = %v, %v", ok, err)
	}
	base.Scope.Epoch = 3
	if _, ok, err := store.Claim(base); err != nil || ok {
		t.Fatalf("non-reinject compact should stay deduped: %v, %v", ok, err)
	}
	base.ReinjectOnCompact = true
	if _, ok, err := store.Claim(base); err != nil || !ok {
		t.Fatalf("compact reinjection claim = %v, %v", ok, err)
	}
}

func TestClearGenerationInvalidatesSessionRepeatClaims(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	_, scope, err := store.Update("s", "main", EventUpdate{Source: "startup"})
	if err != nil {
		t.Fatal(err)
	}
	input := ClaimInput{Scope: scope, Repeat: "session", Action: "inject", Event: "prompt.submitted", Rule: "r", Body: "b"}
	if _, allowed, err := store.Claim(input); err != nil || !allowed {
		t.Fatalf("initial claim = %v, %v", allowed, err)
	}
	if _, allowed, err := store.Claim(input); err != nil || allowed {
		t.Fatalf("same generation duplicate = %v, %v", allowed, err)
	}
	_, input.Scope, err = store.Update("s", "main", EventUpdate{Source: "clear"})
	if err != nil {
		t.Fatal(err)
	}
	if input.Scope.Generation != 1 {
		t.Fatalf("clear generation = %d, want 1", input.Scope.Generation)
	}
	if _, allowed, err := store.Claim(input); err != nil || !allowed {
		t.Fatalf("claim after clear = %v, %v", allowed, err)
	}
}

func TestEveryAndBlockActionsDoNotDeduplicate(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	_, scope, err := store.Update("s", "main", EventUpdate{})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []ClaimInput{
		{Scope: scope, Repeat: "every", Action: "warn", Event: "tool.post", Rule: "r", Body: "warning"},
		{Scope: scope, Repeat: "session", Action: "block", Event: "tool.pre", Rule: "r", Body: "deny"},
	} {
		for i := 0; i < 2; i++ {
			if reservation, allowed, err := store.Claim(input); err != nil || !allowed || reservation != nil {
				t.Fatalf("non-deduplicating claim %d for %#v = %#v, %v, %v", i, input, reservation, allowed, err)
			}
		}
	}
}

func TestConcurrentClaimsOnlyOneWins(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	_, scope, err := store.Update("s", "main", EventUpdate{})
	if err != nil {
		t.Fatal(err)
	}
	input := ClaimInput{Scope: scope, Repeat: "session", Action: "inject", Event: "prompt.submitted", Rule: "pkg/rule", Body: "same"}
	const workers = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	twins := 0
	reservations := make([]*Reservation, 0, 1)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reservation, allowed, err := store.Claim(input)
			if err != nil {
				t.Errorf("Claim: %v", err)
				return
			}
			if allowed {
				mu.Lock()
				twins++
				reservations = append(reservations, reservation)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	for _, reservation := range reservations {
		_ = reservation.Release()
	}
	if twins != 1 {
		t.Fatalf("successful claims = %d, want exactly one", twins)
	}
}

func TestAppendStatsAcceptsOnlyBoundedMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	valid := []byte(`{"at":"2026-10-14T00:00:00Z","provider":"claude-code","event":"PreToolUse","point":"tool.pre","entry":"pkg/rule@tool.pre","action":"inject","result":"output"}`)
	if err := store.AppendStats("session", "main", valid); err != nil {
		t.Fatalf("AppendStats(valid) = %v", err)
	}
	for _, invalid := range [][]byte{
		[]byte(`{"at":"now","provider":"claude-code","event":"PreToolUse","point":"tool.pre","entry":"r","action":"inject","result":"output","prompt":"private"}`),
		[]byte(`{"at":"now"}`),
		[]byte("{\"at\":\"now\"}\n{}"),
		bytes.Repeat([]byte("x"), 512),
	} {
		if err := store.AppendStats("session", "main", invalid); err == nil {
			t.Errorf("AppendStats accepted %q", invalid)
		}
	}
	path := filepath.Join(root, "session", "main", "stats.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("stats mode = %o, want 600", info.Mode().Perm())
	}
}

func TestUpdateRewritesOnlyChangedStateFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return clock }
	approx := true
	if _, _, err := store.Update("s", "main", EventUpdate{Model: "m", EffortRaw: "high", ThinkingEffort: "high", EffortApprox: &approx}); err != nil {
		t.Fatal(err)
	}
	sessionPath, scopePath := filepath.Join(root, "s", "session.json"), filepath.Join(root, "s", "main", "scope.json")
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	session, scope := read(sessionPath), read(scopePath)

	clock = clock.Add(time.Hour)
	again := true
	gotSession, gotScope, err := store.Update("s", "main", EventUpdate{Model: "m", EffortRaw: "high", ThinkingEffort: "high", EffortApprox: &again})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(sessionPath), session) || !bytes.Equal(read(scopePath), scope) {
		t.Fatal("an event that changes nothing rewrote state files")
	}
	if gotSession.Model != "m" || gotScope.PromptCount != 0 {
		t.Fatalf("unchanged update returned session %#v scope %#v", gotSession, gotScope)
	}
	if info, err := os.Stat(scopePath); err != nil || !info.ModTime().Equal(clock) {
		t.Fatalf("scope modification time = %v, %v; want %v so Cleanup sees activity", info.ModTime(), err, clock)
	}

	clock = clock.Add(time.Hour)
	if _, _, err := store.Update("s", "main", EventUpdate{PromptSubmitted: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(sessionPath), session) {
		t.Fatal("a prompt rewrote the unchanged session file")
	}
	if bytes.Equal(read(scopePath), scope) || !strings.Contains(string(read(scopePath)), `"prompt_count":1`) {
		t.Fatalf("prompt count not persisted: %s", read(scopePath))
	}

	if _, _, err := store.Update("s", "main", EventUpdate{Model: "m2"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(read(sessionPath)), `"model":"m2"`) {
		t.Fatalf("model change not persisted: %s", read(sessionPath))
	}
	if _, _, err := store.Update("s", "worker", EventUpdate{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "s", "worker", "scope.json")); err != nil {
		t.Fatalf("a new agent scope must be written even when empty: %v", err)
	}
}
