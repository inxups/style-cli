package source

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type a5CacheFile struct {
	Data string
	Mode os.FileMode
	Time time.Time
	Info os.FileInfo
}

func a5CacheState(t *testing.T, home string) map[string]a5CacheFile {
	t.Helper()
	state := map[string]a5CacheFile{}
	err := filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		value := a5CacheFile{Mode: info.Mode(), Time: info.ModTime(), Info: info}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			value.Data = string(data)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			value.Data, err = os.Readlink(p)
			if err != nil {
				return err
			}
		}
		state[p] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}
func TestM3A5ReadLockedCacheIsReadOnly(t *testing.T) {
	for _, kind := range []string{"dir", "file"} {
		for _, phase := range []string{"complete", "ref-alias", "missing-home", "missing-cache", "bad-content", "extra-file", "manifest", "symlink", "mode", "hash-mismatch", "canceled", "identity"} {
			t.Run(kind+"/"+phase, func(t *testing.T) {
				home := filepath.Join(t.TempDir(), "style")
				name := "STYLE.yaml"
				if kind == "file" {
					name = "single.style.md"
				}
				files := map[string][]byte{name: []byte("locked\n")}
				path := "pkg"
				if kind == "file" {
					path = "nested/single.style.md"
				}
				hashes, content := Hashes(files)
				identity := Git{URL: "https://example.invalid/repo.git", Ref: "refs/heads/main", ObjectFormat: "sha1", Commit: strings.Repeat("a", 40), Object: strings.Repeat("b", 40), ContentSHA256: content}
				if err := publishCache(context.Background(), home, Snapshot{Kind: kind, Path: path, Git: identity, Files: files}); err != nil {
					t.Fatal(err)
				}
				entry := filepath.Join(home, "cache/sources/git/v1", cacheKey(identity, kind, path))
				ctx := context.Background()
				switch phase {
				case "ref-alias":
					identity.Ref = "refs/tags/locked"
				case "missing-home":
					if err := os.RemoveAll(home); err != nil {
						t.Fatal(err)
					}
				case "missing-cache":
					if err := os.RemoveAll(filepath.Join(home, "cache")); err != nil {
						t.Fatal(err)
					}
				case "bad-content":
					if err := os.WriteFile(filepath.Join(entry, "files/"+name), []byte("drift"), 0o600); err != nil {
						t.Fatal(err)
					}
				case "extra-file":
					if err := os.WriteFile(filepath.Join(entry, "extra"), []byte("extra"), 0o600); err != nil {
						t.Fatal(err)
					}
				case "manifest":
					if err := os.WriteFile(filepath.Join(entry, "manifest.json"), []byte("{}"), 0o600); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					p := filepath.Join(entry, "files/"+name)
					if err := os.Remove(p); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(filepath.Join(entry, "manifest.json"), p); err != nil {
						t.Fatal(err)
					}
				case "mode":
					if err := os.Chmod(entry, 0o755); err != nil {
						t.Fatal(err)
					}
				case "hash-mismatch":
					hashes = map[string]string{name: "sha256:" + strings.Repeat("c", 64)}
					identity.ContentSHA256 = Aggregate(hashes)
				case "canceled":
					canceled, cancel := context.WithCancel(ctx)
					cancel()
					ctx = canceled
				case "identity":
					identity.Commit = "invalid"
				}
				before := a5CacheState(t, home)
				result, err := ReadLocked(ctx, home, identity, kind, path, hashes)
				if phase == "complete" || phase == "ref-alias" {
					if err != nil || !reflect.DeepEqual(result.Files, files) || result.Git != identity || !result.CacheHit {
						t.Fatalf("%+v %v", result, err)
					}
				} else if err == nil {
					t.Fatal("invalid/missing cache accepted")
				}
				after := a5CacheState(t, home)
				if len(before) != len(after) {
					t.Fatal("cache paths changed")
				}
				for p, a := range before {
					b, ok := after[p]
					if !ok || a.Data != b.Data || a.Mode != b.Mode || !a.Time.Equal(b.Time) || !os.SameFile(a.Info, b.Info) {
						t.Fatalf("cache mutated: %s", p)
					}
				}
			})
		}
	}
}
