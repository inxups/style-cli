package probe

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/speakeasy-api/agenthooks"
)

func a2Observer(t *testing.T, session *Session, home string, now time.Time) *Observer {
	t.Helper()
	observer := NewObserver(home, home, "codex", "")
	if observer == nil {
		t.Fatal("active observer absent")
	}
	observer.now = func() time.Time { return now }
	observer.clock = func() time.Time { return now }
	observer.pid = 100
	lease := session.Lease("codex")
	observer.lookup = func(pid int) (agenthooks.ProcessIdentity, error) {
		switch pid {
		case 100:
			return agenthooks.ProcessIdentity{PID: 100, ParentPID: 200, Birth: "hook-birth", ExecutableID: lease.Binding.RuntimeID}, nil
		case 200:
			return agenthooks.ProcessIdentity{PID: 200, ParentPID: 1, Birth: "harness-birth", ExecutableID: lease.Binding.HarnessID}, nil
		}
		return agenthooks.ProcessIdentity{}, agenthooks.ErrProcessGone
	}
	return observer
}
func a2Await(t *testing.T, observer *Observer) {
	t.Helper()
	select {
	case <-observer.done:
	case <-time.After(time.Second):
		t.Fatal("observer did not complete")
	}
}

func TestM3A2ObserverPrivacyAndInactivePath(t *testing.T) {
	home := t.TempDir()
	if observer := NewObserver(home, home, "codex", ""); observer != nil {
		t.Fatal("inactive observer allocated work")
	}
	if _, err := os.Stat(filepath.Join(home, "probes")); !os.IsNotExist(err) {
		t.Fatal("inactive observer created files")
	}
	session, home, now := a2Session(t, "codex")
	observer := a2Observer(t, session, home, now)
	observer.Submit(Metadata{"codex", "PreToolUse", "SESSION_SECRET\x1b[31m", "AGENT_SECRET", "TURN_SECRET"})
	a2Await(t, observer)
	receipts, quality := session.Receipts("codex")
	if len(receipts) != 1 || len(quality) != 0 {
		t.Fatalf("missing receipt: %+v %+v", receipts, quality)
	}
	data, err := os.ReadFile(filepath.Join(home, "probes", session.Lease("codex").ReceiptDir, "00.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{"SESSION_SECRET", "AGENT_SECRET", "TURN_SECRET", home, "argv", "environment", "prompt", "payload"} {
		if bytes.Contains(data, []byte(canary)) {
			t.Fatalf("receipt leaked %q", canary)
		}
	}
	observer.Submit(Metadata{"codex", "PreToolUse", "another", "", ""})
	again, _ := session.Receipts("codex")
	if len(again) != 1 {
		t.Fatal("submitted duplicate work")
	}
}

func TestM3A2ObserverBlockedDependenciesAndLateWrites(t *testing.T) {
	session, home, now := a2Session(t, "codex")
	observer := a2Observer(t, session, home, now)
	entered, release := make(chan struct{}), make(chan struct{})
	lookup := observer.lookup
	observer.lookup = func(pid int) (agenthooks.ProcessIdentity, error) {
		if pid == 100 {
			close(entered)
			<-release
		}
		return lookup(pid)
	}
	returned := make(chan struct{})
	go func() { observer.Submit(Metadata{"codex", "PreToolUse", "session", "", ""}); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Submit waited for process reader")
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reader not entered")
	}
	cleanup := session.Cleanup()
	if !cleanup.Complete {
		t.Fatalf("cleanup waited/failed: %+v", cleanup)
	}
	close(release)
	a2Await(t, observer)
	if _, err := os.Stat(filepath.Join(home, "probes", "runs", session.RunID)); !os.IsNotExist(err) {
		t.Fatal("late write resurrected run")
	}
}

func TestM3A2ObserverBudgetAndLeaseRaces(t *testing.T) {
	for _, mode := range []string{"budget", "expired", "lease-replaced", "lease-removed", "wrong-event", "wrong-session", "wrong-root", "limit", "lookup-error", "deep-chain"} {
		t.Run(mode, func(t *testing.T) {
			session, home, now := a2Session(t, "codex")
			observer := a2Observer(t, session, home, now)
			lookup := observer.lookup
			calls := 0
			observer.lookup = func(pid int) (agenthooks.ProcessIdentity, error) {
				calls++
				if calls == 1 {
					switch mode {
					case "budget":
						observer.clock = func() time.Time { return now.Add(ObserverBudget) }
					case "expired":
						observer.now = func() time.Time { return now.Add(time.Hour) }
					case "lease-replaced":
						path := filepath.Join(home, "probes", "codex.lease.json")
						data, _ := os.ReadFile(path)
						os.Remove(path)
						os.WriteFile(path, data, 0o600)
					case "lease-removed":
						os.Remove(filepath.Join(home, "probes", "codex.lease.json"))
					}
				}
				if mode == "lookup-error" {
					return agenthooks.ProcessIdentity{}, os.ErrPermission
				}
				if mode == "deep-chain" {
					return agenthooks.ProcessIdentity{PID: pid, ParentPID: pid + 1, Birth: "born", ExecutableID: session.Lease("codex").Binding.RuntimeID}, nil
				}
				return lookup(pid)
			}
			meta := Metadata{"codex", "PreToolUse", "session", "", ""}
			if mode == "wrong-event" {
				meta.Event = "Stop"
			}
			if mode == "wrong-session" {
				meta.Session = ""
			}
			if mode == "wrong-root" {
				observer.harnessHome = t.TempDir()
			}
			if mode == "limit" {
				for index := 0; index < MaxReceipts; index++ {
					a2WriteReceipt(t, home, session.Lease("codex"), receiptFilename(index), a2Receipt(session.Lease("codex")))
				}
			}
			observer.Submit(meta)
			if mode == "wrong-session" {
				if calls != 0 {
					t.Fatal("empty session performed work")
				}
				return
			}
			a2Await(t, observer)
			receipts, _ := session.Receipts("codex")
			if mode == "limit" {
				if len(receipts) != MaxReceipts {
					t.Fatal("receipt limit changed")
				}
			} else if mode == "deep-chain" {
				if len(receipts) != 1 || calls != MaxAncestors+1 {
					t.Fatalf("ancestor bound not applied: %d %d", len(receipts), calls)
				}
			} else if len(receipts) != 0 {
				t.Fatalf("invalid observation recorded: %s %v", mode, receipts)
			}
		})
	}
}
