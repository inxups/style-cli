package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestSnapshotPackageSkipsVersionControlAndFinderFiles(t *testing.T) {
	source := t.TempDir()
	write := func(rel string, data []byte) {
		t.Helper()
		path := filepath.Join(source, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("STYLE.yaml", []byte("spec: 1\n"))
	write("rules/a.md", []byte("rule\n"))
	write(".DS_Store", []byte{0, 1})
	write("rules/.DS_Store", []byte{0, 1})
	write(".git/objects/pack/big.pack", make([]byte, 300*1024))
	write(".hg/store", []byte("x"))
	write(".svn/entries", []byte("x"))
	write("sub/.git", []byte("gitdir: ../.git/worktrees/sub\n"))
	if err := os.Symlink("/nonexistent", filepath.Join(source, "rules", ".git")); err != nil {
		t.Fatal(err)
	}
	files, single, _, err := snapshotPackage(source)
	if err != nil || single {
		t.Fatalf("snapshotPackage = single %v, err %v", single, err)
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if want := []string{"STYLE.yaml", "rules/a.md"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("snapshot paths = %q, want %q", paths, want)
	}
}

func TestInstallFromCheckoutIgnoresGitAndFinderFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "checkout")
	if err := os.MkdirAll(filepath.Join(source, ".git", "objects"), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "testdata", "manual", "m1b-probes", "STYLE.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string][]byte{
		"STYLE.yaml":            data,
		".DS_Store":             {0, 1},
		".git/objects/big.pack": make([]byte, 300*1024),
	} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(path)), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	styleHome := filepath.Join(root, "style")
	configDir := filepath.Join(root, "codex")
	service := testCodexInstallService(t, configDir, filepath.Join(styleHome, "adapters", "codex", "plugin"))
	if _, err := service.Install(context.Background(), InstallRequest{Source: source, Home: styleHome, Harnesses: []string{"codex"}, Yes: true}); err != nil {
		t.Fatalf("install from a checkout: %v", err)
	}
	stored := filepath.Join(styleHome, "packages", "m1b-probes")
	for _, name := range []string{".git", ".DS_Store"} {
		if _, err := os.Lstat(filepath.Join(stored, name)); !os.IsNotExist(err) {
			t.Fatalf("%s was copied into package storage: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(stored, ".DS_Store"), []byte{2}, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := service.Sync(context.Background(), SyncRequest{Home: styleHome})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.NeedsTrust) != 0 || len(result.Changes) != 0 {
		t.Fatalf("a Finder file in package storage must not need trust: %#v", result)
	}
}
