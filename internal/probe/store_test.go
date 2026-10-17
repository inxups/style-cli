package probe

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/speakeasy-api/agenthooks"
)

func a2Binding(harness string) Binding {
	h := HashString("fixture")
	return Binding{Harness: harness, Route: harness + "-user", HomeSHA256: h, HarnessHomeSHA256: h, RouteRootSHA256: h, RuntimeID: h, RuntimeSHA256: h, HarnessID: h, HarnessSHA256: h, LockSHA256: h, ConfigSHA256: h, Events: []string{"PreToolUse"}}
}
func a2Owner(pid int) (agenthooks.ProcessIdentity, error) {
	return agenthooks.ProcessIdentity{PID: pid, Birth: "fixture-birth"}, nil
}
func a2Session(t *testing.T, harnesses ...string) (*Session, string, time.Time) {
	t.Helper()
	home := t.TempDir()
	now := time.Now().UTC()
	bindings := []Binding{}
	for _, harness := range harnesses {
		binding := a2Binding(harness)
		canonical, err := filepath.EvalSymlinks(home)
		if err != nil {
			t.Fatal(err)
		}
		binding.HomeSHA256 = HashString(canonical)
		binding.HarnessHomeSHA256 = HashString(canonical)
		bindings = append(bindings, binding)
	}
	session, err := Begin(home, bindings, time.Minute, Options{Now: func() time.Time { return now }, Lookup: a2Owner, OwnerPID: 42})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Cleanup(); session.Close() })
	return session, home, now
}
func a2Receipt(lease Lease) Receipt {
	return Receipt{Format: 1, Protocol: Protocol, RunID: lease.RunID, Harness: lease.Binding.Harness, HomeSHA256: lease.Binding.HomeSHA256, HarnessHomeSHA256: lease.Binding.HarnessHomeSHA256, Event: "PreToolUse", SessionSHA256: HashString("session-canary"), ObservedAt: lease.Started, Processes: []Process{{100, 200, "hook-birth", lease.Binding.RuntimeID}, {200, 1, "harness-birth", lease.Binding.HarnessID}}}
}
func a2WriteReceipt(t *testing.T, home string, lease Lease, name string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(home, "probes", lease.ReceiptDir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestM3A2LeaseAcquisitionBusyAndRollback(t *testing.T) {
	session, home, _ := a2Session(t, "codex")
	info, err := os.Stat(filepath.Join(home, "probes", "codex.lease.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private lease: %v %v", info, err)
	}
	for _, name := range []string{"probes", "probes/runs", "probes/runs/" + session.RunID, "probes/" + session.Lease("codex").ReceiptDir} {
		info, err := os.Stat(filepath.Join(home, name))
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("private directory: %s %v %v", name, info, err)
		}
	}
	before, _ := os.ReadFile(filepath.Join(home, "probes", "codex.lease.json"))
	if _, err := Begin(home, []Binding{a2Binding("claude-code"), a2Binding("codex")}, time.Minute, Options{Lookup: a2Owner, OwnerPID: 43}); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected busy, got %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(home, "probes", "codex.lease.json"))
	if !bytes.Equal(before, after) || !session.Active("codex") {
		t.Fatal("changed another run's lease")
	}
	if _, err = os.Stat(filepath.Join(home, "probes", "claude-code.lease.json")); !os.IsNotExist(err) {
		t.Fatal("partial acquisition retained its lease")
	}
	runs, err := os.ReadDir(filepath.Join(home, "probes", "runs"))
	if err != nil || len(runs) != 1 || runs[0].Name() != session.RunID {
		t.Fatalf("partial run leak: %v %v", runs, err)
	}
	lease := session.Lease("codex")
	lease.Binding.Events[0] = "Stop"
	if session.Lease("codex").Binding.Events[0] != "PreToolUse" {
		t.Fatal("lease getter exposed mutable binding")
	}
	if cleanup := session.Cleanup(); !cleanup.Complete || len(cleanup.Remaining) != 0 {
		t.Fatalf("cleanup: %+v", cleanup)
	}
	if session.Active("codex") {
		t.Fatal("lease still active")
	}
}

