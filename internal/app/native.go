package app

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/adapter"
)

type compiledNative struct {
	Hook   adapter.HookSpec
	Order  compile.Order
	Report NativeHookReport
}

// NativeHookReport discloses executable fields only in install/trust/doctor.
// Plan reports intentionally retain their command-free NativeEntry contract.
type NativeHookReport struct {
	Harness         string            `json:"harness"`
	Rule            string            `json:"rule"`
	Event           string            `json:"event"`
	Matcher         string            `json:"matcher,omitempty"`
	Timeout         int               `json:"timeout"`
	DeclaredTimeout int               `json:"declared_timeout,omitempty"`
	Command         string            `json:"command"`
	Route           string            `json:"route"`
	Trust           adapter.TrustKind `json:"trust"`
	TrustSummary    string            `json:"trust_summary"`
}

type NativeHookDiff struct {
	Harness string            `json:"harness"`
	Rule    string            `json:"rule"`
	Before  *NativeHookReport `json:"before"`
	After   *NativeHookReport `json:"after"`
}

func nativeHookReport(target installAdapter, hook adapter.HookSpec, declaredTimeout int) NativeHookReport {
	seconds, _ := hook.Extra["timeout_seconds"].(int)
	trust := target.route.Trust()
	return NativeHookReport{Harness: string(target.provider), Rule: hook.NativeTag, Event: hook.NativeEvent,
		Matcher: hook.Matcher, Timeout: seconds, DeclaredTimeout: declaredTimeout, Command: hook.Command, Route: target.route.Name(), Trust: trust.Kind, TrustSummary: trust.Summary}
}

func (compilation *installCompilation) appendNative(pkg installPackage, target installAdapter, report plan.Report) error {
	for i, planned := range report.Rules {
		if planned.Status != plan.NativeHook {
			continue
		}
		writer, ok := target.definition.Facts.(adapter.NativeWriter)
		if !ok {
			return fmt.Errorf("adapter %s planned a native hook without a native writer", target.provider)
		}
		rule := pkg.resolved.Rules[i]
		for _, native := range spec.NativeRules(rule) {
			if native.Harness != string(target.provider) {
				continue
			}
			native.Package, native.RuleID = pkg.lock.Name, planned.ID
			hook, err := writer.Native(native)
			if err != nil {
				return fmt.Errorf("native %s/%s: %w", pkg.lock.Name, planned.ID, err)
			}
			priority, err := strconv.ParseFloat(fmt.Sprint(rule.Effective["priority"]), 64)
			if err != nil {
				return fmt.Errorf("native %s/%s: invalid priority", pkg.lock.Name, planned.ID)
			}
			compilation.native[target.provider] = append(compilation.native[target.provider], compiledNative{Hook: hook, Report: nativeHookReport(target, hook, native.Timeout),
				Order: compile.Order{Priority: priority, PackageSeq: pkg.lock.Seq, RuleIndex: rule.Index}})
		}
	}
	return nil
}

func (compilation installCompilation) nativeReports(name string) []NativeHookReport {
	reports := make([]NativeHookReport, 0)
	providers := make([]string, 0, len(compilation.native))
	for provider := range compilation.native {
		providers = append(providers, string(provider))
	}
	sort.Strings(providers)
	for _, provider := range providers {
		for _, native := range compilation.native[adapter.ProviderID(provider)] {
			if name == "" || strings.HasPrefix(native.Hook.NativeTag, name+"/") {
				reports = append(reports, native.Report)
			}
		}
	}
	return reports
}

// Fingerprint executable native declarations, including mappings for other
// harnesses and declared timeout before adapter clamping. Author notes do not
// require renewed code-execution approval for development links.
func nativeSourceFingerprint(item lockPackage, files map[string][]byte) (string, error) {
	pkg, err := loadNativeSource(item, files)
	if err != nil {
		return "", err
	}
	type declaration struct {
		Index    int
		Priority any
		Mappings []spec.NativeRule
	}
	var declarations []declaration
	for _, rule := range pkg.Rules {
		if rule.Effective["target"] != "native" {
			continue
		}
		mappings := spec.NativeRules(rule)
		for i := range mappings {
			mappings[i].RuleID, _ = rule.Effective["id"].(string)
		}
		declarations = append(declarations, declaration{Index: rule.Index, Priority: rule.Effective["priority"], Mappings: mappings})
	}
	data, err := json.Marshal(declarations)
	return hashBytes(data), err
}

