package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/spec"
)

func TestInitCreatesValidDirectoryPackage(t *testing.T) {
	service := &Service{}
	parent := t.TempDir()
	created, err := service.Init(context.Background(), InitRequest{Name: "review-style", OutputDir: parent})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "review-style" || created.Single {
		t.Fatalf("Init() = %#v, want directory package review-style", created)
	}
	for _, rel := range []string{"STYLE.yaml", "prompts/default.md", "rules/review.md"} {
		if _, err := os.Stat(filepath.Join(created.Path, rel)); err != nil {
			t.Errorf("missing generated file %s: %v", rel, err)
		}
	}
	assertGeneratedPackageValid(t, created.Path)
}

func TestInitCreatesValidSingleFilePackage(t *testing.T) {
	service := &Service{}
	dir := t.TempDir()
	created, err := service.Init(context.Background(), InitRequest{Name: "one-rule", OutputDir: dir, Single: true})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Single || filepath.Base(created.Path) != "one-rule.style.md" {
		t.Fatalf("Init() = %#v, want one-rule.style.md", created)
	}
	assertGeneratedPackageValid(t, created.Path)
}

func TestInitWithoutNameUsesEmptyWorkingDirectoryAndRejectsNonempty(t *testing.T) {
	service := &Service{}
	empty := t.TempDir()
	created, err := service.Init(context.Background(), InitRequest{OutputDir: empty})
	if err != nil {
		t.Fatalf("Init() without name in empty directory: %v", err)
	}
	if created.Path != empty || created.Name == "" {
		t.Fatalf("Init() = %#v, expected current directory package", created)
	}

	nonempty := t.TempDir()
	keep := filepath.Join(nonempty, "notes.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Init(context.Background(), InitRequest{OutputDir: nonempty}); err == nil {
		t.Fatal("Init() accepted a nonempty current directory")
	} else {
		var conflict *ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("Init() error = %T %v, want ConflictError", err, err)
		}
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "keep" {
		t.Fatalf("existing file changed: %q, %v", data, err)
	}
}

func TestInitWithoutNameNormalizesDirectoryName(t *testing.T) {
	service := &Service{}
	parent := t.TempDir()
	working := filepath.Join(parent, "My Style")
	if err := os.Mkdir(working, 0o755); err != nil {
		t.Fatal(err)
	}
	created, err := service.Init(context.Background(), InitRequest{OutputDir: working})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "my-style" || created.Path != working {
		t.Fatalf("Init() = %#v, want package my-style in current directory", created)
	}
}

func TestInitRefusesExistingDestinationsAndInvalidNames(t *testing.T) {
	service := &Service{}
	parent := t.TempDir()
	destination := filepath.Join(parent, "taken")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "keep")
	if err := os.WriteFile(marker, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Init(context.Background(), InitRequest{Name: "taken", OutputDir: parent}); err == nil {
		t.Fatal("Init() overwrote an existing package directory")
	}
	if _, err := service.Init(context.Background(), InitRequest{Name: "../escape", OutputDir: parent, Single: true}); err == nil || !strings.Contains(err.Error(), "invalid package name") {
		t.Fatalf("Init() invalid name error = %v", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "untouched" {
		t.Fatalf("existing package changed: %q, %v", data, err)
	}
}

func assertGeneratedPackageValid(t *testing.T, source string) {
	t.Helper()
	pkg, err := spec.Load(source)
	if err != nil {
		t.Fatalf("load generated package: %v", err)
	}
	if diagnostics := spec.Validate(pkg); len(diagnostics) != 0 {
		t.Fatalf("generated package diagnostics: %#v", diagnostics)
	}
	if _, err := spec.Merge(pkg); err != nil {
		t.Fatalf("merge generated package: %v", err)
	}
}

func TestRunInitWritesStableSuccessMessage(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr strings.Builder
	if code := RunWithService(context.Background(), &Service{}, root, []string{"init", "review-style"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("init exit = %d, stderr=%s", code, stderr.String())
	}
	if got, want := stdout.String(), "created package review-style\n"; got != want {
		t.Fatalf("init stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunInitHelpSucceedsWithoutWritingFiles(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr strings.Builder
	if code := RunWithService(context.Background(), &Service{}, root, []string{"init", "--help"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("init help exit = %d, stderr=%s", code, stderr.String())
	}
	if got, want := stdout.String(), "style init [name] [--single]\n"; got != want {
		t.Fatalf("init help stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("init help changed directory: entries=%v err=%v", entries, err)
	}
}
