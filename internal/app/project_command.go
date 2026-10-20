package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

func runProjectCommand(ctx context.Context, service *Service, workDir string, global globalOptions, args []string, in io.Reader, out, errOut io.Writer) int {
	root := global.Project
	fail := func(err error) int {
		var usageErr *UsageError
		if errors.As(err, &usageErr) {
			command, _ := lookupCommand(args[0])
			return command.usageError(errOut, err)
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
	scoped, err := service.ForProject(root, global.Home)
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
	return runScoped(ctx, scoped, workDir, args, in, out, errOut)
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
		return runStatsAtHome(ctx, args, out, errOut, time.Now(), s.home)
	}
	if _, err := s.loadLock(s.project.Home); err != nil {
		fmt.Fprintln(errOut, "style stats:", err)
		return ExitInvalid
	}
	return runStatsAtHome(ctx, args, out, errOut, time.Now(), s.project.Home)
}
