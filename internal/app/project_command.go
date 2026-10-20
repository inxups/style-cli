package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
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
func runProjectCommand(ctx context.Context, service *Service, workDir string, args []string, in io.Reader, out, errOut io.Writer) int {
	var root, base string
	fail := func(err error) int {
		var usageErr *UsageError
		if errors.As(err, &usageErr) {
			if command, ok := lookupCommand(args[0]); ok {
				return command.usageError(errOut, err)
			}
			fmt.Fprintf(errOut, "style %s: %v\n", args[0], err)
			return ExitUsage
		}
		report := func(out, errOut io.Writer) int {
			diagnostic := failureDiagnostic(err)
			if diagnostic.Code == "command-failed" {
				diagnostic.Code = "project-invalid"
			}
			writeCommandDiagnostics(errOut, args[0], []Diagnostic{diagnostic})
			_ = encodeJSON(out, map[string][]Diagnostic{"diagnostics": {diagnostic}})
			return ExitInvalid
		}
		if jsonRequested(args) {
			return runJSON(args[0], envelopeScope{Kind: "project", Root: root}, out, errOut, report)
		}
		return report(io.Discard, errOut)
	}
	if command, ok := lookupCommand(args[0]); !ok || !command.Project {
		return fail(&UsageError{Message: "--project is not supported by this command"})
	}
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
	if !jsonRequested(args) {
		// Text output streams, so a confirmation prompt follows its preview.
		// The header waits for the command's first stdout write: a usage
		// error leaves stdout empty.
		out = &headerWriter{out: out, header: func(out io.Writer) {
			fmt.Fprintf(out, "scope=project root=%s home=%s\n", scoped.project.Root, scoped.project.Home)
			fmt.Fprintln(out, "prerequisites: style on harness PATH; same STYLE_HOME; external workspace trust; Claude starts at project root; Codex uses canonical path")
			if coexisting := scoped.coexistingUserPackages(); len(coexisting) > 0 {
				data, _ := json.Marshal(coexisting)
				fmt.Fprintf(out, "coexisting user packages: %s (separate budget/state)\n", data)
			}
		}}
	}
	return RunWithServiceInput(ctx, scoped, workDir, clean, in, out, errOut)
}

// headerWriter writes header once, before the first write that reaches out.
type headerWriter struct {
	out     io.Writer
	header  func(io.Writer)
	written bool
}

func (w *headerWriter) Write(p []byte) (int, error) {
	if !w.written {
		w.written = true
		w.header(w.out)
	}
	return w.out.Write(p)
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
