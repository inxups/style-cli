package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/inxups/style-cli/internal/source"

	"github.com/speakeasy-api/agenthooks/install"
)

// An approval retains the bytes and compiled representation that the user
// accepted. Lock hashes authenticate it; it is never a runtime input.
type approvedSnapshot struct {
	Git      *source.Git        `json:"git,omitempty"`
	Format   int                `json:"format"`
	Files    map[string][]byte  `json:"files"`
	Rendered map[string][]byte  `json:"rendered"`
	Native   []NativeHookReport `json:"native"`
}

func approvalPath(home, name string) string { return filepath.Join(home, "approved", name+".json") }

func approvalBytes(compilation installCompilation, name string, files map[string][]byte, git ...*source.Git) ([]byte, error) {
	view := make(map[string][]byte)
	for provider, events := range compilation.index.Harnesses {
		for event, entries := range events {
			for _, entry := range entries {
				if !strings.HasPrefix(entry.Rule, name+"/") {
					continue
				}
				// Rendered here means static template substitution and runtime
				// fragments. No future event values are invented for the preview.
				data, err := json.MarshalIndent(entry, "", "  ")
				if err != nil {
					return nil, err
				}
				view[provider+"/"+event+"/"+entry.Entry] = data
			}
		}
	}
	for _, body := range compilation.staticBodies {
		if !strings.HasPrefix(body.Rule, name+"/") {
			continue
		}
		view[body.Harness+"/static/"+body.Rule+"/"+body.Slot] = []byte("mode: " + body.Mode + "\n" + body.Body)
	}
	var provenance *source.Git
	if len(git) > 0 {
		provenance = git[0]
	}
	return json.Marshal(approvedSnapshot{Git: provenance, Format: 1, Files: files, Rendered: view, Native: compilation.nativeReports(name)})
}

func loadApproval(home string, item lockPackage) (approvedSnapshot, bool, error) {
	data, err := readOptionalBytes(approvalPath(home, item.Name))
	if err != nil {
		return approvedSnapshot{}, false, err
	}
	if data == nil {
		if item.Approval != "" {
			return approvedSnapshot{}, false, fmt.Errorf("approval baseline for %s is missing", item.Name)
		}
		return approvedSnapshot{}, false, nil
	}
	if item.Approval != "" && hashBytes(data) != item.Approval {
		return approvedSnapshot{}, false, fmt.Errorf("approval baseline for %s does not match the lock", item.Name)
	}
	var result approvedSnapshot
	if err := json.Unmarshal(data, &result); err != nil {
		return result, false, fmt.Errorf("decode approval baseline %s: %w", item.Name, err)
	}
	if item.Source.Git != nil && (result.Git == nil || *result.Git != *item.Source.Git) || item.Source.Git == nil && result.Git != nil {
		return result, false, fmt.Errorf("approval provenance for %s differs from the lock", item.Name)
	}
	hashes, _ := hashPackageFiles(result.Files)
	if result.Format != 1 || !reflectStringMap(hashes, item.Files) {
		return result, false, fmt.Errorf("approval baseline for %s has invalid source hashes", item.Name)
	}
	return result, true, nil
}

func planPrivateFile(path string, after []byte) ([]install.PlannedFile, error) {
	before, err := readOptionalBytes(path)
	if err != nil {
		return nil, err
	}
	return planPrivateBytes(path, before, after), nil
}

func planPrivateBytes(path string, before, after []byte) []install.PlannedFile {
	if before != nil && bytes.Equal(before, after) {
		return nil
	}
	return []install.PlannedFile{{Path: path, Root: filepath.Dir(path), Before: before, After: after, Mode: os.FileMode(0o600)}}
}
