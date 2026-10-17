package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestM3A17PrivateRecordAndIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "私有 record")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = Restrict(file); err != nil {
		t.Fatal(err)
	}
	if err = Check(file); err != nil {
		t.Fatal(err)
	}
	if err = CheckPath(path); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = CheckRoot(root, filepath.Base(path)); err != nil {
		t.Fatal(err)
	}
}
