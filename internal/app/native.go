package app

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/adapter"
)

type compiledNative struct {
	Hook  adapter.HookSpec
	Order compile.Order
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
			compilation.native[target.provider] = append(compilation.native[target.provider], compiledNative{Hook: hook,
				Order: compile.Order{Priority: priority, PackageSeq: pkg.lock.Seq, RuleIndex: rule.Index}})
		}
	}
	return nil
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
