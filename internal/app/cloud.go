package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/probe"
	styleruntime "github.com/inxups/style-cli/internal/runtime"
	"github.com/inxups/style-cli/internal/saferead"
	"github.com/speakeasy-api/agenthooks"
	"github.com/speakeasy-api/agenthooks/install"
)

const cloudDir = ".style/cloud"
const cloudManifestPath = cloudDir + "/manifest.json"
const cloudReceiptName = "cloud-export.json"
const cloudLimit = 16 << 20
const cloudTotalLimit = 32 << 20

// CloudManifest is a portable integrity inventory, not an authorization token.
// The native workspace trust boundary still applies to a checked-out bundle.
type CloudManifest struct {
	Format       int               `json:"format"`
	Target       string            `json:"target"`
	Runtime      string            `json:"runtime"`
	EntryVersion int               `json:"entry_version"`
	Generation   string            `json:"generation_sha256"`
	Files        map[string]string `json:"files"`
	NativeSHA256 string            `json:"native_sha256"`
	Dependencies []string          `json:"external_dependencies"`
}
type cloudReceipt struct {
	Format         int               `json:"format"`
	ManifestSHA256 string            `json:"manifest_sha256"`
	Generation     string            `json:"generation_sha256"`
	Files          map[string]string `json:"files"`
}
type ExportRequest struct {
	Target  string
	Yes     bool
	Confirm func(ExportResult) (bool, error)
}
type ExportResult struct {
	Target               string        `json:"target"`
	Changed              bool          `json:"changed"`
	AwaitingConfirmation bool          `json:"awaiting_confirmation"`
	Manifest             CloudManifest `json:"manifest"`
	// Review every exported body and native command before committing the bundle.
	Content  map[string]string  `json:"content"`
	Native   []NativeHookReport `json:"native_hooks"`
	Warnings []string           `json:"warnings"`
}

func cloudError(message string) error { return fmt.Errorf("cloud-invalid: %s", message) }
func cloudFileName(name string) bool {
	if name == cloudDir+"/index.json" || name == cloudDir+"/explain.json" || name == projectDeclarationPath {
		return true
	}
	return validProjectArtifact(name) && name != ".codex/hooks.json"
}
func decodeCloudJSON(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return cloudError("malformed JSON")
	}
	if d.Decode(new(any)) != io.EOF {
		return cloudError("expected one JSON object")
	}
	return nil
}
func readCloudFile(root, name string) ([]byte, error) {
	anchor, err := saferead.AnchorMutable(root)
	if err != nil {
		return nil, err
	}
	defer anchor.Close()
	return saferead.File(anchor, filepath.FromSlash(name), cloudLimit)
}

