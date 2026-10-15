package install

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// JSON has no comment region in which to retain formatting. A private,
// owner-scoped restoration record preserves the original bytes. Restoration
// is conditional on the entire installed file still matching its digest;
// intervening user/other-owner edits always take precedence.
type jsonRestore struct {
	Format        int    `json:"format"`
	Path          string `json:"path"`
	Original      []byte `json:"original"`
	AppliedSHA256 string `json:"applied_sha256"`
}

func restorePath(path, owner string) string {
	return filepath.Join(".agenthooks", owner, filepath.FromSlash(path)+".restore.json")
}

func jsonDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readJSONRestore(root, path, owner string, read func(string) ([]byte, error)) (jsonRestore, []byte, error) {
	raw, err := read(filepath.Join(root, restorePath(path, owner)))
	if errors.Is(err, os.ErrNotExist) {
		return jsonRestore{}, nil, nil
	}
	if err != nil {
		return jsonRestore{}, nil, err
	}
	var record jsonRestore
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return record, nil, fmt.Errorf("install: invalid JSON restoration record for %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return record, nil, fmt.Errorf("install: trailing JSON restoration record for %s", path)
	}
	digest, err := hex.DecodeString(record.AppliedSHA256)
	if record.Format != 1 || record.Path != path || err != nil || len(digest) != sha256.Size || record.Original != nil && !json.Valid(record.Original) {
		return record, nil, fmt.Errorf("install: invalid JSON restoration record for %s", path)
	}
	return record, raw, nil
}

func planJSONRestore(root, path, owner string, existing, final []byte, read func(string) ([]byte, error)) (*plannedFile, error) {
	record, before, err := readJSONRestore(root, path, owner, read)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		if before == nil {
			return nil, nil
		}
		record = jsonRestore{Format: 1, Path: path}
	} else if before == nil || record.AppliedSHA256 != jsonDigest(existing) {
		// A changed config becomes the new baseline after removing only this
		// owner's handlers. Never restore a snapshot over a later user edit.
		original, remove, _, err := removeOwnedJSON(existing, owner)
		if err != nil {
			return nil, err
		}
		if remove {
			original = nil
		}
		if before == nil && original == nil {
			return nil, nil
		}
		record = jsonRestore{Format: 1, Path: path, Original: original}
	}
	record.AppliedSHA256 = jsonDigest(final)
	after, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	state := StateUpdate
	if before == nil {
		state = StateCreate
	} else if bytes.Equal(before, after) {
		state = StateUnchanged
		if info, err := os.Stat(filepath.Join(root, restorePath(path, owner))); err != nil || info.Mode().Perm() != 0o600 {
			state = StateUpdate
		}
	}
	return &plannedFile{path: restorePath(path, owner), root: root, before: before, final: after, mode: 0o600, state: state}, nil
}

func removalJSONRestore(root, path, owner string) (record jsonRestore, removal *PlannedFile, err error) {
	record, raw, err := readJSONRestore(root, path, owner, os.ReadFile)
	if err != nil || raw == nil {
		return record, nil, err
	}
	return record, &PlannedFile{Path: filepath.Join(root, restorePath(path, owner)), Root: root, Before: raw, Mode: 0o600, Remove: true}, nil
}
