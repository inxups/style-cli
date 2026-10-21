package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/saferead"
)

type DoctorRequest struct {
	Home       string
	WorkDir    string
	ResetState bool
	Shell      string
	// Confirm approves the state reset; the root is the directory it
	// removes. Without Confirm the reset proceeds; the CLI always asks.
	Confirm func(root string) (bool, error)
	// Live, when set, runs the live callback check after the cold checks.
	Live *ProbeRequest
}

type DoctorResult struct {
	Healthy         bool               `json:"healthy"`
	Packages        []InstalledPackage `json:"packages"`
	Adapters        []AdapterReport    `json:"adapters"`
	Diagnostics     []Diagnostic       `json:"diagnostics"`
	Effort          []EffortReport     `json:"effort"`
	TopRules        []RuleCost         `json:"top_rules"`
	LegacyStats     int                `json:"legacy_stats_without_estimate"`
	ExpiredSessions int                `json:"expired_sessions"`
	StateReset      bool               `json:"state_reset"`
	Native          []NativeHookCheck  `json:"native_hooks,omitempty"`
	Live            *ProbeResult       `json:"live,omitempty"`
}

// Doctor checks cold-path integrity. Runtime fail-open behavior never turns a
// corrupt index into a successful health check.
func (s *Service) Doctor(ctx context.Context, request DoctorRequest) (DoctorResult, error) {
	if err := checkContext(ctx); err != nil {
		return DoctorResult{}, err
	}
	home, err := s.resolveStyleHome(request.Home)
	if err != nil {
		return DoctorResult{}, err
	}
	result := DoctorResult{Packages: []InstalledPackage{}, Adapters: []AdapterReport{}, Diagnostics: []Diagnostic{}, Effort: []EffortReport{}, TopRules: []RuleCost{}}
	lock, lockErr := s.loadLock(home)
	if mapping, e := s.captureEffortMapping(home, nil); e != nil {
		result.add("effort-mapping-invalid", "error", e.Error())
	} else if !reflect.DeepEqual(mapping, lock.EffortMapping) {
		result.add("effort-mapping-pending", "warning", "local effort.yaml differs from the compiled lock snapshot; run sync to apply it")
	}
	if lockErr != nil {
		result.add("lock-invalid", "error", lockErr.Error())
	} else {
		listed, err := listLocked(ctx, home, lock)
		if err != nil {
			return DoctorResult{}, err
		}
		result.Packages = listed.Packages
		for _, pkg := range listed.Packages {
			if strings.Contains(pkg.Error, spec.NativeFormatRemoved) {
				result.add(spec.NativeFormatRemoved, "error", pkg.Name+": "+pkg.Error)
			}
			if !pkg.HashMatches {
				if pkg.Git != nil {
					result.add("git-snapshot-changed", "error", fmt.Sprintf("%s differs from its locked Git export; restore locked bytes or use a separate local/link source (%v %s)", pkg.Name, pkg.ChangedFiles, pkg.Error))
				} else {
					result.add("package-changed", "error", fmt.Sprintf("%s differs from lock; run sync/trust (%v %s)", pkg.Name, pkg.ChangedFiles, pkg.Error))
				}
			}
			if pkg.Link == "dev" {
				result.add("link-dev", "warning", pkg.Name+" is a development link; ordinary rule edits are accepted by sync after initial trust; native changes require renewed approval")
			}
		}
		for _, pkg := range lock.Packages {
			if pkg.Replace {
				result.add("replace-active", "warning", pkg.Name+" uses replace mode")
			}
		}
		if err := s.checkIndex(ctx, home, lock); err != nil {
			code := "index-invalid"
			var drift *routeDriftError
			if errors.As(err, &drift) {
				code = "route-drift"
			}
			result.add(code, "error", err.Error())
		}
		if data, err := saferead.ReadFile(filepath.Join(home, "build", "index.json"), 16<<20); err == nil {
			var index compile.Index
			if json.Unmarshal(data, &index) == nil {
				if index.Format >= compile.FailClosedFormat {
					result.add("runtime-index-7", "warning", "rule failure guards require index format 7; unreadable indexes and host termination need installation-level protection")
				}
				if index.Format >= compile.EffortMappingFormat {
					result.add("runtime-index-6", "warning", "user effort mappings require index format 6; upgrade every configured STYLE runner")
				}
				for _, d := range effortMappingDiagnostics(index.EffortMapping) {
					result.Diagnostics = append(result.Diagnostics, d)
				}
				if index.Format == compile.EventFormat {
					result.add("event-index-3", "warning", "event templates require index format 3; upgrade every configured STYLE runner")
				}
				if index.Format == compile.UnknownFormat {
					result.add("runtime-index-5", "warning", "explicit missing-value selectors require index format 5; upgrade every configured STYLE runner")
				}
				if index.Format == compile.ProviderFormat {
					result.add("runtime-index-4", "warning", "model-service rules require index format 4; upgrade every configured STYLE runner")
				}
				refresh := false
				for _, events := range index.Harnesses {
					for _, entries := range events {
						for _, entry := range entries {
							refresh = refresh || entry.Refresh > 0
						}
					}
				}
				if refresh {
					result.add("refresh-index-2", "warning", "refresh uses index format 2 or newer; upgrade every configured STYLE runner before sync; old runners conservatively reject unsupported indices")
					if !state.RefreshSupported() {
						result.add("refresh-unsupported-platform", "error", "refresh session locking is unavailable on this platform")
					}
				}
			}
		}
		for provider, harness := range lock.Harnesses {
			if definition, ok := s.definitions[adapter.ProviderID(provider)]; ok && harness.StaticLayout == "" {
				if _, ok := definition.Facts.(adapter.StaticLayoutFacts); ok {
					result.add("static-migration-pending", "warning", provider+": legacy static layout; next successful route rebuild migrates it")
				}
			}
		}
		result.Native, err = s.checkNative(ctx, home, lock, "")
		if err != nil {
			result.add("native-config", "warning", err.Error())
		}
		for _, native := range result.Native {
			if native.Status != "ok" {
				result.add("native-"+native.Status, "warning", native.Harness+" "+native.Rule+": native hook "+native.Status)
			}
			if native.TrustStatus != "" && native.TrustStatus != "match" {
				result.add("native-trust", "warning", native.Harness+" "+native.Rule+": trusted_hash "+native.TrustStatus)
			}
		}
	}
	inventory, err := s.inspectAdapters(ctx, home, lock)
	if err != nil {
		return DoctorResult{}, err
	}
	result.Adapters = inventory.Adapters
	result.checkRoutes(lock)
	for _, report := range result.Adapters {
		for _, route := range report.Routes {
			if route.Name == lock.Harnesses[report.Provider].Route && route.Registration != nil && route.Registration.State != "enabled" {
				result.add("registration-required", registrationSeverity(*route.Registration), route.Name+": "+route.Registration.State+"; use the listed native registration commands; static user rules remain active")
			}
		}
		if !report.Detected {
			continue
		}
		effort, err := inspectEffort(report, request.WorkDir)
		if err != nil {
			result.add("effort-unavailable", "warning", err.Error())
		} else {
			if digest := lock.EffortMapping.SHA256(report.Provider); digest != "" {
				effort.MappingSHA256 = digest
				effort.MappingSource = "builtin"
				if effort.Tier == "" {
					effort.MappingSource = "unmapped"
				}
				if tier, ok := lock.EffortMapping[report.Provider][strings.ToLower(strings.TrimSpace(effort.Raw))]; ok {
					effort.Tier, effort.MappingSource = tier, "user"
					approx := true
					effort.Approximate = &approx
				}
			}
			result.Effort = append(result.Effort, effort)
			if effort.Tier == "" {
				result.add("effort-unknown", "warning", report.Provider+": effort is unavailable; conditional effort rules will not match")
			}
		}
		if report.Provider == "codex" && runtime.GOOS != "windows" {
			if err := checkLoginShell(ctx, request.Shell, request.WorkDir); err != nil {
				result.add("codex-login-shell", "error", err.Error())
			}
		}
	}
	root := filepath.Join(home, "state")
	if err := checkStateRoot(root); err != nil {
		result.add("state-invalid", "error", err.Error())
	} else {
		result.TopRules, result.LegacyStats, err = readRuleCosts(ctx, root, time.Now())
		if err != nil {
			result.add("stats-invalid", "warning", err.Error())
		}
		for _, rule := range result.TopRules {
			if rule.RepeatedBody {
				result.add("repeat-unchanged", "warning", rule.Provider+" "+rule.Rule+": repeated output has the same body hash; check repeat")
			}
		}
		confirmed := true
		if request.ResetState && request.Confirm != nil {
			if confirmed, err = request.Confirm(root); err != nil {
				return DoctorResult{}, err
			}
		}
		if !confirmed {
			// A declined reset changes nothing, expired sessions included.
			result.Diagnostics = append(result.Diagnostics, declinedDiagnostic("doctor"))
		} else if result.ExpiredSessions, err = maintainState(root, request.ResetState); err != nil {
			result.add("state-cleanup", "error", err.Error())
		} else {
			result.StateReset = request.ResetState
		}
	}
	if err := ctx.Err(); err != nil {
		return DoctorResult{}, err
	}
	if request.Live != nil {
		live, err := s.Probe(ctx, *request.Live)
		for _, finding := range live.Diagnostics {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: finding.Code, Severity: finding.Severity, Message: finding.Message, Provider: finding.Harness})
		}
		if err != nil && len(live.Diagnostics) == 0 {
			result.add("probe-failed", "error", err.Error())
		}
		live.Diagnostics = nil
		result.Live = &live
	}
	sortDiagnostics(result.Diagnostics)
	result.Healthy = !hasErrors(result.Diagnostics)
	return result, nil
}