func readCloud(root string) (CloudManifest, map[string][]byte, error) {
	return readCloudInventory(root, true)
}
func readCloudInventory(root string, verifyExternal bool) (CloudManifest, map[string][]byte, error) {
	var m CloudManifest
	raw, err := readCloudFile(root, cloudManifestPath)
	if err != nil {
		return m, nil, err
	}
	if err = decodeCloudJSON(raw, &m); err != nil {
		return m, nil, err
	}
	if m.Format != 1 || m.Target != "claude-cloud" || m.Runtime != styleRuntimeVersion || m.EntryVersion != projectEntryVersion || !lockedHashPattern.MatchString(m.Generation) || !lockedHashPattern.MatchString(m.NativeSHA256) || len(m.Files) < 5 || len(m.Files) > 512 || m.Dependencies == nil {
		return m, nil, cloudError("unsupported manifest or runtime contract")
	}
	identity, _ := json.Marshal(struct {
		Files  map[string]string
		Native string
	}{m.Files, m.NativeSHA256})
	if hashBytes(identity) != m.Generation {
		return m, nil, cloudError("generation identity mismatch")
	}
	data := map[string][]byte{}
	total := len(raw)
	for name, digest := range m.Files {
		if !cloudFileName(name) || path.Clean(name) != name || strings.Contains(name, "\\") || !lockedHashPattern.MatchString(digest) {
			return m, nil, cloudError("invalid bundle file reference")
		}
		if !verifyExternal && !strings.HasPrefix(name, cloudDir+"/") {
			continue
		}
		b, e := readCloudFile(root, name)
		if e != nil {
			return m, nil, e
		}
		total += len(b)
		if total > cloudTotalLimit || hashBytes(b) != digest {
			return m, nil, cloudError("bundle digest or total limit mismatch")
		}
		data[name] = b
	}
	for _, name := range []string{cloudDir + "/index.json", cloudDir + "/explain.json", projectDeclarationPath, projectLauncherPath, ".claude/settings.json"} {
		if _, ok := m.Files[name]; !ok {
			return m, nil, cloudError("required bundle file missing")
		}
	}
	var index compile.Index
	if err = json.Unmarshal(data[cloudDir+"/index.json"], &index); err != nil {
		return m, nil, cloudError("invalid compiled index")
	}
	if len(index.Harnesses) != 1 || index.Harnesses[string(claude.ProviderName)] == nil {
		return m, nil, cloudError("bundle must contain only Claude Code")
	}
	if _, err = compile.DecodeProvenance(data[cloudDir+"/explain.json"], data[cloudDir+"/index.json"], index); err != nil {
		return m, nil, cloudError("invalid index provenance")
	}
	// Do not leave unlisted files available for a later export to overwrite.
	entries, err := os.ReadDir(filepath.Join(root, cloudDir))
	if err != nil {
		return m, nil, err
	}
	for _, e := range entries {
		if e.Name() != "manifest.json" && m.Files[cloudDir+"/"+e.Name()] == "" {
			return m, nil, cloudError("unlisted cloud artifact")
		}
	}
	return m, data, nil
}
func cloudLogicalSource(rule, source string) (string, error) {
	// Source references are diagnostics, never filesystem reads in the runner.
	if filepath.IsAbs(source) || strings.Contains(source, "\\") || path.Clean(source) != source || source == ".." || strings.HasPrefix(source, "../") {
		return "", cloudError("machine-bound source reference")
	}
	pkg, _, ok := strings.Cut(rule, "/")
	if !ok || !lockedNamePattern.MatchString(pkg) {
		return "", cloudError("invalid logical package")
	}
	return pkg + "/" + source, nil
}
func (s *Service) Export(ctx context.Context, req ExportRequest) (result ExportResult, runErr error) {
	if err := checkContext(ctx); err != nil {
		return result, err
	}
	if s.project == nil || req.Target != "claude-cloud" {
		return result, &UsageError{Message: "export requires --target claude-cloud --project <dir>"}
	}
	home, root := s.project.Home, s.project.Root
	// Recompile for inspection with approved inputs. Export never repairs,
	// syncs, reapproves, fetches sources, or invokes the harness detector.
	prepared, err := s.prepareSync(ctx, TrustRequest{})
	if err != nil {
		return result, err
	}
	if len(prepared.result.NeedsTrust) > 0 || len(prepared.files) > 0 || len(prepared.result.Changes) > 0 {
		return result, cloudError("installation differs from its approved state; sync/trust explicitly before export")
	}
	lock, err := s.loadLock(home)
	if err != nil {
		return result, err
	}
	h, ok := lock.Harnesses[string(claude.ProviderName)]
	if !ok || h.Route != claude.ProjectRouteName || len(lock.Packages) == 0 {
		return result, cloudError("an approved claude-project installation is required")
	}
	inputs := prepared.inputs
	if err = inputs.add(filepath.Join(root, cloudDir)); err != nil {
		return result, err
	}
	if err = inputs.add(filepath.Join(home, cloudReceiptName)); err != nil {
		return result, err
	}
	indexBytes, err := saferead.ReadFile(filepath.Join(home, "build/index.json"), cloudLimit)
	if err != nil {
		return result, err
	}
	var index compile.Index
	if err = json.Unmarshal(indexBytes, &index); err != nil {
		return result, err
	}
	explainBytes, err := saferead.ReadFile(filepath.Join(home, "build/explain.json"), cloudLimit)
	if err != nil {
		return result, err
	}
	provenance, err := compile.DecodeProvenance(explainBytes, indexBytes, index)
	if err != nil {
		return result, err
	}
	index.Harnesses = map[string]map[string][]compile.Entry{string(claude.ProviderName): index.Harnesses[string(claude.ProviderName)]}
	staticEntries := index.Static[string(claude.ProviderName)]
	index.Static = map[string][]compile.StaticEntry{}
	if staticEntries != nil {
		index.Static[string(claude.ProviderName)] = staticEntries
	}
	for event, entries := range index.Harnesses[string(claude.ProviderName)] {
		for i := range entries {
			entries[i].Source, err = cloudLogicalSource(entries[i].Rule, entries[i].Source)
			if err != nil {
				return result, err
			}
		}
		index.Harnesses[string(claude.ProviderName)][event] = entries
	}
	for i := range index.Static[string(claude.ProviderName)] {
		entry := &index.Static[string(claude.ProviderName)][i]
		entry.Source, err = cloudLogicalSource(entry.Rule, entry.Source)
		if err != nil {
			return result, err
		}
	}
	indexBytes, err = compile.Encode(index)
	if err != nil {
		return result, err
	}
	rows := provenance.Harnesses[string(claude.ProviderName)]
	for i := range rows {
		rows[i].Source, err = cloudLogicalSource(rows[i].Rule, rows[i].Source)
		if err != nil {
			return result, err
		}
	}
	provenance.Harnesses = map[string][]compile.ProvenanceRule{string(claude.ProviderName): rows}
	provenance.IndexSHA256 = hashBytes(indexBytes)
	if err = provenance.Validate(index, indexBytes); err != nil {
		return result, err
	}
	explainBytes, err = json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return result, err
	}
	explainBytes = append(explainBytes, '\n')
	data := map[string][]byte{cloudDir + "/index.json": indexBytes, cloudDir + "/explain.json": explainBytes}
	declaration, err := readProjectDeclaration(root)
	if err != nil || declaration == nil {
		return result, cloudError("project declaration is missing")
	}
	names := []string{projectDeclarationPath}
	for name := range declaration.Artifacts {
		if cloudFileName(name) {
			names = append(names, name)
		}
	}
	for _, name := range names {
		if err = inputs.add(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			return result, err
		}
		b, e := readCloudFile(root, name)
		if e != nil {
			return result, e
		}
		data[name] = b
	}
	result = ExportResult{Target: req.Target, Content: map[string]string{}, Native: []NativeHookReport{}, Warnings: []string{"Review all exported text and commands for secrets before committing; ordinary strings are not automatically redacted.", "Preinstall a compatible STYLE runner on PATH; launch the single-repository cloud session from its root.", "Native commands retain their exact approved text; provision every external dependency and validate it in the actual cloud environment.", "Export is a snapshot; changes require another explicit export. Native workspace trust remains external."}}
	for _, pkg := range lock.Packages {
		settings, e := readSettingValues(home, pkg.Name)
		if e != nil {
			return result, e
		}
		if len(settings) != len(pkg.Settings) {
			return result, cloudError("settings keys differ from the approved installation")
		}
		for _, key := range pkg.Settings {
			if _, exists := settings[key]; !exists {
				return result, cloudError("approved setting is missing")
			}
		}
		approved, available, e := loadApproval(home, pkg)
		if e != nil {
			return result, e
		}
		if !available || pkg.Link && !pkg.LinkTrusted {
			return result, cloudError("complete approved inputs are required")
		}
		for _, n := range approved.Native {
			if n.Harness == string(claude.ProviderName) {
				result.Native = append(result.Native, n)
			}
		}
	}
	nativeBytes, _ := json.Marshal(result.Native)
	manifest := CloudManifest{Format: 1, Target: req.Target, Runtime: styleRuntimeVersion, EntryVersion: projectEntryVersion, Files: map[string]string{}, NativeSHA256: hashBytes(nativeBytes), Dependencies: []string{"style compatible with cloud entry v1 on PATH", "POSIX /bin/sh", "Claude Code >= 2.1.301"}}
	if len(result.Native) > 0 {
		manifest.Dependencies = append(manifest.Dependencies, "external programs and files used by the disclosed native commands; not inferred or bundled")
	}
	total := 0
	for name, b := range data {
		if len(b) > cloudLimit {
			return result, cloudError("file size limit exceeded")
		}
		total += len(b)
		if total > cloudTotalLimit {
			return result, cloudError("total size limit exceeded")
		}
		manifest.Files[name] = hashBytes(b)
		result.Content[name] = string(b)
	}
	identity, _ := json.Marshal(struct {
		Files  map[string]string
		Native string
	}{manifest.Files, manifest.NativeSHA256})
	manifest.Generation = hashBytes(identity)
	result.Manifest = manifest
	raw, _ := json.MarshalIndent(manifest, "", "  ")
	data[cloudManifestPath] = append(raw, '\n')
	result.Content[cloudManifestPath] = string(data[cloudManifestPath])
	var previous map[string][]byte
	if _, e := os.Lstat(filepath.Join(root, cloudDir)); e == nil {
		_, previous, err = readCloudInventory(root, false)
		if err != nil {
			return result, err
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return result, e
	}
	var changes []install.PlannedFile
	for name, b := range data {
		if !strings.HasPrefix(name, cloudDir+"/") {
			continue
		}
		more, e := planProjectFile(root, name, b, 0o644)
		if e != nil {
			return result, e
		}
		changes = append(changes, more...)
	}
	for name := range previous {
		if strings.HasPrefix(name, cloudDir+"/") {
			if _, ok := data[name]; !ok {
				more, e := plannedRemoval(filepath.Join(root, filepath.FromSlash(name)))
				if e != nil {
					return result, e
				}
				changes = append(changes, more...)
			}
		}
	}
	receipt := cloudReceipt{1, hashBytes(data[cloudManifestPath]), manifest.Generation, map[string]string{}}
	for name, b := range data {
		if strings.HasPrefix(name, cloudDir+"/") {
			receipt.Files[name] = hashBytes(b)
		}
	}
	receiptBytes, _ := json.Marshal(receipt)
	more, err := planPrivateFile(filepath.Join(home, cloudReceiptName), receiptBytes)
	if err != nil {
		return result, err
	}
	changes = append(changes, more...)
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	if err = inputs.planned(changes); err != nil {
		return result, err
	}
	result.AwaitingConfirmation = len(changes) > 0
	if !result.AwaitingConfirmation {
		return result, nil
	}
	yes := req.Yes
	if !yes && req.Confirm != nil {
		copyBytes, _ := json.Marshal(result)
		var detached ExportResult
		_ = json.Unmarshal(copyBytes, &detached)
		yes, err = req.Confirm(detached)
		if err != nil {
			return result, err
		}
	}
	if !yes {
		return result, nil
	}
	guard, err := acquireInputs(ctx, inputs, home)
	if err != nil {
		return result, err
	}
	defer func() { runErr = errors.Join(runErr, guard.close()) }()
	if err = inputs.verify(); err != nil {
		return result, err
	}
	if err = install.ApplyFiles(ctx, changes); err != nil {
		return result, err
	}
	result.Changed = true
	result.AwaitingConfirmation = false
	return result, nil
}

