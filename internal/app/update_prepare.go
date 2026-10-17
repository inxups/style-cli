package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/inxups/style-cli/internal/source"
	"github.com/speakeasy-api/agenthooks/install"
	"gopkg.in/yaml.v3"
)

type updateCandidate struct {
	old            lockPackage
	item           installPackage
	baseline       approvedSnapshot
	approved       bool
	selected       bool
	storage        bool
	acquired       *SourceReport
	settingsBefore map[string]string
}

type preparedUpdate struct {
	home       string
	inputs     *managementInputs
	candidates []updateCandidate
	files      []install.PlannedFile
	result     UpdateResult
}

func (p preparedUpdate) changed() bool {
	if len(p.files) > 0 {
		return true
	}
	for _, candidate := range p.candidates {
		if candidate.storage {
			return true
		}
	}
	return false
}

func (s *Service) prepareUpdate(ctx context.Context, request UpdateRequest) (p preparedUpdate, err error) {
	p.result = newUpdateResult()
	if err := checkContext(ctx); err != nil {
		return p, err
	}
	if (request.Name == "") == !request.All || request.All && request.Ref != "" {
		return p, updateFailure("source-invalid", "update requires exactly one package name or --all; --ref requires a single Git package", nil)
	}
	if request.Ref != "" && !source.ValidRef(request.Ref) {
		return p, updateFailure("source-invalid", "--ref requires a full branch/tag ref or commit OID", nil)
	}
	p.home, err = s.resolveStyleHome(request.Home)
	if err != nil {
		return p, updateFailure("source-invalid", "cannot resolve STYLE home", err)
	}
	p.inputs, err = s.captureManagement(p.home)
	if err != nil {
		return p, updateFailure("installation-changed", "cannot capture installation inputs safely", err)
	}
	if err := checkUpdatePrivateInputs(p.home, p.inputs); err != nil {
		return p, err
	}
	lock, err := s.loadLock(p.home)
	if err != nil {
		return p, updateFailure("lock-invalid", "cannot read a valid installation lock", err)
	}
	if err := validateUpdateLock(lock); err != nil {
		return p, err
	}
	ordered := append([]lockPackage(nil), lock.Packages...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Seq < ordered[j].Seq })
	changeIndex := map[string]int{}
	for _, item := range ordered {
		if request.All || item.Name == request.Name {
			if request.Ref != "" && item.Source.Git == nil {
				return p, updateFailure("source-invalid", "--ref requires a single Git package", nil)
			}
			changeIndex[item.Name] = len(p.result.Changes)
			p.result.Selected = append(p.result.Selected, item.Name)
			p.result.Changes = append(p.result.Changes, UpdateChange{Package: item.Name, Sequence: item.Seq,
				Before: updateRevision(item), SettingsAdded: []string{}, SettingsRemoved: []string{}})
		}
	}
	if !request.All && len(p.result.Selected) == 0 {
		return p, updateFailure("source-invalid", "the selected package is not installed", nil)
	}
	if len(ordered) == 0 {
		return p, nil
	}
	if err := p.inputs.links(lock); err != nil {
		return p, updateFailure("installation-changed", "cannot capture linked sources safely", err)
	}
	adapters, err := s.lockedAdapters(ctx, p.home, lock)
	if err != nil {
		return p, updateFailure("update-binding-changed", "a locked installation route is unavailable", err)
	}
	if err := p.inputs.routes(adapters); err != nil {
		return p, updateFailure("installation-changed", "cannot capture locked route inputs safely", err)
	}
	for _, item := range ordered {
		_, selected := changeIndex[item.Name]
		candidate, err := s.captureUpdate(ctx, p.home, item, selected, request, p.inputs)
		if err != nil {
			return p, err
		}
		if selected {
			for _, target := range adapters {
				if harnessRequirement(candidate.item.pkg.Metadata, target.provider, target.installation.Version) != nil {
					return p, updatePackageFailure("update-harness-incompatible", item.Name, "the candidate requires a different harness version; the locked binding is retained", nil)
				}
			}
			revision := updateRevision(candidate.item.lock)
			p.result.Changes[changeIndex[item.Name]].After = &revision
			p.result.Changes[changeIndex[item.Name]].Acquisition = candidate.acquired
		}
		p.candidates = append(p.candidates, candidate)
	}
	packages := make([]installPackage, 0, len(p.candidates))
	replace := ""
	for _, candidate := range p.candidates {
		if candidate.item.lock.Replace {
			if replace != "" {
				return p, updateFailure("update-replace-conflict", fmt.Sprintf("packages %q and %q both use mode: replace", replace, candidate.old.Name), nil)
			}
			replace = candidate.old.Name
			if candidate.selected && !candidate.old.Replace && !request.AllowReplace {
				return p, updatePackageFailure("source-invalid", candidate.old.Name, "new mode: replace requires --allow-replace", nil)
			}
		}
		packages = append(packages, candidate.item)
	}
	mapping, err := s.captureEffortMapping(p.home, p.inputs)
	if err != nil {
		return p, err
	}
	compilation, err := s.compileInstalled(ctx, packages, adapters, "", mapping)
	if err != nil {
		return p, err
	}
	p.result.Diagnostics = append(p.result.Diagnostics, compilation.diagnostics...)
	for _, item := range packages {
		for _, target := range adapters {
			if report, ok := item.report[target.provider]; ok {
				p.result.Reports = append(p.result.Reports, UpdatePlanReport{Package: item.lock.Name,
					PlanReport: installPlanReport(target, report, item.tokens[target.provider], item.resolved)})
			}
		}
	}
	p.result.Native = compilation.nativeReports("")
	updated := lock
	updated.Packages = append([]lockPackage(nil), lock.Packages...)
	var unselected []string
	var baselineReports []NativeHookReport
	for _, candidate := range p.candidates {
		var oldNative []NativeHookReport
		if len(candidate.baseline.Files) > 0 {
			oldNative, err = s.baselineNative(candidate.old, candidate.baseline, adapters)
			if err != nil {
				return p, updatePackageFailure("update-baseline-unavailable", candidate.old.Name, "cannot reconstruct approved native entries; explicitly select the package", err)
			}
		}
		baselineReports = append(baselineReports, oldNative...)
		approved, err := approvalBytes(compilation, candidate.old.Name, candidate.item.pkg.Files, candidate.item.lock.Source.Git)
		if err != nil {
			return p, err
		}
		var current approvedSnapshot
		if err := json.Unmarshal(approved, &current); err != nil {
			return p, err
		}
		renderedDiff := diffMaps(candidate.baseline.Rendered, current.Rendered)
		nativeChanged := !slices.Equal(oldNative, current.Native)
		if !candidate.selected {
			// A pre-native approval with no native declarations has a complete
			// compatibility proof. Native-bearing legacy approvals require an
			// explicit selection to establish their ordered representation.
			if candidate.baseline.Native == nil && len(oldNative) > 0 {
				return p, updatePackageFailure("update-baseline-unavailable", candidate.old.Name, "legacy native baseline has no approved order; explicitly select the package or use --all", nil)
			}
			if len(renderedDiff) > 0 || nativeChanged {
				unselected = append(unselected, candidate.old.Name)
			}
			continue
		}
		change := &p.result.Changes[changeIndex[candidate.old.Name]]
		change.BaselineAvailable = candidate.approved
		change.SourceDiff = diffMaps(candidate.baseline.Files, current.Files)
		change.RenderedDiff, change.RenderedChanged = renderedDiff, len(renderedDiff) > 0
		change.NativeBefore, change.NativeAfter = oldNative, current.Native
		change.NativeDiff = nativeDiff(oldNative, current.Native)
		beforeFingerprint := ""
		if len(candidate.baseline.Files) > 0 {
			beforeFingerprint, err = nativeSourceFingerprint(candidate.old, candidate.baseline.Files)
			if err != nil {
				return p, updatePackageFailure("source-invalid", candidate.old.Name, "cannot read the previous native declarations", err)
			}
		}
		afterFingerprint, err := nativeSourceFingerprint(candidate.item.lock, current.Files)
		if err != nil {
			return p, updatePackageFailure("source-invalid", candidate.old.Name, "cannot read candidate native declarations", err)
		}
		change.NativeChanged = nativeChanged || beforeFingerprint != afterFingerprint
		change.SourceChanged = !reflect.DeepEqual(change.Before.Source, change.After.Source) || change.Before.ContentSHA256 != change.After.ContentSHA256 || change.Before.Version != change.After.Version
		change.ApprovalChanged = hashBytes(approved) != candidate.old.Approval
		item := candidate.item.lock
		item.Approval = hashBytes(approved)
		for i := range updated.Packages {
			if updated.Packages[i].Name == item.Name {
				updated.Packages[i] = item
			}
		}
		files, err := planPrivateFile(approvalPath(p.home, item.Name), approved)
		if err != nil {
			return p, updatePackageFailure("update-baseline-unavailable", item.Name, "cannot plan the approved snapshot", err)
		}
		p.files = append(p.files, files...)
		settingsFiles, added, removed, err := planUpdateSettings(p.home, candidate)
		if err != nil {
			return p, err
		}
		change.SettingsAdded, change.SettingsRemoved = added, removed
		p.files = append(p.files, settingsFiles...)
	}
	checks, err := inspectNativeReports(ctx, adapters, baselineReports)
	if err != nil {
		return p, updateFailure("update-binding-changed", "cannot inspect the locked native configuration", err)
	}
	for _, check := range checks {
		if check.Status == "ok" && (check.TrustStatus == "" || check.TrustStatus == "match") {
			continue
		}
		name, _, _ := strings.Cut(check.Rule, "/")
		if i, selected := changeIndex[name]; selected {
			p.result.Changes[i].ConfigurationRepair = true
		} else if !slices.Contains(unselected, name) {
			unselected = append(unselected, name)
		}
	}
	if len(unselected) > 0 {
		sort.Slice(unselected, func(i, j int) bool {
			return packageSequence(lock, unselected[i]) < packageSequence(lock, unselected[j])
		})
		return p, updateFailure("update-unselected-effect", fmt.Sprintf("approved rendered/native effects would change for %q; explicitly use --all or handle those packages first", unselected), nil)
	}
	routeFiles, harnesses, routes, migrations, err := planInstallRoutes(ctx, p.home, adapters, compilation, lock, nil)
	if err != nil {
		return p, updateFailure("update-binding-changed", "cannot plan the locked routes; inspect configuration ownership with doctor", err)
	}
	if len(migrations) > 0 || updateBindingChanged(lock.Harnesses, harnesses) {
		return p, updateFailure("update-binding-changed", "update cannot migrate a locked route or static layout; use the existing management flow first", nil)
	}
	p.result.Routes, updated.Harnesses = routes, harnesses
	p.files = append(p.files, routeFiles...)
	p.files, err = s.projectFiles(p.home, lock, &updated, adapters, compilation, p.files, p.inputs)
	if err != nil {
		return p, err
	}
	// Keep exact lock/settings bytes (including comments and formatting) when
	// their meaning is unchanged. An unchanged update performs no writes.
	if !reflect.DeepEqual(lock, updated) {
		data, err := yaml.Marshal(updated)
		if err != nil {
			return p, err
		}
		files, err := planPrivateFile(filepath.Join(p.home, "style.lock.yaml"), data)
		if err != nil {
			return p, err
		}
		p.files = append(p.files, files...)
	}
	for name, data := range map[string][]byte{"index.json": compilation.indexBytes, "explain.json": compilation.provenanceBytes} {
		files, err := planPrivateFile(filepath.Join(p.home, "build", name), data)
		if err != nil {
			return p, updateFailure("installation-changed", "cannot plan compiled installation files", err)
		}
		p.files = append(p.files, files...)
	}
	p.files, err = uniquePlannedFiles(p.files)
	if err != nil {
		return p, updateFailure("update-binding-changed", "locked routes plan conflicting installation files", err)
	}
	if err := p.inputs.planned(p.files); err != nil {
		return p, err
	}
	if p.changed() {
		p.result.Status, p.result.AwaitingConfirmation = "confirmation-required", true
	}
	return p, nil
}

