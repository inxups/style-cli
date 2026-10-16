package app

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/speakeasy-api/agenthooks"
)

// TestA2NativePlanReports checks the M2-A2 plan contract for the example
// native rule on both harnesses (rules §3.10).
func TestA2NativePlanReports(t *testing.T) {
	root := repositoryRoot(t)
	service := testService(t)
	result, err := service.Plan(context.Background(), PlanRequest{Source: filepath.Join(root, "example", "terse-reviewer"), Harnesses: []string{"codex", "claude-code"}})
	if err != nil {
		t.Fatal(err)
	}
	reports := map[string]PlanReport{}
	for _, report := range result.Reports {
		reports[report.Provider] = report
	}
	native := func(report PlanReport) plan.RulePlan {
		for _, rule := range report.Plan.Rules {
			if rule.ID == "codex-only-notify" {
				return rule
			}
		}
		t.Fatalf("%s plan has no codex-only-notify", report.Provider)
		return plan.RulePlan{}
	}
	codexRule := native(reports["codex"])
	if codexRule.Status != plan.NativeHook || codexRule.Action != "" || codexRule.Needs != nil ||
		!reflect.DeepEqual(codexRule.Native, &plan.NativeEntry{Event: "Stop", Timeout: 60}) {
		t.Fatalf("codex native plan = %#v", codexRule)
	}
	claudeRule := native(reports["claude-code"])
	if claudeRule.Status != plan.Skipped || claudeRule.Native != nil || !reflect.DeepEqual(claudeRule.Reasons, []string{"no native mapping for this harness"}) {
		t.Fatalf("claude native plan = %#v", claudeRule)
	}
	if got := reports["codex"].Tokens.Unestimated; !reflect.DeepEqual(got, []tokens.NativeOutput{{RuleID: "codex-only-notify", Event: "Stop"}}) {
		t.Fatalf("codex unestimated = %#v", got)
	}
	if got := reports["claude-code"].Tokens.Unestimated; got != nil {
		t.Fatalf("claude unestimated = %#v, want none", got)
	}
	for provider, report := range reports {
		for _, rule := range report.Tokens.Rules {
			if rule.RuleID == "codex-only-notify" {
				t.Fatalf("%s per-rule estimates include the native rule", provider)
			}
		}
	}

	var out, errOut bytes.Buffer
	if code := RunWithService(context.Background(), service, root, []string{"plan", "example/terse-reviewer", "--json"}, &out, &errOut); code != ExitOK {
		t.Fatalf("plan --json exit=%d stderr=%s", code, errOut.String())
	}
	text := out.String()
	for _, forbidden := range []string{"skipped-m2", `"command"`, `"notify"`, "Runs the local notify command"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("plan JSON contains %q", forbidden)
		}
	}
	for _, required := range []string{`"status": "native-hook"`, `"native_unestimated"`, `"event": "Stop"`, `"timeout": 60`} {
		if !strings.Contains(text, required) {
			t.Fatalf("plan JSON lacks %q", required)
		}
	}
}

// nonNativeFacts hides the Codex NativeHook method so planning reports the
// native mapping as unwritable.
type nonNativeFacts struct{ adapter.PlanningFacts }

func TestA2NativeFallbackErrorBlocksInstallWithoutChanges(t *testing.T) {
	for _, tc := range []struct {
		fallback string
		code     int
	}{{"error", ExitBlocked}, {"skip", ExitOK}, {"degrade", ExitOK}} {
		t.Run(tc.fallback, func(t *testing.T) {
			root := t.TempDir()
			home, config := filepath.Join(root, "style"), filepath.Join(root, "codex")
			installation := adapter.Installation{Provider: agenthooks.ProviderCodex, Version: "0.159.3", ConfigDir: config}
			facts := codex.DefaultFacts()
			definition := codex.DefinitionForInstallation(installation)
			definition.Detector = fixedDetector{installation: installation}
			definition.Facts = nonNativeFacts{facts}
			definition.StaticPositions = facts
			definition.Routes = []adapter.Route{codex.NewUserRouteForCommand(config, "/tmp/style", &facts)}
			service, err := New(definition)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("STYLE_HOME", home)
			source := writeInstallPackage(t, filepath.Join(root, "pkg"), "spec: style/v0.3\nname: native-fallback\nversion: 0.1.0\ndescription: native fallback\nrules:\n  - id: base\n    target: system\n    text: Base style.\n  - id: notify\n    target: native\n    fallback: "+tc.fallback+"\n    codex: {event: Stop, command: notify}\n")
			var out, errOut bytes.Buffer
			code := RunWithService(context.Background(), service, root, []string{"install", source, "--harness=codex", "--yes"}, &out, &errOut)
			if code != tc.code {
				t.Fatalf("install exit=%d, want %d; stdout=%s stderr=%s", code, tc.code, out.String(), errOut.String())
			}
			if tc.code == ExitBlocked {
				for _, dir := range []string{home, config} {
					if _, err := os.Lstat(dir); !errors.Is(err, fs.ErrNotExist) {
						t.Fatalf("blocked install created %s (err=%v)", dir, err)
					}
				}
				out.Reset()
				errOut.Reset()
				if code := RunWithService(context.Background(), service, root, []string{"plan", source, "--harness=codex"}, &out, &errOut); code != ExitBlocked ||
					!strings.Contains(out.String(), "- notify: install-error (native)\n  ! adapter codex does not support native hooks; fallback is error\n") {
					t.Fatalf("plan exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
				}
			}
		})
	}
}