func TestM3A2ConcurrentAcquisitionAndSharedRecovery(t *testing.T) {
	t.Run("exclusive", func(t *testing.T) {
		home := t.TempDir()
		type acquired struct {
			session *Session
			err     error
		}
		results := make(chan acquired, 2)
		start := make(chan struct{})
		for pid := 40; pid < 42; pid++ {
			go func(pid int) {
				<-start
				session, err := Begin(home, []Binding{a2Binding("codex")}, time.Minute, Options{Lookup: a2Owner, OwnerPID: pid})
				results <- acquired{session, err}
			}(pid)
		}
		close(start)
		wins, busy := 0, 0
		var winner *Session
		for i := 0; i < 2; i++ {
			result := <-results
			if result.err == nil {
				wins++
				winner = result.session
			} else if errors.Is(result.err, ErrBusy) {
				busy++
			} else {
				t.Fatal(result.err)
			}
		}
		if wins != 1 || busy != 1 {
			t.Fatal(wins, busy)
		}
		defer winner.Close()
		if !winner.Active("codex") || !winner.Cleanup().Complete {
			t.Fatal("winner ownership damaged")
		}
	})
	t.Run("shared-old-run", func(t *testing.T) {
		home := t.TempDir()
		start := time.Now().Add(-time.Minute)
		old, err := Begin(home, []Binding{a2Binding("codex"), a2Binding("claude-code")}, time.Second, Options{Lookup: a2Owner, OwnerPID: 42, Now: func() time.Time { return start }})
		if err != nil {
			t.Fatal(err)
		}
		defer old.Close()
		lookup := func(pid int) (agenthooks.ProcessIdentity, error) {
			if pid == 42 {
				return agenthooks.ProcessIdentity{}, agenthooks.ErrProcessGone
			}
			return a2Owner(pid)
		}
		current, err := Begin(home, []Binding{a2Binding("codex")}, time.Minute, Options{Lookup: lookup, OwnerPID: 84})
		if err != nil {
			t.Fatal(err)
		}
		defer current.Close()
		defer current.Cleanup()
		if _, err = os.Stat(filepath.Join(home, "probes", old.Lease("claude-code").ReceiptDir)); err != nil {
			t.Fatal("reclaimed other target receipts")
		}
		second, err := Begin(home, []Binding{a2Binding("claude-code")}, time.Minute, Options{Lookup: lookup, OwnerPID: 85})
		if err != nil {
			t.Fatal(err)
		}
		defer second.Close()
		defer second.Cleanup()
		if _, err = os.Stat(filepath.Join(home, "probes", "runs", old.RunID)); !os.IsNotExist(err) {
			t.Fatal("shared stale run not fully recovered")
		}
	})
}

func TestM3A2LeaseRecoveryAndPIDReuse(t *testing.T) {
	for _, mode := range []string{"gone", "reused", "alive", "permission", "unknown", "wrong-pid", "empty-birth", "not-expired", "bad-marker", "malformed", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			start := time.Now().Add(-2 * time.Minute).UTC()
			old, err := Begin(home, []Binding{a2Binding("codex")}, time.Second, Options{Now: func() time.Time { return start }, Lookup: a2Owner, OwnerPID: 42})
			if err != nil {
				t.Fatal(err)
			}
			runID := old.RunID
			old.Close()
			if mode == "not-expired" {
				start = time.Now().UTC()
				data, _ := os.ReadFile(filepath.Join(home, "probes", "codex.lease.json"))
				var lease Lease
				json.Unmarshal(data, &lease)
				lease.Started = start
				lease.Expires = start.Add(time.Minute)
				encoded, _ := json.Marshal(lease)
				os.WriteFile(filepath.Join(home, "probes", "codex.lease.json"), encoded, 0o600)
			}
			if mode == "bad-marker" {
				os.WriteFile(filepath.Join(home, "probes", "runs", runID, "owner.json"), []byte(`{"run_id":"other","owner":{"pid":42,"birth":"fixture-birth"}}`), 0o600)
			}
			if mode == "malformed" {
				os.WriteFile(filepath.Join(home, "probes", "codex.lease.json"), []byte(`{}`), 0o600)
			}
			if mode == "symlink" {
				path := filepath.Join(home, "probes", "codex.lease.json")
				data, _ := os.ReadFile(path)
				os.Remove(path)
				os.WriteFile(filepath.Join(home, "outside"), data, 0o600)
				os.Symlink(filepath.Join(home, "outside"), path)
			}
			lookup := func(pid int) (agenthooks.ProcessIdentity, error) {
				if pid != 42 {
					return a2Owner(pid)
				}
				switch mode {
				case "gone", "bad-marker", "malformed", "symlink", "not-expired":
					return agenthooks.ProcessIdentity{}, agenthooks.ErrProcessGone
				case "reused":
					return agenthooks.ProcessIdentity{PID: 42, Birth: "different-birth"}, nil
				case "wrong-pid":
					return agenthooks.ProcessIdentity{PID: 43, Birth: "different-birth"}, nil
				case "empty-birth":
					return agenthooks.ProcessIdentity{PID: pid}, nil
				case "alive":
					return a2Owner(pid)
				case "permission":
					return agenthooks.ProcessIdentity{}, os.ErrPermission
				default:
					return agenthooks.ProcessIdentity{}, agenthooks.ErrProcessUnsupported
				}
			}
			current, err := Begin(home, []Binding{a2Binding("codex")}, time.Second, Options{Lookup: lookup, OwnerPID: 84})
			if mode == "gone" || mode == "reused" {
				if err != nil {
					t.Fatal(err)
				}
				defer current.Close()
				defer current.Cleanup()
				if current.RunID == runID {
					t.Fatal("reused old run")
				}
				if _, err = os.Stat(filepath.Join(home, "probes", "runs", runID)); !os.IsNotExist(err) {
					t.Fatal("old run not recovered")
				}
			} else if err == nil {
				current.Cleanup()
				current.Close()
				t.Fatal("unsafe recovery succeeded")
			}
			if mode == "bad-marker" {
				if _, statErr := os.Lstat(filepath.Join(home, "probes", "codex.lease.json")); statErr != nil {
					t.Fatal("unverified ownership removed another lease")
				}
			}
		})
	}
}

