package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func refreshEvent(t *testing.T, s *Store, session, agent, harness string, update EventUpdate, id string, fn func(*RefreshFrame)) {
	t.Helper()
	if !RefreshSupported() {
		t.Skip("refresh platform")
	}
	g, err := s.BeginRefresh(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	_, scope, err := s.UpdateLocked(session, agent, update, g)
	if err != nil {
		t.Fatal(err)
	}
	f, err := g.Advance(scope, harness, update, id)
	if err != nil {
		t.Fatal(err)
	}
	if fn != nil {
		fn(f)
	}
}
func refreshClaim(t *testing.T, f *RefreshFrame, n int, body string, want bool) (*Reservation, *RefreshTrace) {
	t.Helper()
	trace := &RefreshTrace{}
	r, ok, err := f.guard.store.Claim(ClaimInput{Scope: f.scope, Repeat: "session", Action: "inject", Event: "tool.pre", Rule: "pkg/r", Body: body, Refresh: n, RefreshFrame: f, RefreshTrace: trace, ReinjectOnCompact: true})
	if err != nil || ok != want {
		t.Fatalf("refresh n=%d q=%d body=%s allowed=%v want=%v err=%v trace=%+v", n, f.q, body, ok, want, err, trace)
	}
	return r, trace
}
func refreshStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRefreshRollingCadenceAndClock(t *testing.T) {
	for _, n := range []int{1, 3, 1000000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			s := refreshStore(t)
			refreshEvent(t, s, "session", "main", "codex", EventUpdate{Source: "startup"}, "", func(f *RefreshFrame) { refreshClaim(t, f, n, "A", true) })
			for q := 1; q <= 10; q++ {
				refreshEvent(t, s, "session", "main", "codex", EventUpdate{PromptSubmitted: true}, fmt.Sprint(q), func(f *RefreshFrame) {
					if f.q != uint64(q) {
						t.Fatal(f.q)
					}
					refreshClaim(t, f, n, "A", q%n == 0)
				})
			}
		})
	}
	s := refreshStore(t)
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", func(f *RefreshFrame) { refreshClaim(t, f, 3, "A", true) })
	for q := 1; q <= 10; q++ {
		refreshEvent(t, s, "s", "main", "codex", EventUpdate{PromptSubmitted: true}, fmt.Sprint(q), func(f *RefreshFrame) {
			if q == 7 || q == 8 || q == 10 {
				refreshClaim(t, f, 3, "A", q != 8)
			}
		})
	}
}

func TestRefreshNativeReplayLateUnknownAndFallback(t *testing.T) {
	s := refreshStore(t)
	for _, tc := range []struct {
		prompt    bool
		id        string
		q         uint64
		available bool
		clock     string
	}{
		{true, "native-secret-A", 1, true, "native-turn-id"}, {true, "native-secret-A", 1, true, "native-turn-id"},
		{true, "native-secret-B", 2, true, "native-turn-id"}, {false, "native-secret-A", 1, true, "native-turn-id"},
		{true, "native-secret-A", 1, true, "native-turn-id"}, {false, "unknown-secret", 2, false, "unregistered-turn-id"},
		{false, "", 2, true, "prompt-count-fallback"}, {true, "", 3, true, "prompt-count-fallback"}, {true, "", 4, true, "prompt-count-fallback"},
		{false, "native-secret-B", 2, true, "native-turn-id"},
	} {
		refreshEvent(t, s, "s", "main", "codex", EventUpdate{PromptSubmitted: tc.prompt}, tc.id, func(f *RefreshFrame) {
			if f.q != tc.q || f.available != tc.available || f.quality != tc.clock {
				t.Fatalf("%+v", f)
			}
		})
	}
	data, err := os.ReadFile(filepath.Join(s.root, component("s"), component("main"), "refresh", "codex.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") {
		t.Fatal("raw turn ID leaked")
	}
	var value refreshState
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if value.Q != 4 || len(value.Turns) != 2 {
		t.Fatal(value)
	}
	_, scope, err := s.Read("s", "main")
	if err != nil || scope.PromptCount != 6 {
		t.Fatal(scope, err)
	} // Legacy duplicate callbacks still count.
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "unknown", func(f *RefreshFrame) {
		refreshClaim(t, f, 1, "first", true)
		_, tr := refreshClaim(t, f, 1, "first", false)
		if tr.Reason != "clock-unavailable" {
			t.Fatal(tr)
		}
	})
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "native-secret-A", func(f *RefreshFrame) {
		_, tr := refreshClaim(t, f, 1, "first", false)
		if tr.Reason != "not-due" {
			t.Fatal(tr)
		}
	})
}

