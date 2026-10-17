package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/inxups/style-cli/internal/saferead"
	"github.com/speakeasy-api/agenthooks/install"
)

const projectLockFormat = 3
const projectEntryVersion = 1
const projectDeclarationPath = ".style/project.json"
const projectLauncherPath = ".style/hooks/style"
const projectLauncher = "#!/bin/sh\nexec style --project-hook --project-entry-root=\"${CLAUDE_PROJECT_DIR:?CLAUDE_PROJECT_DIR is required}\" \"$@\"\n"

type projectScope struct{ Root, Base, Home string }
type projectBinding struct {
	Declaration  string                           `json:"declaration_sha256,omitempty" yaml:"declaration_sha256,omitempty"`
	Scope        string                           `json:"scope" yaml:"scope"`
	Root         string                           `json:"root" yaml:"root"`
	Base         string                           `json:"base_home" yaml:"base_home"`
	Home         string                           `json:"home" yaml:"home"`
	EntryVersion int                              `json:"entry_version" yaml:"entry_version"`
	Runner       string                           `json:"runner" yaml:"runner"`
	Harnesses    map[string]projectHarnessBinding `json:"harnesses" yaml:"harnesses"`
}
type projectHarnessBinding struct {
	ConfigDir string `json:"config_dir" yaml:"config_dir"`
	Version   string `json:"version" yaml:"version"`
	Route     string `json:"route" yaml:"route"`
}
type projectDeclaration struct {
	Format       int                          `json:"format"`
	EntryVersion int                          `json:"entry_version"`
	Effects      string                       `json:"effects_sha256"`
	PackageOrder map[string]int               `json:"package_order"`
	Artifacts    map[string]string            `json:"artifacts"`
	Static       map[string]map[string]string `json:"static,omitempty"`
}
type projectRecord struct {
	Root      string                           `json:"root"`
	Home      string                           `json:"home"`
	Harnesses map[string]projectHarnessBinding `json:"harnesses"`
	Packages  []string                         `json:"packages"`
}
type projectRegistry struct {
	Format   int                      `json:"format"`
	Projects map[string]projectRecord `json:"projects"`
}

func canonicalProjectPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
func canonicalFuturePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var tail []string
	for {
		resolved, e := filepath.EvalSymlinks(abs)
		if e == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(e, os.ErrNotExist) {
			return "", e
		}
		tail = append(tail, filepath.Base(abs))
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", e
		}
		abs = parent
	}
}
func projectHome(base, root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(base, "projects", fmt.Sprintf("%x", sum))
}
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func projectFailure(message string) error {
	return &ConflictError{Message: "project-binding-invalid: " + message}
}

