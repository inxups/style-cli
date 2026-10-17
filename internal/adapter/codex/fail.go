package codex

import "github.com/speakeasy-api/agenthooks"

// FailurePoints is a conservative mapping available before session/state I/O.
func (*Enricher) FailurePoints(event *agenthooks.Event) []string {
	if event == nil {
		return nil
	}
	switch event.Kind {
	case agenthooks.KindToolPre, agenthooks.KindPermission:
		return []string{"tool.pre"}
	case agenthooks.KindPromptSubmitted:
		return []string{"prompt.submitted", "skill.explicit"}
	case agenthooks.KindPromptExpansion:
		return []string{"skill.explicit"}
	}
	return nil
}