func TestRefreshLifecycleAndScopeIsolation(t *testing.T) {
	s := refreshStore(t)
	for i, h := range []string{"codex", "claude-code"} {
		refreshEvent(t, s, "s", "main", h, EventUpdate{}, "", func(f *RefreshFrame) { refreshClaim(t, f, 3, "A", i == 0) })
	}
	for i := 1; i <= 2; i++ {
		refreshEvent(t, s, "s", "main", "codex", EventUpdate{PromptSubmitted: true}, fmt.Sprint(i), nil)
	}
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{Source: "resume"}, "", func(f *RefreshFrame) { refreshClaim(t, f, 3, "A", false) })
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{Source: "compact"}, "", func(f *RefreshFrame) {
		if f.q != 2 {
			t.Fatal(f.q)
		}
		refreshClaim(t, f, 3, "A", true)
	})
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{Source: "fork"}, "", func(f *RefreshFrame) { refreshClaim(t, f, 3, "A", false) })
	for _, pair := range [][2]string{{"new-session", "main"}, {"s", "child"}} {
		for i := 0; i < 4; i++ {
			refreshEvent(t, s, pair[0], pair[1], "codex", EventUpdate{}, "", func(f *RefreshFrame) {
				if f.q != 0 {
					t.Fatal(f.q)
				}
				refreshClaim(t, f, 3, "A", i == 0)
			})
		}
	}
	for _, source := range []string{"clear", "startup"} {
		refreshEvent(t, s, "s", "main", "codex", EventUpdate{Source: source}, "", func(f *RefreshFrame) {
			if f.q != 0 {
				t.Fatal(f.q)
			}
			refreshClaim(t, f, 3, "A", true)
		})
	}
	for i := 0; i < 2; i++ {
		other, _ := New(filepath.Join(t.TempDir(), "projects", fmt.Sprint(i), "state"))
		refreshEvent(t, other, "s", "main", "codex", EventUpdate{}, "", func(f *RefreshFrame) { refreshClaim(t, f, 3, "A", true) })
	}
}

func TestRefreshBodyMigrationIntervalsAndRollback(t *testing.T) {
	s := refreshStore(t)
	_, scope, err := s.Update("s", "main", EventUpdate{})
	if err != nil {
		t.Fatal(err)
	}
	ordinary, _, err := s.Claim(ClaimInput{Scope: scope, Repeat: "session", Action: "inject", Rule: "pkg/r", Body: "A", ReinjectOnCompact: true})
	if err != nil {
		t.Fatal(err)
	}
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", func(f *RefreshFrame) {
		_, tr := refreshClaim(t, f, 3, "A", false)
		if tr.Reason != "seeded" {
			t.Fatal(tr)
		}
		refreshClaim(t, f, 3, "B", true)
	})
	for q := 1; q <= 3; q++ {
		refreshEvent(t, s, "s", "main", "codex", EventUpdate{PromptSubmitted: true}, fmt.Sprint(q), func(f *RefreshFrame) { refreshClaim(t, f, 3, "A", q == 3) })
	}
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", func(f *RefreshFrame) {
		refreshClaim(t, f, 5, "A", false) // New policy seeds the existing ordinary claim.
		r, tr := refreshClaim(t, f, 3, "B", true)
		if err := r.Release(); err != nil {
			t.Fatal(err)
		}
		if tr.Reason != "cap-release" {
			t.Fatal(tr)
		}
		_, tr = refreshClaim(t, f, 3, "B", true)
		if tr.Last == nil || *tr.Last != 0 {
			t.Fatal(tr)
		}
		r, _ = refreshClaim(t, f, 3, "C", true)
		if err := r.Release(); err != nil {
			t.Fatal(err)
		}
		refreshClaim(t, f, 3, "C", true)
		if _, err := os.Stat(ordinary.path); err != nil {
			t.Fatal("old ordinary claim was removed", err)
		}
		_, ok, err := s.Claim(ClaimInput{Scope: f.scope, Repeat: "session", Action: "inject", Rule: "pkg/r", Body: "B", ReinjectOnCompact: true})
		if err != nil || ok {
			t.Fatal("disabled refresh lost ordinary dedup", ok, err)
		}
	})
	for q := 4; q <= 8; q++ {
		refreshEvent(t, s, "s", "main", "codex", EventUpdate{PromptSubmitted: true}, fmt.Sprint(q), nil)
	}
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", func(f *RefreshFrame) {
		refreshClaim(t, f, 5, "A", true)
		refreshClaim(t, f, 3, "B", true)
		refreshClaim(t, f, 3, "A", true)
	})
}

