package agenthooks

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestM3A2KernelProcessIdentity(t *testing.T) {
	identity, err := ReadProcessIdentity(os.Getpid())
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		if !errors.Is(err, ErrProcessUnsupported) {
			t.Fatalf("platform boundary: %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if identity.PID != os.Getpid() || identity.ParentPID != os.Getppid() || identity.Birth == "" || !filepath.IsAbs(identity.Executable) || len(identity.ExecutableID) != 71 {
		t.Fatalf("incomplete OS identity: %+v", identity)
	}
	again, err := ReadProcessIdentity(os.Getpid())
	if err != nil || identity.Birth != again.Birth || identity.ExecutableID != again.ExecutableID {
		t.Fatal("unstable process identity")
	}
	if _, err = ReadProcessIdentity(-1); !errors.Is(err, ErrProcessGone) {
		t.Fatalf("invalid PID: %v", err)
	}
	if _, err = ReadProcessIdentity(2147483647); !errors.Is(err, ErrProcessGone) {
		t.Fatalf("missing PID: %v", err)
	}
}
func TestM3A2ExecutableMetadataIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, []byte("bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := ExecutableIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, func() os.FileMode {
		if runtime.GOOS == "windows" {
			return 0o400
		}
		return 0o600
	}()); err != nil {
		t.Fatal(err)
	}
	after, err := ExecutableIdentity(path)
	if err != nil || before == after {
		t.Fatal("file mode not bound")
	}
	if _, err = ExecutableIdentity("relative"); err == nil {
		t.Fatal("relative executable accepted")
	}
	if _, err = ExecutableIdentity(filepath.Dir(path)); err == nil {
		t.Fatal("directory accepted")
	}
}
