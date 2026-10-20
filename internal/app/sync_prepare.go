package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func (s *Service) prepareSync(ctx context.Context, request TrustRequest) (syncPreparation, error) {
	prepared := syncPreparation{result: SyncResult{Changes: []PackageChange{}, NeedsTrust: []string{}, Routes: []string{}}}
	if err := checkContext(ctx); err != nil {
		return prepared, err
	}
	home, err := s.resolveStyleHome(request.Home)
	if err != nil {
		return prepared, err
	}
	prepared.home = home
	prepared.inputs, err = s.captureManagement(home)
	if err != nil {
		return prepared, err
	}
	prepared.result.ApprovalTarget = request.Name
	lockPath := filepath.Join(home, "style.lock.yaml")
	lockBefore, err := readOptionalBytes(lockPath)
	if err != nil {
		return prepared, err
	}
	lock, err := decodeLock(lockBefore)
	if err == nil {
		err = s.validateScopeLock(home, lock)
	}
	if err != nil {
		return prepared, err
	}
	if err = prepared.inputs.links(lock); err != nil {
		return prepared, err
	}
	if request.Name != "" {
		if _, found := findLockedPackage(lock, request.Name); !found {
			return prepared, &RejectedError{Message: fmt.Sprintf("package %q is not installed", request.Name)}
		}
	}
	indexPath := filepath.Join(home, "build", "index.json")
	indexBefore, err := readOptionalBytes(indexPath)
	if err != nil {
		return prepared, err
	}
	snapshots := make(map[string]map[string][]byte, len(lock.Packages))
	baselines := make(map[string]approvedSnapshot, len(lock.Packages))
	baselineAvailable := make(map[string]bool, len(lock.Packages))
	for _, item := range lock.Packages {
		if err := checkContext(ctx); err != nil {
			return prepared, err
		}
		files, err := installedSnapshot(home, item)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return prepared, err
		}
		if files == nil {
			files = map[string][]byte{}
		}
		if err := checkGitSnapshot(item, files); err != nil {
			return prepared, err
		}
		baseline, available, err := loadApproval(home, item)
		if err != nil {
			return prepared, err
		}
		if err := checkNativeFormat(item, files); err != nil {
			return prepared, err
		}
		baselines[item.Name], baselineAvailable[item.Name] = baseline, available
		snapshots[item.Name] = files
		changed := filesChanged(item, files)
		nativeChanged := false
		if len(changed) > 0 {
			if !available {
				// Without old bytes a development link cannot prove that an
				// executable native entry was not removed or modified.
				nativeChanged = item.Link
			} else {
				before, beforeErr := nativeSourceFingerprint(item, baseline.Files)
				after, afterErr := nativeSourceFingerprint(item, files)
				nativeChanged = beforeErr != nil || afterErr != nil || before != after
			}
		}
		needs := len(changed) > 0 && (!item.Link || nativeChanged) || item.Link && !item.LinkTrusted
		if len(changed) > 0 || needs || item.Name == request.Name {
			prepared.result.Changes = append(prepared.result.Changes, PackageChange{Package: item.Name, Link: item.Link, Files: changed, NeedsTrust: needs, NativeChanged: nativeChanged})
		}
		if needs && item.Name != request.Name {
			prepared.result.NeedsTrust = append(prepared.result.NeedsTrust, item.Name)
		}
	}
	if request.Name == "" && len(prepared.result.NeedsTrust) > 0 {
		return prepared, nil
	}
	checks, err := s.checkNative(ctx, home, lock, request.Name)
	if err != nil {
		return prepared, err
	}
	requireNativeRepairs(&prepared.result, checks, lock, request.Name)
	if request.Name == "" && len(prepared.result.NeedsTrust) > 0 {
		return prepared, nil
	}
	if err := checkStateRoot(filepath.Join(home, "state")); err != nil {
		return prepared, err
	}
	currentSnapshots := snapshots
	snapshots = make(map[string]map[string][]byte, len(lock.Packages))
	for _, item := range lock.Packages {
		baseline, available := baselines[item.Name], baselineAvailable[item.Name]
		snapshots[item.Name] = currentSnapshots[item.Name]
		if request.Name != "" && item.Name != request.Name && packageNeedsTrust(prepared.result, item.Name) {
			if !available {
				return prepared, &ConflictError{Message: fmt.Sprintf("cannot rebuild %s while its source differs and its approval baseline is unavailable; restore its locked bytes first", item.Name)}
			}
			snapshots[item.Name] = baseline.Files
		}
	}
	packages := make([]installPackage, 0, len(lock.Packages))
	updated := lockDocument{Format: lock.Format, Project: lock.Project, Packages: []lockPackage{}, Harnesses: cloneHarnesses(lock.Harnesses)}
	replaceOwner := ""
	for _, item := range lock.Packages {
		parsed, err := s.parseInstalledSnapshot(ctx, home, item, snapshots[item.Name])
		if err != nil {
			return prepared, err
		}
		if parsed.lock.Replace {
			if !item.Replace && !(item.Name == request.Name && request.AllowReplace) {
				return prepared, &RejectedError{Message: fmt.Sprintf("package %s newly uses replace; run sync %s --allow-replace", item.Name, item.Name)}
			}
			if replaceOwner != "" {
				return prepared, &ConflictError{Message: fmt.Sprintf("replace mode conflicts between %s and %s", replaceOwner, item.Name)}
			}
			replaceOwner = item.Name
		}
		if item.Link && item.Name == request.Name {
			parsed.lock.LinkTrusted = true
		}
		packages = append(packages, parsed)
	}
	adapters, err := s.lockedAdapters(ctx, home, lock)
	if err != nil {
		return prepared, err
	}
	if err = prepared.inputs.routes(adapters); err != nil {
		return prepared, err
	}
	mapping, err := s.captureEffortMapping(home, prepared.inputs)
	if err != nil {
		return prepared, err
	}
	compilation, err := s.compileInstalled(ctx, packages, adapters, request.Name, mapping)
	if err != nil {
		return prepared, err
	}
	prepared.result.Diagnostics, prepared.result.Reports = compilation.diagnostics, compilation.reports
	if request.Name != "" {
		prepared.result.Native = compilation.nativeReports("")
	}
	prepared.files, updated.Harnesses, prepared.result.Routes, prepared.result.StaticMigrations, err = planInstallRoutes(ctx, home, adapters, compilation, lock, nil)
	if err != nil {
		return prepared, err
	}
	for _, item := range packages {
		original, _ := findLockedPackage(lock, item.lock.Name)
		baseline, available := baselines[item.lock.Name], baselineAvailable[item.lock.Name]
		approval, err := approvalBytes(compilation, item.lock.Name, snapshots[item.lock.Name], item.lock.Source.Git)
		if err != nil {
			return prepared, err
		}
		var current approvedSnapshot
		if err := json.Unmarshal(approval, &current); err != nil {
			return prepared, err
		}
		for i := range prepared.result.Changes {
			change := &prepared.result.Changes[i]
			if change.Package != item.lock.Name {
				continue
			}
			change.BaselineAvailable = available
			change.SourceDiff = diffMaps(baseline.Files, currentSnapshots[item.lock.Name])
			change.RenderedDiff = diffMaps(baseline.Rendered, current.Rendered)
			if request.Name != "" {
				var previous []NativeHookReport
				if available {
					previous, err = s.baselineNative(original, baseline, adapters)
					if err != nil {
						return prepared, err
					}
				}
				change.NativeDiff = nativeDiff(previous, current.Native)
			}
		}
		item.lock.Approval = hashBytes(approval)
		updated.Packages = append(updated.Packages, item.lock)
		changes, err := planPrivateFile(approvalPath(home, item.lock.Name), approval)
		if err != nil {
			return prepared, err
		}
		prepared.files = append(prepared.files, changes...)
		if len(item.settings) > 0 || len(original.Settings) > 0 {
			data, err := yaml.Marshal(item.settings)
			if err != nil {
				return prepared, err
			}
			changes, err := planPrivateFile(filepath.Join(home, "settings", item.lock.Name+".yaml"), data)
			if err != nil {
				return prepared, err
			}
			prepared.files = append(prepared.files, changes...)
		}
	}
	// Syncing an empty installation only performs state maintenance.
	if len(lock.Packages) > 0 {
		prepared.files, err = s.projectFiles(home, lock, &updated, adapters, compilation, prepared.files, prepared.inputs)
		if err != nil {
			return prepared, err
		}
		lockAfter, err := yaml.Marshal(updated)
		if err != nil {
			return prepared, err
		}
		prepared.files = append(prepared.files, planPrivateBytes(lockPath, lockBefore, lockAfter)...)
		prepared.files = append(prepared.files, planPrivateBytes(indexPath, indexBefore, compilation.indexBytes)...)
		provenanceChanges, err := planPrivateFile(filepath.Join(home, "build", "explain.json"), compilation.provenanceBytes)
		if err != nil {
			return syncPreparation{}, err
		}
		prepared.files = append(prepared.files, provenanceChanges...)
	}
	prepared.files, err = uniquePlannedFiles(prepared.files)
	if err == nil {
		err = prepared.inputs.planned(prepared.files)
	}
	return prepared, err
}

func packageNeedsTrust(result SyncResult, name string) bool {
	for _, change := range result.Changes {
		if change.Package == name {
			return change.NeedsTrust
		}
	}
	return false
}

func detachedSyncResult(result SyncResult) SyncResult {
	data, _ := json.Marshal(result)
	var detached SyncResult
	_ = json.Unmarshal(data, &detached)
	return detached
}
