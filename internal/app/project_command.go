package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func projectInvocation(args []string) bool {
	for _, arg := range args[1:] {
		if arg == "--project" || strings.HasPrefix(arg, "--project=") {
			return true
		}
	}
	return false
}
func runProjectCommand(ctx context.Context, service *Service, workDir string, args []string, out, errOut io.Writer) int {
	jsonMode := false
	for _, arg := range args {
		if arg == "--json" {
			jsonMode = true
		}
		if value, ok := strings.CutPrefix(arg, "--json="); ok {
			jsonMode, _ = strconv.ParseBool(value)
		}
	}
	fail := func(err error) int {
		fmt.Fprintf(errOut, "style %s: %v\n", args[0], err)
		if jsonMode {
			_ = encodeJSON(out, map[string]any{"status": "error", "diagnostics": []Diagnostic{{Code: "project-invalid", Severity: "error", Message: err.Error()}}})
		}
		return ExitInvalid
	}
	switch args[0] {
	case "export", "plan", "install", "update", "uninstall", "list", "sync", "trust", "doctor", "stats", "probe", "explain", "adapters":
	default:
		return fail(&UsageError{Message: "--project is not supported by this command"})
	}
	var root, base string
	clean := []string{args[0]}
	seen := map[string]bool{}
	for i := 1; i < len(args); i++ {
		name, value, inline := strings.Cut(args[i], "=")
		if name != "--project" && name != "--style-home" {
			clean = append(clean, args[i])
			continue
		}
		if seen[name] {
			return fail(&UsageError{Message: "duplicate " + name})
		}
		seen[name] = true
		if !inline {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return fail(&UsageError{Message: name + " requires a directory"})
			}
			i++
			value = args[i]
		}
		if strings.TrimSpace(value) == "" {
			return fail(&UsageError{Message: name + " requires a directory"})
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(workDir, value)
		}
		if name == "--project" {
			root = value
		} else {
			base = value
		}
	}
	scoped, err := service.ForProject(root, base)
	if err != nil {
		return fail(err)
	}
	var report bytes.Buffer
	code := RunWithService(ctx, scoped, workDir, clean, &report, errOut)
	scope := map[string]any{"kind": "project", "root": scoped.project.Root, "base_home": scoped.project.Base, "home": scoped.project.Home, "external_prerequisites": []string{"style must be available on the harness PATH", "harness workspace trust remains external", "launch Claude Code at the project root", "launch Codex through the canonical project path"}}
	coexisting := map[string][]string{}
	if user, e := loadLock(scoped.project.Base); e == nil {
		for id := range user.Harnesses {
			for _, pkg := range user.Packages {
				coexisting[id] = append(coexisting[id], pkg.Name)
			}
			sort.Strings(coexisting[id])
		}
	}
	scope["coexisting_user_packages"] = coexisting
	scope["budget_scope"] = "selected installation only; active user packages have a separate budget and state"
	if jsonMode {
		value := map[string]any{}
		if report.Len() > 0 {
			var raw any
			if err = json.Unmarshal(report.Bytes(), &raw); err != nil {
				return fail(fmt.Errorf("invalid project command report: %w", err))
			}
			if object, ok := raw.(map[string]any); ok {
				value = object
			} else {
				value["diagnostics"] = raw
			}
		} else if code != ExitOK {
			value["status"] = "error"
		}
		value["scope"] = scope
		if err = encodeJSON(out, value); err != nil {
			return ExitInvalid
		}
	} else {
		fmt.Fprintf(out, "scope=project root=%s home=%s\n", scoped.project.Root, scoped.project.Home)
		fmt.Fprintln(out, "prerequisites: style on harness PATH; same STYLE_HOME; external workspace trust; Claude starts at project root; Codex uses canonical path")
		if len(coexisting) > 0 {
			data, _ := json.Marshal(coexisting)
			fmt.Fprintf(out, "coexisting user packages: %s (separate budget/state)\n", data)
		}
		if _, err = io.Copy(out, &report); err != nil {
			return ExitInvalid
		}
	}
	return code
}
func runScopedStats(ctx context.Context, s *Service, args []string, out, errOut io.Writer) int {
	if s.project == nil {
		return runStats(ctx, args, out, errOut)
	}
	if _, err := s.loadLock(s.project.Home); err != nil {
		fmt.Fprintln(errOut, "style stats:", err)
		return ExitInvalid
	}
	return runStatsAtHome(ctx, args, out, errOut, time.Now(), s.project.Home)
}