func (s *Service) captureUpdate(ctx context.Context, home string, item lockPackage, selected bool, request UpdateRequest, inputs *managementInputs) (updateCandidate, error) {
	candidate := updateCandidate{old: item, selected: selected}
	if !selected && item.Approval == "" {
		return candidate, updatePackageFailure("update-baseline-unavailable", item.Name, "an authenticated approved baseline is required; explicitly select the package or use --all", nil)
	}
	baseline, available, err := loadApproval(home, item)
	if err != nil {
		return candidate, updatePackageFailure("update-baseline-unavailable", item.Name, "the approved baseline is missing or invalid; restore it before updating", err)
	}
	candidate.baseline, candidate.approved = baseline, available && item.Approval != ""
	var files map[string][]byte
	if !selected {
		if !candidate.approved {
			return candidate, updatePackageFailure("update-baseline-unavailable", item.Name, "an authenticated approved baseline is required", nil)
		}
		files = cloneFileMap(baseline.Files)
	} else {
		if !item.Link {
			stored, err := installedSnapshot(home, item)
			if err != nil {
				return candidate, updatePackageFailure("update-storage-changed", item.Name, "locked managed storage is missing or unreadable; restore it before updating", err)
			}
			if err := checkGitSnapshot(item, stored); err != nil {
				return candidate, err
			}
			if len(filesChanged(item, stored)) > 0 {
				return candidate, updatePackageFailure("update-storage-changed", item.Name, "managed storage differs from the lock; restore it or use the existing local trust flow before updating", nil)
			}
			if !candidate.approved {
				candidate.baseline.Files = stored
			}
		}
		if item.Source.Git != nil {
			ref := item.Source.Git.Ref
			if request.Ref != "" {
				ref = request.Ref
			}
			snapshot, report, err := s.acquireGit(ctx, source.Request{Source: "git+" + item.Source.Git.URL, Ref: ref, Path: item.Source.Path, Home: home})
			if err != nil {
				return candidate, err
			}
			if snapshot.Kind != item.Source.Kind || snapshot.Path != item.Source.Path || snapshot.Git.URL != item.Source.Git.URL {
				return candidate, updatePackageFailure("source-invalid", item.Name, "source identity or package kind changed; uninstall and install the new source", nil)
			}
			files, item.Source.Git = cloneFileMap(snapshot.Files), &snapshot.Git
			candidate.acquired = report
			item.Files, _ = hashPackageFiles(files)
		} else {
			if err := inputs.add(item.Source.Path); err != nil {
				return candidate, updatePackageFailure("source-invalid", item.Name, "cannot capture the original source", err)
			}
			var single bool
			files, single, _, err = snapshotPackage(item.Source.Path)
			if err != nil {
				return candidate, updatePackageFailure("source-invalid", item.Name, "the original source is missing or invalid", err)
			}
			if single != (item.Source.Kind == "file") {
				return candidate, updatePackageFailure("source-invalid", item.Name, "source kind changed; uninstall and install the new source", nil)
			}
		}
		if item.Link {
			item.LinkTrusted = true
			if !candidate.approved {
				// No authenticated rendered baseline is claimed. The complete
				// current source and native effects will be disclosed.
				candidate.baseline.Files = map[string][]byte{}
			}
		}
	}
	candidate.settingsBefore, err = capturedUpdateSettings(home, item.Name, inputs)
	if err != nil {
		return candidate, updatePackageFailure("source-invalid", item.Name, "cannot read existing private settings", err)
	}
	supplied := map[string]string{}
	for key, value := range candidate.settingsBefore {
		// Newly declared keys use only their package default, even when an
		// unrelated value happens to exist in the private settings file.
		if !selected || slices.Contains(candidate.old.Settings, key) {
			supplied[key] = value
		}
	}
	parsed, err := s.parseSnapshotWithSettings(ctx, home, item, files, func() (map[string]string, error) { return supplied, nil })
	if err != nil {
		var usage *UsageError
		if errors.As(err, &usage) && strings.HasPrefix(usage.Message, "setting ") && strings.Contains(usage.Message, " requires --set ") {
			return candidate, updatePackageFailure("source-invalid", item.Name, usage.Message+"; configure the new package through the existing install/configuration flow before retrying update", err)
		}
		return candidate, updatePackageFailure("source-invalid", item.Name, "candidate name, metadata, runtime requirement, or required settings are invalid; configure required values through the existing management flow", err)
	}
	if !selected {
		if parsed.lock.Version != item.Version || parsed.lock.Replace != item.Replace || !slices.Equal(parsed.lock.Settings, item.Settings) {
			return candidate, updatePackageFailure("update-baseline-unavailable", item.Name, "approved metadata differs from the lock; explicitly select the package", nil)
		}
		parsed.lock = item
	}
	candidate.item = parsed
	candidate.storage = selected && !item.Link && !reflectStringMap(candidate.old.Files, parsed.lock.Files)
	return candidate, nil
}