// ForProject returns an isolated service; it never changes environment or cwd.
func (s *Service) ForProject(root, base string) (*Service, error) {
	root, err := canonicalProjectPath(root)
	if err != nil {
		return nil, projectFailure("project directory must exist")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, projectFailure("project root is not a directory")
	}
	base, err = resolveStyleHome(base)
	if err != nil {
		return nil, err
	}
	base, err = canonicalFuturePath(base)
	if err != nil {
		return nil, err
	}
	if within(base, root) || within(root, base) {
		return nil, projectFailure("project and STYLE home cannot contain one another")
	}
	for _, env := range []struct{ name, sub string }{{"CODEX_HOME", ".codex"}, {"CLAUDE_CONFIG_DIR", ".claude"}} {
		value := os.Getenv(env.name)
		if value == "" {
			home, e := os.UserHomeDir()
			if e != nil {
				return nil, e
			}
			value = filepath.Join(home, env.sub)
		}
		config, e := canonicalFuturePath(value)
		if e != nil {
			return nil, e
		}
		if within(config, root) {
			return nil, projectFailure("project root is inside a harness user configuration directory")
		}
	}
	if err := checkProjectAncestors(root); err != nil {
		return nil, err
	}
	scoped := *s
	scoped.project = &projectScope{Root: root, Base: base, Home: projectHome(base, root)}
	scoped.definitions = map[adapter.ProviderID]adapter.Definition{}
	for id, definition := range s.definitions {
		switch id {
		case codex.ProviderName:
			facts := codex.Facts{Project: true}
			definition.Facts = facts
			definition.StaticPositions = facts
			definition.Routes = []adapter.Route{codex.NewProjectRoute(root)}
		case claude.ProviderName:
			facts := claude.NewFacts(adapter.Installation{ConfigDir: filepath.Join(root, ".claude")})
			facts.Project = true
			definition.Facts = facts
			definition.StaticPositions = facts
			definition.Routes = []adapter.Route{claude.NewProjectRoute(root)}
		default:
			continue
		}
		scoped.definitions[id] = definition
	}
	return &scoped, nil
}
func checkProjectAncestors(root string) error {
	for parent := filepath.Dir(root); ; parent = filepath.Dir(parent) {
		if _, err := os.Lstat(filepath.Join(parent, projectDeclarationPath)); err == nil {
			return projectFailure("nested STYLE projects are not supported")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	return nil
}
func (s *Service) resolveStyleHome(explicit string) (string, error) {
	if s.project == nil {
		return resolveStyleHome(explicit)
	}
	if explicit != "" {
		home, err := canonicalFuturePath(explicit)
		if err != nil {
			return "", err
		}
		if home != s.project.Base && home != s.project.Home {
			return "", projectFailure("STYLE home differs from the selected project binding")
		}
	}
	return s.project.Home, nil
}
func (s *Service) loadLock(home string) (lockDocument, error) {
	lock, err := loadLock(home)
	if err == nil {
		err = s.validateScopeLock(home, lock)
	}
	return lock, err
}
func (s *Service) validateScopeLock(home string, lock lockDocument) error {
	if s.project == nil {
		if lock.Project != nil {
			return projectFailure("project lock requires an explicit project scope")
		}
		return nil
	}
	if lock.Project == nil {
		if len(lock.Packages) > 0 || len(lock.Harnesses) > 0 {
			return projectFailure("project storage contains an unbound user lock")
		}
		return nil
	}
	p := s.project
	b := lock.Project
	if lock.Format != projectLockFormat || b.Scope != "project" || b.Root != p.Root || b.Base != p.Base || b.Home != home || b.EntryVersion != projectEntryVersion {
		return projectFailure("root or installation identity differs; reinstall explicitly at this root")
	}
	if b.Declaration == "" || !lockedHashPattern.MatchString(b.Declaration) {
		return projectFailure("missing approved project declaration digest")
	}
	data, readErr := readOptionalBytes(filepath.Join(p.Root, projectDeclarationPath))
	if readErr != nil {
		return readErr
	}
	if data != nil && hashBytes(data) != b.Declaration {
		return projectFailure("project declaration changed; restore its approved bytes")
	}
	if len(b.Harnesses) != len(lock.Harnesses) {
		return projectFailure("harness binding inventory differs")
	}
	for id, h := range lock.Harnesses {
		expected := codex.ProjectRouteName
		if id == string(claude.ProviderName) {
			expected = claude.ProjectRouteName
		}
		if h.Route != expected || b.Harnesses[id] != (projectHarnessBinding{ConfigDir: h.ConfigDir, Version: h.Version, Route: h.Route}) {
			return projectFailure("harness binding differs")
		}
	}
	return nil
}

// Validate persisted identities without resolving or reading their external
// roots. Scope selection later compares them with the current canonical root.
func validateProjectLockIdentity(lock lockDocument) error {
	b := lock.Project
	if b == nil || b.Scope != "project" || b.EntryVersion != projectEntryVersion || !lockedHashPattern.MatchString(b.Declaration) || len(b.Harnesses) == 0 || len(b.Harnesses) != len(lock.Harnesses) {
		return projectFailure("invalid lock binding")
	}
	for _, path := range []string{b.Root, b.Base, b.Home, b.Runner} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return projectFailure("invalid absolute binding path")
		}
	}
	if b.Home != projectHome(b.Base, b.Root) || within(b.Base, b.Root) || within(b.Root, b.Base) {
		return projectFailure("invalid project storage identity")
	}
	for id, h := range lock.Harnesses {
		expected := ""
		switch adapter.ProviderID(id) {
		case codex.ProviderName:
			expected = codex.ProjectRouteName
		case claude.ProviderName:
			expected = claude.ProjectRouteName
		default:
			return projectFailure("unsupported project harness")
		}
		if h.Route != expected || !filepath.IsAbs(h.ConfigDir) || h.Version == "" || b.Harnesses[id] != (projectHarnessBinding{ConfigDir: h.ConfigDir, Version: h.Version, Route: h.Route}) {
			return projectFailure("invalid locked project harness binding")
		}
	}
	return nil
}
func bindingFor(p *projectScope, lock lockDocument) (*projectBinding, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = canonicalProjectPath(exe)
	if err != nil {
		return nil, err
	}
	b := &projectBinding{Scope: "project", Root: p.Root, Base: p.Base, Home: p.Home, EntryVersion: projectEntryVersion, Runner: exe, Harnesses: map[string]projectHarnessBinding{}}
	for id, h := range lock.Harnesses {
		b.Harnesses[id] = projectHarnessBinding{ConfigDir: h.ConfigDir, Version: h.Version, Route: h.Route}
	}
	return b, nil
}
func readProjectDeclaration(root string) (*projectDeclaration, error) {
	path := filepath.Join(root, projectDeclarationPath)
	if err := install.CheckPath(root, path); err != nil {
		return nil, err
	}
	data, err := saferead.ReadFile(path, 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var d projectDeclaration
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&d); err != nil || dec.Decode(new(any)) != io.EOF {
		return nil, projectFailure("project declaration is malformed")
	}
	if d.Format != 1 || d.EntryVersion != projectEntryVersion || len(d.Artifacts) == 0 || !lockedHashPattern.MatchString(d.Effects) || len(d.PackageOrder) == 0 {
		return nil, projectFailure("unsupported project declaration")
	}
	sequences := map[int]bool{}
	for name, seq := range d.PackageOrder {
		if !lockedNamePattern.MatchString(name) || seq < 1 || sequences[seq] {
			return nil, projectFailure("invalid project package order")
		}
		sequences[seq] = true
	}
	for name, digest := range d.Artifacts {
		if !validProjectArtifact(name) || !lockedHashPattern.MatchString(digest) {
			return nil, projectFailure("invalid managed artifact")
		}
	}
	for id, files := range d.Static {
		if id != string(claude.ProviderName) {
			return nil, projectFailure("invalid static provider")
		}
		for name, digest := range files {
			if !strings.HasPrefix(name, "rules/style-") || !validProjectArtifact(".claude/"+name) || d.Artifacts[".claude/"+name] != digest {
				return nil, projectFailure("invalid static declaration")
			}
		}
	}
	return &d, nil
}
func validProjectArtifact(name string) bool {
	if name == projectLauncherPath || name == ".codex/hooks.json" || name == ".claude/settings.json" {
		return true
	}
	owner := strings.TrimSuffix(strings.TrimPrefix(name, ".claude/rules/style-"), ".md")
	return strings.HasPrefix(name, ".claude/rules/style-") && strings.HasSuffix(name, ".md") && lockedNamePattern.MatchString(owner)
}
func (s *Service) staticState(lock lockDocument, id adapter.ProviderID) adapter.StaticState {
	state := staticStateFor(lock, id)
	if s.project != nil && len(lock.Packages) == 0 && id == claude.ProviderName {
		if d, err := readProjectDeclaration(s.project.Root); err == nil && d != nil {
			state.Layout = claude.StaticSlotAppend
			state.Files = d.Static[string(id)]
		}
	}
	return state
}