func (result *DoctorResult) add(code, severity, message string) {
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: code, Severity: severity, Message: message})
}

func (result *DoctorResult) checkRoutes(lock lockDocument) {
	seen := make(map[string]bool)
	for _, report := range result.Adapters {
		seen[report.Provider] = true
		if report.Error != "" {
			result.add("adapter-detect", "error", report.Provider+": "+report.Error)
		}
		locked, selected := lock.Harnesses[report.Provider]
		if selected && report.DetectedConfigDir != "" && report.DetectedConfigDir != report.ConfigDir {
			result.add("config-root-changed", "warning", report.Provider+": checking installed root "+report.ConfigDir+"; detection now reports "+report.DetectedConfigDir)
		}
		if selected && !report.Detected {
			result.add("adapter-missing", "error", report.Provider+" is installed in lock but is not detected")
		}
		found := false
		for _, route := range report.Routes {
			if route.Error != "" {
				result.add("route-status", "error", route.Name+": "+route.Error)
			}
			if !selected || route.Name != locked.Route {
				continue
			}
			found = true
			if !route.Available || !route.Installed {
				result.add("route-missing", "error", route.Name+": "+route.Detail)
			} else if !route.Trusted {
				result.add("route-trust-unverified", "warning", route.Name+": "+route.TrustSummary+"; external confirmation cannot be inferred from generated files")
			}
		}
		if selected && !found {
			result.add("route-unknown", "error", report.Provider+": locked route "+locked.Route+" is not registered")
		}
		if locked.FailClosed {
			result.add("fail-closed", "warning", report.Provider+": fail_closed is enabled; invalid index blocks tool calls")
		}
	}
	for provider := range lock.Harnesses {
		if !seen[provider] {
			result.add("adapter-unknown", "error", provider+" is installed in lock but is not registered")
		}
	}
}