// Removal is part of the same project transaction as the last package. A
// modified export is not silently deleted; restore it or review a new export.
func (s *Service) removeCloudExport(home string, inputs *managementInputs) ([]install.PlannedFile, error) {
	if s.project == nil {
		return nil, nil
	}
	file := filepath.Join(home, cloudReceiptName)
	if err := inputs.add(file); err != nil {
		return nil, err
	}
	if err := install.CheckPath(home, file); err != nil {
		return nil, err
	}
	raw, err := saferead.ReadFile(file, cloudLimit)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var receipt cloudReceipt
	if err = decodeCloudJSON(raw, &receipt); err != nil {
		return nil, err
	}
	if receipt.Format != 1 || !lockedHashPattern.MatchString(receipt.ManifestSHA256) || receipt.Files[cloudManifestPath] != receipt.ManifestSHA256 || len(receipt.Files) != 3 {
		return nil, cloudError("invalid private export receipt")
	}
	var changes []install.PlannedFile
	for name, hash := range receipt.Files {
		if name != cloudManifestPath && name != cloudDir+"/index.json" && name != cloudDir+"/explain.json" {
			return nil, cloudError("invalid export receipt file")
		}
		if err = inputs.add(filepath.Join(s.project.Root, filepath.FromSlash(name))); err != nil {
			return nil, err
		}
		b, e := readCloudFile(s.project.Root, name)
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return nil, e
		}
		if e == nil && hashBytes(b) != hash {
			return nil, cloudError("modified exported file; review export before removal")
		}
		more, e := plannedRemoval(filepath.Join(s.project.Root, filepath.FromSlash(name)))
		if e != nil {
			return nil, e
		}
		changes = append(changes, more...)
	}
	more, err := plannedRemoval(file)
	return append(changes, more...), err
}
func (s *Service) checkCloudExport(home string, result *DoctorResult) {
	if s.project == nil {
		return
	}
	raw, err := readOptionalBytes(filepath.Join(home, cloudReceiptName))
	if raw == nil && err == nil {
		return
	}
	var receipt cloudReceipt
	if err != nil || decodeCloudJSON(raw, &receipt) != nil {
		result.add("cloud-export-invalid", "error", "cloud export receipt is invalid")
		return
	}
	manifest, _, err := readCloudInventory(s.project.Root, false)
	if err != nil {
		result.add("cloud-export-invalid", "error", "cloud bundle failed integrity validation")
		return
	}
	manifestBytes, manifestErr := readCloudFile(s.project.Root, cloudManifestPath)
	if manifestErr != nil || hashBytes(manifestBytes) != receipt.ManifestSHA256 {
		result.add("cloud-export-invalid", "error", "cloud manifest differs from export receipt")
		return
	}
	for name, digest := range manifest.Files {
		if strings.HasPrefix(name, cloudDir+"/") {
			continue
		}
		data, readErr := readCloudFile(s.project.Root, name)
		if readErr != nil || hashBytes(data) != digest {
			result.add("cloud-export-stale", "error", "cloud bundle is stale; run explicit export again")
			return
		}
	}
	d, err := readCloudFile(s.project.Root, projectDeclarationPath)
	if err != nil || manifest.Files[projectDeclarationPath] != hashBytes(d) || receipt.Generation != manifest.Generation {
		result.add("cloud-export-stale", "error", "cloud bundle is stale; run explicit export again")
	}
}
func runCloudHook(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, enricher adapter.RuntimeEnricher, closed bool) int {
	fail := func(err error) int {
		return projectHookFailure(ctx, args, in, out, errOut, enricher, agenthooks.ProviderClaudeCode, closed, err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	root, err := canonicalProjectPath(cwd)
	if err != nil {
		return fail(err)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--project-entry-root=") {
			p, e := canonicalProjectPath(strings.TrimPrefix(arg, "--project-entry-root="))
			if e != nil || p != root {
				return fail(cloudError("cloud sessions must start from the project root"))
			}
		}
	}
	_, data, err := readCloud(root)
	if err != nil {
		return fail(err)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return fail(err)
	}
	userHome, err = canonicalFuturePath(userHome)
	if err != nil || within(root, userHome) {
		return fail(cloudError("cloud HOME must be outside the shared repository"))
	}
	home := filepath.Join(userHome, ".style", "cloud-state", strings.TrimPrefix(hashBytes([]byte(root)), "sha256:"))
	store, err := state.New(filepath.Join(home, "state"))
	if err != nil {
		return fail(err)
	}
	core, err := engine.Load(bytes.NewReader(data[cloudDir+"/index.json"]), store)
	if err != nil {
		return fail(err)
	}
	runtime, err := styleruntime.New(core, store, enricher, agenthooks.ProviderClaudeCode, closed)
	if err != nil {
		return fail(err)
	}
	runtime.CodecStateDir = filepath.Join(home, "cache", "agenthooks")
	runtime.Observer = probe.NewObserver(home, "", string(claude.ProviderName), "")
	return runtime.Run(ctx, args, in, out, errOut)
}
