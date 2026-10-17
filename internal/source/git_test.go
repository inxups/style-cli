package source_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/inxups/style-cli/internal/source"
	"github.com/inxups/style-cli/internal/testfs"
	"github.com/inxups/style-cli/internal/testgit"
)

func entries(text string) []testgit.Entry {
	return []testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte("name: demo\nversion: 1.0.0\n")}, {Path: "pkg/rules/review.md", Data: []byte(text)}, {Path: "single.style.md", Data: []byte("single\n")}}
}
func resolve(t *testing.T, f *testgit.Fixture, ref, p, home string) source.Snapshot {
	t.Helper()
	result, err := f.Resolver().Resolve(context.Background(), source.Request{Source: testgit.URL, Ref: ref, Path: p, Home: home})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestM3A3SourceSyntax(t *testing.T) {
	for _, c := range []struct{ name, url, ref, path string }{
		{"bare-https", "https://example.invalid/repo.git", "", ""}, {"ssh", "git+ssh://host/repo", "", ""}, {"file", "git+file:///tmp/repo", "", ""}, {"http", "git+http://host/repo", "", ""}, {"ext", "git+ext://command", "", ""}, {"credentials", "git+https://user:CANARY@host/repo", "", ""}, {"query", testgit.URL + "?CANARY", "", ""}, {"fragment", testgit.URL + "#CANARY", "", ""}, {"empty-host", "git+https:///repo", "", ""},
		{"short-ref", testgit.URL, "main", ""}, {"short-oid", testgit.URL, "1234abcd", ""}, {"revision", testgit.URL, "refs/heads/main~1", ""}, {"peel", testgit.URL, "refs/tags/v1^{}", ""}, {"range", testgit.URL, "a..b", ""}, {"reflog", testgit.URL, "refs/heads/main@{1}", ""}, {"lock-ref", testgit.URL, "refs/heads/main.lock", ""}, {"dot-ref", testgit.URL, "refs/heads/.hidden", ""},
		{"absolute", testgit.URL, "", "/pkg"}, {"parent", testgit.URL, "", "pkg/../other"}, {"dot", testgit.URL, "", "./pkg"}, {"empty-segment", testgit.URL, "", "pkg//other"}, {"trailing-slash", testgit.URL, "", "pkg/"}, {"windows", testgit.URL, "", `C:\pkg`}, {"glob", testgit.URL, "", "pkg/*"}, {"shell", testgit.URL, "", "$(CANARY)"}, {"control", testgit.URL, "", "pkg\nother"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := source.Parse(source.Request{Source: c.url, Ref: c.ref, Path: c.path})
			if source.Code(err) != "source-invalid" || strings.Contains(err.Error(), "CANARY") {
				t.Fatalf("unsafe or missing failure: %v", err)
			}
		})
	}
	t.Run("canonical", func(t *testing.T) {
		p, err := source.Parse(source.Request{Source: "git+HTTPS://EXAMPLE.invalid/Team/a%2Fb.git/", Ref: "refs/heads/feature/name", Path: "packages/中文"})
		if err != nil || p.URL != "https://example.invalid/Team/a%2Fb.git/" || p.Path != "packages/中文" {
			t.Fatalf("%+v %v", p, err)
		}
	})
	for _, ref := range []string{strings.Repeat("a", 40), strings.Repeat("b", 64), "refs/tags/v1"} {
		t.Run(ref, func(t *testing.T) {
			if !source.ValidRef(ref) {
				t.Fatal(ref)
			}
		})
	}
}

