package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/warn"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/internal/tokenestimate"
)

func appendPlacement(result *Result, pkg *spec.ResolvedPackage, info PackageInfo, rule spec.Rule, planned plan.RulePlan, placement plan.Placement, harness string, context tokenestimate.Context) error {
	if placement.Status == plan.Skipped || placement.Status == plan.Never || placement.Status == plan.SkippedM2 || placement.Status == plan.InstallError {
		return nil
	}
	order, err := ruleOrder(rule, info.Seq)
	if err != nil {
		return err
	}
	id := text(rule.Effective["id"])
	ref := info.Name + "/" + id
	source := rule.BodySource
	if source == "" {
		source = rule.Source
	}
	bodyHash := digest([]byte(rule.Body))
	sourceHash := sourceDigest(pkg.Package.Files, source)
	if sourceHash == "" {
		return fmt.Errorf("source bytes for %q are unavailable", source)
	}
	if placement.StaticSlot != "" {
		if placement.Event != "" || !knownStaticSlot(placement.StaticSlot) {
			return fmt.Errorf("placement for %q uses invalid static slot %q", id, placement.StaticSlot)
		}
		body, err := compileBody(rule.Body, context)
		if err != nil {
			return err
		}
		if body.Static == nil {
			return fmt.Errorf("static placement %q has a dynamic body", id)
		}
		staticText := *body.Static
		if placement.StaticFallback {
			staticText = placement.StaticPrefix + staticText
		}
		estimate := tokens.EstimateText(staticText)
		result.Index.Static[harness] = append(result.Index.Static[harness], StaticEntry{Rule: ref, Slot: placement.StaticSlot, Source: source, BodySHA256: bodyHash, Estimate: estimate})
		result.StaticBodies = append(result.StaticBodies, StaticBody{Harness: harness, Rule: ref, Slot: placement.StaticSlot, Mode: staticMode(rule, placement), Body: staticText, Order: order})
		return nil
	}
	if placement.Event == "" {
		return fmt.Errorf("placement for %q has neither event nor static slot", id)
	}
	if !knownEvent(placement.Event) {
		return fmt.Errorf("placement for %q uses unknown event %q", id, placement.Event)
	}
	body, err := compileBody(rule.Body, context)
	if err != nil {
		return err
	}
	entry, err := makeEntry(pkg, rule, planned, placement, order, ref, source, sourceHash, body, bodyHash, context)
	if err != nil {
		return err
	}
	result.Index.Harnesses[harness][placement.Event] = append(result.Index.Harnesses[harness][placement.Event], entry)
	return nil
}

func knownEvent(event string) bool {
	switch event {
	case plan.SessionStart, plan.PromptSubmitted, plan.SkillExplicit, plan.ToolPre,
		plan.ToolPost, plan.ToolError, plan.SubagentStart:
		return true
	default:
		return false
	}
}

func knownStaticSlot(slot string) bool {
	switch slot {
	case "developer_instructions", "model_instructions_file", "claude_md":
		return true
	default:
		return false
	}
}

func makeEntry(pkg *spec.ResolvedPackage, rule spec.Rule, planned plan.RulePlan, placement plan.Placement, order Order, ref, source, sourceHash string, body Body, bodyHash string, context tokenestimate.Context) (Entry, error) {
	effective := rule.Effective
	whenRule, err := normalizedWhen(effective, placement.Event)
	if err != nil {
		return Entry{}, err
	}
	match, err := normalizedMatch(effective, placement.Event)
	if err != nil {
		return Entry{}, err
	}
	repeat := placement.Repeat
	if repeat == "" && placement.Action != "block" {
		repeat = text(effective["repeat"])
	}
	if placement.Action == "block" {
		repeat = "every"
	}
	if repeat == "" {
		repeat = "session"
	}
	entry := Entry{Rule: ref, Entry: ref + "@" + placement.Event, Source: source, SourceSHA256: sourceHash,
		BodySHA256: bodyHash, Order: order, Action: placement.Action, Match: match, When: whenRule,
		Repeat: repeat, ReinjectOnCompact: boolValue(effective["reinject_on_compact"]),
		Reevaluate: placement.Reevaluate, Needs: sortedStrings(planned.Needs), Body: body,
		Estimate: tokens.Estimate{}, DegradedFrom: optionalString(placement.DegradedFrom)}
	if placement.Event == plan.SessionStart {
		sources, err := stringsValue(effective["sources"])
		if err != nil {
			return Entry{}, fmt.Errorf("sources: %w", err)
		}
		if boolValue(effective["reinject_on_compact"]) && !contains(sources, "compact") {
			sources = append(sources, "compact")
		}
		entry.Sources = &sources
	}
	if placement.Action == "warn" {
		modelSource, explicit := effective["model_text"].(string)
		if !explicit {
			modelSource = rule.Body
		}
		modelBody, err := compileBody(modelSource, context)
		if err != nil {
			return Entry{}, fmt.Errorf("model_text: %w", err)
		}
		if !explicit && modelBody.Static != nil {
			first := warn.FirstSentence(*modelBody.Static)
			modelBody.Static = &first
		} else if !explicit {
			entry.ModelBodyFirstSentence = true
		}
		entry.ModelBody = &modelBody
	}
	estimate, err := tokens.EstimatePlacement(rule, placement, pkg)
	if err != nil {
		return Entry{}, err
	}
	entry.Estimate = estimate
	return entry, nil
}

func staticMode(rule spec.Rule, placement plan.Placement) string {
	if placement.StaticFallback {
		return "append"
	}
	if mode := text(rule.Effective["mode"]); mode != "" {
		return mode
	}
	return "append"
}

func normalizedWhen(fields map[string]any, event string) (when.Rule, error) {
	raw := stringMap(fields["when"])
	if raw == nil {
		raw = map[string]any{}
	}
	return when.Normalize(raw, event)
}

func normalizedMatch(fields map[string]any, event string) (*Match, error) {
	if event != plan.ToolPre && event != plan.ToolPost && event != plan.ToolError && event != plan.SkillExplicit {
		return nil, nil
	}
	target := text(fields["target"])
	match := &Match{}
	var err error
	switch target {
	case "tool":
		match.Canonical, err = stringsValue(fields["tool"])
		if err == nil {
			match.Names, err = stringsValue(fields["names"])
		}
	case "mcp":
		values, valueErr := stringsValue(fields["mcp"])
		err = valueErr
		for _, value := range values {
			server, tool, ok := strings.Cut(value, "/")
			if !ok || server == "" || tool == "" {
				return nil, fmt.Errorf("mcp selector %q must be server/tool", value)
			}
			match.MCP = append(match.MCP, MCPSelector{Server: server, Tool: tool})
		}
	case "skill":
		match.Skill, err = stringsValue(fields["skill"])
		for i := range match.Skill {
			match.Skill[i] = strings.ToLower(match.Skill[i])
		}
	}
	if err != nil {
		return nil, err
	}
	if target == "skill" {
		sort.Strings(match.Skill)
	}
	return match, nil
}

func sourceDigest(files map[string][]byte, source string) string {
	pathName := source
	if before, _, ok := strings.Cut(source, "#"); ok {
		pathName = before
	}
	data, ok := files[path.Clean(pathName)]
	if !ok {
		return ""
	}
	return digest(data)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func boolValue(value any) bool { result, _ := value.(bool); return result }

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