func loadNativeSource(item lockPackage, files map[string][]byte) (*spec.ResolvedPackage, error) {
	singleName := ""
	if item.Source.Kind == "file" {
		singleName = filepath.Base(item.Source.Path)
	}
	pkg, err := spec.LoadSnapshot(item.Source.Path, files, singleName)
	if err != nil {
		return nil, err
	}
	return spec.Merge(pkg)
}

func (s *Service) baselineNative(item lockPackage, baseline approvedSnapshot, adapters []installAdapter) ([]NativeHookReport, error) {
	if baseline.Native != nil {
		return baseline.Native, nil
	}
	// Older approval snapshots retain authenticated source bytes, so the
	// native baseline can be reconstructed without reading edited sources.
	pkg, err := loadNativeSource(item, baseline.Files)
	if err != nil {
		return nil, err
	}
	var reports []NativeHookReport
	for _, target := range adapters {
		if harnessRequirement(pkg.Package.Metadata, target.provider, target.installation.Version) != nil {
			continue
		}
		writer, ok := target.definition.Facts.(adapter.NativeWriter)
		if !ok {
			continue
		}
		for _, rule := range pkg.Rules {
			if rule.Effective["target"] != "native" {
				continue
			}
			for _, native := range spec.NativeRules(rule) {
				if native.Harness != string(target.provider) {
					continue
				}
				native.Package, native.RuleID = item.Name, fmt.Sprint(rule.Effective["id"])
				hook, err := writer.Native(native)
				if err != nil {
					return nil, err
				}
				reports = append(reports, nativeHookReport(target, hook, native.Timeout))
			}
		}
	}
	return reports, nil
}

func nativeDiff(before, after []NativeHookReport) []NativeHookDiff {
	old, current := map[string]NativeHookReport{}, map[string]NativeHookReport{}
	for _, hook := range before {
		old[hook.Harness+"\x00"+hook.Rule] = hook
	}
	for _, hook := range after {
		current[hook.Harness+"\x00"+hook.Rule] = hook
	}
	keys := map[string]bool{}
	for key := range old {
		keys[key] = true
	}
	for key := range current {
		keys[key] = true
	}
	var ordered []string
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	var changes []NativeHookDiff
	for _, key := range ordered {
		previous, had := old[key]
		next, has := current[key]
		if had && has && previous == next {
			continue
		}
		harness, rule, _ := strings.Cut(key, "\x00")
		change := NativeHookDiff{Harness: harness, Rule: rule}
		if had {
			change.Before = &previous
		}
		if has {
			change.After = &next
		}
		changes = append(changes, change)
	}
	return changes
}

func writeNativeHooks(out io.Writer, reports []NativeHookReport) {
	if len(reports) == 0 {
		return
	}
	fmt.Fprintln(out, "Native commands:")
	for _, hook := range reports {
		fmt.Fprintf(out, "  %s %s: event=%s matcher=%q timeout=%ds\n    command: %s\n    route: %s; trust=%s; %s\n",
			hook.Harness, hook.Rule, hook.Event, hook.Matcher, hook.Timeout, hook.Command, hook.Route, hook.Trust, hook.TrustSummary)
	}
}

func (compilation *installCompilation) sortNative() {
	for _, hooks := range compilation.native {
		sort.SliceStable(hooks, func(i, j int) bool {
			a, b := hooks[i].Order, hooks[j].Order
			if a.Priority != b.Priority {
				return a.Priority < b.Priority
			}
			if a.PackageSeq != b.PackageSeq {
				return a.PackageSeq < b.PackageSeq
			}
			return a.RuleIndex < b.RuleIndex
		})
	}
}

func (compilation installCompilation) hooksForProvider(provider adapter.ProviderID) []adapter.HookSpec {
	hooks := hooksForProvider(provider, compilation.index)
	for _, native := range compilation.native[provider] {
		hooks = append(hooks, native.Hook)
	}
	return hooks
}
