package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"

	"github.com/inxups/style-cli/internal/source"
	"gopkg.in/yaml.v3"
)

type preparedFrozen struct {
	transaction preparedUpdate
	result      FrozenResult
}

func (s *Service) prepareFrozen(ctx context.Context, request FrozenRequest) (p preparedFrozen, err error) {
	p.result = newFrozenResult()
	fail := func(message string, cause error) (preparedFrozen, error) { return p, frozenMetadata(message, cause) }
	if err := checkContext(ctx); err != nil {
		return p, err
	}
	tx := &p.transaction
	tx.home, err = s.resolveStyleHome(request.Home)
	if err != nil {
		return p, updateFailure("source-invalid", "cannot resolve STYLE home", err)
	}
	tx.inputs, err = s.captureManagement(tx.home)
	if err != nil {
		return p, updateFailure("installation-changed", "cannot capture frozen installation inputs safely", err)
	}
	if err := checkUpdatePrivateInputs(tx.home, tx.inputs); err != nil {
		return p, err
	}
	// Never exchange storage through a symlink, including a packages parent.
	for _, entry := range tx.inputs.trees[filepath.Join(tx.home, "packages")].files {
		if entry.info.Mode()&os.ModeSymlink != 0 {
			return p, updateFailure("frozen-source-mismatch", "managed package storage contains a symbolic link; restore regular locked storage first", nil)
		}
	}
	raw, exists := tx.inputs.trees[filepath.Join(tx.home, "style.lock.yaml")].files["."]
	if !exists {
		return fail("a nonempty explicit format 1 or 2 lock is required", nil)
	}
	p.result.LockSHA256 = hashBytes(raw.data)
	var header struct {
		Format int `yaml:"format"`
	}
	if yaml.Unmarshal(raw.data, &header) != nil || header.Format != lockFormat && header.Format != gitLockFormat && header.Format != projectLockFormat {
		return fail("frozen requires an explicit supported lock format", nil)
	}
	lock, err := decodeLock(raw.data)
	if err == nil {
		err = s.validateScopeLock(tx.home, lock)
	}
	if err != nil {
		return p, updateFailure("lock-invalid", "cannot decode a valid frozen lock", err)
	}
	if err := validateUpdateLock(lock); err != nil {
		return p, err
	}
	if len(lock.Packages) == 0 || len(lock.Harnesses) == 0 {
		return fail("frozen requires packages and locked installation bindings", nil)
	}
	for _, binding := range lock.Harnesses {
		if binding.Route == "" || binding.Version == "" || !filepath.IsAbs(binding.ConfigDir) || !lockedHashPattern.MatchString(binding.Surface) || !lockedHashPattern.MatchString(binding.Rendered) || binding.Trust == "" {
			return fail("the lock lacks complete route, version, configuration or output bindings", nil)
		}
	}
	if err := tx.inputs.links(lock); err != nil {
		return p, updateFailure("frozen-source-mismatch", "cannot capture live linked sources", err)
	}
	adapters, err := s.lockedAdapters(ctx, tx.home, lock)
	if err != nil {
		return p, updateFailure("frozen-binding-mismatch", "a locked installation route is unavailable", err)
	}
	if err := tx.inputs.routes(adapters); err != nil {
		return p, updateFailure("frozen-binding-mismatch", "cannot capture locked route inputs", err)
	}
	ordered := append([]lockPackage(nil), lock.Packages...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Seq < ordered[j].Seq })
	var packages []installPackage
	replaceCount := 0
	for _, item := range ordered {
		p.result.Selected = append(p.result.Selected, item.Name)
		p.result.Changes = append(p.result.Changes, UpdateChange{Package: item.Name, Sequence: item.Seq, Before: updateRevision(item), SettingsAdded: []string{}, SettingsRemoved: []string{}})
		if !lockedHashPattern.MatchString(item.Approval) {
			return fail("every package requires an authenticated approval digest", nil)
		}
		baseline, available, err := loadApproval(tx.home, item)
		if err != nil || !available {
			return fail("a locked approval baseline is missing or invalid", err)
		}
		files, origin, repair, err := frozenContent(ctx, tx.home, item, baseline, tx.inputs)
		if err != nil {
			return p, err
		}
		stored, err := capturedUpdateSettings(tx.home, item.Name, tx.inputs)
		if err != nil {
			return fail("private settings are invalid", err)
		}
		keys := frozenKeys(stored)
		sort.Strings(keys)
		if !slices.Equal(keys, item.Settings) {
			return fail("stored settings must exactly match the locked keys; defaults are not used", nil)
		}
		parsed, err := s.parseSnapshotWithSettings(ctx, tx.home, item, files, func() (map[string]string, error) { return stored, nil })
		if err != nil {
			return p, frozenPackageFailure("frozen-build-mismatch", item.Name, "locked content no longer passes package/runtime validation", err)
		}
		if !reflect.DeepEqual(parsed.lock, item) {
			return p, frozenPackageFailure("frozen-build-mismatch", item.Name, "name, version, settings, replace or source identity differs from the lock", nil)
		}
		if item.Replace && !request.AllowReplace {
			return p, updateFailure("source-invalid", "locked mode: replace requires --allow-replace", nil)
		}
		if item.Replace {
			replaceCount++
			if replaceCount > 1 {
				return p, updateFailure("frozen-build-mismatch", "the lock contains more than one mode: replace package", nil)
			}
		}
		for _, target := range adapters {
			if harnessRequirement(parsed.pkg.Metadata, target.provider, target.installation.Version) != nil {
				return p, frozenPackageFailure("frozen-binding-mismatch", item.Name, "locked harness version is incompatible", nil)
			}
		}
		packages = append(packages, parsed)
		tx.candidates = append(tx.candidates, updateCandidate{old: item, item: parsed, baseline: baseline, approved: true, selected: true, storage: repair})
		p.result.Contents = append(p.result.Contents, FrozenContent{Package: item.Name, Snapshot: origin, StorageRepair: repair})
	}
	compilation, err := s.compileInstalled(ctx, packages, adapters, "")
	if err != nil {
		return p, err
	}
	if lock.MinIndexFormat != minimumIndex(compilation.index.Format) {
		return fail("minimum runtime requirement differs from the approved lock; use normal management to upgrade", nil)
	}
	p.result.Diagnostics = append(p.result.Diagnostics, compilation.diagnostics...)
	for i, candidate := range tx.candidates {
		current, err := approvalBytes(compilation, candidate.old.Name, candidate.item.pkg.Files, candidate.old.Source.Git)
		if err != nil {
			return p, err
		}
		if hashBytes(current) != candidate.old.Approval {
			return p, frozenPackageFailure("frozen-build-mismatch", candidate.old.Name, "the compiled source, rendered or native approval differs; normal management approval is required", nil)
		}
		change := &p.result.Changes[i]
		after := updateRevision(candidate.old)
		change.After = &after
		change.BaselineAvailable = true
		change.NativeBefore = candidate.baseline.Native
		change.NativeAfter = compilation.nativeReports(candidate.old.Name)
		for _, target := range adapters {
			if report, ok := candidate.item.report[target.provider]; ok {
				p.result.Reports = append(p.result.Reports, UpdatePlanReport{Package: candidate.old.Name, PlanReport: installPlanReport(target, report, candidate.item.tokens[target.provider], candidate.item.resolved)})
			}
		}
	}
	p.result.Native = compilation.nativeReports("")
	files, harnesses, routes, migrations, err := planInstallRoutes(ctx, tx.home, adapters, compilation, lock, nil)
	if err != nil {
		return p, updateFailure("frozen-binding-mismatch", "locked renderer ownership or static layout cannot be reproduced; inspect with doctor and restore approved files", err)
	}
	if len(migrations) > 0 || !sameFrozenBindings(lock.Harnesses, harnesses) {
		return p, updateFailure("frozen-build-mismatch", "route, version, configuration root, surface, rendered, trust or static layout differs from the lock; use the existing management flow first", nil)
	}
	if s.project != nil {
		next := lock
		files, err = s.projectFiles(tx.home, lock, &next, adapters, compilation, files, tx.inputs)
		if err != nil {
			return p, err
		}
		if !reflect.DeepEqual(next, lock) {
			return p, projectFailure("frozen binding cannot change")
		}
	}
	for _, file := range files {
		// Frozen may recreate absent owner-authorized outputs. It never replaces
		// an edited managed block or merges a new baseline over an existing file.
		if file.Remove || file.Before != nil && !bytes.Equal(file.Before, file.After) {
			return p, updateFailure("frozen-build-mismatch", "frozen cannot adopt edits to an existing managed configuration or restoration record", nil)
		}
	}
	p.result.Routes = routes
	tx.files = append(tx.files, files...)
	for name, data := range map[string][]byte{"index.json": compilation.indexBytes, "explain.json": compilation.provenanceBytes} {
		planned, err := planPrivateFile(filepath.Join(tx.home, "build", name), data)
		if err != nil {
			return p, updateFailure("installation-changed", "cannot plan derived build files", err)
		}
		tx.files = append(tx.files, planned...)
	}
	tx.files, err = uniquePlannedFiles(tx.files)
	if err != nil {
		return p, updateFailure("frozen-binding-mismatch", "locked routes plan conflicting files", err)
	}
	if err := tx.inputs.planned(tx.files); err != nil {
		return p, err
	}
	if tx.changed() {
		p.result.Status, p.result.AwaitingConfirmation = "confirmation-required", true
	}
	tx.result = p.result.UpdateResult
	return p, nil
}