func TestM3A2CleanupOwnershipAndFailure(t *testing.T) {
	t.Run("replacement", func(t *testing.T) {
		session, home, _ := a2Session(t, "codex")
		old := session.Lease("codex")
		replacement := old
		replacement.RunID = strings.Repeat("b", 32)
		replacement.ReceiptDir = receiptDir(replacement.RunID, "codex")
		data, _ := json.Marshal(replacement)
		path := filepath.Join(home, "probes", "codex.lease.json")
		os.Remove(path)
		os.WriteFile(path, data, 0o600)
		cleanup := session.Cleanup()
		if cleanup.Complete || len(cleanup.Remaining) == 0 {
			t.Fatal("replacement not reported")
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(data, after) {
			t.Fatal("deleted another lease")
		}
	})
	t.Run("stop-failure", func(t *testing.T) {
		session, _, _ := a2Session(t, "codex")
		normal := session.stop
		session.stop = func(string, heldLease) error { return os.ErrPermission }
		cleanup := session.Cleanup()
		if cleanup.Complete || len(cleanup.Remaining) != 2 {
			t.Fatalf("missing residual: %+v", cleanup)
		}
		session.stop = normal
	})
	t.Run("run-replacement", func(t *testing.T) {
		session, home, _ := a2Session(t, "codex")
		path := filepath.Join(home, "probes", "runs", session.RunID)
		os.Rename(path, path+"-held")
		os.Mkdir(path, 0o700)
		os.WriteFile(filepath.Join(path, "user-canary"), []byte("keep"), 0o600)
		cleanup := session.Cleanup()
		if cleanup.Complete {
			t.Fatal("replaced run accepted")
		}
		if _, err := os.Stat(filepath.Join(path, "user-canary")); err != nil {
			t.Fatal("deleted replacement run")
		}
	})
}

func TestM3A2ReceiptsStrictBoundsAndWhitelist(t *testing.T) {
	modes := []string{"valid", "old-run", "wrong-harness", "wrong-home", "wrong-harness-home", "empty-session", "future", "expired", "at-expiry", "event", "unknown-field", "null", "tail", "oversize", "mode", "symlink", "directory", "too-many", "bad-name", "process-cycle", "process-gap", "birth", "many-processes"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			session, home, now := a2Session(t, "codex")
			lease := session.Lease("codex")
			receipt := a2Receipt(lease)
			switch mode {
			case "old-run":
				receipt.RunID = strings.Repeat("a", 32)
			case "wrong-harness":
				receipt.Harness = "claude-code"
			case "wrong-home":
				receipt.HomeSHA256 = HashString("other")
			case "wrong-harness-home":
				receipt.HarnessHomeSHA256 = HashString("other")
			case "empty-session":
				receipt.SessionSHA256 = ""
			case "future":
				receipt.ObservedAt = now.Add(time.Hour)
			case "expired":
				receipt.ObservedAt = now.Add(-time.Second)
			case "at-expiry":
				receipt.ObservedAt = lease.Expires
			case "event":
				receipt.Event = "Stop"
			case "process-cycle":
				receipt.Processes[1].PID = 100
			case "process-gap":
				receipt.Processes[0].ParentPID = 999
			case "birth":
				receipt.Processes[0].Birth = ""
			case "many-processes":
				receipt.Processes = append(receipt.Processes, make([]Process, MaxAncestors+1)...)
			}
			a2WriteReceipt(t, home, lease, "00.json", receipt)
			path := filepath.Join(home, "probes", lease.ReceiptDir, "00.json")
			switch mode {
			case "unknown-field":
				data, _ := os.ReadFile(path)
				data = append([]byte(`{"prompt":"SECRET",`), data[1:]...)
				os.WriteFile(path, data, 0o600)
			case "null":
				os.WriteFile(path, []byte(`null`), 0o600)
			case "tail":
				data, _ := os.ReadFile(path)
				os.WriteFile(path, append(data, []byte(` {}`)...), 0o600)
			case "oversize":
				os.WriteFile(path, bytes.Repeat([]byte(" "), MaxReceiptBytes+1), 0o600)
			case "mode":
				os.Chmod(path, 0o644)
			case "symlink":
				os.Rename(path, filepath.Join(home, "external"))
				os.Symlink(filepath.Join(home, "external"), path)
			case "directory":
				os.Remove(path)
				os.Mkdir(path, 0o700)
			case "too-many":
				for index := 1; index <= MaxReceipts; index++ {
					a2WriteReceipt(t, home, lease, fmt.Sprintf("%02d.json", index), receipt)
				}
			case "bad-name":
				os.Rename(path, filepath.Join(filepath.Dir(path), "unexpected.json"))
			}
			receipts, quality := session.Receipts("codex")
			if mode == "valid" {
				if len(receipts) != 1 || len(quality) != 0 {
					t.Fatalf("valid receipt rejected: %v %v", receipts, quality)
				}
			} else if len(receipts) != 0 || len(quality) == 0 {
				t.Fatalf("invalid receipt accepted: %s %v %v", mode, receipts, quality)
			}
			data, _ := json.Marshal(receipt)
			if bytes.Contains(data, []byte("session-canary")) {
				t.Fatal("raw identity leaked")
			}
		})
	}
}