func TestRefreshLockDeadlineIdentityAndSnapshot(t *testing.T) {
	if !RefreshSupported() {
		t.Skip("refresh platform")
	}
	s := refreshStore(t)
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{PromptSubmitted: true}, "private-turn", func(f *RefreshFrame) { refreshClaim(t, f, 3, "A", true) })
	lockPath := filepath.Join(s.root, component("s"), "refresh.lock")
	before, _ := os.Stat(lockPath)
	g, err := s.BeginRefresh(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := s.BeginRefresh(context.Background(), "s"); err == nil {
		t.Fatal("busy lock accepted")
	}
	if elapsed := time.Since(start); elapsed < RefreshLockWait || elapsed > 100*time.Millisecond {
		t.Fatal(elapsed)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.BeginRefresh(ctx, "s"); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := CopySnapshot(context.Background(), s.root, t.TempDir(), "s", "main"); err == nil {
		t.Fatal("snapshot ignored lock")
	}
	g.Close()
	dest := t.TempDir()
	snapshot, err := CopySnapshot(context.Background(), s.root, dest, "s", "main")
	if err != nil || snapshot.Files != 4 {
		t.Fatal(snapshot, err)
	}
	after, _ := os.Stat(lockPath)
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("snapshot changed lock")
	}
	if _, err := os.Stat(filepath.Join(dest, component("s"), "refresh.lock")); !os.IsNotExist(err) {
		t.Fatal("snapshot copied source lock")
	}
	copyStore, _ := New(dest)
	refreshEvent(t, copyStore, "s", "main", "codex", EventUpdate{}, "private-turn", func(f *RefreshFrame) { refreshClaim(t, f, 3, "A", false) })
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if _, err := CopySnapshot(context.Background(), s.root, t.TempDir(), "s", "main"); err == nil {
		t.Fatal("missing source lock was created")
	}
}