func frozenContent(ctx context.Context, home string, item lockPackage, baseline approvedSnapshot, inputs *managementInputs) (map[string][]byte, string, bool, error) {
	matches := func(files map[string][]byte) bool {
		hashes, _ := hashPackageFiles(files)
		return reflectStringMap(hashes, item.Files) && checkGitSnapshot(item, files) == nil
	}
	managed, storageErr := installedSnapshot(home, item)
	if item.Link {
		if !item.LinkTrusted || storageErr != nil || !matches(managed) {
			return nil, "", false, frozenPackageFailure("frozen-source-mismatch", item.Name, "live linked content must match the lock and already be trusted", storageErr)
		}
		return cloneFileMap(managed), "link", false, nil
	}
	if storageErr == nil && matches(managed) {
		return cloneFileMap(managed), "managed", false, nil
	}
	if item.Source.Git != nil {
		snapshot, err := source.ReadLocked(ctx, home, *item.Source.Git, item.Source.Kind, item.Source.Path, item.Files)
		if err == nil && matches(snapshot.Files) {
			return cloneFileMap(snapshot.Files), "cache", true, nil
		}
		if err := checkContext(ctx); err != nil {
			return nil, "", false, err
		}
	} else {
		// A missing or changed upstream is harmless when the approval retains the
		// complete locked snapshot. Capture a usable upstream before using it.
		if err := inputs.add(item.Source.Path); err == nil {
			files, single, _, err := snapshotPackage(item.Source.Path)
			if err == nil && single == (item.Source.Kind == "file") && matches(files) {
				return cloneFileMap(files), "source", true, nil
			}
		}
	}
	if matches(baseline.Files) {
		return cloneFileMap(baseline.Files), "approved", true, nil
	}
	return nil, "", false, frozenPackageFailure("frozen-content-unavailable", item.Name, fmt.Sprintf("no complete locked %s snapshot is available", item.Source.Kind), nil)
}

// YAML omitempty turns an empty static_files map into nil. Both represent the
// same bound layout; all nonempty path hashes and other fields remain exact.
func sameFrozenBindings(before, after map[string]lockHarness) bool {
	if len(before) != len(after) {
		return false
	}
	for provider, old := range before {
		current, ok := after[provider]
		if !ok || !reflectStringMap(old.StaticFiles, current.StaticFiles) {
			return false
		}
		old.StaticFiles, current.StaticFiles = nil, nil
		if !reflect.DeepEqual(old, current) {
			return false
		}
	}
	return true
}