func TestM3A2ProbeFilesystemRefusal(t *testing.T) {
	for _, mode := range []string{"home-symlink", "probes-symlink", "runs-symlink", "lease-symlink", "probes-file", "probes-public", "lease-public", "lease-directory", "unknown-owner"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			external := t.TempDir()
			sentinel := filepath.Join(external, "keep")
			os.WriteFile(sentinel, []byte("canary"), 0o600)
			if mode == "home-symlink" {
				link := filepath.Join(t.TempDir(), "home")
				os.Symlink(home, link)
				home = link
			} else if mode == "probes-symlink" {
				os.Symlink(external, filepath.Join(home, "probes"))
			} else if mode == "probes-file" {
				os.WriteFile(filepath.Join(home, "probes"), nil, 0o600)
			} else {
				os.Mkdir(filepath.Join(home, "probes"), 0o700)
				switch mode {
				case "runs-symlink":
					os.Symlink(external, filepath.Join(home, "probes", "runs"))
				case "lease-symlink":
					os.Symlink(sentinel, filepath.Join(home, "probes", "codex.lease.json"))
				case "probes-public":
					os.Chmod(filepath.Join(home, "probes"), 0o755)
				case "lease-public":
					os.WriteFile(filepath.Join(home, "probes", "codex.lease.json"), []byte(`{}`), 0o644)
				case "lease-directory":
					os.Mkdir(filepath.Join(home, "probes", "codex.lease.json"), 0o700)
				}
			}
			lookup := a2Owner
			if mode == "unknown-owner" {
				lookup = func(int) (agenthooks.ProcessIdentity, error) { return agenthooks.ProcessIdentity{}, os.ErrPermission }
			}
			session, err := Begin(home, []Binding{a2Binding("codex")}, time.Second, Options{Lookup: lookup, OwnerPID: 42})
			if err == nil {
				session.Cleanup()
				session.Close()
				t.Fatal("unsafe probe filesystem accepted")
			}
			data, _ := os.ReadFile(sentinel)
			if string(data) != "canary" {
				t.Fatal("external path changed")
			}
		})
	}
}

func TestM3A17LeaseIdentityRemainsPinnedUntilCleanup(t *testing.T) {
	session, home, _ := a2Session(t, "codex")
	held := session.leases["codex"]
	if held.pin == nil {
		t.Fatal("active lease has no kernel pin")
	}
	if _, err := held.pin.Stat(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "probes", "codex.lease.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, held.data, 0o600); err != nil {
		t.Fatal(err)
	}
	// Even identical bytes and timestamps must not regain the old file identity.
	if err := os.Chtimes(path, held.info.ModTime(), held.info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if session.Active("codex") {
		t.Fatal("replacement became original lease")
	}
	result := session.Cleanup()
	if result.Complete {
		t.Fatal("replacement cleanup should be conservative")
	}
	if _, err := held.pin.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatal("cleanup leaked lease handle", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, held.data) {
		t.Fatal("cleanup removed replacement", err)
	}
}
