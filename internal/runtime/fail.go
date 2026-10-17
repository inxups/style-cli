package runtime

import "github.com/speakeasy-api/agenthooks"

func (r *Runtime) ruleFailClosed(event *agenthooks.Event) bool {
	if r == nil || event == nil || r.loadErr != nil || r.InitializationError != nil || r.Engine == nil {
		return false
	}
	mapper, ok := r.Adapter.(interface {
		FailurePoints(*agenthooks.Event) []string
	})
	return ok && r.Engine.HasFailClosed(string(event.Provider), mapper.FailurePoints(event)...)
}
