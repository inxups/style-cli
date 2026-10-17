package saferead

import (
	"os"
	"path/filepath"
	"testing"
)

func TestM3A2MutableAnchorRetainsDirectoryOwnership(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := AnchorMutable(home)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = os.Rename(home, home+"-original"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := root.OpenFile("private", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err = os.Stat(filepath.Join(home, "private")); !os.IsNotExist(err) {
		t.Fatal("anchor followed replacement directory")
	}
	if _, err = os.Stat(filepath.Join(home+"-original", "private")); err != nil {
		t.Fatal(err)
	}
	link := home + "-alias"
	if err = os.Symlink(home, link); err != nil {
		t.Fatal(err)
	}
	if unsafe, err := AnchorMutable(link); err == nil {
		unsafe.Close()
		t.Fatal("mutable anchor accepted a symlink")
	}
}