func planUpdateSettings(home string, candidate updateCandidate) ([]install.PlannedFile, []string, []string, error) {
	stored := candidate.settingsBefore
	oldKeys := append([]string{}, candidate.old.Settings...)
	for name := range stored {
		if !slices.Contains(oldKeys, name) {
			oldKeys = append(oldKeys, name)
		}
	}
	added, removed := []string{}, []string{}
	for _, name := range candidate.item.lock.Settings {
		if !slices.Contains(candidate.old.Settings, name) {
			added = append(added, name)
		}
	}
	for _, name := range oldKeys {
		if !slices.Contains(candidate.item.lock.Settings, name) {
			removed = append(removed, name)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	if reflectStringMap(stored, candidate.item.settings) {
		return nil, added, removed, nil
	}
	path := filepath.Join(home, "settings", candidate.old.Name+".yaml")
	if len(candidate.item.settings) == 0 {
		files, err := plannedRemoval(path)
		return files, added, removed, err
	}
	data, err := yaml.Marshal(candidate.item.settings)
	if err != nil {
		return nil, added, removed, err
	}
	files, err := planPrivateFile(path, data)
	return files, added, removed, err
}

func checkUpdatePrivateInputs(home string, inputs *managementInputs) error {
	// A symlink would capture only the link identity, while the private value
	// read through it could change outside the guarded tree after approval.
	build := inputs.parents[filepath.Join(home, "build")]
	if build != nil && build.Mode()&os.ModeSymlink != 0 {
		return updateFailure("installation-changed", "private build inputs must not use a symbolic link directory; restore regular installation files before updating", nil)
	}
	for _, name := range []string{"style.lock.yaml", "approved", "settings", "build/index.json", "build/explain.json"} {
		for _, entry := range inputs.trees[filepath.Join(home, filepath.FromSlash(name))].files {
			if entry.info.Mode()&os.ModeSymlink != 0 {
				return updateFailure("installation-changed", "private lock, approval, settings and build inputs must not be symbolic links; restore regular installation files before updating", nil)
			}
		}
	}
	return nil
}

func capturedUpdateSettings(home, name string, inputs *managementInputs) (map[string]string, error) {
	values := map[string]string{}
	file, exists := inputs.trees[filepath.Join(home, "settings")].files[name+".yaml"]
	if !exists {
		return values, nil
	}
	if !file.info.Mode().IsRegular() {
		return nil, updateFailure("source-invalid", "private settings must be a regular file", nil)
	}
	if err := yaml.Unmarshal(file.data, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func validateUpdateLock(lock lockDocument) error {
	seen, sequences := map[string]bool{}, map[int]bool{}
	for _, item := range lock.Packages {
		if !lockedNamePattern.MatchString(item.Name) || seen[item.Name] || item.Seq < 1 || sequences[item.Seq] || item.Version == "" || item.Source.Path == "" || item.Source.Kind != "dir" && item.Source.Kind != "file" || item.Link != item.Source.Link || item.Source.Git == nil && !filepath.IsAbs(item.Source.Path) || len(item.Files) == 0 {
			return updateFailure("lock-invalid", "the installation lock has an invalid package identity", nil)
		}
		seen[item.Name], sequences[item.Seq] = true, true
		for name, hash := range item.Files {
			if !lockedHashPattern.MatchString(hash) || filepath.IsAbs(name) || name == "." || name == ".." || strings.HasPrefix(name, "../") || filepath.ToSlash(filepath.Clean(name)) != name {
				return updateFailure("lock-invalid", "the installation lock has invalid package file hashes or paths", nil)
			}
		}
	}
	return nil
}

func updateBindingChanged(before, after map[string]lockHarness) bool {
	if len(before) != len(after) {
		return true
	}
	for provider, old := range before {
		current, ok := after[provider]
		if !ok || old.Route != current.Route || old.Version != current.Version || old.ConfigDir != current.ConfigDir || old.FailClosed != current.FailClosed || old.StaticLayout != current.StaticLayout {
			return true
		}
	}
	return false
}
