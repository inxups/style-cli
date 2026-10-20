package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func runExplain(ctx context.Context, s *Service, workDir string, args []string, out, errOut io.Writer) int {
	request, jsonMode, err := parseExplainArgs(args)
	if err == nil {
		request.Harness, err = s.singleHarness([]string{request.Harness})
	}
	if err != nil {
		return explainCommand.usageError(errOut, err)
	}
	for _, path := range []*string{&request.Payload, &request.Index, &request.StateDir, &request.Facts, &request.Provenance} {
		if *path != "" && !filepath.IsAbs(*path) {
			*path = filepath.Join(workDir, *path)
		}
	}
	result, runErr := s.Explain(ctx, request)
	if jsonMode {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if encoder.Encode(result) != nil {
			return ExitInvalid
		}
	} else {
		writeExplainText(out, result)
		if result.Count != nil {
			writeCountText(out, *result.Count)
		}
	}
	if runErr != nil {
		if !jsonMode {
			// The report stays on stdout; stderr says why the exit code is 1.
			summary := runErr.Error()
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Severity == "error" {
					summary = diagnostic.Code + ": " + diagnostic.Message
					break
				}
			}
			fmt.Fprintf(errOut, "style explain: %s\n", summary)
		}
		return ExitInvalid
	}
	return ExitOK
}
func parseExplainArgs(args []string) (ExplainRequest, bool, error) {
	parsed, err := explainCommand.parse(args)
	if err != nil {
		return ExplainRequest{}, false, err
	}
	request := ExplainRequest{
		Harness: strings.Join(parsed.List("--harness"), ","), Event: parsed.String("--event"), Payload: parsed.String("--payload"),
		Index: parsed.String("--index"), StateDir: parsed.String("--state-dir"), Facts: parsed.String("--facts"), Provenance: parsed.String("--provenance"),
		FailClosed: parsed.Bool("--fail-closed"),
	}
	if request.Count, err = countOptions(parsed); err == nil {
		// Like count and plan, an invalid --exact value is a usage error.
		err = request.Count.Validate()
	}
	if err != nil {
		return request, false, err
	}
	return request, parsed.Bool("--json"), nil
}
func writeExplainText(out io.Writer, result ExplainResult) {
	quote := func(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }
	event := ""
	if result.Event != "" {
		event = " " + quote(result.Event)
	}
	fmt.Fprintf(out, "explain %s%s: %s\n", quote(result.Harness), event, result.Status)
	if inputs := textFields("index", result.Inputs.IndexSHA256, "payload", result.Inputs.PayloadSHA256); inputs != "" {
		fmt.Fprintf(out, "inputs %s\n", inputs)
	}
	quality := ""
	if result.State.Quality != "" {
		quality = " (" + result.State.Quality + ")"
	}
	fmt.Fprintf(out, "state %s%s before=%+v after=%+v\n", result.State.Mode, quality, result.State.Before, result.State.After)
	for _, fact := range result.Facts {
		fmt.Fprintf(out, "fact %s: available=%t source=%s\n", quote(fact.Field), fact.Available, fact.Source)
	}
	for _, point := range result.Points {
		fmt.Fprintf(out, "point %s candidates=%d\n", quote(point.Point), point.CandidateCount)
	}
	for _, entry := range result.Entries {
		fmt.Fprintf(out, "entry %s point=%s source=%s action=%s selector=%s sources=%s when=%s render=%s block=%s repeat=%s output=%s\n", quote(entry.Entry), quote(entry.Point), quote(entry.Source), quote(entry.Action), entry.Selector, entry.Sources, entry.When, entry.Render, entry.Block, entry.Repeat, entry.Output)
		if entry.Refresh != nil {
			fmt.Fprintf(out, "  refresh interval=%d current=%d clock=%s reason=%s", entry.Refresh.Interval, entry.Refresh.Current, entry.Refresh.Clock, entry.Refresh.Reason)
			if entry.Refresh.Last != nil {
				fmt.Fprintf(out, " last=%d", *entry.Refresh.Last)
			}
			fmt.Fprintln(out)
		}
		for _, check := range entry.Conditions {
			fmt.Fprintf(out, "  when %s: %s available=%t source=%s\n", quote(check.Field), check.Result, check.Available, check.Source)
		}
		if entry.Render != "not-evaluated" {
			fmt.Fprintf(out, "  render model_bytes=%d model_sha256=%s user_bytes=%d user_sha256=%s\n", entry.Model.Bytes, entry.Model.SHA256, entry.User.Bytes, entry.User.SHA256)
		}
	}
	fmt.Fprintf(out, "compilation %s coverage=%s\n", result.CompileProvenance.Status, result.CompileProvenance.Coverage)
	for _, rule := range result.CompileProvenance.Rules {
		into := ""
		if rule.DeduplicatedInto != "" {
			into = " into=" + quote(rule.DeduplicatedInto)
		}
		fmt.Fprintf(out, "rule %s: %s%s\n", quote(rule.Rule), quote(rule.Status), into)
	}
	for _, item := range result.CompileProvenance.Static {
		fmt.Fprintf(out, "static %s slot=%s: %s\n", quote(item.Rule), quote(item.Slot), item.Status)
	}
	for _, item := range result.CompileProvenance.Native {
		fmt.Fprintf(out, "native %s: %s\n", quote(item.Rule), item.Status)
	}
	if result.Wire != nil {
		fmt.Fprintf(out, "wire stdout=%s\nwire stderr=%s\nwire exit_code=%d\n", quote(result.Wire.Stdout), quote(result.Wire.Stderr), result.Wire.ExitCode)
	} else {
		fmt.Fprintln(out, "wire null")
	}
	for _, finding := range result.Diagnostics {
		fmt.Fprintf(out, "%s %s: %s\n", finding.Severity, finding.Code, quote(finding.Message))
	}
}
