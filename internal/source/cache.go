package source

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/inxups/style-cli/internal/saferead"
	"github.com/speakeasy-api/agenthooks/privatefs"
	"runtime"
)

type manifest struct {
	Format int               `json:"format"`
	Kind   string            `json:"kind"`
	Path   string            `json:"path"`
	Git    Git               `json:"git"`
	Files  map[string]string `json:"files"`
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("OS randomness unavailable")
	}
	return hex.EncodeToString(b[:])
}

func cacheRoot(home string, create bool) (*os.Root, error) {
	if create {
		if info, err := os.Lstat(home); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return nil, fail("source-cache-invalid", "source cache requires a regular STYLE_HOME")
		}
		if err := os.MkdirAll(home, 0o700); err != nil {
			return nil, err
		}
		if err := privatefs.PrepareDirectory(home); err != nil {
			return nil, err
		}
	}
	root, err := saferead.AnchorMutable(home)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	current := ""
	for _, part := range []string{"cache", "sources", "git", "v1"} {
		current = filepath.Join(current, part)
		info, e := root.Lstat(current)
		if errors.Is(e, os.ErrNotExist) && create {
			e = root.Mkdir(current, 0o700)
			if e != nil && !errors.Is(e, os.ErrExist) {
				return nil, e
			}
			if e = privatefs.PrepareRoot(root, current); e != nil {
				return nil, e
			}
			info, e = root.Lstat(current)
		}
		if e != nil {
			return nil, e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fail("source-cache-invalid", "source cache contains a non-directory or symlink")
		}
	}
	return root.OpenRoot(current)
}

func readCache(home string, identity Git, kind, p string) (Snapshot, error) {
	root, err := cacheRoot(home, false)
	if err != nil {
		return Snapshot{}, err
	}
	defer root.Close()
	return readEntry(root, cacheKey(identity, kind, p), identity, kind, p)
}

func readEntry(root *os.Root, key string, identity Git, kind, p string) (Snapshot, error) {
	invalid := func() (Snapshot, error) {
		return Snapshot{}, fail("source-cache-invalid", "source cache entry failed verification")
	}
	info, err := root.Lstat(key)
	if err != nil {
		return Snapshot{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 || privatefs.CheckRoot(root, key) != nil {
		return invalid()
	}
	entry, err := root.OpenRoot(key)
	if err != nil {
		return invalid()
	}
	defer entry.Close()
	data, err := saferead.File(entry, "manifest.json", 1<<20)
	if err != nil {
		return invalid()
	}
	var m manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&m) != nil || decoder.Decode(new(any)) != io.EOF || m.Format != 1 || m.Kind != kind || m.Path != p || m.Git.URL != identity.URL || m.Git.ObjectFormat != identity.ObjectFormat || m.Git.Commit != identity.Commit || m.Git.Object != identity.Object || ValidateIdentity(m.Git, m.Kind, m.Path, m.Files) != nil {
		return invalid()
	}
	if identity.ContentSHA256 != "" && m.Git.ContentSHA256 != identity.ContentSHA256 {
		return invalid()
	}
	expected := map[string]bool{".": true, "manifest.json": true, "files": true}
	files := make(map[string][]byte, len(m.Files))
	for p := range m.Files {
		name := filepath.Join("files", filepath.FromSlash(p))
		expected[filepath.ToSlash(name)] = true
		for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
			expected[filepath.ToSlash(parent)] = true
		}
		body, e := saferead.File(entry, name, MaxFileBytes)
		if e != nil {
			return invalid()
		}
		files[p] = body
	}
	err = fs.WalkDir(entry.FS(), ".", func(name string, d fs.DirEntry, e error) error {
		if e != nil || !expected[name] || d.Type()&os.ModeSymlink != 0 {
			return fail("source-cache-invalid", "unexpected cache entry")
		}
		info, e := entry.Lstat(filepath.FromSlash(name))
		if e != nil {
			return e
		}
		if !d.IsDir() && !info.Mode().IsRegular() || privatefs.CheckRoot(entry, filepath.FromSlash(name)) != nil {
			return fail("source-cache-invalid", "cache permissions changed")
		}
		if runtime.GOOS != "windows" && d.IsDir() && info.Mode().Perm() != 0o700 {
			return fail("source-cache-invalid", "cache directory permissions changed")
		}
		return nil
	})
	if err != nil || validateFiles(files) != nil {
		return invalid()
	}
	hashes, content := Hashes(files)
	if content != m.Git.ContentSHA256 || !equalHashes(hashes, m.Files) {
		return invalid()
	}
	return Snapshot{Kind: kind, Path: p, Git: m.Git, Files: files, CacheHit: true}, nil
}

func equalHashes(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for p, h := range a {
		if b[p] != h {
			return false
		}
	}
	return true
}

