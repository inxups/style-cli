package plan

import (
	"fmt"

	"github.com/inxups/style-cli/core/spec"
)

// planNative resolves a target: native rule for one adapter (rules §3.10).
// Native rules have no degradation ladder: a missing mapping is skipped
// regardless of fallback, and an unwritable mapping is skipped or, with
// fallback: error, blocks installation.
func planNative(hook spec.NativeHook, adapter Adapter) NativeHookPlan {
	result := NativeHookPlan{ID: stringValue(hook.Effective["id"]), Source: hook.Source}

	var mapping *spec.NativeMapping
	for _, candidate := range spec.NativeMappings(hook) {
		if candidate.Harness == adapter.Name() {
			candidate := candidate
			mapping = &candidate
			break
		}
	}
	if mapping == nil {
		result.Status = Skipped
		result.Reasons = []string{"no native mapping for this harness"}
		return result
	}
	mapping.HookID = result.ID
	var err error
	if native, ok := adapter.(NativeAdapter); ok {
		var hook NativeEntry
		if hook, err = native.NativeHook(*mapping); err == nil {
			result.Status = NativeHook
			result.Native = &hook
			return result
		}
	} else {
		err = fmt.Errorf("adapter %s does not support native hooks", adapter.Name())
	}
	if stringValue(hook.Effective["fallback"]) == "error" {
		result.Status = InstallError
		result.Fatal = true
		result.Reasons = []string{err.Error() + "; fallback is error"}
		return result
	}
	result.Status = Skipped
	result.Reasons = []string{err.Error() + "; native rules have no degradation ladder"}
	return result
}