func TestM3A3RealGitFixedRefAndRawObjects(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			original := entries("raw\r\n$Format:%H$\n")
			original = append(original, testgit.Entry{Path: "pkg/.gitattributes", Data: []byte("* text eol=crlf filter=CANARY\n*.md export-ignore export-subst diff=CANARY\npointer.dat filter=lfs\n")}, testgit.Entry{Path: "pkg/pointer.dat", Data: []byte("version https://git-lfs.github.com/spec/v1\noid sha256:" + strings.Repeat("a", 64) + "\nsize 987\n")}, testgit.Entry{Path: "unselected/link", Mode: "120000", Data: []byte("/outside")}, testgit.Entry{Path: "pkg/.DS_Store", Data: []byte("ignored")})
			f := testgit.New(t, format, original)
			first := f.Commit
			f.Run(nil, "tag", "light", first)
			f.Run(nil, "tag", "-a", "annotated", "-m", "tag fixture", first)
			for _, ref := range []string{"", "refs/heads/main", "refs/tags/light", "refs/tags/annotated", first} {
				t.Run("ref-"+ref, func(t *testing.T) {
					r := resolve(t, f, ref, "pkg", "")
					if r.Git.Commit != first || r.Git.ObjectFormat != format || r.Kind != "dir" || !bytes.Equal(r.Files["rules/review.md"], original[1].Data) || !bytes.Equal(r.Files["pointer.dat"], original[4].Data) || r.Files[".gitattributes"] == nil || len(r.Files) != 4 {
						t.Fatalf("wrong raw export: %+v", r)
					}
					if ref == "" && r.Git.Ref != "refs/heads/main" {
						t.Fatal(r.Git.Ref)
					}
				})
			}
			t.Run("single", func(t *testing.T) {
				r := resolve(t, f, first, "single.style.md", "")
				if r.Kind != "file" || len(r.Files) != 1 || string(r.Files["single.style.md"]) != "single\n" {
					t.Fatalf("%+v", r)
				}
			})
			t.Run("moving-advertised-tip", func(t *testing.T) {
				moved := false
				f.Before = func(c source.Command) error {
					if !moved && slicesContain(c.Args, "fetch") {
						moved = true
						f.MakeCommit(entries("later\n"))
					}
					return nil
				}
				r := resolve(t, f, "refs/heads/main", "pkg", "")
				f.Before = nil
				if !moved || r.Git.Commit != first || string(r.Files["rules/review.md"]) != string(original[1].Data) {
					t.Fatal("moving branch replaced the captured commit")
				}
			})
			t.Run("revision-only", func(t *testing.T) {
				a := resolve(t, f, "refs/heads/main", "pkg", "")
				f.MakeCommit(entries("later\n"))
				b := resolve(t, f, "refs/heads/main", "pkg", "")
				if a.Git.Commit == b.Git.Commit || a.Git.ContentSHA256 != b.Git.ContentSHA256 {
					t.Fatal("commit and content identities were conflated")
				}
			})
			t.Run("noncommit-tag", func(t *testing.T) {
				blob := f.Run([]byte("blob"), "hash-object", "-w", "--stdin")
				f.Run(nil, "update-ref", "refs/tags/blob", blob)
				_, err := f.Resolver().Resolve(context.Background(), source.Request{Source: testgit.URL, Ref: "refs/tags/blob", Path: "pkg"})
				if source.Code(err) != "source-invalid" {
					t.Fatal(err)
				}
			})
			t.Run("missing-fixed-oid", func(t *testing.T) {
				_, err := f.Resolver().Resolve(context.Background(), source.Request{Source: testgit.URL, Ref: strings.Repeat("1", len(first)), Path: "pkg"})
				if source.Code(err) != "source-fetch-failed" {
					t.Fatal(err)
				}
			})
			files, _ := os.ReadDir(filepath.Join(f.Root, "scratch"))
			if len(files) != 0 {
				t.Fatal("temporary repositories leaked")
			}
			for _, c := range f.Commands {
				for _, forbidden := range []string{"checkout", "archive", "submodule", "--filters", "--textconv"} {
					if slicesContain(c, forbidden) {
						t.Fatal(c)
					}
				}
			}
		})
	}
}

