package claude

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
)

func TestRulesMigrationRestoresLegacyBytesAndModes(t *testing.T) {
	for name, original := range map[string]string{"empty": "", "lf": "User\n", "crlf": "用户\r\n\r\n", "no-newline": "用户 instructions", "foreign-owner": "<!-- style:outside -->\nKeep\n<!-- /style:outside -->\n"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "CLAUDE.md")
			blocks := []adapter.StaticBlock{{Owner: "first", Body: "First\nSecond"}, {Owner: "second", Body: "Other"}}
			legacy, err := mergeClaudeMD([]byte(original), blocks)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, legacy, 0o640); err != nil {
				t.Fatal(err)
			}
			r := NewUserRoute(dir, nil).(*route).BindStaticState(adapter.StaticState{LegacyOwners: []adapter.Owner{"first", "second"}})
			change, err := r.Plan(context.Background(), nil, blocks)
			if err != nil || len(change.Migrations) != 2 {
				t.Fatalf("migration: %v, %+v", err, change.Migrations)
			}
			if err := r.Apply(context.Background(), change); err != nil {
				t.Fatal(err)
			}
			if original == "" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("STYLE-only legacy file remains: %v", err)
				}
			} else {
				data, err := os.ReadFile(path)
				info, statErr := os.Stat(path)
				if err != nil || statErr != nil || string(data) != original || info.Mode().Perm() != fixtureMode(0o640) {
					t.Fatalf("legacy bytes/mode: %q %v %v", data, err, statErr)
				}
			}
			for _, owner := range []string{"first", "second"} {
				path := filepath.Join(dir, "rules", "style-"+owner+".md")
				data, err := os.ReadFile(path)
				info, statErr := os.Stat(path)
				if err != nil || statErr != nil || info.Mode().Perm() != fixtureMode(0o644) || !bytes.HasPrefix(data, []byte("<!-- style-rules:v1 "+owner+" -->\n")) || bytes.Contains(data, []byte("paths:")) {
					t.Fatalf("rules format/mode: %q %v %v", data, err, statErr)
				}
			}
			second, err := r.Plan(context.Background(), nil, blocks)
			if err != nil || len(second.Files) != 0 {
				t.Fatalf("repeat plan: %v, %d changes", err, len(second.Files))
			}
		})
	}
}

func TestRulesOwnershipRequiresStateFormatAndExactBytes(t *testing.T) {
	for _, kind := range []string{"foreign", "same-bytes", "forged-header", "drift", "missing", "bad-format", "invalid-path", "unsupported-layout"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "rules", "style-first.md")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			block := adapter.StaticBlock{Owner: "first", Body: "Approved"}
			data, err := rulesBytes(block)
			if err != nil {
				t.Fatal(err)
			}
			state := adapter.StaticState{Layout: StaticSlotAppend, Files: map[string]string{}}
			switch kind {
			case "foreign":
				data = []byte("User instructions")
			case "forged-header":
				data = []byte("<!-- style-rules:v1 first -->\nUnapproved\n<!-- /style-rules:v1 first -->\n")
			case "drift", "missing", "bad-format":
				state.Files["rules/style-first.md"] = staticDigest(data)
				if kind == "drift" {
					data = append(data, []byte("changed")...)
				}
				if kind == "bad-format" {
					data = []byte("Unmarked")
					state.Files["rules/style-first.md"] = staticDigest(data)
				}
			case "invalid-path":
				state.Files["../outside"] = staticDigest(data)
			case "unsupported-layout":
				state.Layout = "future"
			}
			if kind != "missing" {
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			r := NewUserRoute(dir, nil).(*route).BindStaticState(state)
			if _, err := r.Plan(context.Background(), nil, []adapter.StaticBlock{block}); err == nil {
				t.Fatal("unowned or invalid rules accepted")
			}
			if _, err := r.Remove(context.Background(), "style"); (kind == "drift" || kind == "missing" || kind == "bad-format" || kind == "invalid-path" || kind == "unsupported-layout") && err == nil {
				t.Fatal("invalid owned rules removed")
			}
			if kind != "missing" {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, data) {
					t.Fatal("failed plan wrote file")
				}
			}
		})
	}
}

func TestRulesSkipEmptyEffectiveBodies(t *testing.T) {
	dir := t.TempDir()
	r := NewUserRoute(dir, nil)
	change, err := r.Plan(context.Background(), nil, []adapter.StaticBlock{{Owner: "empty"}, {Owner: "whitespace", Body: "\n\t "}, {Owner: "present", Body: "Instructions"}})
	if err != nil || len(change.Files) != 1 || len(change.Static.Files) != 1 || findChange(change, filepath.Join(dir, "rules", "style-present.md")) == nil {
		t.Fatalf("empty static bodies generated files: %v", err)
	}
}

func TestRulesUpdatePreservesModeAndOwnerRemovalKeepsOtherState(t *testing.T) {
	dir := t.TempDir()
	r := NewUserRoute(dir, nil)
	blocks := []adapter.StaticBlock{{Owner: "first", Body: "A"}, {Owner: "second", Body: "B"}}
	change, err := r.Plan(context.Background(), nil, blocks)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rules", "style-first.md")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	blocks[0].Body = "Updated"
	change, err = r.Plan(context.Background(), nil, blocks)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != fixtureMode(0o600) {
		t.Fatal("update changed existing rules mode")
	}
	change, err = r.Remove(context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	change, err = r.Plan(context.Background(), nil, blocks[1:])
	if err != nil || len(change.Files) != 0 {
		t.Fatalf("remaining ownership lost: %v", err)
	}
}

type rulesCancelContext struct {
	context.Context
	calls int
}

func (c *rulesCancelContext) Err() error {
	c.calls++
	if c.calls >= 6 {
		return context.Canceled
	}
	return nil
}

func TestRulesMigrationStalePlanAndInterruptedApplyKeepLegacy(t *testing.T) {
	for _, kind := range []string{"stale", "cancel", "symlink-after-plan"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "CLAUDE.md")
			block := adapter.StaticBlock{Owner: "first", Body: "Approved"}
			before, err := mergeClaudeMD([]byte("用户"), []adapter.StaticBlock{block})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, before, 0o640); err != nil {
				t.Fatal(err)
			}
			r := NewUserRoute(dir, nil).(*route).BindStaticState(adapter.StaticState{LegacyOwners: []adapter.Owner{"first"}})
			change, err := r.Plan(context.Background(), nil, []adapter.StaticBlock{block})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			switch kind {
			case "stale":
				before = []byte("External edit")
				if err := os.WriteFile(path, before, 0o640); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				ctx = &rulesCancelContext{Context: ctx}
			case "symlink-after-plan":
				outside := t.TempDir()
				if err := os.Symlink(outside, filepath.Join(dir, "rules")); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.Apply(ctx, change); err == nil {
				t.Fatal("interrupted/stale migration succeeded")
			} else if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, before) {
				t.Fatal("failed migration damaged legacy file")
			}
			if info, err := os.Stat(path); err != nil || info.Mode().Perm() != fixtureMode(0o640) {
				t.Fatal("rollback damaged legacy mode")
			}
			if _, err := os.Stat(filepath.Join(dir, "rules", "style-first.md")); !os.IsNotExist(err) {
				t.Fatal("failed migration left rules file")
			}
			entries, _ := os.ReadDir(dir)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".agenthooks-") {
					t.Fatal("transaction debris remains")
				}
			}
		})
	}
}
