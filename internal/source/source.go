// Package source acquires immutable, bounded Git exports. Package parsing and
// approval remain the responsibility of the caller.
package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxFiles     = 1024
	MaxBytes     = 16 << 20
	MaxFileBytes = 256 << 10
)

// Error deliberately excludes remote output, URLs and local environment data.
type Error struct{ Code, Message string }

func (e *Error) Error() string        { return e.Code + ": " + e.Message }
func fail(code, message string) error { return &Error{code, message} }
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

type Request struct{ Source, Ref, Path, Home string }
type Selector struct{ URL, Ref, Path string }

// Git is the provenance stored in lock format 2. OIDs and STYLE content hashes
// are different identities and must never substitute for each other.
type Git struct {
	URL           string `json:"url" yaml:"url"`
	Ref           string `json:"ref" yaml:"ref"`
	ObjectFormat  string `json:"object_format" yaml:"object_format"`
	Commit        string `json:"commit" yaml:"commit"`
	Object        string `json:"object" yaml:"object"`
	ContentSHA256 string `json:"content_sha256" yaml:"content_sha256"`
}
type Snapshot struct {
	Kind       string
	Path       string
	Git        Git
	Files      map[string][]byte
	CacheHit   bool
	GitVersion string
}
type Acquirer interface {
	Resolve(context.Context, Request) (Snapshot, error)
}

// Remote recognizes explicit remote syntax before CLI relative-path handling.
// Unsupported transports still reach Parse and cannot turn into local paths.
func Remote(value string) bool {
	return strings.Contains(value, "://") || strings.HasPrefix(strings.ToLower(value), "git+")
}

func Parse(r Request) (Selector, error) {
	if len(r.Source) < 4 || !strings.EqualFold(r.Source[:4], "git+") {
		return Selector{}, fail("source-invalid", "use an explicit git+https source")
	}
	u, err := url.Parse(r.Source[4:])
	if err != nil || u == nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(r.Source, "#") || u.Path == "" || !strings.HasPrefix(u.Path, "/") || hasControl(r.Source) || hasControl(u.Path) {
		return Selector{}, fail("source-invalid", "Git source requires an HTTPS repository URL without credentials, query or fragment")
	}
	u.Scheme, u.Host = "https", strings.ToLower(u.Host)
	if r.Ref != "" && !ValidRef(r.Ref) {
		return Selector{}, fail("source-invalid", "ref must be a canonical branch/tag ref or a full lowercase commit OID")
	}
	p := r.Path
	if p == "" {
		p = "."
	}
	if !ValidPath(p, true) {
		return Selector{}, fail("source-invalid", "package path must be a canonical repository-relative path")
	}
	return Selector{u.String(), r.Ref, p}, nil
}

var oidPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var hashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func ValidOID(s, format string) bool {
	return oidPattern.MatchString(s) && (format == "sha1" && len(s) == 40 || format == "sha256" && len(s) == 64)
}
func CommitSelector(s string) bool { return oidPattern.MatchString(s) }
func ValidRef(s string) bool {
	if CommitSelector(s) {
		return true
	}
	if !(strings.HasPrefix(s, "refs/heads/") || strings.HasPrefix(s, "refs/tags/")) || !utf8.ValidString(s) || hasControl(s) || strings.ContainsAny(s, ` ~^:?*[\`) || strings.Contains(s, "..") || strings.Contains(s, "@{") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}

func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
func ValidPath(s string, root bool) bool {
	if root && s == "." {
		return true
	}
	if s == "" || !utf8.ValidString(s) || hasControl(s) || strings.ContainsAny(s, `\:*?[]<>|"$`+"`") || strings.HasPrefix(s, "/") {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
	}
	return path.Clean(s) == s
}
func ignored(s string) bool {
	for _, part := range strings.Split(s, "/") {
		switch part {
		case ".git", ".hg", ".svn", ".DS_Store":
			return true
		}
	}
	return false
}
func fold(s string) string {
	return strings.Map(func(r rune) rune {
		smallest := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < smallest {
				smallest = next
			}
		}
		return smallest
	}, s)
}

// Hashes uses the established STYLE package.lock aggregation, not Git hashing.
func Hashes(files map[string][]byte) (map[string]string, string) {
	hashes := make(map[string]string, len(files))
	for p, data := range files {
		sum := sha256.Sum256(data)
		hashes[p] = "sha256:" + hex.EncodeToString(sum[:])
	}
	return hashes, Aggregate(hashes)
}
func Aggregate(hashes map[string]string) string {
	paths := make([]string, 0, len(hashes))
	for p := range hashes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		fmt.Fprintf(h, "%s\x00%s\n", p, hashes[p])
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func validateFiles(files map[string][]byte) error {
	if len(files) == 0 {
		return fail("source-invalid", "selected source contains no files")
	}
	if len(files) > MaxFiles {
		return fail("source-limit", "Git export exceeds the file count limit")
	}
	total := 0
	prefixes := map[string]string{}
	for p, data := range files {
		if !ValidPath(p, false) || ignored(p) {
			return fail("source-invalid", "Git export contains a forbidden path")
		}
		parts := strings.Split(p, "/")
		for i := range parts {
			prefix := strings.Join(parts[:i+1], "/")
			key := fold(prefix)
			if old, ok := prefixes[key]; ok && old != prefix {
				return fail("source-invalid", "Git export contains case-folding path collisions")
			}
			prefixes[key] = prefix
		}
		if len(data) > MaxFileBytes {
			return fail("source-limit", "Git export exceeds the per-file byte limit")
		}
		total += len(data)
		if total > MaxBytes {
			return fail("source-limit", "Git export exceeds the total byte limit")
		}
		if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
			return fail("source-invalid", "Git export requires UTF-8 text without NUL bytes")
		}
	}
	return nil
}

// ValidateIdentity is shared by the cache and lock reader. It checks the
// complete provenance/hash combination before any writer can preserve it.
func ValidateIdentity(g Git, kind, p string, hashes map[string]string) error {
	selector, err := Parse(Request{Source: "git+" + g.URL, Ref: g.Ref, Path: p})
	if err != nil || selector.URL != g.URL || selector.Path != p || g.Ref == "" || !ValidOID(g.Commit, g.ObjectFormat) || !ValidOID(g.Object, g.ObjectFormat) || !hashPattern.MatchString(g.ContentSHA256) || Aggregate(hashes) != g.ContentSHA256 || len(hashes) == 0 || len(hashes) > MaxFiles || (kind != "dir" && kind != "file") {
		return fail("source-invalid", "Git provenance or content identity is inconsistent")
	}
	if CommitSelector(g.Ref) && g.Ref != g.Commit {
		return fail("source-invalid", "commit selector differs from the locked commit")
	}
	if kind == "file" && (!strings.HasSuffix(p, ".style.md") || len(hashes) != 1 || hashes[path.Base(p)] == "") {
		return fail("source-invalid", "Git single-file identity is inconsistent")
	}
	prefixes := map[string]string{}
	for file, hash := range hashes {
		if !ValidPath(file, false) || ignored(file) || !hashPattern.MatchString(hash) {
			return fail("source-invalid", "Git file identity is invalid")
		}
		parts := strings.Split(file, "/")
		for i := range parts {
			p := strings.Join(parts[:i+1], "/")
			if i < len(parts)-1 && hashes[p] != "" {
				return fail("source-invalid", "Git file path is also a parent directory")
			}
			key := fold(p)
			if old, ok := prefixes[key]; ok && old != p {
				return fail("source-invalid", "Git file paths collide")
			}
			prefixes[key] = p
		}
	}
	return nil
}

func cacheKey(g Git, kind, p string) string {
	data, _ := json.Marshal([]string{g.URL, g.ObjectFormat, g.Commit, p, kind})
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
