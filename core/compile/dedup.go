package compile

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/tokens"
)

type ruleKey struct{ harness, rule string }

type equivalentPlacement struct {
	Event        string
	Slot         string
	Mode         string
	Text         string
	Runtime      *Entry
	Capabilities []plan.Capability
}

type dedupCandidate struct {
	key        ruleKey
	order      Order
	placements []equivalentPlacement
}

// deduplicate compares complete compiled rules, rather than templates or
// individual events. Hashes only select a bucket; full structural equality
// decides equivalence, preserving nil/empty condition semantics as well.
func deduplicate(result *Result, inputs []PackageInput) error {
	candidates := make(map[ruleKey]*dedupCandidate)
	get := func(harness, rule string, order Order) *dedupCandidate {
		key := ruleKey{harness, rule}
		if candidates[key] == nil {
			candidates[key] = &dedupCandidate{key: key, order: order}
		}
		return candidates[key]
	}
	for _, body := range result.StaticBodies {
		candidate := get(body.Harness, body.Rule, body.Order)
		candidate.placements = append(candidate.placements, equivalentPlacement{Slot: body.Slot, Mode: body.Mode, Text: body.Body})
	}
	for harness, events := range result.Index.Harnesses {
		for event, entries := range events {
			for _, entry := range entries {
				candidate := get(harness, entry.Rule, entry.Order)
				entry.Rule, entry.Entry, entry.Source, entry.SourceSHA256, entry.BodySHA256 = "", "", "", "", ""
				entry.Order, entry.Estimate, entry.DegradedFrom = Order{}, tokens.Estimate{}, nil
				candidate.placements = append(candidate.placements, equivalentPlacement{Event: event, Runtime: &entry})
			}
		}
	}
	// Capabilities are planning facts, not index fields. Retain them in the
	// equivalence check so a user-only warning cannot absorb model output.
	for _, input := range inputs {
		for _, report := range input.Plans {
			for _, rule := range report.Rules {
				candidate := candidates[ruleKey{report.Adapter, report.Package + "/" + rule.ID}]
				if candidate == nil {
					continue
				}
				for _, placement := range rule.Placements {
					if placement.Event == "" || !activePlacement(placement) {
						continue
					}
					for i := range candidate.placements {
						if candidate.placements[i].Event == placement.Event {
							candidate.placements[i].Capabilities = placement.Capabilities
						}
					}
				}
			}
		}
	}
	ordered := make([]*dedupCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		sort.SliceStable(candidate.placements, func(i, j int) bool {
			a, b := candidate.placements[i], candidate.placements[j]
			if a.Event != b.Event {
				return a.Event < b.Event
			}
			return a.Slot < b.Slot
		})
		ordered = append(ordered, candidate)
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.key.harness != b.key.harness {
			return a.key.harness < b.key.harness
		}
		return lessOrder(a.order, b.order)
	})
	type bucketKey struct{ harness, hash string }
	groups := make(map[bucketKey][][]*dedupCandidate)
	for _, candidate := range ordered {
		data, err := json.Marshal(candidate.placements)
		if err != nil {
			return fmt.Errorf("compile: compare placements: %w", err)
		}
		key := bucketKey{candidate.key.harness, digest(data)}
		classes := groups[key]
		found := false
		for i, class := range classes {
			if reflect.DeepEqual(class[0].placements, candidate.placements) {
				classes[i] = append(class, candidate)
				found = true
				break
			}
		}
		if !found {
			classes = append(classes, []*dedupCandidate{candidate})
		}
		groups[key] = classes
	}
	removed := make(map[ruleKey]bool)
	for _, classes := range groups {
		for _, class := range classes {
			if len(class) < 2 {
				continue
			}
			packages := make(map[string]bool)
			ambiguous := false
			for _, candidate := range class {
				pkg, _, _ := strings.Cut(candidate.key.rule, "/")
				ambiguous = ambiguous || packages[pkg]
				packages[pkg] = true
			}
			if len(packages) < 2 {
				continue
			}
			if ambiguous {
				for _, candidate := range class {
					result.DeduplicationWarnings = append(result.DeduplicationWarnings, DeduplicationWarning{
						Harness: candidate.key.harness, Rule: candidate.key.rule, Reason: "multiple equivalent rules in one package"})
				}
				continue
			}
			for _, candidate := range class[1:] {
				relation := Deduplication{Harness: candidate.key.harness, Rule: candidate.key.rule, Into: class[0].key.rule}
				for _, placement := range candidate.placements {
					relation.Placements = append(relation.Placements, PlacementRef{Event: placement.Event, StaticSlot: placement.Slot})
				}
				result.Deduplicated = append(result.Deduplicated, relation)
				removed[candidate.key] = true
			}
		}
	}
	sort.Slice(result.Deduplicated, func(i, j int) bool {
		a, b := result.Deduplicated[i], result.Deduplicated[j]
		return a.Harness < b.Harness || a.Harness == b.Harness && a.Rule < b.Rule
	})
	sort.Slice(result.DeduplicationWarnings, func(i, j int) bool {
		a, b := result.DeduplicationWarnings[i], result.DeduplicationWarnings[j]
		return a.Harness < b.Harness || a.Harness == b.Harness && a.Rule < b.Rule
	})
	for harness, events := range result.Index.Harnesses {
		for event, entries := range events {
			kept := entries[:0]
			for _, entry := range entries {
				if !removed[ruleKey{harness, entry.Rule}] {
					kept = append(kept, entry)
				}
			}
			events[event] = kept
		}
	}
	for harness, entries := range result.Index.Static {
		kept := entries[:0]
		for _, entry := range entries {
			if !removed[ruleKey{harness, entry.Rule}] {
				kept = append(kept, entry)
			}
		}
		result.Index.Static[harness] = kept
	}
	kept := result.StaticBodies[:0]
	for _, body := range result.StaticBodies {
		if !removed[ruleKey{body.Harness, body.Rule}] {
			kept = append(kept, body)
		}
	}
	result.StaticBodies = kept
	return nil
}

func activePlacement(placement plan.Placement) bool {
	return placement.Status != plan.Skipped && placement.Status != plan.Never && placement.Status != plan.InstallError
}
