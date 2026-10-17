package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
)

const explainUsage = "style explain --harness h --event NativeEvent --payload f.json [--index f] [--state-dir d] [--facts f] [--provenance f] [--fail-closed] [--json]"

func runExplain(ctx context.Context, s *Service, workDir string, args []string, out, errOut io.Writer) int {
	request, jsonMode, err := parseExplainArgs(args)
	if err != nil {
		fmt.Fprintln(errOut, "style explain: "+err.Error())
		fmt.Fprintln(errOut, explainUsage)
		return ExitUsage
	}
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Fprintln(out, explainUsage)
			return ExitOK
		}
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
	}
	if runErr != nil {
		return ExitInvalid
	}
	return ExitOK
}
func parseExplainArgs(args []string) (ExplainRequest, bool, error) {
	request := ExplainRequest{}
	jsonMode := false
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		arg, value, inline := strings.Cut(args[i], "=")
		if arg == "--help" || arg == "-h" {
			return request, false, nil
		}
		if seen[arg] {
			return request, false, fmt.Errorf("duplicate option %q", arg)
		}
		seen[arg] = true
		if arg == "--json" || arg == "--fail-closed" {
			b := true
			var err error
			if inline {
				b, err = strconv.ParseBool(value)
				if err != nil {
					return request, false, fmt.Errorf("invalid boolean option %s", arg)
				}
			}
			if arg == "--json" {
				jsonMode = b
			} else {
				request.FailClosed = b
			}
			continue
		}
		var target *string
		switch arg {
		case "--harness":
			target = &request.Harness
		case "--event":
			target = &request.Event
		case "--payload":
			target = &request.Payload
		case "--index":
			target = &request.Index
		case "--state-dir":
			target = &request.StateDir
		case "--facts":
			target = &request.Facts
		case "--provenance":
			target = &request.Provenance
		default:
			return request, false, fmt.Errorf("unknown option %q", arg)
		}
		if !inline {
			i++
			if i >= len(args) || strings.HasPrefix(args[i], "--") {
				return request, false, fmt.Errorf("missing value for %s", arg)
			}
			value = args[i]
		}
		if value == "" {
			return request, false, fmt.Errorf("empty option %s", arg)
		}
		*target = value
	}
	if request.Harness == "" || request.Event == "" || request.Payload == "" || strings.Contains(request.Harness, ",") || request.Harness == "auto" {
		return request, false, fmt.Errorf("one harness, native event and payload are required")
	}
	return request, jsonMode, nil
}
func writeExplainText(out io.Writer, result ExplainResult) {
	quote := func(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }
	fmt.Fprintf(out, "explain %s %s: %s\n", quote(result.Harness), quote(result.Event), result.Status)
	fmt.Fprintf(out, "inputs index=%s payload=%s\nstate %s (%s) before=%+v after=%+v\n", result.Inputs.IndexSHA256, result.Inputs.PayloadSHA256, result.State.Mode, result.State.Quality, result.State.Before, result.State.After)
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
		fmt.Fprintf(out, "rule %s: %s into=%s\n", quote(rule.Rule), quote(rule.Status), quote(rule.DeduplicatedInto))
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
