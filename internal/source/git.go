package source

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Command and Runner are trusted dependency ports for controlled transport and
// fault fixtures. They are never populated from CLI options or environment.
type Command struct {
	Program        string
	Args, Env      []string
	Dir            string
	Stdout, Stderr io.Writer
}
type Runner interface {
	Run(context.Context, Command) error
}
type RunnerFunc func(context.Context, Command) error

func (f RunnerFunc) Run(ctx context.Context, c Command) error { return f(ctx, c) }

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, c Command) error {
	cmd := exec.CommandContext(ctx, c.Program, c.Args...)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = c.Dir, c.Env, c.Stdout, c.Stderr
	cmd.WaitDelay = 250 * time.Millisecond
	isolateProcess(cmd)
	return cmd.Run()
}

type Resolver struct {
	Runner           Runner
	GitPath, TempDir string
}
type gitSession struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	runner              Runner
	program, root, repo string
	env, flags          []string
}

func (r *Resolver) Resolve(ctx context.Context, request Request) (Snapshot, error) {
	selector, err := Parse(request)
	if err != nil {
		return Snapshot{}, err
	}
	if ctx == nil {
		return Snapshot{}, fail("source-invalid", "a context is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return Snapshot{}, fail("canceled", "Git acquisition was canceled")
	}
	program := r.GitPath
	if program == "" {
		program, err = exec.LookPath("git")
	}
	if err != nil || program == "" {
		return Snapshot{}, fail("git-unavailable", "a Git executable is required")
	}
	program, err = filepath.Abs(program)
	if err != nil {
		return Snapshot{}, fail("git-unavailable", "Git executable cannot be resolved")
	}
	program, err = filepath.EvalSymlinks(program)
	if err != nil {
		return Snapshot{}, fail("git-unavailable", "Git executable cannot be resolved")
	}
	root, err := os.MkdirTemp(r.TempDir, "style-git-*")
	if err != nil {
		return Snapshot{}, fail("source-fetch-failed", "cannot create private Git workspace")
	}
	defer os.RemoveAll(root)
	for _, dir := range []string{"home", "xdg", "template", "hooks"} {
		if os.Mkdir(filepath.Join(root, dir), 0o700) != nil {
			return Snapshot{}, fail("source-fetch-failed", "cannot prepare private Git workspace")
		}
	}
	runner := r.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	g := &gitSession{ctx: ctx, cancel: cancel, runner: runner, program: program, root: root, repo: filepath.Join(root, "repo")}
	g.env = []string{
		"PATH=" + filepath.Dir(program) + string(os.PathListSeparator) + "/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + filepath.Join(root, "home"), "XDG_CONFIG_HOME=" + filepath.Join(root, "xdg"), "LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM=" + os.DevNull, "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CEILING_DIRECTORIES=" + filepath.Dir(root),
		"GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1", "GIT_ATTR_NOSYSTEM=1", "GIT_NO_LAZY_FETCH=1",
	}
	g.flags = []string{"--no-replace-objects", "--literal-pathspecs"}
	for _, setting := range []string{"protocol.allow=never", "protocol.https.allow=always", "http.followRedirects=false", "http.sslVerify=true", "http.proxy=", "http.emptyAuth=false", "http.proactiveAuth=none", "credential.interactive=false", "credential.helper=", "core.askPass=", "core.hooksPath=" + filepath.Join(root, "hooks"), "core.fsmonitor=false", "init.templateDir=" + filepath.Join(root, "template"), "gc.auto=0", "maintenance.auto=false", "fetch.recurseSubmodules=false", "fetch.writeCommitGraph=false", "fetch.fsckObjects=true", "transfer.fsckObjects=true"} {
		g.flags = append(g.flags, "-c", setting)
	}
	version, err := g.run(256, false, "--version")
	if err != nil {
		if ctx.Err() != nil {
			return Snapshot{}, fail("canceled", "Git acquisition was canceled")
		}
		return Snapshot{}, fail("git-unavailable", "Git executable could not be started")
	}
	gitVersion := strings.TrimSpace(string(version))
	if !strings.HasPrefix(gitVersion, "git version ") || hasControl(gitVersion) {
		return Snapshot{}, fail("git-capability-unsupported", "Git version response is unsupported")
	}
	if selector.Ref != "" && !CommitSelector(selector.Ref) {
		if _, err := g.run(1024, false, "check-ref-format", selector.Ref); err != nil {
			if ctx.Err() != nil {
				return Snapshot{}, fail("canceled", "Git acquisition was canceled")
			}
			return Snapshot{}, fail("source-invalid", "Git ref format is invalid")
		}
	}
	// Ask only for exact names. A tag's peeled advertisement is checked again
	// against the fixed fetched object, never against a later moving name.
	args := []string{"ls-remote", "--symref", "--", selector.URL}
	if selector.Ref == "" || CommitSelector(selector.Ref) {
		args = append(args, "HEAD")
	} else {
		args = append(args, selector.Ref, selector.Ref+"^{}")
	}
	advert, err := g.run(1<<20, false, args...)
	if err != nil {
		return Snapshot{}, err
	}
	ref, advertised, peeled, format, err := parseAdvertisement(advert, selector.Ref)
	if err != nil {
		return Snapshot{}, err
	}
	selector.Ref = ref
	if _, err = g.run(4096, false, "init", "--bare", "--object-format="+format, "--template="+filepath.Join(root, "template"), g.repo); err != nil {
		if ctx.Err() != nil {
			return Snapshot{}, fail("canceled", "Git acquisition was canceled")
		}
		return Snapshot{}, fail("git-capability-unsupported", "Git cannot initialize the required object format")
	}
	if _, err = g.run(4096, true, "fetch", "--quiet", "--depth=1", "--no-tags", "--no-recurse-submodules", "--", selector.URL, advertised); err != nil {
		return Snapshot{}, err
	}
	commitBytes, err := g.run(256, true, "rev-parse", "--verify", advertised+"^{commit}")
	if err != nil {
		if ctx.Err() != nil {
			return Snapshot{}, fail("canceled", "Git acquisition was canceled")
		}
		return Snapshot{}, fail("source-invalid", "selected Git object is not a commit")
	}
	commit := strings.TrimSpace(string(commitBytes))
	if !ValidOID(commit, format) || (CommitSelector(ref) || strings.HasPrefix(ref, "refs/heads/")) && commit != advertised || peeled != "" && commit != peeled {
		return Snapshot{}, fail("source-invalid", "fetched commit differs from the captured ref")
	}
	object, kind, err := g.selectObject(commit, selector.Path, format)
	if err != nil {
		return Snapshot{}, err
	}
	identity := Git{URL: selector.URL, Ref: ref, ObjectFormat: format, Commit: commit, Object: object}
	// A branch/tag is always resolved first; the cache never chooses a tip.
	if request.Home != "" {
		if cached, e := readCache(request.Home, identity, kind, selector.Path); e == nil {
			cached.Git.Ref, cached.CacheHit, cached.GitVersion = ref, true, gitVersion
			return cached, nil
		}
	}
	files, err := g.export(object, kind, selector.Path, format)
	if err != nil {
		return Snapshot{}, err
	}
	if err = validateFiles(files); err != nil {
		return Snapshot{}, err
	}
	_, identity.ContentSHA256 = Hashes(files)
	result := Snapshot{Kind: kind, Path: selector.Path, Git: identity, Files: files, GitVersion: gitVersion}
	if request.Home != "" {
		if err = publishCache(ctx, request.Home, result); err != nil {
			return Snapshot{}, err
		}
	}
	if ctx.Err() != nil {
		return Snapshot{}, fail("canceled", "Git acquisition was canceled")
	}
	return result, nil
}

func parseAdvertisement(data []byte, requested string) (ref, object, peeled, format string, err error) {
	bad := func() (string, string, string, string, error) {
		return "", "", "", "", fail("source-invalid", "remote ref advertisement is missing, ambiguous or invalid")
	}
	entries := map[string]string{}
	head := ""
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		value, name, ok := strings.Cut(line, "\t")
		if !ok {
			return bad()
		}
		if strings.HasPrefix(value, "ref: ") {
			if name != "HEAD" || head != "" {
				return bad()
			}
			head = strings.TrimPrefix(value, "ref: ")
			continue
		}
		if !CommitSelector(value) || entries[name] != "" {
			return bad()
		}
		entries[name] = value
	}
	ref = requested
	if requested == "" {
		if !strings.HasPrefix(head, "refs/heads/") || !ValidRef(head) {
			return bad()
		}
		ref = head
		object = entries["HEAD"]
	} else if CommitSelector(requested) {
		object = requested
	} else {
		object = entries[requested]
		peeled = entries[requested+"^{}"]
	}
	if object == "" {
		return bad()
	}
	if len(object) == 40 {
		format = "sha1"
	} else {
		format = "sha256"
	}
	// HEAD advertises the remote object format for explicit commit selectors.
	if CommitSelector(requested) && entries["HEAD"] != "" && len(entries["HEAD"]) != len(object) {
		return bad()
	}
	if peeled != "" && !ValidOID(peeled, format) {
		return bad()
	}
	return ref, object, peeled, format, nil
}

