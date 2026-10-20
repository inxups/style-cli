package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// liveRequest reads doctor's live-check options; --harness and --timeout
// only apply with --live.
func liveRequest(parsed parsedArgs, errOut io.Writer) (*ProbeRequest, error) {
	if !parsed.Bool("--live") {
		for _, name := range []string{"--harness", "--timeout"} {
			if parsed.Has(name) {
				return nil, errors.New(name + " requires --live")
			}
		}
		return nil, nil
	}
	request := &ProbeRequest{Harnesses: parsed.List("--harness"), Timeout: time.Minute}
	if parsed.Has("--timeout") {
		duration, err := time.ParseDuration(parsed.String("--timeout"))
		if err != nil || duration < time.Second || duration > 10*time.Minute {
			return nil, errors.New("timeout must be between 1s and 10m")
		}
		request.Timeout = duration
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
	return request, nil
}
func probeQuote(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }

// writeLiveText writes the live part of a doctor report; its findings are
// already among the doctor diagnostics.
func writeLiveText(out io.Writer, result ProbeResult) {
	fmt.Fprintf(out, "live %s: %s (timeout=%s)\n", probeQuote(result.RunID), result.Status, result.Timeout)
	for _, target := range result.Targets {
		route := ""
		if target.Route != "" {
			route = " route=" + probeQuote(target.Route)
		}
		fmt.Fprintf(out, "  %s%s: %s origin=%s callback_observed=%t\n", probeQuote(target.Harness), route, target.Status, target.Origin, target.CallbackObserved)
		if target.CallbackObserved {
			fmt.Fprintf(out, "    event=%s session_sha256=%s; rule execution and model delivery remain unassessed.\n", probeQuote(target.Event), target.SessionSHA256)
		}
	}
	fmt.Fprintf(out, "  cleanup complete=%t\n", result.Cleanup.Complete)
	for _, path := range result.Cleanup.Remaining {
		fmt.Fprintf(out, "    remaining=%s\n", probeQuote(path))
	}
}