func TestM3A3SelectedTreeLimitsAndModes(t *testing.T) {
	for _, kind := range []string{"symlink", "gitlink", "case-file", "case-dir", "control", "backslash", "nul", "utf8", "oversize", "file-count", "total", "wrong-file", "missing"} {
		t.Run(kind, func(t *testing.T) {
			data := entries("text\n")
			expected := "source-invalid"
			switch kind {
			case "symlink":
				data = append(data, testgit.Entry{Path: "pkg/link", Mode: "120000", Data: []byte("outside")})
			case "gitlink":
				data = append(data, testgit.Entry{Path: "pkg/module", Mode: "160000", OID: strings.Repeat("a", 40)})
			case "case-file":
				data = append(data, testgit.Entry{Path: "pkg/STYLE.YAML", Data: []byte("duplicate")})
			case "case-dir":
				data = append(data, testgit.Entry{Path: "pkg/Rules/other.md", Data: []byte("duplicate")})
			case "control":
				data = append(data, testgit.Entry{Path: "pkg/new\nline", Data: []byte("text")})
			case "backslash":
				data = append(data, testgit.Entry{Path: `pkg/back\slash`, Data: []byte("text")})
			case "nul":
				data = append(data, testgit.Entry{Path: "pkg/nul", Data: []byte{0}})
			case "utf8":
				data = append(data, testgit.Entry{Path: "pkg/binary", Data: []byte{255}})
			case "oversize":
				expected = "source-limit"
				data = append(data, testgit.Entry{Path: "pkg/big", Data: bytes.Repeat([]byte("a"), source.MaxFileBytes+1)})
			case "file-count":
				expected = "source-limit"
				for i := 0; i < source.MaxFiles; i++ {
					data = append(data, testgit.Entry{Path: fmt.Sprintf("pkg/files/%04d", i), Data: []byte("x")})
				}
			case "total":
				expected = "source-limit"
				for i := 0; i < 65; i++ {
					data = append(data, testgit.Entry{Path: fmt.Sprintf("pkg/big/%04d", i), Data: bytes.Repeat([]byte("x"), source.MaxFileBytes)})
				}
			}
			f := testgit.New(t, "sha1", data)
			p := "pkg"
			if kind == "wrong-file" {
				p = "pkg/STYLE.yaml"
			}
			if kind == "missing" {
				p = "absent"
			}
			_, err := f.Resolver().Resolve(context.Background(), source.Request{Source: testgit.URL, Path: p})
			if source.Code(err) != expected {
				t.Fatalf("want %s: %v", expected, err)
			}
		})
	}
}

func TestM3A3EnvironmentIsolationAndFailures(t *testing.T) {
	f := testgit.New(t, "sha1", entries("ordinary\n"))
	for _, name := range []string{"GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_EXEC_PATH", "GIT_PROXY_COMMAND", "GIT_ASKPASS", "SSH_ASKPASS", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_OBJECT_DIRECTORY", "GIT_DIR", "GIT_WORK_TREE", "GIT_SSL_NO_VERIFY", "GIT_TEMPLATE_DIR", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "HTTP_PROXY", "SSL_CERT_FILE", "CURL_CA_BUNDLE"} {
		t.Setenv(name, "CANARY")
	}
	f.Before = func(c source.Command) error {
		for _, variable := range c.Env {
			if strings.Contains(variable, "CANARY") {
				return errors.New("inherited hostile environment")
			}
		}
		if !slicesContain(c.Args, "protocol.allow=never") || !slicesContain(c.Args, "protocol.https.allow=always") || !slicesContain(c.Args, "http.followRedirects=false") || !slicesContain(c.Args, "http.sslVerify=true") {
			return errors.New("missing transport isolation")
		}
		return nil
	}
	r := resolve(t, f, "", "pkg", "")
	if r.Git.Commit != f.Commit {
		t.Fatal(r)
	}
	f.Before = nil
	for _, scenario := range []string{"stderr-flood", "stdout-flood", "fetch-failure", "canceled", "missing-git", "workspace-failure"} {
		t.Run(scenario, func(t *testing.T) {
			resolver := f.Resolver()
			ctx := context.Background()
			expected := "source-fetch-failed"
			switch scenario {
			case "missing-git":
				resolver.GitPath = filepath.Join(f.Root, "missing")
				expected = "git-unavailable"
			case "workspace-failure":
				resolver.TempDir = filepath.Join(f.Root, "missing-parent", "no-dir")
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				expected = "canceled"
			default:
				resolver.Runner = source.RunnerFunc(func(ctx context.Context, c source.Command) error {
					if slicesContain(c.Args, "fetch") {
						if scenario == "stderr-flood" {
							for i := 0; i < 1024; i++ {
								_, _ = c.Stderr.Write(bytes.Repeat([]byte("CANARY"), 1024))
							}
						} else if scenario == "stdout-flood" {
							_, _ = c.Stdout.Write(bytes.Repeat([]byte("CANARY"), 1<<20))
							return nil
						}
						return errors.New("CANARY remote error")
					}
					return f.RunCommand(ctx, c)
				})
				if scenario == "stdout-flood" {
					expected = "source-limit"
				}
			}
			_, err := resolver.Resolve(ctx, source.Request{Source: testgit.URL, Path: "pkg"})
			if source.Code(err) != expected || strings.Contains(err.Error(), "CANARY") {
				t.Fatalf("want %s, got %v", expected, err)
			}
			files, _ := os.ReadDir(filepath.Join(f.Root, "scratch"))
			if len(files) != 0 {
				t.Fatal("failed acquisition leaked workspace")
			}
		})
	}
}