type treeEntry struct {
	mode, kind, oid, name string
	size                  int64
}

func parseTree(data []byte, format string, sized bool) ([]treeEntry, error) {
	if len(data) > 0 && data[len(data)-1] != 0 {
		return nil, fail("source-invalid", "incomplete Git tree output")
	}
	result := []treeEntry{}
	for _, raw := range bytes.Split(data, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		header, name, ok := strings.Cut(string(raw), "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 && !sized || sized && len(fields) != 4 || !ValidOID(fields[2], format) || !ValidPath(name, false) {
			return nil, fail("source-invalid", "Git tree contains an invalid entry")
		}
		entry := treeEntry{mode: fields[0], kind: fields[1], oid: fields[2], name: name}
		if sized && fields[3] != "-" {
			size, err := strconv.ParseInt(fields[3], 10, 64)
			if err != nil || size < 0 {
				return nil, fail("source-invalid", "invalid Git object size")
			}
			entry.size = size
		}
		result = append(result, entry)
	}
	return result, nil
}
func (g *gitSession) selectObject(commit, selected, format string) (string, string, error) {
	if selected == "." {
		tree, err := g.run(256, true, "rev-parse", "--verify", commit+"^{tree}")
		oid := strings.TrimSpace(string(tree))
		if err != nil || !ValidOID(oid, format) {
			return "", "", fail("source-invalid", "selected tree is unavailable")
		}
		return oid, "dir", nil
	}
	data, err := g.run(1<<20, true, "ls-tree", "-z", "--full-tree", commit, "--", selected)
	if err != nil {
		return "", "", err
	}
	entries, err := parseTree(data, format, false)
	if err != nil {
		return "", "", err
	}
	if len(entries) != 1 || entries[0].name != selected {
		return "", "", fail("source-invalid", "selected package path is missing or ambiguous")
	}
	e := entries[0]
	if e.mode == "040000" && e.kind == "tree" {
		return e.oid, "dir", nil
	}
	if (e.mode == "100644" || e.mode == "100755") && e.kind == "blob" && strings.HasSuffix(selected, ".style.md") {
		return e.oid, "file", nil
	}
	return "", "", fail("source-invalid", "selected package must be a directory or regular .style.md file")
}
func (g *gitSession) export(object, kind, selected, format string) (map[string][]byte, error) {
	entries := []treeEntry{{mode: "100644", kind: "blob", oid: object, name: path.Base(selected)}}
	if kind == "dir" {
		data, err := g.run(1<<20, true, "ls-tree", "-r", "-z", "-l", object)
		if err != nil {
			return nil, err
		}
		entries, err = parseTree(data, format, true)
		if err != nil {
			return nil, err
		}
	}
	count, size := 0, int64(0)
	for _, e := range entries {
		if ignored(e.name) {
			continue
		}
		count++
		size += e.size
		if count > MaxFiles || e.size > MaxFileBytes || size > MaxBytes {
			return nil, fail("source-limit", "Git export exceeds its file or byte limits")
		}
	}
	files := map[string][]byte{}
	total := int64(0)
	for _, e := range entries {
		if ignored(e.name) {
			continue
		}
		if e.kind != "blob" || e.mode != "100644" && e.mode != "100755" {
			return nil, fail("source-invalid", "selected source contains a symlink, gitlink or special object")
		}
		if len(files) >= MaxFiles || e.size > MaxFileBytes || total+e.size > MaxBytes {
			return nil, fail("source-limit", "Git export exceeds its file or byte limits")
		}
		if _, exists := files[e.name]; exists {
			return nil, fail("source-invalid", "Git tree contains duplicate paths")
		}
		blob, err := g.run(MaxFileBytes, true, "cat-file", "blob", e.oid)
		if err != nil {
			return nil, err
		}
		files[e.name] = blob
		total += int64(len(blob))
		if total > MaxBytes {
			return nil, fail("source-limit", "Git export exceeds its total byte limit")
		}
	}
	return files, nil
}

