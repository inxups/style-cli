package state

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/inxups/style-cli/internal/saferead"
)

type Snapshot struct {
	ScopeExists    bool   `json:"seed_scope_exists"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Files          int    `json:"files"`
}

var seenSnapshotName = regexp.MustCompile(`^(s\.[0-9]+\.[0-9]+\.[a-f0-9]{16}\.[a-f0-9]{16}|t\.[0-9]+\.[a-f0-9]{16}\.[a-f0-9]{16}\.[a-f0-9]{16})$`)

// CopySnapshot captures only one session and agent using Store's ID encoding.
// The destination must be a new private root. Source bytes and metadata stay read-only.
func CopySnapshot(ctx context.Context, source, destination, sessionID, agentID string) (Snapshot, error) {
	if agentID == "" {
		agentID = "main"
	}
	root, err := saferead.Anchor(source)
	if err != nil {
		return Snapshot{}, err
	}
	defer root.Close()
	session := component(sessionID)
	agent := filepath.Join(session, component(agentID))
	seen := filepath.Join(agent, "seen")
	refreshDir := filepath.Join(agent, "refresh")
	refreshPresent := func() (bool, error) {
		if err := saferead.CheckDirectories(root, filepath.Join(refreshDir, "placeholder")); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	hasRefresh, err := refreshPresent()
	if err != nil {
		return Snapshot{}, err
	}
	var guard *RefreshGuard
	if hasRefresh {
		anchor, err := saferead.AnchorMutable(source)
		if err != nil {
			return Snapshot{}, err
		}
		guard, err = openRefreshGuard(ctx, nil, anchor, session, true)
		if err != nil {
			return Snapshot{}, err
		}
		defer guard.Close()
	}
	type item struct {
		Name   string      `json:"name"`
		SHA256 string      `json:"sha256"`
		Data   []byte      `json:"-"`
		Info   os.FileInfo `json:"-"`
	}
	capture := func() ([]item, bool, error) {
		present, err := refreshPresent()
		if err != nil {
			return nil, false, err
		}
		if present != hasRefresh {
			return nil, false, saferead.ErrChanged
		}
		if guard != nil {
			if err := guard.Check(); err != nil {
				return nil, false, err
			}
		}
		result := make([]item, 0)
		scopeExists := false
		total := 0
		names := []string{filepath.Join(session, "session.json"), filepath.Join(agent, "scope.json")}
		refreshNames := map[string]bool{}
		if hasRefresh {
			dir, err := root.Open(refreshDir)
			if err != nil {
				return nil, false, err
			}
			children, readErr := dir.ReadDir(10001)
			_ = dir.Close()
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return nil, false, readErr
			}
			if len(children) > 10000 {
				return nil, false, fmt.Errorf("snapshot refresh file limit")
			}
			for _, entry := range children {
				name := strings.TrimSuffix(entry.Name(), ".json")
				if entry.Name() != name+".json" || !(safeComponentPattern.MatchString(name) || len(name) == 65 && name[0] == '!' && refreshHash.MatchString(name[1:])) {
					return nil, false, fmt.Errorf("invalid snapshot refresh name")
				}
				full := filepath.Join(refreshDir, entry.Name())
				refreshNames[full] = true
			}
		}
		if err := saferead.CheckDirectories(root, filepath.Join(seen, "placeholder")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, false, err
		}
		dir, err := root.Open(seen)
		if err == nil {
			children, readErr := dir.ReadDir(10001)
			_ = dir.Close()
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return nil, false, readErr
			}
			if len(children) > 10000 {
				return nil, false, fmt.Errorf("snapshot seen limit")
			}
			for _, entry := range children {
				if !seenSnapshotName.MatchString(entry.Name()) {
					return nil, false, fmt.Errorf("invalid snapshot reservation")
				}
				names = append(names, filepath.Join(seen, entry.Name()))
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, false, err
		}
		for name := range refreshNames {
			names = append(names, name)
		}
		if len(names) > 10000 {
			return nil, false, fmt.Errorf("snapshot file limit")
		}
		for i, name := range names {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			limit := int64(64 << 10)
			if i >= 2 {
				limit = 0
			}
			if refreshNames[name] {
				limit = maxRefreshBytes
			}
			info, statErr := root.Lstat(name)
			data, err := saferead.File(root, name, limit)
			if errors.Is(err, os.ErrNotExist) && i < 2 {
				continue
			}
			if err != nil {
				return nil, false, err
			}
			if statErr != nil {
				return nil, false, saferead.ErrChanged
			}
			if i < 2 {
				trimmed := bytes.TrimSpace(data)
				if !utf8.Valid(data) || len(trimmed) == 0 || trimmed[0] != '{' {
					return nil, false, fmt.Errorf("invalid snapshot state object")
				}
				var target any = &Session{}
				if i == 1 {
					target = &Scope{}
					scopeExists = true
				}
				decoder := json.NewDecoder(bytes.NewReader(data))
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(target); err != nil {
					return nil, false, fmt.Errorf("invalid snapshot state")
				}
				if session, ok := target.(*Session); ok {
					if err := validateProviderState(*session); err != nil {
						return nil, false, err
					}
				}
				if decoder.Decode(new(any)) != io.EOF {
					return nil, false, fmt.Errorf("trailing snapshot state")
				}
			}
			if refreshNames[name] {
				if _, err := decodeRefreshState(data); err != nil {
					return nil, false, err
				}
			}
			total += len(data)
			if total > 16<<20 {
				return nil, false, fmt.Errorf("snapshot byte limit")
			}
			result = append(result, item{Name: name, SHA256: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), Data: data, Info: info})
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
		return result, scopeExists, nil
	}
	first, exists, err := capture()
	if err != nil {
		return Snapshot{}, err
	}
	second, secondExists, err := capture()
	if err != nil {
		return Snapshot{}, err
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if exists != secondExists || !bytes.Equal(a, b) {
		return Snapshot{}, saferead.ErrChanged
	}
	for i, it := range second {
		info, e := root.Lstat(it.Name)
		if e != nil || !saferead.Same(first[i].Info, it.Info) || !saferead.Same(first[i].Info, info) {
			return Snapshot{}, saferead.ErrChanged
		}
	}
	for _, it := range first {
		target := filepath.Join(destination, it.Name)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return Snapshot{}, err
		}
		if err := os.WriteFile(target, it.Data, 0600); err != nil {
			return Snapshot{}, err
		}
	}
	if present, err := refreshPresent(); err != nil || present != hasRefresh {
		return Snapshot{}, saferead.ErrChanged
	}
	if guard != nil {
		if err := guard.Check(); err != nil {
			return Snapshot{}, err
		}
	}
	return Snapshot{exists, fmt.Sprintf("sha256:%x", sha256.Sum256(a)), len(first)}, nil
}