func mkdirParents(root *os.Root, name string) error {
	current := ""
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		current = filepath.Join(current, part)
		err := root.Mkdir(current, 0o700)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fail("source-cache-invalid", "cache staging directory changed")
		}
	}
	return nil
}
func createPrivate(root *os.Root, name string, data []byte) error {
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err = privatefs.Prepare(f); err != nil {
		f.Close()
		return err
	}
	_, writeErr := f.Write(data)
	return errors.Join(writeErr, f.Close())
}

func publishCache(ctx context.Context, home string, snapshot Snapshot) error {
	if ctx.Err() != nil {
		return fail("canceled", "Git acquisition was canceled")
	}
	root, err := cacheRoot(home, true)
	if err != nil {
		return fail("source-cache-invalid", "cannot prepare the private source cache")
	}
	defer root.Close()
	key := cacheKey(snapshot.Git, snapshot.Kind, snapshot.Path)
	if _, e := readEntry(root, key, snapshot.Git, snapshot.Kind, snapshot.Path); e == nil {
		return nil
	}
	// Random names are owned by this invocation. Nothing scans or deletes other
	// publishers' staging directories, and only complete entries are renamed.
	stage := ""
	for attempts := 0; attempts < 10; attempts++ {
		stage = ".stage-" + randomID()
		if err = root.Mkdir(stage, 0o700); err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return fail("source-cache-invalid", "cannot stage source cache")
		}
	}
	if err != nil {
		return fail("source-cache-invalid", "cannot stage source cache")
	}
	defer root.RemoveAll(stage)
	if err = mkdirParents(root, filepath.Join(stage, "files")); err != nil {
		return fail("source-cache-invalid", "cannot stage source cache directories")
	}
	paths := make([]string, 0, len(snapshot.Files))
	for p := range snapshot.Files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	for _, p := range paths {
		if ctx.Err() != nil {
			return fail("canceled", "Git acquisition was canceled")
		}
		name := filepath.Join(stage, "files", filepath.FromSlash(p))
		if err = mkdirParents(root, filepath.Dir(name)); err != nil {
			return fail("source-cache-invalid", "cannot stage source cache directories")
		}
		if err = createPrivate(root, name, snapshot.Files[p]); err != nil {
			return fail("source-cache-invalid", "cannot write source cache")
		}
	}
	hashes, _ := Hashes(snapshot.Files)
	data, _ := json.Marshal(manifest{Format: 1, Kind: snapshot.Kind, Path: snapshot.Path, Git: snapshot.Git, Files: hashes})
	if err = createPrivate(root, filepath.Join(stage, "manifest.json"), data); err != nil {
		return fail("source-cache-invalid", "cannot write source manifest")
	}
	if _, err = readEntry(root, stage, snapshot.Git, snapshot.Kind, snapshot.Path); err != nil {
		return err
	}
	for attempts := 0; attempts < 5; attempts++ {
		if ctx.Err() != nil {
			return fail("canceled", "Git acquisition was canceled")
		}
		if _, e := readEntry(root, key, snapshot.Git, snapshot.Kind, snapshot.Path); e == nil {
			return nil
		}
		if e := root.Rename(stage, key); e == nil {
			return nil
		}
		// Quarantine a corrupt entry by name; removing a symlink here never
		// follows its target. A concurrent complete publisher is reverified.
		if _, e := readEntry(root, key, snapshot.Git, snapshot.Kind, snapshot.Path); e == nil {
			return nil
		}
		quarantine := ".invalid-" + randomID()
		if e := root.Rename(key, quarantine); e != nil {
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			return fail("source-cache-invalid", "cannot replace invalid source cache")
		}
		if e := root.RemoveAll(quarantine); e != nil {
			return fail("source-cache-invalid", "cannot remove quarantined source cache")
		}
	}
	return fail("source-cache-invalid", "concurrent source cache publication did not settle")
}

// ReadLocked reads and verifies one complete existing export. It never starts
// Git, creates directories, repairs an entry or publishes cache files.
func ReadLocked(ctx context.Context, home string, identity Git, kind, p string, hashes map[string]string) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := ValidateIdentity(identity, kind, p, hashes); err != nil {
		return Snapshot{}, err
	}
	snapshot, err := readCache(home, identity, kind, p)
	if err != nil {
		return Snapshot{}, err
	}
	actual, _ := Hashes(snapshot.Files)
	if !equalHashes(actual, hashes) {
		return Snapshot{}, fail("source-cache-invalid", "locked cache file hashes differ")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	// A shared immutable export may have been acquired under another ref. The
	// selector belongs to the lock; the cache authenticates its fixed objects.
	snapshot.Git = identity
	return snapshot, nil
}
