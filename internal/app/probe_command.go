package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func runProbe(ctx context.Context, s *Service, args []string, out, errOut io.Writer) int {
	request, jsonMode, err := parseProbeArgs(args)
	if err == nil {
		_, err = s.parseHarnesses(request.Harnesses)
	}
	if err != nil {
		return probeCommand.usageError(errOut, err)
	}
	request.Ready = func(ready ProbeReady) error {
		if _, err := fmt.Fprintf(errOut, "probe ready: run_id=%s; trigger a subscribed event in your harness session.\n", ready.RunID); err != nil {
			return err
		}
		for _, target := range ready.Targets {
			if _, err := fmt.Fprintf(errOut, "  %s route=%s events=%s\n", probeQuote(target.Harness), probeQuote(target.Route), probeQuote(strings.Join(target.Events, ","))); err != nil {
				return err
			}
		}
		return nil
	}
	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, runErr := s.Probe(signalCtx, request)
	if jsonMode {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if encoder.Encode(result) != nil {
			return ExitInvalid
		}
	} else {
		writeProbeText(out, result)
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
			fmt.Fprintf(errOut, "style probe: %s\n", summary)
		}
		return ExitInvalid
	}
	return ExitOK
}
func parseProbeArgs(args []string) (ProbeRequest, bool, error) {
	parsed, err := probeCommand.parse(args)
	if err != nil {
		return ProbeRequest{}, false, err
	}
	request := ProbeRequest{Harnesses: parsed.List("--harness"), Timeout: time.Minute}
	if parsed.Has("--timeout") {
		duration, err := time.ParseDuration(parsed.String("--timeout"))
		if err != nil || duration < time.Second || duration > 10*time.Minute {
			return request, false, fmt.Errorf("timeout must be between 1s and 10m")
		}
		request.Timeout = duration
	}
	return request, parsed.Bool("--json"), nil
}
func probeQuote(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }
func writeProbeText(out io.Writer, result ProbeResult) {
	fmt.Fprintf(out, "probe %s: %s (timeout=%s)\n", probeQuote(result.RunID), result.Status, result.Timeout)
	for _, target := range result.Targets {
		route := ""
		if target.Route != "" {
			route = " route=" + probeQuote(target.Route)
		}
		fmt.Fprintf(out, "%s%s: %s origin=%s callback_observed=%t\n", probeQuote(target.Harness), route, target.Status, target.Origin, target.CallbackObserved)
		if target.CallbackObserved {
			fmt.Fprintf(out, "  event=%s session_sha256=%s; rule execution and model delivery remain unassessed.\n", probeQuote(target.Event), target.SessionSHA256)
		}
	}
	fmt.Fprintf(out, "cleanup complete=%t\n", result.Cleanup.Complete)
	for _, path := range result.Cleanup.Remaining {
		fmt.Fprintf(out, "  remaining=%s\n", probeQuote(path))
	}
	for _, finding := range result.Diagnostics {
		where := ""
		if finding.Harness != "" {
			where = " harness=" + probeQuote(finding.Harness)
		}
		fmt.Fprintf(out, "%s %s%s: %s\n", finding.Severity, finding.Code, where, probeQuote(finding.Message))
	}
}
