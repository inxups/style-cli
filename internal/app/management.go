package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/saferead"
	"github.com/speakeasy-api/agenthooks"
	"github.com/speakeasy-api/agenthooks/install"
)

var errManagementChanged = errors.New("management-input-changed: captured installation inputs changed; rerun the command to review a fresh candidate")
var errManagementBusy = errors.New("management-busy: another management commit is active or its owner cannot be verified")

type inputFile struct {
	info os.FileInfo
	data []byte
}
type inputTree struct {
	path  string
	files map[string]inputFile
}
type managementInputs struct {
	trees   map[string]inputTree
	parents map[string]os.FileInfo
}

func captureManagement(home string) (*managementInputs, error) {
	inputs := &managementInputs{trees: map[string]inputTree{}, parents: map[string]os.FileInfo{}}
	for _, name := range []string{"style.lock.yaml", "packages", "approved", "settings", "build/index.json", "build/explain.json"} {
		if err := inputs.add(filepath.Join(home, filepath.FromSlash(name))); err != nil {
			return nil, err
		}
	}
	return inputs, nil
}

func captureTree(path string) (inputTree, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return inputTree{}, err
	}
	tree := inputTree{path: path, files: map[string]inputFile{}}
	_, err = os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return tree, nil
	}
	if err != nil {
		return tree, err
	}
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		if rel != "." {
			switch d.Name() {
			case ".git", ".hg", ".svn", ".DS_Store":
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return errManagementChanged
		}
		file := inputFile{info: info}
		if info.Mode()&os.ModeSymlink != 0 {
			target, e := os.Readlink(p)
			if e != nil {
				return e
			}
			file.data = []byte("symlink\x00" + target)
		}
		if info.Mode().IsRegular() {
			file.data, err = saferead.ReadFile(p, info.Size())
			if err != nil {
				return err
			}
			after, e := os.Lstat(p)
			if e != nil || !saferead.Same(info, after) {
				return errManagementChanged
			}
		}
		tree.files[rel] = file
		return nil
	})
	return tree, err
}
func (m *managementInputs) add(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, exists := m.trees[path]; exists {
		return nil
	}
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		if _, ok := m.parents[parent]; !ok {
			info, e := os.Lstat(parent)
			if e == nil {
				m.parents[parent] = info
			} else if !errors.Is(e, os.ErrNotExist) {
				return e
			}
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	tree, err := captureTree(path)
	if err != nil {
		return err
	}
	m.trees[path] = tree
	return nil
}
func (m *managementInputs) links(lock lockDocument) error {
	for _, item := range lock.Packages {
		if item.Link {
			if err := m.add(item.Source.Path); err != nil {
				return err
			}
		}
	}
	return nil
}
func (m *managementInputs) routes(adapters []installAdapter) error {
	for _, target := range adapters {
		if route, ok := target.route.(adapter.ManagementInputs); ok {
			paths, err := route.ManagementPaths()
			if err != nil {
				return err
			}
			for _, p := range paths {
				if err := m.add(p); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (m *managementInputs) planned(files []install.PlannedFile) error {
	for _, file := range files {
		if err := m.add(file.Path); err != nil {
			return err
		}
		tree := m.trees[filepath.Clean(file.Path)]
		actual, exists := tree.files["."]
		if exists != (file.Before != nil) || exists && (actual.info.IsDir() || !bytes.Equal(actual.data, file.Before)) {
			return errManagementChanged
		}
	}
	return m.verify()
}
func (m *managementInputs) verify() error {
	for p, before := range m.parents {
		now, err := os.Lstat(p)
		if err != nil || !os.SameFile(before, now) || before.Mode() != now.Mode() {
			return errManagementChanged
		}
	}
	paths := make([]string, 0, len(m.trees))
	for p := range m.trees {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		before := m.trees[p]
		now, err := captureTree(p)
		if err != nil || len(before.files) != len(now.files) {
			return errManagementChanged
		}
		for rel, original := range before.files {
			actual, ok := now.files[rel]
			if !ok || !os.SameFile(original.info, actual.info) || original.info.Mode() != actual.info.Mode() || !bytes.Equal(original.data, actual.data) {
				return errManagementChanged
			}
			if original.info.Mode().IsRegular() && !saferead.Same(original.info, actual.info) {
				return errManagementChanged
			}
		}
	}
	return nil
}

type managementOwner struct {
	PID   int    `json:"pid"`
	Birth string `json:"birth"`
	Nonce string `json:"nonce"`
}
type managementGuard struct {
	root *os.Root
	data []byte
	info os.FileInfo
}

const managementName = ".management.lock"

func acquireManagement(ctx context.Context, home string) (*managementGuard, error) {
	return acquireManagementWith(ctx, home, agenthooks.ReadProcessIdentity)
}
func acquireManagementWith(ctx context.Context, home string, lookup func(int) (agenthooks.ProcessIdentity, error)) (*managementGuard, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, err := lookup(os.Getpid())
	if err != nil || owner.PID != os.Getpid() || owner.Birth == "" {
		return nil, errManagementBusy
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	root, err := saferead.AnchorMutable(home)
	if err != nil {
		return nil, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		root.Close()
		return nil, err
	}
	identity := managementOwner{owner.PID, owner.Birth, hex.EncodeToString(nonce[:])}
	data, _ := json.Marshal(identity)
	stage := ".management-" + identity.Nonce
	acquired := false
	defer func() {
		_ = root.Remove(stage)
		if !acquired {
			_ = root.Close()
		}
	}()
	f, err := root.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	_, writeErr := f.Write(data)
	err = errors.Join(writeErr, f.Close())
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err = root.Link(stage, managementName); err == nil {
			info, e := root.Lstat(managementName)
			if e != nil {
				return nil, e
			}
			acquired = true
			return &managementGuard{root, data, info}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		oldData, e := saferead.File(root, managementName, 1024)
		var old managementOwner
		info, statErr := root.Lstat(managementName)
		if e != nil || statErr != nil || info.Mode().Perm() != 0o600 || json.Unmarshal(oldData, &old) != nil || old.PID <= 0 || old.Birth == "" || len(old.Nonce) != 32 {
			return nil, errManagementBusy
		}
		process, e := lookup(old.PID)
		if e == nil && (process.PID != old.PID || process.Birth == "" || process.Birth == old.Birth) || e != nil && !errors.Is(e, agenthooks.ErrProcessGone) {
			return nil, errManagementBusy
		}
		stale := &managementGuard{root: root, data: oldData, info: info}
		if err = stale.remove(); err != nil {
			return nil, errManagementBusy
		}
	}
	return nil, errManagementBusy
}
func (g *managementGuard) remove() error {
	data, err := saferead.File(g.root, managementName, 1024)
	if err != nil {
		return err
	}
	info, err := g.root.Lstat(managementName)
	if err != nil || !os.SameFile(info, g.info) || !bytes.Equal(data, g.data) {
		return errManagementChanged
	}
	// Move then check identity before deletion. Never remove a new owner's lock.
	stopped := ".management-stopped-" + hashBytes(g.data)[7:]
	if _, err := g.root.Lstat(stopped); !errors.Is(err, os.ErrNotExist) {
		return errManagementChanged
	}
	if err = g.root.Rename(managementName, stopped); err != nil {
		return err
	}
	data, err = saferead.File(g.root, stopped, 1024)
	moved, statErr := g.root.Lstat(stopped)
	if err != nil || statErr != nil || !os.SameFile(moved, g.info) || !bytes.Equal(data, g.data) {
		if g.root.Link(stopped, managementName) == nil {
			_ = g.root.Remove(stopped)
		}
		return errManagementChanged
	}
	return g.root.Remove(stopped)
}
func (g *managementGuard) close() error {
	defer g.root.Close()
	if err := g.remove(); err != nil {
		return fmt.Errorf("release management guard: %w", err)
	}
	return nil
}
