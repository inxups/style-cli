package compile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/tokenestimate"
)

// Build compiles validated, merged packages using their already selected plan
// placements. It neither reruns fallback policy nor reads files from disk.
// Blocked or mismatched plans are rejected before an index can be installed.
func Build(request Request) (Result, error) {
	if strings.TrimSpace(request.StyleVersion) == "" {
		return Result{}, fmt.Errorf("compile: style version is required")
	}
	result := Result{Index: Index{Format: Format, StyleVersion: request.StyleVersion,
		Packages: make([]PackageInfo, 0, len(request.Packages)),
		Static:   make(map[string][]StaticEntry), Harnesses: make(map[string]map[string][]Entry)}}
	names, sequences := make(map[string]bool), make(map[int]bool)
	for _, input := range request.Packages {
		if input.Package == nil || input.Package.Package == nil {
			return Result{}, fmt.Errorf("compile: resolved package is required")
		}
		metadata := input.Package.Package.Metadata
		info := PackageInfo{Name: text(metadata["name"]), Version: text(metadata["version"]),
			Seq: input.Sequence, Lock: input.Lock, Link: input.Link}
		if info.Name == "" || info.Version == "" || info.Seq <= 0 || info.Lock == "" {
			return Result{}, fmt.Errorf("compile: package name, version, positive sequence and lock are required")
		}
		if names[info.Name] || sequences[info.Seq] {
			return Result{}, fmt.Errorf("compile: duplicate package name or installation sequence")
		}
		names[info.Name], sequences[info.Seq] = true, true
		result.Index.Packages = append(result.Index.Packages, info)
		pkg, err := withSettings(input.Package, input.Settings)
		if err != nil {
			return Result{}, fmt.Errorf("compile: package %q: %w", info.Name, err)
		}
		seenHarnesses := make(map[string]bool)
		for _, report := range input.Plans {
			if err := checkPlan(pkg, report, seenHarnesses); err != nil {
				return Result{}, err
			}
			seenHarnesses[report.Adapter] = true
			if result.Index.Harnesses[report.Adapter] == nil {
				result.Index.Harnesses[report.Adapter] = make(map[string][]Entry)
				result.Index.Static[report.Adapter] = make([]StaticEntry, 0)
			}
			for i, rule := range pkg.Rules {
				context := tokenestimate.Context{Package: pkg.Package.Metadata, Rule: rule.Effective}
				for _, placement := range report.Rules[i].Placements {
					if err := appendPlacement(&result, pkg, info, rule, report.Rules[i], placement, report.Adapter, context); err != nil {
						return Result{}, fmt.Errorf("compile: %s/%s: %w", info.Name, text(rule.Effective["id"]), err)
					}
				}
			}
		}
	}
	sortResult(&result)
	return result, nil
}

func checkPlan(pkg *spec.ResolvedPackage, report plan.Report, seen map[string]bool) error {
	name := text(pkg.Package.Metadata["name"])
	if report.Package != name || strings.TrimSpace(report.Adapter) == "" || seen[report.Adapter] {
		return fmt.Errorf("compile: mismatched or duplicate harness plan for package %q", name)
	}
	if report.Blocked || len(pkg.Rules) != len(report.Rules) {
		return fmt.Errorf("compile: blocked or incomplete plan for package %q", name)
	}
	ids, indices := make(map[string]bool), make(map[int]bool)
	for i, rule := range pkg.Rules {
		id := text(rule.Effective["id"])
		action := text(rule.Effective["action"])
		if action == "" {
			action = "inject"
		}
		target := text(rule.Effective["target"])
		planned := report.Rules[i]
		if id == "" || ids[id] || rule.Index < 0 || indices[rule.Index] || planned.ID != id ||
			planned.Source != rule.Source || planned.Target != target || planned.Action != action || planned.Fatal ||
			!knownStatus(planned.Status) || planned.Status == plan.InstallError {
			return fmt.Errorf("compile: plan rule %d does not match resolved package %q", i, name)
		}
		for _, placement := range planned.Placements {
			if !knownStatus(placement.Status) || !knownAction(placement.Action) {
				return fmt.Errorf("compile: invalid placement for rule %q in package %q", id, name)
			}
		}
		ids[id], indices[rule.Index] = true, true
	}
	return nil
}

func knownStatus(status plan.Status) bool {
	switch status {
	case plan.Native, plan.Degraded, plan.Partial, plan.Skipped, plan.Never, plan.SkippedM2, plan.InstallError:
		return true
	default:
		return false
	}
}

func knownAction(action string) bool {
	switch action {
	case "inject", "warn", "block":
		return true
	default:
		return false
	}
}

func sortResult(result *Result) {
	sort.Slice(result.Index.Packages, func(i, j int) bool { return result.Index.Packages[i].Seq < result.Index.Packages[j].Seq })
	for _, events := range result.Index.Harnesses {
		for event, entries := range events {
			sort.SliceStable(entries, func(i, j int) bool {
				a, b := entries[i], entries[j]
				if lessOrder(a.Order, b.Order) {
					return true
				}
				if lessOrder(b.Order, a.Order) {
					return false
				}
				return a.Entry < b.Entry
			})
			events[event] = entries
		}
	}
	sort.SliceStable(result.StaticBodies, func(i, j int) bool {
		a, b := result.StaticBodies[i], result.StaticBodies[j]
		if a.Harness != b.Harness {
			return a.Harness < b.Harness
		}
		if lessOrder(a.Order, b.Order) {
			return true
		}
		if lessOrder(b.Order, a.Order) {
			return false
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Slot != b.Slot {
			return a.Slot < b.Slot
		}
		if a.Mode != b.Mode {
			return a.Mode < b.Mode
		}
		return a.Body < b.Body
	})
	for harness := range result.Index.Static {
		orders := make(map[string]Order)
		for _, body := range result.StaticBodies {
			if body.Harness == harness {
				orders[body.Rule] = body.Order
			}
		}
		entries := result.Index.Static[harness]
		sort.SliceStable(entries, func(i, j int) bool {
			a, b := entries[i], entries[j]
			if lessOrder(orders[a.Rule], orders[b.Rule]) {
				return true
			}
			if lessOrder(orders[b.Rule], orders[a.Rule]) {
				return false
			}
			if a.Rule != b.Rule {
				return a.Rule < b.Rule
			}
			if a.Slot != b.Slot {
				return a.Slot < b.Slot
			}
			if a.Source != b.Source {
				return a.Source < b.Source
			}
			return a.BodySHA256 < b.BodySHA256
		})
		result.Index.Static[harness] = entries
	}
}

func lessOrder(a, b Order) bool {
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if a.PackageSeq != b.PackageSeq {
		return a.PackageSeq < b.PackageSeq
	}
	return a.RuleIndex < b.RuleIndex
}