func TestRefreshRejectsCorruptionSymlinksAndOverflow(t *testing.T) {
	if !RefreshSupported() {
		t.Skip("refresh platform")
	}
	for _, bad := range []string{`null`, `{}`, `{"format":2,"gen":0,"q":0,"turns":{},"cursors":{}}`, `{"format":1,"gen":0,"q":0,"turns":{},"cursors":{},"unknown":1}`, `{"format":1,"gen":0,"q":0,"turns":{"raw":1},"cursors":{}}`} {
		t.Run(bad, func(t *testing.T) {
			s := refreshStore(t)
			refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", nil)
			path := filepath.Join(s.root, component("s"), component("main"), "refresh", "codex.json")
			os.WriteFile(path, []byte(bad), 0600)
			g, err := s.BeginRefresh(context.Background(), "s")
			if err != nil {
				t.Fatal(err)
			}
			_, scope, err := s.UpdateLocked("s", "main", EventUpdate{}, g)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := g.Advance(scope, "codex", EventUpdate{Source: "startup"}, ""); err == nil {
				t.Fatal("bad state silently reset")
			}
			g.Close()
			if _, err := CopySnapshot(context.Background(), s.root, t.TempDir(), "s", "main"); err == nil {
				t.Fatal("snapshot accepted corrupt state")
			}
			data, _ := os.ReadFile(path)
			if string(data) != bad {
				t.Fatal("corrupt state changed")
			}
		})
	}
	s := refreshStore(t)
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", func(f *RefreshFrame) {
		f.state.Q = ^uint64(0)
		if err := f.save(); err != nil {
			t.Fatal(err)
		}
	})
	g, err := s.BeginRefresh(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	_, scope, err := s.UpdateLocked("s", "main", EventUpdate{}, g)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Advance(scope, "codex", EventUpdate{PromptSubmitted: true}, "new"); err == nil {
		t.Fatal("overflow accepted")
	}
	g.Close()
	for _, part := range []string{"root", "session", "scope", "refresh", "state", "lock", "seen"} {
		t.Run(part, func(t *testing.T) {
			s := refreshStore(t)
			refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", nil)
			path := map[string]string{"root": s.root, "session": filepath.Join(s.root, component("s")), "scope": filepath.Join(s.root, component("s"), component("main")), "refresh": filepath.Join(s.root, component("s"), component("main"), "refresh"), "state": filepath.Join(s.root, component("s"), component("main"), "refresh", "codex.json"), "lock": filepath.Join(s.root, component("s"), "refresh.lock"), "seen": filepath.Join(s.root, component("s"), component("main"), "seen")}[part]
			os.RemoveAll(path)
			target := t.TempDir()
			if part == "state" || part == "lock" {
				target = filepath.Join(target, "file")
				os.WriteFile(target, nil, 0600)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			g, err := s.BeginRefresh(context.Background(), "s")
			if err == nil {
				defer g.Close()
				_, scope, e := s.UpdateLocked("s", "main", EventUpdate{}, g)
				if e == nil {
					var f *RefreshFrame
					f, e = g.Advance(scope, "codex", EventUpdate{}, "")
					if e == nil {
						_, _, e = s.Claim(ClaimInput{Scope: scope, Repeat: "session", Action: "inject", Rule: "r", Body: "x", Refresh: 1, RefreshFrame: f})
					}
				}
				err = e
			}
			if err == nil {
				t.Fatal("symlink accepted")
			}
		})
	}
}

// The child uses the production guard/clock/claim path. Test retries only
// explicit busy failures so a simultaneous barrier does not depend on fsync
// throughput completing all peers inside one 20ms acquisition window.
func TestRefreshProcessHelper(t *testing.T) {
	root := os.Getenv("STYLE_REFRESH_HELPER")
	if root == "" {
		t.Skip("subprocess helper")
	}
	s, _ := New(root)
	id := os.Getenv("STYLE_REFRESH_TURN")
	if err := os.WriteFile(filepath.Join(root, "ready-"+os.Getenv("STYLE_REFRESH_WORKER")), nil, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "go")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("barrier timeout")
		}
		time.Sleep(time.Millisecond)
	}
	var g *RefreshGuard
	var err error
	for {
		g, err = s.BeginRefresh(context.Background(), "s")
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "busy") || time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	defer g.Close()
	if id == "crash" {
		os.WriteFile(filepath.Join(root, "locked"), nil, 0600)
		time.Sleep(time.Hour)
	}
	_, scope, err := s.UpdateLocked("s", "main", EventUpdate{PromptSubmitted: true}, g)
	if err != nil {
		t.Fatal(err)
	}
	f, err := g.Advance(scope, "codex", EventUpdate{PromptSubmitted: true}, id)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, err := s.Claim(ClaimInput{Scope: scope, Repeat: "session", Action: "inject", Rule: "r", Body: "body", Refresh: 1, RefreshFrame: f})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		if err := os.WriteFile(filepath.Join(root, "output-"+os.Getenv("STYLE_REFRESH_WORKER")), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRefreshSynchronizedProcessesAndCrash(t *testing.T) {
	if !RefreshSupported() {
		t.Skip("refresh platform")
	}
	for _, same := range []bool{true, false} {
		t.Run(fmt.Sprint(same), func(t *testing.T) {
			root := t.TempDir()
			var children []*exec.Cmd
			for i := 0; i < 6; i++ {
				id := fmt.Sprint(i)
				if same {
					id = "same"
				}
				cmd := exec.Command(os.Args[0], "-test.run=^TestRefreshProcessHelper$")
				cmd.Env = append(os.Environ(), "STYLE_REFRESH_HELPER="+root, "STYLE_REFRESH_WORKER="+fmt.Sprint(i), "STYLE_REFRESH_TURN="+id)
				out, err := os.Create(filepath.Join(root, "worker-"+fmt.Sprint(i)+".log"))
				if err != nil {
					t.Fatal(err)
				}
				cmd.Stdout = out
				cmd.Stderr = out
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				out.Close()
				children = append(children, cmd)
			}
			waitRefreshFiles(t, root, "ready-*", 6)
			os.WriteFile(filepath.Join(root, "go"), nil, 0600)
			for i, cmd := range children {
				if err := cmd.Wait(); err != nil {
					data, _ := os.ReadFile(filepath.Join(root, "worker-"+fmt.Sprint(i)+".log"))
					t.Fatalf("%v: %s", err, data)
				}
			}
			files, _ := filepath.Glob(filepath.Join(root, "output-*"))
			want := 6
			if same {
				want = 1
			}
			if len(files) != want {
				t.Fatal(len(files), want)
			}
			s, _ := New(root)
			refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", func(f *RefreshFrame) {
				if f.state.Q != uint64(want) || len(f.state.Turns) != want {
					t.Fatal(f.state)
				}
			})
		})
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go"), nil, 0600)
	cmd := exec.Command(os.Args[0], "-test.run=^TestRefreshProcessHelper$")
	cmd.Env = append(os.Environ(), "STYLE_REFRESH_HELPER="+root, "STYLE_REFRESH_WORKER=crash", "STYLE_REFRESH_TURN=crash")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitRefreshFiles(t, root, "locked", 1)
	before, _ := os.Stat(filepath.Join(root, component("s"), "refresh.lock"))
	cmd.Process.Kill()
	cmd.Wait()
	s, _ := New(root)
	g, err := s.BeginRefresh(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	g.Close()
	after, _ := os.Stat(filepath.Join(root, component("s"), "refresh.lock"))
	if !os.SameFile(before, after) {
		t.Fatal("crash replaced lock inode")
	}
}
func waitRefreshFiles(t *testing.T, root, pattern string, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		files, _ := filepath.Glob(filepath.Join(root, pattern))
		if len(files) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("barrier %s: %d want %d", pattern, len(files), want)
		}
		time.Sleep(time.Millisecond)
	}
}
