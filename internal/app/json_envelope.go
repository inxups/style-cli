package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// envelopeFormat versions the --json envelope itself; command reports keep
// their own format fields inside result.
const envelopeFormat = 2

// jsonEnvelope is the only shape --json writes to stdout. Usage errors (exit 2)
// write nothing to stdout.
type jsonEnvelope struct {
	Format      int               `json:"format"`
	Command     string            `json:"command"`
	Status      string            `json:"status"`
	Scope       any               `json:"scope"`
	Diagnostics []json.RawMessage `json:"diagnostics"`
	Result      json.RawMessage   `json:"result"`
}

func jsonRequested(args []string) bool {
	for _, arg := range args[1:] {
		if arg == "--json" {
			return true
		}
	}
	return false
}

// envelopeStatus mirrors the exit code so scripts can rely on either.
func envelopeStatus(code int) string {
	switch code {
	case ExitOK:
		return "ok"
	case ExitSyncChanged:
		return "needs-trust"
	case ExitBlocked:
		return "blocked"
	default:
		return "error"
	}
}

// runJSON captures a command's report and writes it wrapped in the envelope.
func runJSON(command string, scope any, out, errOut io.Writer, run func(out, errOut io.Writer) int) int {
	var report bytes.Buffer
	failures := &failureLines{prefix: "style " + command + ": ", next: errOut}
	code := run(&report, failures)
	if code == ExitUsage {
		return code
	}
	envelope, err := wrapReport(command, code, scope, report.Bytes(), failures.messages)
	if err == nil {
		err = encodeJSON(out, envelope)
	}
	if err != nil {
		fmt.Fprintf(errOut, "style %s: encode report: %v\n", command, err)
		return ExitInvalid
	}
	return code
}

func wrapReport(command string, code int, scope any, report []byte, failures []string) (jsonEnvelope, error) {
	envelope := jsonEnvelope{Format: envelopeFormat, Command: command, Status: envelopeStatus(code), Scope: scope, Diagnostics: []json.RawMessage{}, Result: json.RawMessage("null")}
	if report = bytes.TrimSpace(report); len(report) > 0 {
		result, diagnostics, err := splitDiagnostics(report)
		if err != nil {
			return envelope, err
		}
		envelope.Result = result
		envelope.Diagnostics = append(envelope.Diagnostics, diagnostics...)
	}
	if code != ExitOK && len(envelope.Diagnostics) == 0 {
		// Failures reported only on stderr still explain themselves in JSON.
		for _, message := range failures {
			diagnostic, _ := json.Marshal(Diagnostic{Code: "command-failed", Severity: "error", Message: message})
			envelope.Diagnostics = append(envelope.Diagnostics, diagnostic)
		}
	}
	return envelope, nil
}

// splitDiagnostics moves a report's top-level diagnostics into the envelope and
// keeps every other member in order. A bare array report is all diagnostics.
func splitDiagnostics(report []byte) (json.RawMessage, []json.RawMessage, error) {
	var diagnostics []json.RawMessage
	switch report[0] {
	case '[':
		err := json.Unmarshal(report, &diagnostics)
		return json.RawMessage("null"), diagnostics, err
	case '{':
	default:
		return report, nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(report))
	if _, err := decoder.Token(); err != nil {
		return nil, nil, err
	}
	var result bytes.Buffer
	result.WriteByte('{')
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, nil, err
		}
		key, _ := token.(string)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, err
		}
		if key == "diagnostics" {
			if string(value) != "null" {
				if err := json.Unmarshal(value, &diagnostics); err != nil {
					return nil, nil, err
				}
			}
			continue
		}
		if result.Len() > 1 {
			result.WriteByte(',')
		}
		name, _ := json.Marshal(key)
		result.Write(name)
		result.WriteByte(':')
		result.Write(value)
	}
	result.WriteByte('}')
	return result.Bytes(), diagnostics, nil
}

// failureLines forwards stderr and remembers "style <command>: ..." errors.
type failureLines struct {
	prefix   string
	next     io.Writer
	pending  []byte
	messages []string
}

func (w *failureLines) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 {
			break
		}
		if message, ok := strings.CutPrefix(string(w.pending[:i]), w.prefix); ok {
			w.messages = append(w.messages, message)
		}
		w.pending = w.pending[i+1:]
	}
	return w.next.Write(p)
}

// envelopeScope and projectEnvelopeScope describe where a command ran. Struct
// fields keep "kind" first and every other key in a fixed order.
type envelopeScope struct {
	Kind string `json:"kind"`
	Home string `json:"home,omitempty"`
}

type projectEnvelopeScope struct {
	Kind                   string              `json:"kind"`
	Root                   string              `json:"root"`
	BaseHome               string              `json:"base_home"`
	Home                   string              `json:"home"`
	ExternalPrerequisites  []string            `json:"external_prerequisites"`
	CoexistingUserPackages map[string][]string `json:"coexisting_user_packages"`
	BudgetScope            string              `json:"budget_scope"`
}

func (s *Service) jsonScope() any {
	if s.project == nil {
		scope := envelopeScope{Kind: "user"}
		if home, err := s.resolveStyleHome(""); err == nil {
			scope.Home = home
		}
		return scope
	}
	return projectEnvelopeScope{
		Kind: "project", Root: s.project.Root, BaseHome: s.project.Base, Home: s.project.Home,
		ExternalPrerequisites:  []string{"style must be available on the harness PATH", "harness workspace trust remains external", "launch Claude Code at the project root", "launch Codex through the canonical project path"},
		CoexistingUserPackages: s.coexistingUserPackages(),
		BudgetScope:            "selected installation only; active user packages have a separate budget and state",
	}
}

// coexistingUserPackages lists the user-scope packages active next to a
// project installation, by harness.
func (s *Service) coexistingUserPackages() map[string][]string {
	coexisting := map[string][]string{}
	if user, err := loadLock(s.project.Base); err == nil {
		for id := range user.Harnesses {
			for _, pkg := range user.Packages {
				coexisting[id] = append(coexisting[id], pkg.Name)
			}
			sort.Strings(coexisting[id])
		}
	}
	return coexisting
}
