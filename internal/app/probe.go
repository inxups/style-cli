package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/probe"
	"github.com/inxups/style-cli/internal/saferead"
	"github.com/speakeasy-api/agenthooks"
)

type ProbeRequest struct {
	Home string
	// Harnesses selects targets; empty or auto selects every locked harness.
	Harnesses []string
	Timeout   time.Duration
	Ready     func(ProbeReady) error
}
type ProbeReady struct {
	RunID   string
	Targets []ProbeReadyTarget
}
type ProbeReadyTarget struct {
	Harness, Route string
	Events         []string
}
type ProbeDiagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Harness  string `json:"harness,omitempty"`
}
type ProbeTarget struct {
	Harness          string     `json:"harness"`
	Route            string     `json:"route"`
	Status           string     `json:"status"`
	CallbackObserved bool       `json:"callback_observed"`
	Origin           string     `json:"origin"`
	RuntimeSHA256    string     `json:"runtime_sha256,omitempty"`
	HarnessSHA256    string     `json:"harness_sha256,omitempty"`
	IndexSHA256      string     `json:"index_sha256,omitempty"`
	IndexStatus      string     `json:"index_status"`
	Execution        string     `json:"execution"`
	Events           []string   `json:"events"`
	Event            string     `json:"event,omitempty"`
	SessionSHA256    string     `json:"session_sha256,omitempty"`
	AgentSHA256      string     `json:"agent_sha256,omitempty"`
	TurnSHA256       string     `json:"turn_sha256,omitempty"`
	ObservedAt       *time.Time `json:"observed_at,omitempty"`
}
type ProbeResult struct {
	Format      int               `json:"format"`
	Command     string            `json:"command"`
	RunID       string            `json:"run_id"`
	Status      string            `json:"status"`
	Timeout     string            `json:"timeout"`
	Targets     []ProbeTarget     `json:"targets"`
	Cleanup     probe.Cleanup     `json:"cleanup"`
	Diagnostics []ProbeDiagnostic `json:"diagnostics"`
}
type probeDependencies struct {
	lookup   probe.ProcessLookup
	lookPath func(string) (string, error)
	now      func() time.Time
	poll     func(context.Context) error
	begin    func(string, []probe.Binding, time.Duration, probe.Options) (*probe.Session, error)
}

