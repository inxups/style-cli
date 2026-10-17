package source_test

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/inxups/style-cli/internal/source"
	"github.com/inxups/style-cli/internal/testgit"
)

func TestM3A3LoopbackTLSAndRedirectPolicy(t *testing.T) {
	f := testgit.New(t, "sha1", entries("HTTPS raw export\n"))
	var redirected atomic.Int32
	backend := &cgi.Handler{Path: f.Git, Args: []string{"http-backend"}, Dir: f.Root, Env: []string{"GIT_PROJECT_ROOT=" + f.Root, "GIT_HTTP_EXPORT_ALL=1", "HOME=" + filepath.Join(f.Root, "home"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/redirect"):
			http.Redirect(w, r, "/repository.git/info/refs?service=git-upload-pack", http.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/auth"):
			w.Header().Set("WWW-Authenticate", `Basic realm="fixture"`)
			w.WriteHeader(http.StatusUnauthorized)
		default:
			redirected.Add(1)
			backend.ServeHTTP(w, r)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal("loopback TLS fixture requires a permitted local listener: ", err)
	}
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: handler}}
	server.StartTLS()
	defer server.Close()
	cert, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(f.Root, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &source.Resolver{GitPath: f.Git, TempDir: filepath.Join(f.Root, "scratch"), Runner: source.RunnerFunc(func(ctx context.Context, c source.Command) error {
		c.Env = append(c.Env, "GIT_SSL_CAINFO="+ca)
		return (source.ExecRunner{}).Run(ctx, c)
	})}
	for _, scenario := range []string{"https", "untrusted-certificate", "redirect", "auth"} {
		t.Run(scenario, func(t *testing.T) {
			url := "git+" + server.URL + "/repository.git"
			r := *resolver
			if scenario == "untrusted-certificate" {
				r.Runner = source.ExecRunner{}
			}
			if scenario == "redirect" {
				url = "git+" + server.URL + "/redirect"
			}
			if scenario == "auth" {
				url = "git+" + server.URL + "/auth"
			}
			before := redirected.Load()
			snapshot, err := r.Resolve(context.Background(), source.Request{Source: url, Path: "pkg", Home: t.TempDir()})
			if scenario == "https" {
				if err != nil || snapshot.Git.Commit != f.Commit {
					t.Fatalf("%+v %v", snapshot, err)
				}
			} else {
				if source.Code(err) != "source-fetch-failed" {
					t.Fatal(err)
				}
				if scenario == "redirect" && redirected.Load() != before {
					t.Fatal("client followed redirect")
				}
			}
		})
	}
}

func TestM3A3CacheConcurrentPublishAndDiskFailure(t *testing.T) {
	f := testgit.New(t, "sha1", entries("concurrent\n"))
	home := t.TempDir()
	var wg sync.WaitGroup
	results := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.Resolver().Resolve(context.Background(), source.Request{Source: testgit.URL, Path: "pkg", Home: home})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !resolve(t, f, "", "pkg", home).CacheHit {
		t.Fatal("concurrent publish not reusable")
	}
	dirs, err := os.ReadDir(filepath.Join(home, "cache", "sources", "git", "v1"))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("%v %v", dirs, err)
	}
	t.Run("disk-full", func(t *testing.T) {
		resolver := f.Resolver()
		resolver.Runner = source.RunnerFunc(func(ctx context.Context, c source.Command) error {
			if slicesContain(c.Args, "fetch") {
				return &os.PathError{Op: "write", Path: "private-pack", Err: syscall.ENOSPC}
			}
			return f.RunCommand(ctx, c)
		})
		_, err := resolver.Resolve(context.Background(), source.Request{Source: testgit.URL, Path: "pkg", Home: t.TempDir()})
		if source.Code(err) != "source-fetch-failed" {
			t.Fatal(err)
		}
		left, _ := os.ReadDir(filepath.Join(f.Root, "scratch"))
		if len(left) != 0 {
			t.Fatal("disk failure leaked private repository")
		}
	})
	t.Run("cache-not-directory", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, "cache"), []byte("occupied"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := f.Resolver().Resolve(context.Background(), source.Request{Source: testgit.URL, Path: "pkg", Home: home})
		if source.Code(err) != "source-cache-invalid" {
			t.Fatal(err)
		}
	})
}

func TestM3A3HostileGitConfigurationNeverExecutes(t *testing.T) {
	f := testgit.New(t, "sha1", append(entries("$Format:%H$\n"), testgit.Entry{Path: "pkg/.gitattributes", Data: []byte("* filter=hostile\n*.md export-ignore export-subst\n")}))
	marker := filepath.Join(f.Root, "CANARY")
	host := filepath.Join(f.Root, "host")
	if err := os.Mkdir(host, 0o700); err != nil {
		t.Fatal(err)
	}
	configuration := "[credential]\n helper = !touch " + marker + "\n[core]\n askPass = touch " + marker + "\n hooksPath = " + host + "\n[filter \"hostile\"]\n required = true\n clean = touch " + marker + "\n smudge = touch " + marker + "\n[url \"ext::touch " + marker + "\"]\n insteadOf = https://fixture.invalid/\n[http]\n proxy = http://127.0.0.1:1\n sslVerify = false\n"
	config := filepath.Join(host, ".gitconfig")
	if err := os.WriteFile(config, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", host)
	t.Setenv("XDG_CONFIG_HOME", host)
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_SYSTEM", config)
	f.Run(nil, "config", "core.hooksPath", host)
	for _, name := range []string{"post-checkout", "post-merge", "post-rewrite", "pre-auto-gc"} {
		if err := os.WriteFile(filepath.Join(host, name), []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := resolve(t, f, "", "pkg", t.TempDir())
	if string(snapshot.Files["rules/review.md"]) != "$Format:%H$\n" {
		t.Fatal("attributes transformed export")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("untrusted Git configuration executed")
	}
}

func TestM3A3ExecCancellationStopsProcessGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("process cancellation fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := (source.ExecRunner{}).Run(ctx, source.Command{Program: "/bin/sh", Args: []string{"-c", "sleep 30 & wait"}, Env: []string{"PATH=/usr/bin:/bin"}})
	if err == nil || ctx.Err() == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("%v %v", time.Since(start), err)
	}
}