type boundedOutput struct {
	data     bytes.Buffer
	limit    int
	exceeded bool
	cancel   context.CancelFunc
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	left := b.limit - b.data.Len()
	if len(p) > left {
		p = p[:left]
		b.exceeded = true
		b.cancel()
	}
	_, _ = b.data.Write(p)
	return n, nil
}

type boundedDiscard struct{ remaining int }

func (b *boundedDiscard) Write(p []byte) (int, error) {
	n := len(p)
	if b.remaining > 0 {
		b.remaining -= min(n, b.remaining)
	}
	return n, nil
}
func (g *gitSession) run(limit int, repository bool, args ...string) ([]byte, error) {
	if g.ctx.Err() != nil {
		return nil, fail("canceled", "Git acquisition was canceled")
	}
	commandCtx, stop := context.WithCancel(g.ctx)
	defer stop()
	out := &boundedOutput{limit: limit, cancel: stop}
	stderr := &boundedDiscard{remaining: 64 << 10}
	all := append([]string(nil), g.flags...)
	if repository {
		all = append(all, "--git-dir="+g.repo)
	}
	all = append(all, args...)
	err := g.runner.Run(commandCtx, Command{Program: g.program, Args: all, Env: append([]string(nil), g.env...), Dir: g.root, Stdout: out, Stderr: stderr})
	if out.exceeded {
		return nil, fail("source-limit", "Git output exceeds the acquisition limit")
	}
	if g.ctx.Err() != nil {
		return nil, fail("canceled", "Git acquisition was canceled")
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fail("git-unavailable", "Git executable is unavailable")
		}
		return nil, fail("source-fetch-failed", "Git acquisition failed; verify the public repository, selector and connectivity")
	}
	return out.data.Bytes(), nil
}
