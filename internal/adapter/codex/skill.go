package codex

import "regexp"

var explicitSkillPattern = regexp.MustCompile(`(^|[^A-Za-z0-9_$])\$([A-Za-z0-9][A-Za-z0-9_-]*)`)

// ExplicitSkills returns the skill names explicitly referenced in a prompt.
// Names are returned in first-occurrence order, with duplicates removed.
func ExplicitSkills(prompt string) []string {
	if prompt == "" {
		return nil
	}

	matches := explicitSkillPattern.FindAllStringSubmatch(prompt, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		name := match[2]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}
