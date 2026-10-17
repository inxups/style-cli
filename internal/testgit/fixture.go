// Package testgit supplies controlled real-Git repositories to offline tests.
// It is imported only by tests; no CLI/environment option enables file transport.
package testgit

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/inxups/style-cli/internal/source"
)

const URL = "git+https://fixture.invalid/team/styles.git"

type Entry struct {
	Path, Mode, OID string
	Data            []byte
}
type Fixture struct {
	T                               testing.TB
	Root, Repo, Git, Format, Commit string
	env                             []string
	count                           int
	mu                              sync.Mutex
	Commands                        [][]string
	Before                          func(source.Command) error
}

func New(t testing.TB, format string, entries []Entry) *Fixture {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("Git is required for A3 fixtures: ", err)
	}
	f := &Fixture{T: t, Root: t.TempDir(), Git: git, Format: format}
	f.Repo = filepath.Join(f.Root, "repository.git")
	for _, name := range []string{"home", "template", "scratch"} {
		if err := os.Mkdir(filepath.Join(f.Root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	f.env = []string{"PATH=" + filepath.Dir(git) + string(os.PathListSeparator) + os.Getenv("PATH"), "HOME=" + filepath.Join(f.Root, "home"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "LC_ALL=C", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid", "GIT_AUTHOR_DATE=2026-10-17T00:00:00Z", "GIT_COMMITTER_DATE=2026-10-17T00:00:00Z"}
	cmd := exec.Command(git, "init", "--bare", "--object-format="+format, "--initial-branch=main", "--template="+filepath.Join(f.Root, "template"), f.Repo)
	cmd.Env = f.env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture init: %v %s", err, out)
	}
	f.MakeCommit(entries)
	return f
}
func (f *Fixture) Run(input []byte, args ...string) string {
	f.T.Helper()
	all := append([]string{"--git-dir=" + f.Repo, "-c", "core.hooksPath=" + filepath.Join(f.Root, "template"), "-c", "commit.gpgSign=false"}, args...)
	cmd := exec.Command(f.Git, all...)
	cmd.Env = f.env
	cmd.Dir = f.Root
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.T.Fatalf("fixture Git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
func (f *Fixture) MakeCommit(entries []Entry) string {
	f.T.Helper()
	// Build raw tree objects rather than a platform index: Git for Windows may
	// silently omit filenames its working tree cannot represent. Export tests
	// must still receive those forbidden raw Git entries and reject them.
	type node struct {
		mode, oid string
		children  map[string]*node
	}
	root := &node{children: map[string]*node{}}
	blobs := map[string]string{}
	for _, e := range entries {
		mode := e.Mode
		if mode == "" {
			mode = "100644"
		}
		oid := e.OID
		if oid == "" {
			oid = blobs[string(e.Data)]
			if oid == "" {
				oid = f.Run(e.Data, "hash-object", "-w", "--stdin")
				blobs[string(e.Data)] = oid
			}
		}
		parent := root
		parts := strings.Split(e.Path, "/")
		for _, part := range parts[:len(parts)-1] {
			if parent.children[part] == nil {
				parent.children[part] = &node{children: map[string]*node{}}
			}
			parent = parent.children[part]
		}
		parent.children[parts[len(parts)-1]] = &node{mode: mode, oid: oid}
	}
	var treeObject func(*node) string
	treeObject = func(parent *node) string {
		names := make([]string, 0, len(parent.children))
		for name := range parent.children {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool {
			left, right := names[i], names[j]
			if parent.children[left].children != nil {
				left += "/"
			}
			if parent.children[right].children != nil {
				right += "/"
			}
			return left < right
		})
		var raw bytes.Buffer
		for _, name := range names {
			child := parent.children[name]
			mode, oid := child.mode, child.oid
			if child.children != nil {
				mode, oid = "40000", treeObject(child)
			}
			binary, err := hex.DecodeString(oid)
			if err != nil {
				f.T.Fatal(err)
			}
			fmt.Fprintf(&raw, "%s %s\x00", mode, name)
			raw.Write(binary)
		}
		return f.Run(raw.Bytes(), "hash-object", "-t", "tree", "--literally", "-w", "--stdin")
	}
	tree := treeObject(root)
	f.count++
	args := []string{"commit-tree", tree}
	if f.Commit != "" {
		args = append(args, "-p", f.Commit)
	}
	f.Commit = f.Run([]byte(fmt.Sprintf("fixture %d\n", f.count)), args...)
	f.Run(nil, "update-ref", "refs/heads/main", f.Commit)
	return f.Commit
}
func (f *Fixture) Resolver() *source.Resolver {
	return &source.Resolver{Runner: runner{f}, GitPath: f.Git, TempDir: filepath.Join(f.Root, "scratch")}
}
func (f *Fixture) RunCommand(ctx context.Context, c source.Command) error {
	f.mu.Lock()
	f.Commands = append(f.Commands, append([]string(nil), c.Args...))
	before := f.Before
	f.mu.Unlock()
	if before != nil {
		if err := before(c); err != nil {
			return err
		}
	}
	args := append([]string{"-c", "protocol.file.allow=always"}, c.Args...)
	for i, arg := range args {
		if arg == strings.TrimPrefix(URL, "git+") {
			args[i] = "file://" + filepath.ToSlash(f.Repo)
		}
	}
	c.Args = args
	return (source.ExecRunner{}).Run(ctx, c)
}

// runner adapts to source.Runner without colliding with fixture setup Run.
type runner struct{ fixture *Fixture }

func (r runner) Run(ctx context.Context, c source.Command) error { return r.fixture.RunCommand(ctx, c) }