func defaultProbeDependencies() probeDependencies {
	return probeDependencies{lookup: agenthooks.ReadProcessIdentity, lookPath: exec.LookPath, now: time.Now, begin: probe.Begin, poll: func(ctx context.Context) error {
		timer := time.NewTimer(50 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}}
}

type probeFile struct {
	path, sha string
	info      os.FileInfo
}
type probeExecutable struct {
	path, id, sha string
	info          os.FileInfo
}
type probeTargetBinding struct {
	binding          probe.Binding
	route            adapter.ProbeRoute
	files            map[string]probeFile
	runtime, harness probeExecutable
}

// Probe waits for callbacks from existing installations. Neither preflight nor
// observation executes a provider command or rewrites installation state.
func (s *Service) Probe(ctx context.Context, request ProbeRequest) (result ProbeResult, runErr error) {
	if request.Timeout == 0 {
		request.Timeout = time.Minute
	}
	result = ProbeResult{Format: 1, Command: "probe", Status: "error", Timeout: request.Timeout.String(), Targets: []ProbeTarget{}, Cleanup: probe.Cleanup{Complete: true, Remaining: []string{}}, Diagnostics: []ProbeDiagnostic{}}
	add := func(code, severity, message, harness string) {
		for _, item := range result.Diagnostics {
			if item.Code == code && item.Harness == harness {
				return
			}
		}
		result.Diagnostics = append(result.Diagnostics, ProbeDiagnostic{code, severity, message, harness})
	}
	fail := func(code, message string) error { add(code, "error", message, ""); return errors.New(code) }
	if err := checkContext(ctx); err != nil {
		return result, fail("probe-interrupted", "Probe was interrupted before observation.")
	}
	if request.Timeout < time.Second || request.Timeout > 10*time.Minute {
		return result, fail("probe-usage", "Timeout must be between 1s and 10m.")
	}
	deps := defaultProbeDependencies()
	if s.probeDeps != nil {
		deps = *s.probeDeps
	}
	home, err := s.resolveStyleHome(request.Home)
	if err != nil {
		return result, fail("probe-not-installed", "Cannot resolve the installation root.")
	}
	homeRoot, err := saferead.Anchor(home)
	if err != nil {
		return result, fail("probe-not-installed", "No readable STYLE installation root.")
	}
	lockBytes, err := saferead.File(homeRoot, "style.lock.yaml", 16<<20)
	homeRoot.Close()
	if err != nil {
		return result, fail("probe-not-installed", "No readable installed STYLE lock.")
	}
	lock, err := decodeLock(lockBytes)
	if err == nil {
		err = s.validateScopeLock(home, lock)
	}
	if err != nil {
		return result, fail("probe-not-installed", "Installed STYLE lock is invalid.")
	}
	selected := []adapter.ProviderID{}
	auto := len(request.Harnesses) == 0
	for _, harness := range request.Harnesses {
		if harness == "claude" {
			harness = "claude-code"
		}
		if harness == "auto" {
			auto = true
			continue
		}
		if _, ok := s.definitions[adapter.ProviderID(harness)]; !ok {
			return result, fail("runtime-unavailable", "Harness is not registered for runtime observation.")
		}
		if !slices.Contains(selected, adapter.ProviderID(harness)) {
			selected = append(selected, adapter.ProviderID(harness))
		}
	}
	if auto {
		for _, id := range s.providers() {
			if lock.Harnesses[string(id)].Route != "" && !slices.Contains(selected, id) {
				selected = append(selected, id)
			}
		}
		// Unknown locked targets cannot be silently omitted from an all-target probe.
		for id, installed := range lock.Harnesses {
			if installed.Route != "" {
				if _, ok := s.definitions[adapter.ProviderID(id)]; !ok {
					return result, fail("runtime-unavailable", "An installed harness has no registered runtime observation adapter.")
				}
			}
		}
	}
	if len(selected) == 0 || len(lock.Packages) == 0 {
		return result, fail("probe-not-installed", "No installed STYLE runtime targets.")
	}
	bindings := []probeTargetBinding{}
	unavailable := false
	for _, id := range selected {
		installed, ok := lock.Harnesses[string(id)]
		target := ProbeTarget{Harness: string(id), Route: installed.Route, Status: "unavailable", Origin: "unverified", IndexStatus: "unavailable", Execution: "not-assessed", Events: []string{}}
		result.Targets = append(result.Targets, target)
		index := len(result.Targets) - 1
		if !ok || installed.Route == "" {
			add("probe-not-installed", "error", "Harness has no locked STYLE installation.", string(id))
			unavailable = true
			continue
		}
		definition := s.definitions[id]
		route := routeByName(definition, installed.Route)
		if route == nil {
			add("runtime-unavailable", "error", "Locked route is unavailable.", string(id))
			unavailable = true
			continue
		}
		route = bindRoute(route, adapter.Installation{Provider: id, Version: installed.Version, ConfigDir: installed.ConfigDir}, home, staticStateFor(lock, id))
		inspector, ok := route.(adapter.ProbeRoute)
		if !ok {
			add("runtime-unavailable", "error", "Route does not support runtime origin verification.", string(id))
			unavailable = true
			continue
		}
		files := map[string]probeFile{}
		capture := func(path string) ([]byte, error) {
			data, file, err := captureProbeFile(path, 16<<20)
			if err == nil {
				files[file.path] = file
			}
			return data, err
		}
		if s.project != nil {
			data, bindingErr := capture(filepath.Join(home, "binding.json"))
			var bound projectBinding
			if bindingErr != nil || json.Unmarshal(data, &bound) != nil || !reflect.DeepEqual(&bound, lock.Project) {
				add("project-binding-invalid", "error", "Project runtime binding is missing or differs from the lock.", string(id))
				unavailable = true
				continue
			}
			declaration, declarationErr := capture(filepath.Join(s.project.Root, projectDeclarationPath))
			launcher, launcherErr := capture(filepath.Join(s.project.Root, projectLauncherPath))
			if declarationErr != nil || hashBytes(declaration) != bound.Declaration || launcherErr != nil || string(launcher) != projectLauncher {
				add("project-binding-invalid", "error", "Project declaration or launcher is missing or changed.", string(id))
				unavailable = true
				continue
			}
		}
		setup, err := inspector.InspectProbe(ctx, capture)
		if err != nil {
			add("probe-installation-unavailable", "error", "Runtime configuration, registration, command or trust binding is unavailable or changed.", string(id))
			unavailable = true
			continue
		}
		result.Targets[index].Events = setup.Events
		if len(setup.Events) == 0 {
			add("probe-no-runtime-hook", "error", "Installation has no STYLE runner subscription.", string(id))
			unavailable = true
			continue
		}
		runtimePath := setup.RuntimeCommand
		if !filepath.IsAbs(runtimePath) {
			runtimePath, err = deps.lookPath(runtimePath)
		}
		runtimeFile, fileErr := captureProbeExecutable(runtimePath, true)
		if err != nil || fileErr != nil {
			add("runtime-unavailable", "error", "Installed runtime lacks the observation capability or a verifiable executable.", string(id))
			unavailable = true
			continue
		}
		if s.project != nil {
			expected, bindingErr := canonicalProjectPath(lock.Project.Runner)
			actual, actualErr := canonicalProjectPath(runtimePath)
			if bindingErr != nil || actualErr != nil || expected != actual {
				add("project-binding-invalid", "error", "PATH runtime differs from the approved project runner.", string(id))
				unavailable = true
				continue
			}
		}
		harnessPath, err := deps.lookPath(setup.HarnessCommand)
		if err != nil {
			add("origin-unverified", "error", "Native harness executable cannot be bound without executing discovery.", string(id))
			unavailable = true
			continue
		}
		harnessFile, err := captureProbeExecutable(harnessPath, false)
		if err != nil {
			add("origin-unverified", "error", "Native harness executable cannot be safely captured.", string(id))
			unavailable = true
			continue
		}
		root, rootErr := filepath.EvalSymlinks(setup.Root)
		configRoot, configErr := filepath.EvalSymlinks(setup.ConfigRoot)
		canonicalHome, homeErr := filepath.EvalSymlinks(home)
		if rootErr != nil || configErr != nil || homeErr != nil {
			add("probe-installation-unavailable", "error", "Route roots cannot be bound.", string(id))
			unavailable = true
			continue
		}
		_, lockFile, err := captureProbeFile(filepath.Join(home, "style.lock.yaml"), 16<<20)
		if err != nil || lockFile.sha != probe.Hash(lockBytes) {
			add("installation-changed", "error", "Installation changed during preflight.", string(id))
			unavailable = true
			continue
		}
		files[lockFile.path] = lockFile
		indexData, indexFile, indexErr := captureProbeFile(filepath.Join(home, "build", "index.json"), 16<<20)
		if indexErr == nil {
			files[indexFile.path] = indexFile
			result.Targets[index].IndexSHA256 = indexFile.sha
			result.Targets[index].IndexStatus = "captured"
			var captured compile.Index
			if json.Unmarshal(indexData, &captured) != nil {
				result.Targets[index].IndexStatus = "invalid"
				add("probe-index-unavailable", "warning", "Index cannot establish execution success; callback observation remains available.", string(id))
			}
		} else {
			add("probe-index-unavailable", "warning", "Index is unavailable; callback observation does not assess rule execution.", string(id))
		}
		names := make([]string, 0, len(files))
		for path := range files {
			names = append(names, path)
		}
		sort.Strings(names)
		parts := []string{}
		for _, path := range names {
			parts = append(parts, probe.HashString(path)+":"+files[path].sha)
		}
		binding := probe.Binding{Harness: string(id), Route: installed.Route, HomeSHA256: probe.HashString(canonicalHome), HarnessHomeSHA256: probe.HashString(configRoot), RouteRootSHA256: probe.HashString(root), RuntimeID: runtimeFile.id, RuntimeSHA256: runtimeFile.sha, HarnessID: harnessFile.id, HarnessSHA256: harnessFile.sha, LockSHA256: probe.Hash(lockBytes), ConfigSHA256: probe.Hash([]byte(strings.Join(parts, "\n"))), Events: setup.Events}
		bindings = append(bindings, probeTargetBinding{binding, inspector, files, runtimeFile, harnessFile})
		result.Targets[index].Status = "waiting"
		result.Targets[index].RuntimeSHA256 = runtimeFile.sha
		result.Targets[index].HarnessSHA256 = harnessFile.sha
	}
	if unavailable {
		return result, errors.New("probe preflight unavailable")
	}
	if _, err = deps.lookup(os.Getpid()); err != nil {
		return result, fail("origin-unverified", "This platform or permission level cannot capture process birth and executable identities.")
	}
	leases := []probe.Binding{}
	for _, target := range bindings {
		leases = append(leases, target.binding)
	}
	session, err := deps.begin(home, leases, request.Timeout, probe.Options{Now: deps.now, Lookup: deps.lookup})
	if err != nil {
		var beginFailure *probe.BeginError
		if errors.As(err, &beginFailure) {
			result.Cleanup = beginFailure.Cleanup
			add("probe-cleanup-failed", "error", "Initialization left this run residual files; let leases expire before identity-checked recovery.", "")
		}
		code := "probe-io-error"
		if errors.Is(err, probe.ErrBusy) {
			code = "probe-busy"
		}
		return result, fail(code, "Cannot acquire all target leases; only this run's acquired leases are rolled back.")
	}
	result.RunID = session.RunID
	defer func() {
		result.Cleanup = session.Cleanup()
		session.Close()
		if !result.Cleanup.Complete {
			result.Status = "error"
			add("probe-cleanup-failed", "error", "This run has residual files; let its lease expire and rerun probe for identity-checked recovery.", "")
			runErr = errors.New("probe cleanup failed")
		}
	}()
	ready := ProbeReady{RunID: session.RunID, Targets: []ProbeReadyTarget{}}
	for _, target := range bindings {
		ready.Targets = append(ready.Targets, ProbeReadyTarget{target.binding.Harness, target.binding.Route, append([]string{}, target.binding.Events...)})
	}
	if request.Ready != nil {
		if err = request.Ready(ready); err != nil {
			return result, fail("probe-io-error", "Cannot publish the observation readiness notice.")
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	for {
		if ctx.Err() != nil {
			result.Status = "interrupted"
			return result, fail("probe-interrupted", "Probe was interrupted; observed targets are preserved.")
		}
		observed := 0
		for index, target := range bindings {
			if !session.Active(target.binding.Harness) {
				return result, fail("probe-lease-changed", "Probe lease identity changed during observation.")
			}
			if !probeBindingUnchanged(target, false) {
				result.Targets[index].Status = "installation-changed"
				return result, fail("installation-changed", "Installation binding changed during observation.")
			}
			if result.Targets[index].CallbackObserved {
				observed++
				continue
			}
			receipts, quality := session.Receipts(target.binding.Harness)
			for _, code := range quality {
				add(code, "warning", "Invalid, excessive or unreadable receipts were excluded from observation.", target.binding.Harness)
			}
			for _, receipt := range receipts {
				if deps.now().Before(receipt.ObservedAt) {
					add("probe-receipt-invalid", "warning", "A receipt outside the current observation window was excluded.", target.binding.Harness)
					continue
				}
				code := verifyProbeOrigin(target, receipt, deps.lookup)
				if code != "" {
					result.Targets[index].Status = "unverified"
					add(code, "warning", "Callback origin or runtime identity could not be verified.", target.binding.Harness)
					continue
				}
				result.Targets[index].Status = "callback-observed"
				result.Targets[index].CallbackObserved = true
				result.Targets[index].Origin = "process-linked"
				result.Targets[index].Event = receipt.Event
				result.Targets[index].SessionSHA256 = receipt.SessionSHA256
				result.Targets[index].AgentSHA256 = receipt.AgentSHA256
				result.Targets[index].TurnSHA256 = receipt.TurnSHA256
				at := receipt.ObservedAt
				result.Targets[index].ObservedAt = &at
				observed++
				break
			}
		}
		if observed == len(bindings) {
			for index, target := range bindings {
				if !probeBindingUnchanged(target, true) {
					result.Targets[index].Status = "installation-changed"
					return result, fail("installation-changed", "Executable or installation binding changed before completion.")
				}
			}
			if ctx.Err() != nil {
				result.Status = "interrupted"
				return result, fail("probe-interrupted", "Probe was interrupted before completion.")
			}
			result.Status = "observed"
			return result, nil
		}
		expired := false
		for _, target := range bindings {
			if !deps.now().Before(session.Lease(target.binding.Harness).Expires) {
				expired = true
			}
		}
		if waitCtx.Err() != nil || expired {
			// Parent cancellation can arrive during receipt/configuration reads,
			// after the loop's first check. It is not an observation timeout.
			if ctx.Err() != nil {
				result.Status = "interrupted"
				return result, fail("probe-interrupted", "Probe was interrupted; observed targets are preserved.")
			}
			result.Status = "timed-out"
			if observed > 0 {
				result.Status = "partial"
			}
			for index := range result.Targets {
				if result.Targets[index].Status == "waiting" {
					result.Targets[index].Status = "timed-out"
				}
			}
			return result, fail("probe-timeout", "The observation window ended before every target had a verified callback.")
		}
		if err = deps.poll(waitCtx); err != nil {
			if ctx.Err() != nil {
				result.Status = "interrupted"
				return result, fail("probe-interrupted", "Probe was interrupted; observed targets are preserved.")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				return result, fail("probe-io-error", "Observation polling failed.")
			}
		}
	}
}

func captureProbeFile(path string, limit int64) ([]byte, probeFile, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, probeFile{}, err
	}
	before, err := os.Lstat(absolute)
	if err != nil {
		return nil, probeFile{}, err
	}
	data, err := saferead.ReadFile(absolute, limit)
	if err != nil {
		return nil, probeFile{}, err
	}
	after, err := os.Lstat(absolute)
	if err != nil || !saferead.Same(before, after) {
		return nil, probeFile{}, saferead.ErrChanged
	}
	return data, probeFile{absolute, probe.Hash(data), after}, nil
}
func captureProbeExecutable(path string, capability bool) (probeExecutable, error) {
	if path == "" {
		return probeExecutable{}, errors.New("no executable")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return probeExecutable{}, err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return probeExecutable{}, err
	}
	data, file, err := captureProbeFile(canonical, 256<<20)
	if err != nil {
		return probeExecutable{}, err
	}
	if runtime.GOOS != "windows" && file.info.Mode().Perm()&0o111 == 0 || runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(canonical), ".exe") {
		return probeExecutable{}, errors.New("executable required")
	}
	if capability && !bytes.Contains(data, []byte(probe.Protocol)) {
		return probeExecutable{}, errors.New("runtime observer unavailable")
	}
	id, err := agenthooks.ExecutableIdentity(canonical)
	if err != nil {
		return probeExecutable{}, err
	}
	return probeExecutable{canonical, id, file.sha, file.info}, nil
}
func probeBindingUnchanged(target probeTargetBinding, full bool) bool {
	for _, file := range target.files {
		_, current, err := captureProbeFile(file.path, 16<<20)
		if err != nil || current.sha != file.sha || !saferead.Same(current.info, file.info) {
			return false
		}
	}
	for _, exe := range []probeExecutable{target.runtime, target.harness} {
		id, err := agenthooks.ExecutableIdentity(exe.path)
		if err != nil || id != exe.id {
			return false
		}
		if full {
			current, err := captureProbeExecutable(exe.path, false)
			if err != nil || current.sha != exe.sha {
				return false
			}
		}
	}
	return true
}
func verifyProbeOrigin(target probeTargetBinding, receipt probe.Receipt, lookup probe.ProcessLookup) string {
	if len(receipt.Processes) < 2 {
		return "origin-unverified"
	}
	if receipt.Processes[0].ExecutableID != target.binding.RuntimeID {
		return "runtime-mismatch"
	}
	if !target.route.VerifyProbeOrigin(adapter.ProbeOrigin{PluginRootSHA256: receipt.PluginRootSHA256, RouteRootSHA256: target.binding.RouteRootSHA256}) {
		return "origin-unverified"
	}
	for _, process := range receipt.Processes[1:] {
		if process.ExecutableID != target.binding.HarnessID {
			continue
		}
		live, err := lookup(process.PID)
		if err == nil && live.PID == process.PID && live.Birth == process.Birth && live.ExecutableID == process.ExecutableID {
			return ""
		}
	}
	return "origin-unverified"
}
