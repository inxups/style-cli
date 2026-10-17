//go:build darwin || linux

package probe

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestM3A2SpecialFilesCannotBlockReaders(t *testing.T) {
	for _, mode := range []string{"lease-fifo", "receipt-fifo"} {
		t.Run(mode, func(t *testing.T) {
			session, home, _ := a2Session(t, "codex")
			if mode == "lease-fifo" {
				path := filepath.Join(home, "probes", "codex.lease.json")
				if err := os.Rename(path, path+"-previous"); err != nil {
					t.Fatal(err)
				}
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
				if observer := NewObserver(home, home, "codex", ""); observer != nil {
					t.Fatal("FIFO became an observer")
				}
				if next, err := Begin(home, []Binding{a2Binding("codex")}, time.Second, Options{Lookup: a2Owner, OwnerPID: 84}); err == nil || !errors.Is(err, ErrBusy) {
					if next != nil {
						next.Cleanup()
						next.Close()
					}
					t.Fatalf("unsafe FIFO lease: %v", err)
				}
			} else {
				path := filepath.Join(home, "probes", session.Lease("codex").ReceiptDir, "00.json")
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
				receipts, quality := session.Receipts("codex")
				if len(receipts) != 0 || len(quality) == 0 {
					t.Fatal("FIFO receipt accepted")
				}
			}
		})
	}
}
