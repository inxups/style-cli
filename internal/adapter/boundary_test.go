package adapter

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCoreDoesNotImportProviderOrAdapterImplementations(t *testing.T) {
	var files []string
	err := filepath.WalkDir(filepath.Join("..", "..", "core"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("parse import path in %s: %v", path, err)
			}
			if importPath == "github.com/speakeasy-api/agenthooks" || strings.HasPrefix(importPath, "github.com/speakeasy-api/agenthooks/") || importPath == "github.com/inxups/style-cli/internal/adapter" {
				t.Errorf("core source %s imports provider implementation %q", path, importPath)
			}
		}
	}
}
