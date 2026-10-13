package when

import (
	"path"
	"strings"
)

func validateModelGlob(pattern string) error {
	_, err := path.Match(strings.TrimPrefix(pattern, "**/"), "")
	return err
}

func anyModelMatches(patterns []string, model string) bool {
	for _, pattern := range patterns {
		if matchModel(pattern, model) {
			return true
		}
	}
	return false
}

// matchModel uses path glob semantics (* never crosses /), with STYLE's
// leading **/ extension allowing zero or more complete path segments.
func matchModel(pattern, model string) bool {
	pattern, model = strings.ToLower(pattern), strings.ToLower(model)
	pattern, recursive := strings.CutPrefix(pattern, "**/")
	for {
		if matched, err := path.Match(pattern, model); err == nil && matched {
			return true
		}
		if !recursive {
			return false
		}
		_, remaining, hasSlash := strings.Cut(model, "/")
		if !hasSlash {
			return false
		}
		model = remaining
	}
}