func (s *Service) checkIndex(ctx context.Context, home string, lock lockDocument) error {
	path := filepath.Join(home, "build", "index.json")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) && len(lock.Packages) == 0 && len(lock.Harnesses) == 0 {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read index: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	if err != nil {
		return err
	}
	store, _ := state.New(filepath.Join(home, "state"))
	if _, err := engine.LoadIndexBytes(data, store); err != nil {
		return err
	}
	packages, err := s.loadInstalledPackages(ctx, home, lock)
	if err != nil {
		return err
	}
	adapters, err := s.lockedAdapters(ctx, home, lock)
	if err != nil {
		return err
	}
	for i := range adapters {
		if facts, ok := adapters[i].definition.Facts.(adapter.StaticLayoutFacts); ok {
			adapters[i].definition.Facts = facts.WithStaticLayout(lock.Harnesses[string(adapters[i].provider)].StaticLayout)
		}
	}
	expected, err := s.compileInstalled(ctx, packages, adapters, "", lock.EffortMapping)
	if err != nil {
		return err
	}
	// Re-encoding permits whitespace changes while detecting stale settings,
	// body edits and lock changes, beyond merely comparing package headers.
	if !equalJSON(data, expected.indexBytes) {
		return fmt.Errorf("index differs from the locked packages/settings; run sync")
	}
	for _, target := range adapters {
		hooks := expected.hooksForProvider(target.provider)
		if lock.Harnesses[string(target.provider)].FailClosed {
			for i := range hooks {
				if hooks[i].Command != "" {
					continue
				}
				hooks[i].Extra = cloneExtra(hooks[i].Extra)
				if hooks[i].Extra == nil {
					hooks[i].Extra = map[string]any{}
				}
				hooks[i].Extra["fail_closed"] = true
			}
		}
		planned, err := target.route.Plan(ctx, hooks, staticBlocksForProvider(expected.staticBodies, target.provider))
		if err != nil {
			return &routeDriftError{target.route.Name() + ": " + err.Error()}
		}
		for _, file := range planned.Files {
			if file.Remove || !bytes.Equal(file.Before, file.After) {
				return &routeDriftError{target.route.Name() + ": generated route/static instructions differ; run sync"}
			}
		}
	}
	return nil
}

type routeDriftError struct{ message string }

func (err *routeDriftError) Error() string { return err.message }

func equalJSON(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}
