//go:build !windows

package saferead

import (
	"path/filepath"
	"syscall"
	"testing"
)

func TestM3A1FIFORefusedWithoutOpening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.fifo")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(path, 32<<20); err == nil {
		t.Fatal("FIFO was accepted as a replay input")
	}
}