func readRegistry(base string) (projectRegistry, error) {
	result := projectRegistry{Format: 1, Projects: map[string]projectRecord{}}
	path := filepath.Join(base, "project-registry.json")
	if err := install.CheckPath(base, path); err != nil {
		return result, err
	}
	data, err := saferead.ReadFile(path, 4<<20)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&result); err != nil || dec.Decode(new(any)) != io.EOF || result.Format != 1 || result.Projects == nil {
		return result, projectFailure("project registry is malformed; restore it before management")
	}
	return result, nil
}
func (s *Service) captureManagement(home string) (*managementInputs, error) {
	inputs, err := captureManagement(home)
	if err != nil {
		return nil, err
	}
	base := home
	if s.project != nil {
		base = s.project.Base
		inputs.guardHomes = []string{base, home}
		if err = install.CheckPath(home, filepath.Join(home, "binding.json")); err != nil {
			return nil, err
		}
		if err = inputs.add(filepath.Join(s.project.Root, projectDeclarationPath)); err != nil {
			return nil, err
		}
		if err = inputs.add(filepath.Join(s.project.Root, projectLauncherPath)); err != nil {
			return nil, err
		}
		if err = inputs.add(filepath.Join(home, "binding.json")); err != nil {
			return nil, err
		}
	}
	base, err = canonicalFuturePath(base)
	if err != nil {
		return nil, err
	}
	registry, err := readRegistry(base)
	if err != nil {
		return nil, err
	}
	if err = inputs.add(filepath.Join(base, "project-registry.json")); err != nil {
		return nil, err
	}
	if len(registry.Projects) == 0 && s.project == nil {
		if _, e := os.Stat(filepath.Join(base, "projects")); errors.Is(e, os.ErrNotExist) {
			return inputs, nil
		}
	}
	for _, path := range []string{filepath.Join(base, "project-registry.json"), filepath.Join(base, "projects"), filepath.Join(base, "style.lock.yaml")} {
		// Capture directory identities and exact known records without traversing runtime state.
		if path == filepath.Join(base, "projects") {
			continue
		}
		if err = inputs.add(path); err != nil {
			return nil, err
		}
	}
	dirs, err := os.ReadDir(filepath.Join(base, "projects"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, dir := range dirs {
		p := filepath.Join(base, "projects", dir.Name())
		if dir.Type()&os.ModeSymlink != 0 || !dir.IsDir() {
			return nil, projectFailure("unrecognized project storage")
		}
		if _, ok := registry.Projects[dir.Name()]; !ok {
			if _, err := os.Lstat(filepath.Join(p, "style.lock.yaml")); errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, projectFailure("unregistered project storage; restore the registry")
		}
	}
	for key, record := range registry.Projects {
		if record.Home != projectHome(base, record.Root) || filepath.Base(record.Home) != key {
			return nil, projectFailure("registry identity mismatch")
		}
		if s.project != nil && record.Root != s.project.Root && (within(record.Root, s.project.Root) || within(s.project.Root, record.Root)) {
			return nil, projectFailure("nested STYLE projects are not supported")
		}
		if err = inputs.add(filepath.Join(record.Home, "style.lock.yaml")); err != nil {
			return nil, err
		}
		lock, e := loadLock(record.Home)
		if e != nil || lock.Project == nil || lock.Project.Root != record.Root || len(lock.Packages) == 0 {
			return nil, projectFailure("registered project is missing or inconsistent; restore it before management")
		}
		expected := recordFor(lock.Project, lock)
		if !reflect.DeepEqual(record, expected) {
			return nil, projectFailure("registry is stale; restore it before management")
		}
	}
	return inputs, nil
}
func recordFor(binding *projectBinding, lock lockDocument) projectRecord {
	r := projectRecord{Root: binding.Root, Home: binding.Home, Harnesses: binding.Harnesses, Packages: []string{}}
	for _, item := range lock.Packages {
		r.Packages = append(r.Packages, item.Name)
	}
	sort.Strings(r.Packages)
	return r
}
func (s *Service) projectCollision(home string, lock lockDocument, inputs *managementInputs) error {
	base := home
	if s.project != nil {
		base = s.project.Base
	}
	base, err := canonicalFuturePath(base)
	if err != nil {
		return err
	}
	registry, err := readRegistry(base)
	if err != nil {
		return err
	}
	var others []lockDocument
	if s.project != nil {
		user, e := loadLock(base)
		if e != nil {
			return e
		}
		if user.Project != nil {
			return projectFailure("user root contains a project lock")
		}
		others = append(others, user)
	} else {
		for _, record := range registry.Projects {
			other, e := loadLock(record.Home)
			if e != nil {
				return e
			}
			others = append(others, other)
		}
	}
	for _, other := range others {
		for id, h := range lock.Harnesses {
			old, ok := other.Harnesses[id]
			if !ok {
				continue
			}
			left, err := canonicalFuturePath(old.ConfigDir)
			if err != nil {
				return err
			}
			right, err := canonicalFuturePath(h.ConfigDir)
			if err != nil {
				return err
			}
			if left != right {
				continue
			}
			for _, pkg := range lock.Packages {
				if _, ok := findLockedPackage(other, pkg.Name); ok {
					return projectFailure(fmt.Sprintf("package %q is already active for %s in another scope; uninstall it there or rename the package", pkg.Name, id))
				}
			}
		}
	}
	return nil
}

func (s *Service) projectFiles(home string, old lockDocument, next *lockDocument, adapters []installAdapter, compilation installCompilation, files []install.PlannedFile, inputs *managementInputs) ([]install.PlannedFile, error) {
	if err := s.projectCollision(home, *next, inputs); err != nil {
		return nil, err
	}
	if s.project == nil {
		if next.Harnesses["claude-code"].Route == claude.PluginRouteName {
			next.Format = projectLockFormat
		} else if next.Format == projectLockFormat && next.Project == nil {
			next.Format = lockFormat
			for _, pkg := range next.Packages {
				if pkg.Source.Git != nil {
					next.Format = gitLockFormat
				}
			}
		}
		return files, nil
	}
	p := s.project
	user, err := loadLock(p.Base)
	if err != nil {
		return nil, err
	}
	for _, target := range adapters {
		config := target.installation.ConfigDir
		read := func(path string) ([]byte, error) {
			if err := install.CheckPath(config, path); err != nil {
				return nil, err
			}
			if err := inputs.add(path); err != nil {
				return nil, err
			}
			return saferead.ReadFile(path, 16<<20)
		}
		owned, err := install.UserOwner(target.provider, config, "style", read)
		if err != nil {
			return nil, err
		}
		if owned {
			known := user.Harnesses[string(target.provider)].ConfigDir
			canonical, err := canonicalFuturePath(known)
			if err != nil || len(user.Packages) == 0 || known == "" || canonical != config {
				return nil, projectFailure("harness has a user STYLE owner outside this base home; select the original STYLE home or uninstall that owner first")
			}
		}
	}
	binding, err := bindingFor(p, *next)
	if err != nil {
		return nil, err
	}
	next.Project = binding
	next.Format = projectLockFormat
	registry, err := readRegistry(p.Base)
	if err != nil {
		return nil, err
	}
	declaration, err := readProjectDeclaration(p.Root)
	if err != nil {
		return nil, err
	}
	removing := len(next.Packages) == 0
	if removing {
		cloudRemovals, e := s.removeCloudExport(home, inputs)
		if e != nil {
			return nil, e
		}
		files = append(files, cloudRemovals...)
		delete(registry.Projects, filepath.Base(home))
		for _, name := range []string{projectDeclarationPath, projectLauncherPath} {
			removals, e := plannedRemoval(filepath.Join(p.Root, name))
			if e != nil {
				return nil, e
			}
			files = append(files, removals...)
		}
		removals, e := plannedRemoval(filepath.Join(home, "binding.json"))
		if e != nil {
			return nil, e
		}
		files = append(files, removals...)
	} else {
		registry.Projects[filepath.Base(home)] = recordFor(binding, *next)
		launcher := filepath.Join(p.Root, projectLauncherPath)
		before, e := readOptionalBytes(launcher)
		if e != nil {
			return nil, e
		}
		if before != nil && !bytes.Equal(before, []byte(projectLauncher)) {
			return nil, projectFailure("foreign or modified project launcher")
		}
		launchFiles, e := planProjectFile(p.Root, projectLauncherPath, []byte(projectLauncher), 0o755)
		if e != nil {
			return nil, e
		}
		files = append(files, launchFiles...)
		effectBytes, e := json.Marshal(struct {
			Index  any `json:"index"`
			Static any `json:"static"`
			Native any `json:"native"`
		}{compilation.index, compilation.staticBodies, compilation.nativeReports("")})
		if e != nil {
			return nil, e
		}
		order := map[string]int{}
		for _, pkg := range next.Packages {
			order[pkg.Name] = pkg.Seq
		}
		desired := projectDeclaration{Effects: hashBytes(effectBytes), PackageOrder: order, Format: 1, EntryVersion: projectEntryVersion, Artifacts: map[string]string{projectLauncherPath: hashBytes([]byte(projectLauncher))}, Static: map[string]map[string]string{}}
		overlay := func(path string) ([]byte, error) {
			for _, f := range files {
				if filepath.Clean(f.Path) == filepath.Clean(path) {
					if f.Remove {
						return nil, os.ErrNotExist
					}
					return f.After, nil
				}
			}
			return saferead.ReadFile(path, 16<<20)
		}
		for _, target := range adapters {
			digester, ok := target.route.(interface {
				ProjectDigest(func(string) ([]byte, error)) (map[string]string, error)
			})
			if !ok {
				return nil, projectFailure("route lacks project identity")
			}
			hashes, e := digester.ProjectDigest(overlay)
			if e != nil {
				return nil, e
			}
			for path, digest := range hashes {
				rel, e := filepath.Rel(p.Root, path)
				if e != nil || !validProjectArtifact(filepath.ToSlash(rel)) {
					return nil, projectFailure("renderer artifact escaped project")
				}
				desired.Artifacts[filepath.ToSlash(rel)] = digest
			}
			static := next.Harnesses[string(target.provider)].StaticFiles
			if len(static) > 0 {
				desired.Static[string(target.provider)] = static
				for name, digest := range static {
					rel := ".claude/" + name
					if !validProjectArtifact(rel) {
						return nil, projectFailure("invalid static artifact")
					}
					desired.Artifacts[rel] = digest
				}
			}
		}
		if len(desired.Static) == 0 {
			desired.Static = nil
		}
		if len(old.Packages) == 0 && declaration != nil {
			for _, target := range adapters {
				digester := target.route.(interface {
					ProjectDigest(func(string) ([]byte, error)) (map[string]string, error)
				})
				actual, e := digester.ProjectDigest(os.ReadFile)
				if e != nil {
					return nil, projectFailure("clone is missing a declared hook artifact")
				}
				for path, digest := range actual {
					rel, _ := filepath.Rel(p.Root, path)
					if declaration.Artifacts[filepath.ToSlash(rel)] != digest {
						return nil, projectFailure("clone hook definitions differ from the declaration")
					}
				}
			}
		}
		if len(old.Packages) == 0 && declaration != nil && !reflect.DeepEqual(*declaration, desired) {
			return nil, projectFailure("new clone does not exactly reproduce declared STYLE effects; provide all original sources and review again")
		}
		if len(old.Packages) == 0 && declaration == nil {
			// Existing STYLE hooks without a declaration belong to another installation.
			for _, target := range adapters {
				digester := target.route.(interface {
					ProjectDigest(func(string) ([]byte, error)) (map[string]string, error)
				})
				hashes, e := digester.ProjectDigest(os.ReadFile)
				if e != nil && !errors.Is(e, os.ErrNotExist) {
					return nil, e
				}
				for path, digest := range hashes {
					if emptyProjectDigest(digest) {
						continue
					}
					if _, e := os.Stat(path); e == nil {
						return nil, projectFailure("existing STYLE project handlers have no declaration")
					}
				}
			}
		}
		data, e := json.MarshalIndent(desired, "", "  ")
		if e != nil {
			return nil, e
		}
		data = append(data, '\n')
		binding.Declaration = hashBytes(data)
		more, e := planProjectFile(p.Root, projectDeclarationPath, data, 0o644)
		if e != nil {
			return nil, e
		}
		files = append(files, more...)
		data, e = json.Marshal(binding)
		if e != nil {
			return nil, e
		}
		more, e = planPrivateFile(filepath.Join(home, "binding.json"), data)
		if e != nil {
			return nil, e
		}
		files = append(files, more...)
	}
	data, err := json.Marshal(registry)
	if err != nil {
		return nil, err
	}
	more, err := planPrivateFile(filepath.Join(p.Base, "project-registry.json"), data)
	if err != nil {
		return nil, err
	}
	files = append(files, more...)
	return files, nil
}
func emptyProjectDigest(hash string) bool { return hash == hashBytes([]byte("{}")) }
func planProjectFile(root, name string, data []byte, mode os.FileMode) ([]install.PlannedFile, error) {
	path := filepath.Join(root, name)
	if err := install.CheckPath(root, path); err != nil {
		return nil, err
	}
	before, err := readOptionalBytes(path)
	if err != nil {
		return nil, err
	}
	if before != nil && bytes.Equal(before, data) {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode().Perm() == mode {
			return nil, nil
		}
	}
	return []install.PlannedFile{{Root: root, Path: path, Before: before, After: data, Mode: mode}}, nil
}

// All collaborating management processes acquire shared roots in path order.
type managementClosers struct{ guards []*managementGuard }

func (g *managementClosers) close() error {
	var result error
	for i := len(g.guards) - 1; i >= 0; i-- {
		result = errors.Join(result, g.guards[i].close())
	}
	return result
}
func acquireInputs(ctx context.Context, inputs *managementInputs, home string) (*managementClosers, error) {
	paths := append([]string{home}, inputs.guardHomes...)
	for i, path := range paths {
		canonical, err := canonicalFuturePath(path)
		if err != nil {
			return nil, err
		}
		paths[i] = canonical
	}
	sort.Strings(paths)
	g := &managementClosers{}
	last := ""
	for _, path := range paths {
		if path == last {
			continue
		}
		last = path
		guard, err := acquireManagement(ctx, path)
		if err != nil {
			return nil, errors.Join(err, g.close())
		}
		g.guards = append(g.guards, guard)
	}
	return g, nil
}

func approvalProjectIdentity(binding *projectBinding) *projectBinding {
	if binding == nil {
		return nil
	}
	copy := *binding
	copy.Declaration = ""
	return &copy
}

func (s *Service) projectSequence(name string, fallback int) (int, error) {
	if s.project == nil {
		return fallback, nil
	}
	d, err := readProjectDeclaration(s.project.Root)
	if err != nil {
		return 0, err
	}
	if d != nil {
		if seq, ok := d.PackageOrder[name]; ok {
			return seq, nil
		}
	}
	return fallback, nil
}