func TestM3A3CacheRepairAndPreviewIsolation(t *testing.T) {
	f := testgit.New(t, "sha1", entries("cache\n"))
	home := t.TempDir()
	a := resolve(t, f, "", "pkg", home)
	b := resolve(t, f, "", "pkg", home)
	if a.CacheHit || !b.CacheHit || !reflect.DeepEqual(a.Files, b.Files) {
		t.Fatal("cache hit changed the export")
	}
	for _, name := range []string{"style.lock.yaml", "packages", "approved", "settings", "build"} {
		if _, err := os.Stat(filepath.Join(home, name)); !os.IsNotExist(err) {
			t.Fatalf("preview created %s", name)
		}
	}
	cache := filepath.Join(home, "cache", "sources", "git", "v1")
	dirs, _ := os.ReadDir(cache)
	if len(dirs) != 1 {
		t.Fatal(dirs)
	}
	entry := filepath.Join(cache, dirs[0].Name())
	for _, corruption := range []string{"bytes", "extra", "manifest", "symlink", "permissions"} {
		t.Run(corruption, func(t *testing.T) {
			switch corruption {
			case "bytes":
				if err := os.WriteFile(filepath.Join(entry, "files", "STYLE.yaml"), []byte("corrupt"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "extra":
				if err := os.WriteFile(filepath.Join(entry, "extra"), []byte("extra"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "manifest":
				if err := os.WriteFile(filepath.Join(entry, "manifest.json"), []byte(`{"format":99}`), 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				name := filepath.Join(entry, "files", "STYLE.yaml")
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(f.Root, "CANARY"), name); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				testfs.MakePublic(t, filepath.Join(entry, "files", "STYLE.yaml"), 0o644)
			}
			r := resolve(t, f, "", "pkg", home)
			if r.CacheHit || !reflect.DeepEqual(a.Files, r.Files) {
				t.Fatal("corrupt cache was reused")
			}
			if !resolve(t, f, "", "pkg", home).CacheHit {
				t.Fatal("repair did not publish a complete entry")
			}
		})
	}
	t.Run("parent-symlink", func(t *testing.T) {
		other := t.TempDir()
		aliasHome := t.TempDir()
		if err := os.Symlink(other, filepath.Join(aliasHome, "cache")); err != nil {
			t.Fatal(err)
		}
		_, err := f.Resolver().Resolve(context.Background(), source.Request{Source: testgit.URL, Path: "pkg", Home: aliasHome})
		if source.Code(err) != "source-cache-invalid" {
			t.Fatal(err)
		}
		files, _ := os.ReadDir(other)
		if len(files) != 0 {
			t.Fatal("followed cache symlink")
		}
	})
	for _, d := range dirs {
		if strings.HasPrefix(d.Name(), ".") {
			t.Fatal("staging entry leaked")
		}
	}
}

func slicesContain(values []string, want string) bool {
	for _, s := range values {
		if s == want {
			return true
		}
	}
	return false
}

func TestM3A3AcquisitionHonorsShortDeadline(t *testing.T) {
	f := testgit.New(t, "sha1", entries("cancel\n"))
	for _, stage := range []string{"--version", "fetch"} {
		t.Run(stage, func(t *testing.T) {
			resolver := f.Resolver()
			resolver.Runner = source.RunnerFunc(func(ctx context.Context, c source.Command) error {
				if slicesContain(c.Args, stage) {
					<-ctx.Done()
					return ctx.Err()
				}
				return f.RunCommand(ctx, c)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			start := time.Now()
			_, err := resolver.Resolve(ctx, source.Request{Source: testgit.URL, Path: "pkg", Home: t.TempDir()})
			if source.Code(err) != "canceled" || time.Since(start) > 2*time.Second {
				t.Fatalf("%v %v", time.Since(start), err)
			}
			files, _ := os.ReadDir(filepath.Join(f.Root, "scratch"))
			if len(files) != 0 {
				t.Fatal("cancellation leaked workspace")
			}
		})
	}
}
