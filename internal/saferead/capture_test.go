package saferead

import (
	"os"
	"path/filepath"
	"testing"
)

func TestM3A1BoundedRegularFileAndAnchors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "input")
	if err := os.WriteFile(path, []byte("abcd"), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := ReadFile(path, 4); err != nil || string(data) != "abcd" {
		t.Fatalf("%q %v", data, err)
	}
	if _, err := ReadFile(path, 3); err == nil {
		t.Fatal("oversize accepted")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(link, 4); err == nil {
		t.Fatal("symlink accepted")
	}
	if _, err := ReadFile(root, 4); err == nil {
		t.Fatal("directory accepted")
	}
	anchor, err := Anchor(root)
	if err != nil {
		t.Fatal(err)
	}
	defer anchor.Close()
	if _, err := File(anchor, "link", 4); err == nil {
		t.Fatal("linked file read")
	}
}
