package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/speakeasy-api/agenthooks/install"
	"gopkg.in/yaml.v3"
)

// InstallRequest contains all non-interactive install choices. A caller must
// explicitly confirm the capability report with Yes before any mutation.
type InstallRequest struct {
	Source       string
	Home         string
	Harnesses    []string
	Routes       map[string]string
	Settings     map[string]string
	Link         bool
	Yes          bool
	AllowReplace bool
	FailClosed   []string
}

// InstallResult is the stable summary written after an install transaction.
type InstallResult struct {
	Package     string       `json:"package"`
	Version     string       `json:"version"`
	Changed     bool         `json:"changed"`
	Link        bool         `json:"link"`
	Replace     bool         `json:"replace"`
	Routes      []string     `json:"routes"`
	Reports     []PlanReport `json:"reports"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type installPackage struct {
	lock     lockPackage
	pkg      *spec.Package
	resolved *spec.ResolvedPackage
	settings map[string]string
	report   map[adapter.ProviderID]plan.Report
	tokens   map[adapter.ProviderID]tokens.Report
}

type installAdapter struct {
	provider     adapter.ProviderID
	definition   adapter.Definition
	installation adapter.Installation
	route        adapter.Route
	hooks        []adapter.HookSpec
}

type installCompilation struct {
	index        compile.Index
	indexBytes   []byte
	staticBodies []compile.StaticBody
	reports      []PlanReport
	diagnostics  []Diagnostic
}

// Install validates, plans, compiles, and applies one package as a single
// recoverable transaction across package storage, state files, and provider
// configuration changesets.
func (s *Service) Install(ctx context.Context, request InstallRequest) (InstallResult, error) {
	if err := checkContext(ctx); err != nil {
		return InstallResult{}, err
	}
	if !request.Yes {
		return InstallResult{}, &UsageError{Message: "installation requires capability confirmation; pass --yes in non-interactive mode"}
	}
	if strings.TrimSpace(request.Source) == "" {
		return InstallResult{}, &UsageError{Message: "package source is required"}
	}
	home, err := resolveStyleHome(request.Home)
	if err != nil {
		return InstallResult{}, err
	}
	lock, err := loadLock(home)
	if err != nil {
		return InstallResult{}, err
	}
	files, single, sourcePath, err := snapshotPackage(request.Source)
	if err != nil {
		return InstallResult{}, err
	}
	singleName := ""
	if single {
		singleName = filepath.Base(sourcePath)
	}
	original, err := spec.LoadSnapshot(sourcePath, files, singleName)
	if err != nil {
		return InstallResult{}, err
	}
	name, _ := original.Metadata["name"].(string)
	version, _ := original.Metadata["version"].(string)
	if name == "" || version == "" {
		return InstallResult{}, &UsageError{Message: "package name and version are required"}
	}
	fileHashes, _ := hashPackageFiles(files)
	if err := validateRuntimeRequirement(original.Metadata["requires"]); err != nil {
		return InstallResult{}, err
	}
	settings, settingNames, err := collectSettings(original, request.Settings)
	if err != nil {
		return InstallResult{}, err
	}
	var existingLock *lockPackage
	if existing, found := findLockedPackage(lock, name); found {
		stored, settingErr := readSettingValues(home, name)
		if existing.Version == version && existing.Link == request.Link && (!request.Link || existing.Source.Path == sourcePath) && reflectStringMap(existing.Files, fileHashes) && settingErr == nil && reflectStringMap(stored, settings) {
			existingLock = &existing
		} else {
			return InstallResult{}, &ConflictError{Message: fmt.Sprintf("package %q is already installed; use sync/trust or uninstall before replacing it", name)}
		}
	}
	storagePath := filepath.Join(home, "packages", name)
	if single {
		storagePath += ".style.md"
	}
	if existingLock == nil {
		if _, err := os.Lstat(storagePath); err == nil {
			return InstallResult{}, &ConflictError{Message: fmt.Sprintf("package storage already exists outside the lock: %s", storagePath)}
		} else if !errors.Is(err, os.ErrNotExist) {
			return InstallResult{}, err
		}
	}
	preview, err := spec.Merge(original)
	if err != nil {
		return InstallResult{}, err
	}
	replace := packageUsesReplace(preview)
	if replace && existingLock == nil && !request.AllowReplace {
		return InstallResult{}, &UsageError{Message: "package uses mode: replace; pass --allow-replace to confirm"}
	}
	if replace {
		for _, installed := range lock.Packages {
			if installed.Replace && installed.Name != name {
				return InstallResult{}, &ConflictError{Message: fmt.Sprintf("package %q already uses replace mode", installed.Name)}
			}
		}
	}

	parsed := original
	parsed.Files = cloneFileMap(files)
	diagnostics := spec.Validate(parsed)
	if hasSpecErrors(diagnostics) {
		return InstallResult{}, &ValidationError{Diagnostics: convertDiagnostics(diagnostics)}
	}
	resolved, err := spec.Merge(parsed)
	if err != nil {
		return InstallResult{}, err
	}
	resolved, err = resolvedWithSettings(resolved, settings)
	if err != nil {
		return InstallResult{}, err
	}

	adapters, err := s.installAdapters(ctx, request, lock)
	if err != nil {
		return InstallResult{}, err
	}
	newLock := lockPackage{
		Name: name, Version: version, Seq: nextSequence(lock),
		Source: lockSource{Kind: sourceKind(single), Path: sourcePath, Link: request.Link},
		Files:  fileHashes, Settings: settingNames, Replace: replace, Link: request.Link,
	}
	packages, err := s.loadInstalledPackages(ctx, home, lock)
	if err != nil {
		return InstallResult{}, err
	}
	if existingLock == nil {
		packages = append(packages, installPackage{lock: newLock, pkg: parsed, resolved: resolved, settings: settings,
			report: make(map[adapter.ProviderID]plan.Report), tokens: make(map[adapter.ProviderID]tokens.Report)})
	} else {
		newLock = *existingLock
	}

	result := InstallResult{Package: name, Version: version, Link: request.Link, Replace: replace}
	compilation, err := s.compileInstalled(ctx, packages, adapters, name)
	if err != nil {
		return InstallResult{}, err
	}
	result.Reports, result.Diagnostics = compilation.reports, compilation.diagnostics
	indexBytes := compilation.indexBytes
	updatedLock := lockDocument{Format: lockFormat, Packages: append([]lockPackage(nil), lock.Packages...)}
	if existingLock == nil {
		updatedLock.Packages = append(updatedLock.Packages, newLock)
	}
	plannedChanges, harnesses, routes, err := planInstallRoutes(ctx, home, adapters, compilation, lock, request.FailClosed)
	if err != nil {
		return InstallResult{}, err
	}
	updatedLock.Harnesses, result.Routes = harnesses, routes
	for _, item := range packages {
		approval, err := approvalBytes(compilation, item.lock.Name, item.pkg.Files)
		if err != nil {
			return InstallResult{}, err
		}
		for i := range updatedLock.Packages {
			if updatedLock.Packages[i].Name == item.lock.Name {
				updatedLock.Packages[i].Approval = hashBytes(approval)
			}
		}
		approvalChanges, err := planPrivateFile(approvalPath(home, item.lock.Name), approval)
		if err != nil {
			return InstallResult{}, err
		}
		plannedChanges = append(plannedChanges, approvalChanges...)
	}
	lockBytes, err := yaml.Marshal(updatedLock)
	if err != nil {
		return InstallResult{}, err
	}
	plannedChanges = append(plannedChanges, plannedFile(home, "style.lock.yaml", lockBytes, 0o600)...)
	plannedChanges = append(plannedChanges, plannedFile(filepath.Join(home, "build"), "index.json", indexBytes, 0o600)...)
	if len(settings) > 0 {
		settingsBytes, err := yaml.Marshal(settings)
		if err != nil {
			return InstallResult{}, err
		}
		plannedChanges = append(plannedChanges, plannedFile(filepath.Join(home, "settings"), name+".yaml", settingsBytes, 0o600)...)
	}
	plannedChanges, err = uniquePlannedFiles(plannedChanges)
	if err != nil {
		return InstallResult{}, err
	}
	if len(plannedChanges) == 0 {
		return result, nil
	}
	var staged *packageSwap
	stagedCommitted := false
	defer func() {
		if staged != nil && !stagedCommitted {
			staged.rollback()
		}
	}()
	if !request.Link && existingLock == nil {
		staged, err = stagePackageFiles(home, name, single, files)
		if err != nil {
			return InstallResult{}, fmt.Errorf("stage package: %w", err)
		}
		if err := staged.apply(); err != nil {
			return InstallResult{}, fmt.Errorf("install package: %w", err)
		}
	}
	if err := install.ApplyFiles(ctx, plannedChanges); err != nil {
		return InstallResult{}, fmt.Errorf("apply install changes: %w", err)
	}
	if staged != nil {
		staged.commit()
		stagedCommitted = true
	}
	result.Changed = true
	sort.Strings(result.Routes)
	sortDiagnostics(result.Diagnostics)
	return result, nil
}

// ValidationError carries stable command diagnostics without exposing source text.
type ValidationError struct{ Diagnostics []Diagnostic }

func (err *ValidationError) Error() string { return "package validation failed" }

// BlockedError identifies a supported plan that policy prevents installing.
type BlockedError struct{ Message string }

func (err *BlockedError) Error() string { return err.Message }

func (s *Service) compileInstalled(ctx context.Context, packages []installPackage, adapters []installAdapter, reportPackage string) (installCompilation, error) {
	result := installCompilation{diagnostics: make([]Diagnostic, 0)}
	compileInputs := make([]compile.PackageInput, 0, len(packages))
	for _, item := range packages {
		compileInputs = append(compileInputs, compile.PackageInput{Package: item.resolved, Sequence: item.lock.Seq,
			Lock: packageLockHash(item.lock.Files), Link: item.lock.Link, Settings: item.settings})
	}
	allTokenReports := make(map[adapter.ProviderID][]tokens.Report)
	for _, target := range adapters {
		if err := checkContext(ctx); err != nil {
			return installCompilation{}, err
		}
		planner, err := adapter.BuildPlanAdapter(target.provider, target.definition.Facts)
		if err != nil {
			return installCompilation{}, err
		}
		for index := range packages {
			planned, err := plan.Build(packages[index].resolved, planner)
			if err != nil {
				return installCompilation{}, fmt.Errorf("plan %s/%s: %w", packages[index].lock.Name, target.provider, err)
			}
			if planned.Blocked {
				return installCompilation{}, &BlockedError{Message: fmt.Sprintf("plan for %s is blocked on %s", packages[index].lock.Name, target.provider)}
			}
			budget, err := tokens.AnalyzeWithLayout(packages[index].resolved, planned, layoutFor(target.provider), spec.Validate(packages[index].pkg)...)
			if err != nil {
				return installCompilation{}, fmt.Errorf("token analysis %s/%s: %w", packages[index].lock.Name, target.provider, err)
			}
			for _, finding := range budget.Diagnostics {
				if finding.Severity == spec.SeverityError {
					return installCompilation{}, &BlockedError{Message: fmt.Sprintf("%s %s blocks install: %s", finding.Code, finding.Event, finding.Message)}
				}
				result.diagnostics = append(result.diagnostics, Diagnostic{Code: finding.Code, Rule: finding.RuleID, Provider: string(target.provider), Severity: finding.Severity, Message: finding.Message})
			}
			packages[index].report[target.provider] = planned
			packages[index].tokens[target.provider] = budget
			allTokenReports[target.provider] = append(allTokenReports[target.provider], budget)
			compileInputs = appendCompileInput(compileInputs, packages[index], target.provider)
			if packages[index].lock.Name == reportPackage {
				result.reports = append(result.reports, installPlanReport(target, planned, budget, packages[index].resolved))
			}
		}
	}
	for _, target := range adapters {
		findings, err := tokens.AnalyzeInstalled(allTokenReports[target.provider], target.definition.Limits.Input)
		if err != nil {
			return installCompilation{}, err
		}
		if len(findings) > 0 {
			finding := findings[0]
			return installCompilation{}, &BlockedError{Message: fmt.Sprintf("%s %s blocks install: %s", finding.Code, finding.Event, finding.Message)}
		}
	}
	compiled, err := compile.Build(compile.Request{StyleVersion: styleRuntimeVersion, Packages: compileInputs})
	if err != nil {
		return installCompilation{}, err
	}
	result.index = compiled.Index
	result.staticBodies = compiled.StaticBodies
	result.indexBytes, err = compile.Encode(compiled.Index)
	if err != nil {
		return installCompilation{}, err
	}
	sortDiagnostics(result.diagnostics)
	return result, nil
}

func hasSpecErrors(values []spec.Diagnostic) bool {
	for _, value := range values {
		if value.Severity == spec.SeverityError {
			return true
		}
	}
	return false
}

func cloneFileMap(files map[string][]byte) map[string][]byte {
	result := make(map[string][]byte, len(files))
	for path, data := range files {
		result[filepath.ToSlash(path)] = append([]byte(nil), data...)
	}
	return result
}

func findLockedPackage(lock lockDocument, name string) (lockPackage, bool) {
	for _, item := range lock.Packages {
		if item.Name == name {
			return item, true
		}
	}
	return lockPackage{}, false
}

func reflectStringMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func readSettingValues(home, name string) (map[string]string, error) {
	path := filepath.Join(home, "settings", name+".yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func collectSettings(pkg *spec.Package, supplied map[string]string) (map[string]string, []string, error) {
	result := make(map[string]string)
	declared := make(map[string]bool)
	var names []string
	settings, _ := pkg.Metadata["settings"].([]any)
	for _, raw := range settings {
		fields, _ := raw.(map[string]any)
		name, _ := fields["name"].(string)
		if name == "" {
			continue
		}
		declared[name] = true
		names = append(names, name)
		if value, ok := supplied[name]; ok {
			result[name] = value
			continue
		}
		if value, ok := fields["default"].(string); ok {
			result[name] = value
			continue
		}
		return nil, nil, &UsageError{Message: fmt.Sprintf("setting %q requires --set %s=value", name, name)}
	}
	for name := range supplied {
		if !declared[name] {
			return nil, nil, &UsageError{Message: fmt.Sprintf("unknown setting %q", name)}
		}
	}
	sort.Strings(names)
	return result, names, nil
}

func resolvedWithSettings(pkg *spec.ResolvedPackage, values map[string]string) (*spec.ResolvedPackage, error) {
	copyPackage := *pkg.Package
	copyPackage.Metadata = make(map[string]any, len(pkg.Package.Metadata))
	for key, value := range pkg.Package.Metadata {
		copyPackage.Metadata[key] = value
	}
	settings, _ := copyPackage.Metadata["settings"].([]any)
	updated := make([]any, 0, len(settings))
	for _, raw := range settings {
		fields, _ := raw.(map[string]any)
		copyFields := make(map[string]any, len(fields))
		for key, value := range fields {
			copyFields[key] = value
		}
		name, _ := copyFields["name"].(string)
		if value, ok := values[name]; ok {
			copyFields["default"] = value
		}
		updated = append(updated, copyFields)
	}
	copyPackage.Metadata["settings"] = updated
	copyPackage.Files = pkg.Package.Files
	copyResolved := *pkg
	copyResolved.Package = &copyPackage
	return &copyResolved, nil
}

func packageUsesReplace(pkg *spec.ResolvedPackage) bool {
	if pkg == nil {
		return false
	}
	for _, rule := range pkg.Rules {
		if rule.Effective["mode"] == "replace" {
			return true
		}
	}
	return false
}

func validateRuntimeRequirement(value any) error {
	requires, _ := value.(map[string]any)
	constraint, _ := requires["runtime"].(string)
	if constraint != "" && !versionSatisfies(styleRuntimeVersion, constraint) {
		return &UsageError{Message: fmt.Sprintf("STYLE runtime %s does not satisfy requires.runtime %s", styleRuntimeVersion, constraint)}
	}
	return nil
}

var constraintPattern = regexp.MustCompile(`^(>=|<=|>|<|=|\^|~)?\s*(\d+)\.(\d+)\.(\d+).*$`)

func versionSatisfies(actual, constraint string) bool {
	match := constraintPattern.FindStringSubmatch(strings.TrimSpace(constraint))
	if len(match) != 5 {
		return false
	}
	parse := func(value string) [3]int {
		var result [3]int
		fmt.Sscanf(value, "%d.%d.%d", &result[0], &result[1], &result[2])
		return result
	}
	a, b := parse(actual), [3]int{}
	fmt.Sscanf(match[2]+"."+match[3]+"."+match[4], "%d.%d.%d", &b[0], &b[1], &b[2])
	cmp := compareVersion(a, b)
	switch match[1] {
	case ">=":
		return cmp >= 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case "<":
		return cmp < 0
	case "^":
		return cmp >= 0 && a[0] == b[0]
	case "~":
		return cmp >= 0 && a[0] == b[0] && a[1] == b[1]
	default:
		return cmp == 0
	}
}

func compareVersion(a, b [3]int) int {
	for index := range a {
		if a[index] < b[index] {
			return -1
		}
		if a[index] > b[index] {
			return 1
		}
	}
	return 0
}

func sourceKind(single bool) string {
	if single {
		return "file"
	}
	return "dir"
}

func nextSequence(lock lockDocument) int {
	next := 1
	for _, item := range lock.Packages {
		if item.Seq >= next {
			next = item.Seq + 1
		}
	}
	return next
}

func (s *Service) loadInstalledPackages(ctx context.Context, home string, lock lockDocument) ([]installPackage, error) {
	result := make([]installPackage, 0, len(lock.Packages))
	for _, item := range lock.Packages {
		if err := checkContext(ctx); err != nil {
			return nil, err
		}
		files, err := installedSnapshot(home, item)
		if err != nil {
			return nil, err
		}
		currentHashes, _ := hashPackageFiles(files)
		if !reflectStringMap(currentHashes, item.Files) {
			return nil, &ConflictError{Message: fmt.Sprintf("installed package %q changed; run sync/trust before installing another package", item.Name)}
		}
		parsed, err := parseInstalledSnapshot(ctx, home, item, files)
		if err != nil {
			return nil, err
		}
		result = append(result, parsed)
	}
	return result, nil
}

func (s *Service) installAdapters(ctx context.Context, request InstallRequest, lock lockDocument) ([]installAdapter, error) {
	selected := make(map[adapter.ProviderID]bool)
	requested := make(map[adapter.ProviderID]bool)
	for _, raw := range request.Harnesses {
		for _, value := range strings.Split(raw, ",") {
			value = strings.ToLower(strings.TrimSpace(value))
			if value == "auto" {
				continue
			}
			if value == "claude" {
				value = "claude-code"
			}
			provider := adapter.ProviderID(value)
			if _, ok := s.definitions[provider]; !ok {
				return nil, &UsageError{Message: fmt.Sprintf("unknown harness %q", value)}
			}
			requested[provider] = true
		}
	}
	for raw := range request.Routes {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "claude" {
			name = "claude-code"
		}
		provider := adapter.ProviderID(name)
		if _, ok := s.definitions[provider]; !ok {
			return nil, &UsageError{Message: fmt.Sprintf("unknown harness %q in --route", raw)}
		}
		requested[provider] = true
	}
	for _, raw := range request.FailClosed {
		for _, value := range strings.Split(raw, ",") {
			value = strings.ToLower(strings.TrimSpace(value))
			if value == "claude" {
				value = "claude-code"
			}
			if value != "claude-code" {
				return nil, &UsageError{Message: "--fail-closed is supported only by claude-code"}
			}
			provider := adapter.ProviderID(value)
			if _, ok := s.definitions[provider]; !ok {
				return nil, &UsageError{Message: "--fail-closed requires the claude-code adapter"}
			}
			requested[provider] = true
		}
	}
	for provider := range requested {
		selected[provider] = true
	}
	if len(request.Harnesses) == 0 || hasAuto(request.Harnesses) {
		for provider := range s.definitions {
			definition := s.definitions[provider]
			_, detected, err := definition.Detect(ctx)
			if err != nil {
				return nil, err
			}
			if detected {
				selected[provider] = true
			}
		}
	}
	for provider := range lock.Harnesses {
		selected[adapter.ProviderID(provider)] = true
	}
	providers := make([]adapter.ProviderID, 0, len(selected))
	for provider := range selected {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool {
		left, right := providerOrder(providers[i]), providerOrder(providers[j])
		if left != right {
			return left < right
		}
		return providers[i] < providers[j]
	})
	result := make([]installAdapter, 0, len(providers))
	for _, provider := range providers {
		definition := s.definitions[provider]
		installation, detected, err := definition.Detect(ctx)
		if err != nil {
			return nil, err
		}
		if !detected {
			if requested[provider] || lock.Harnesses[string(provider)].Route != "" {
				return nil, &UsageError{Message: fmt.Sprintf("harness %s was selected but is not detected", provider)}
			}
			continue
		}
		if previous := lock.Harnesses[string(provider)]; previous.ConfigDir != "" {
			installation.ConfigDir = previous.ConfigDir
		}
		var chosen adapter.Route
		previous := lock.Harnesses[string(provider)].Route
		requestedRoute := routeRequest(request.Routes, provider)
		if requestedRoute != "" {
			chosen = routeByName(definition, requestedRoute)
			if chosen == nil {
				return nil, &UsageError{Message: fmt.Sprintf("unknown route %q for %s", requestedRoute, provider)}
			}
		} else if previous != "" {
			chosen = routeByName(definition, previous)
		}
		if chosen != nil {
			available, reason, err := chosen.Available(ctx, installation)
			if err != nil {
				return nil, err
			}
			if !available {
				return nil, &UsageError{Message: fmt.Sprintf("route %s unavailable: %s", chosen.Name(), reason)}
			}
		} else {
			for _, candidate := range definition.Routes {
				available, _, err := candidate.Available(ctx, installation)
				if err != nil {
					return nil, err
				}
				if available {
					chosen = candidate
					break
				}
			}
		}
		if chosen == nil {
			return nil, &UsageError{Message: fmt.Sprintf("no available route for %s", provider)}
		}
		home, err := resolveStyleHome(request.Home)
		if err != nil {
			return nil, err
		}
		chosen = bindRoute(chosen, installation, home)
		result = append(result, installAdapter{provider: provider, definition: definition, installation: installation, route: chosen})
	}
	return result, nil
}

func hasAuto(values []string) bool {
	for _, raw := range values {
		for _, value := range strings.Split(raw, ",") {
			if strings.EqualFold(strings.TrimSpace(value), "auto") {
				return true
			}
		}
	}
	return false
}

func providerOrder(provider adapter.ProviderID) int {
	if provider == adapter.ProviderID("codex") {
		return 0
	}
	if provider == adapter.ProviderID("claude-code") {
		return 1
	}
	return 2
}

func routeRequest(routes map[string]string, provider adapter.ProviderID) string {
	if value := routes[string(provider)]; value != "" {
		return value
	}
	if provider == adapter.ProviderID("claude-code") {
		return routes["claude"]
	}
	return ""
}

func routeByName(definition adapter.Definition, name string) adapter.Route {
	for _, route := range definition.Routes {
		if route.Name() == name {
			return route
		}
	}
	return nil
}

func appendCompileInput(result []compile.PackageInput, pkg installPackage, provider adapter.ProviderID) []compile.PackageInput {
	input := compile.PackageInput{Package: pkg.resolved, Sequence: pkg.lock.Seq, Lock: packageLockHash(pkg.lock.Files), Link: pkg.lock.Link, Settings: pkg.settings}
	for target, planned := range pkg.report {
		if target == provider {
			input.Plans = []plan.Report{planned}
			break
		}
	}
	for index := range result {
		if result[index].Package.Package.Metadata["name"] == pkg.lock.Name {
			result[index].Plans = append(result[index].Plans, input.Plans...)
			return result
		}
	}
	return append(result, input)
}

func packageLockHash(files map[string]string) string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var buffer bytes.Buffer
	for _, path := range paths {
		buffer.WriteString(path)
		buffer.WriteByte(0)
		buffer.WriteString(files[path])
		buffer.WriteByte('\n')
	}
	return hashBytes(buffer.Bytes())
}

func installPlanReport(target installAdapter, planned plan.Report, budget tokens.Report, pkg *spec.ResolvedPackage) PlanReport {
	cost := estimateSessionCost(pkg, planned, budget, 60, 3)
	return PlanReport{Provider: string(target.provider), Route: target.route.Name(),
		NativeLimit: NativeLimit{Value: target.definition.Limits.Input.Value, Unit: string(target.definition.Limits.Input.Unit),
			Approximate: target.definition.Limits.Approximate, Source: target.definition.Limits.Source, Overflow: string(target.definition.Limits.Overflow)},
		Budget: budgetSummary(pkg), Plan: planned, Tokens: budget, SessionCost: cost}
}

func hooksForProvider(provider adapter.ProviderID, index compile.Index) []adapter.HookSpec {
	if provider == adapter.ProviderID("codex") {
		return codex.Surface()
	}
	if provider != adapter.ProviderID("claude-code") {
		return nil
	}
	var hooks []adapter.HookSpec
	observeModel := false
	for event, entries := range index.Harnesses[string(provider)] {
		for _, entry := range entries {
			observeModel = observeModel || slices.Contains(entry.Needs, "model")
		}
		native := ""
		switch event {
		case plan.SessionStart:
			native = "SessionStart"
		case plan.PromptSubmitted:
			native = "UserPromptSubmit"
		case plan.SkillExplicit:
			native = "UserPromptExpansion"
		case plan.ToolPre:
			native = "PreToolUse"
		case plan.ToolPost:
			native = "PostToolUse"
		case plan.ToolError:
			native = "PostToolUseFailure"
		case plan.SubagentStart:
			native = "SubagentStart"
		}
		if native == "" {
			continue
		}
		matchers := make(map[string]bool)
		if event == plan.ToolPre || event == plan.ToolPost || event == plan.ToolError {
			for _, entry := range entries {
				if entry.Match == nil {
					matchers[""] = true
					continue
				}
				for _, name := range entry.Match.Names {
					matchers[name] = true
				}
				for _, name := range entry.Match.Canonical {
					switch name {
					case "shell":
						matchers["Bash"] = true
					case "file.edit":
						for _, tool := range []string{"Edit", "MultiEdit", "NotebookEdit"} {
							matchers[tool] = true
						}
					case "file.write":
						matchers["Write"] = true
					case "mcp":
						matchers["mcp__.*"] = true
					case "*":
						matchers[""] = true
					}
				}
				if len(entry.Match.MCP) > 0 {
					matchers["mcp__.*"] = true
				}
			}
		} else {
			matchers[""] = true
		}
		for matcher := range matchers {
			if matcher != "" && !strings.HasPrefix(matcher, "mcp__") {
				matcher = regexp.QuoteMeta(matcher)
			}
			if matcher == "mcp__.*" {
				matcher = `mcp__.*`
			}
			if matcher != "" {
				matcher = "^(" + matcher + ")$"
			}
			hooks = append(hooks, adapter.HookSpec{NativeEvent: native, Matcher: matcher, Points: []string{event}})
		}
	}
	if observeModel {
		// The switch itself only updates session state. Model-dependent rules
		// use that state when their next evaluation point runs, so wait for the
		// write instead of racing the next prompt with an async observer.
		hooks = append(hooks, adapter.HookSpec{NativeEvent: "PostModelSwitch", Points: []string{"model.changed"}, Extra: map[string]any{"blocking": true}})
	}
	return claude.Surface(hooks)
}

func staticBlocksForProvider(bodies []compile.StaticBody, provider adapter.ProviderID) []adapter.StaticBlock {
	groups := make(map[string][]string)
	slots := make(map[string]string)
	for _, body := range bodies {
		if body.Harness != string(provider) {
			continue
		}
		owner, _, ok := strings.Cut(body.Rule, "/")
		if !ok {
			continue
		}
		groups[owner] = append(groups[owner], body.Body)
		slots[owner] = body.Slot
	}
	owners := make([]string, 0, len(groups))
	for owner := range groups {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	result := make([]adapter.StaticBlock, 0, len(owners))
	for _, owner := range owners {
		result = append(result, adapter.StaticBlock{Owner: adapter.Owner(owner), Slot: slots[owner], Body: strings.Join(groups[owner], "\n\n")})
	}
	return result
}

func containsProvider(values []string, provider adapter.ProviderID) bool {
	for _, raw := range values {
		for _, value := range strings.Split(raw, ",") {
			value = strings.ToLower(strings.TrimSpace(value))
			if value == string(provider) || provider == adapter.ProviderID("claude-code") && value == "claude" {
				return true
			}
		}
	}
	return false
}

func cloneExtra(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func hashSurface(hooks []adapter.HookSpec) string {
	data, _ := json.Marshal(hooks)
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func hashChangeset(changes adapter.Changeset) string {
	files := append([]adapter.FileChange(nil), changes.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	var buffer bytes.Buffer
	for _, file := range files {
		buffer.WriteString(file.Path)
		buffer.WriteByte(0)
		buffer.Write(file.After)
		buffer.WriteByte('\n')
	}
	return hashBytes(buffer.Bytes())
}

func trustState(info adapter.TrustInfo) string {
	switch info.Kind {
	case adapter.TrustPreseed:
		return "preseeded"
	case adapter.TrustWorkspace:
		return "workspace"
	case adapter.TrustUserConfirmation:
		return "user"
	default:
		return "managed"
	}
}

func routePlannedFiles(changes adapter.Changeset) []install.PlannedFile {
	result := make([]install.PlannedFile, 0, len(changes.Files))
	for _, file := range changes.Files {
		result = append(result, install.PlannedFile{Root: file.Root, Path: file.Path, Before: file.Before, After: file.After, Mode: file.Mode, Remove: file.Remove})
	}
	return result
}

func plannedFile(root, name string, after []byte, mode os.FileMode) []install.PlannedFile {
	path := filepath.Join(root, name)
	before, err := readOptionalBytes(path)
	if err != nil || before != nil && bytes.Equal(before, after) {
		return nil
	}
	return []install.PlannedFile{{Path: path, Root: root, Before: before, After: after, Mode: mode}}
}

func uniquePlannedFiles(files []install.PlannedFile) ([]install.PlannedFile, error) {
	byPath := make(map[string]install.PlannedFile, len(files))
	for _, file := range files {
		path := filepath.Clean(file.Path)
		if previous, ok := byPath[path]; ok {
			if previous.Remove != file.Remove || !bytes.Equal(previous.After, file.After) {
				return nil, fmt.Errorf("multiple install routes plan conflicting changes to %s", path)
			}
			continue
		}
		file.Path = path
		byPath[path] = file
	}
	result := make([]install.PlannedFile, 0, len(byPath))
	for _, file := range byPath {
		result = append(result, file)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}
