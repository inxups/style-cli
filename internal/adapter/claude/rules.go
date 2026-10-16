package claude

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/speakeasy-api/agenthooks/install"
)

func (r *route) BindStaticState(state adapter.StaticState) adapter.Route {
	bound := *r
	bound.static = adapter.StaticState{Layout: state.Layout, Files: map[string]string{}, LegacyOwners: append([]adapter.Owner(nil), state.LegacyOwners...)}
	for path, hash := range state.Files {
		bound.static.Files[path] = hash
	}
	return &bound
}

func rulesName(owner adapter.Owner) (string, error) {
	name := string(owner)
	if name == "" || len(name) > 64 {
		return "", fmt.Errorf("claude route: invalid static owner %q", owner)
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return "", fmt.Errorf("claude route: invalid static owner %q", owner)
		}
	}
	return "rules/style-" + name + ".md", nil
}

func rulesBytes(block adapter.StaticBlock) ([]byte, error) {
	if block.Slot != "" && block.Slot != StaticSlotAppend {
		return nil, fmt.Errorf("claude route: unsupported static slot %q", block.Slot)
	}
	if _, err := rulesName(block.Owner); err != nil {
		return nil, err
	}
	if strings.Contains(block.Body, "<!-- style:") || strings.Contains(block.Body, "<!-- /style:") || strings.Contains(block.Body, "<!-- style-rules:") || strings.Contains(block.Body, "<!-- /style-rules:") {
		return nil, fmt.Errorf("claude route: reserved marker in static block %s", block.Owner)
	}
	body := block.Body
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return []byte(fmt.Sprintf("<!-- style-rules:v1 %s -->\n%s<!-- /style-rules:v1 %s -->\n", block.Owner, body, block.Owner)), nil
}

func staticDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (r *route) readStatic(path string) ([]byte, os.FileMode, error) {
	if err := install.CheckPath(r.dir, path); err != nil {
		return nil, 0, err
	}
	return readOptional(path)
}

func (r *route) ownedRules() (map[string][]byte, map[string]os.FileMode, error) {
	files := map[string][]byte{}
	modes := map[string]os.FileMode{}
	if r.static.Layout != "" && r.static.Layout != LegacyStaticSlot && r.static.Layout != StaticSlotAppend {
		return nil, nil, fmt.Errorf("claude route: unsupported static layout %q", r.static.Layout)
	}
	if r.static.Layout != StaticSlotAppend && len(r.static.Files) > 0 {
		return nil, nil, fmt.Errorf("claude route: rules ownership without rules layout")
	}
	names := make([]string, 0, len(r.static.Files))
	for name := range r.static.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		owner := adapter.Owner(strings.TrimSuffix(strings.TrimPrefix(name, "rules/style-"), ".md"))
		canonical, err := rulesName(owner)
		if err != nil || canonical != name {
			return nil, nil, fmt.Errorf("claude route: invalid managed rules path %q", name)
		}
		path := filepath.Join(r.dir, filepath.FromSlash(name))
		data, mode, err := r.readStatic(path)
		if err != nil {
			return nil, nil, err
		}
		if data == nil || staticDigest(data) != r.static.Files[name] || !bytes.HasPrefix(data, []byte(fmt.Sprintf("<!-- style-rules:v1 %s -->\n", owner))) || !bytes.HasSuffix(data, []byte(fmt.Sprintf("<!-- /style-rules:v1 %s -->\n", owner))) {
			return nil, nil, fmt.Errorf("claude route: managed rules file missing or changed: %s; restore its approved bytes", path)
		}
		files[name], modes[name] = data, mode
	}
	return files, modes, nil
}

func (r *route) rulesChanges(blocks []adapter.StaticBlock) (adapter.Changeset, error) {
	result := adapter.Changeset{Static: &adapter.StaticState{Layout: StaticSlotAppend, Files: map[string]string{}}}
	owned, modes, err := r.ownedRules()
	if err != nil {
		return result, err
	}
	desired := map[string][]byte{}
	for _, block := range blocks {
		if strings.TrimSpace(block.Body) == "" {
			continue
		}
		name, err := rulesName(block.Owner)
		if err != nil {
			return result, err
		}
		if _, exists := desired[name]; exists {
			return result, fmt.Errorf("claude route: duplicate static owner %s", block.Owner)
		}
		body, err := rulesBytes(block)
		if err != nil {
			return result, err
		}
		desired[name] = body
	}
	names := map[string]bool{}
	for name := range owned {
		names[name] = true
	}
	for name := range desired {
		names[name] = true
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		path := filepath.Join(r.dir, filepath.FromSlash(name))
		before, mode, err := r.readStatic(path)
		if err != nil {
			return result, err
		}
		if before != nil && owned[name] == nil {
			return result, fmt.Errorf("claude route: rules file exists outside installation ownership: %s", path)
		}
		after, keep := desired[name]
		if keep {
			result.Static.Files[name] = staticDigest(after)
		}
		if bytes.Equal(before, after) {
			continue
		}
		if mode == 0 {
			mode = 0o644
		}
		if prior := modes[name]; prior != 0 {
			mode = prior
		}
		result.Files = append(result.Files, adapter.FileChange{Path: path, Root: r.dir, Mode: mode, Before: before, After: after, CreateOnly: before == nil, Remove: !keep})
	}
	// Remove only installed package markers. Unrelated legacy blocks are retained.
	before, mode, err := r.readStatic(filepath.Join(r.dir, "CLAUDE.md"))
	if err != nil {
		return result, err
	}
	regions, err := parseClaudeRegions(string(before))
	if err != nil {
		return result, err
	}
	owners := map[string]bool{}
	for _, owner := range r.static.LegacyOwners {
		owners[string(owner)] = true
	}
	for _, region := range regions {
		if owners[region.owner] {
			to := ""
			name, _ := rulesName(adapter.Owner(region.owner))
			if _, ok := desired[name]; ok {
				to = filepath.Join(r.dir, filepath.FromSlash(name))
			}
			result.Migrations = append(result.Migrations, adapter.StaticMigration{Harness: string(ProviderName), Package: adapter.Owner(region.owner), From: filepath.Join(r.dir, "CLAUDE.md"), To: to})
		}
	}
	after := []byte(removeClaudeRegions(string(before), regions, owners))
	if !bytes.Equal(before, after) {
		result.Files = append(result.Files, adapter.FileChange{Path: filepath.Join(r.dir, "CLAUDE.md"), Root: r.dir, Mode: mode, Before: before, After: after, Remove: len(after) == 0})
	}
	return result, nil
}
