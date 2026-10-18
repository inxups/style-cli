package compile

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"unicode/utf8"

	"github.com/inxups/style-cli/core/plan"
)

// Provenance is optional cold-path metadata bound to exact index bytes. It is
// deliberately separate from Index and contains no body, setting or command.
type Provenance struct {
	Format      int                               `json:"format"`
	IndexSHA256 string                            `json:"index_sha256"`
	Harnesses   map[string][]ProvenanceRule       `json:"harnesses"`
	NativeHooks map[string][]ProvenanceNativeHook `json:"native_hooks"`
}
type ProvenanceRule struct {
	Refresh          *plan.RefreshPolicy   `json:"refresh,omitempty"`
	Harness          string                `json:"harness"`
	Rule             string                `json:"rule"`
	Source           string                `json:"source"`
	Target           string                `json:"target"`
	Action           string                `json:"action,omitempty"`
	Status           string                `json:"status"`
	Placements       []ProvenancePlacement `json:"placements"`
	DeduplicatedInto string                `json:"deduplicated_into,omitempty"`
	ReasonCodes      []string              `json:"reason_codes"`
	Native           *plan.NativeEntry     `json:"native,omitempty"`
}

// ProvenanceNativeHook is cold-path metadata; author commands are never recorded.
type ProvenanceNativeHook struct {
	Harness     string            `json:"harness"`
	Rule        string            `json:"rule"`
	Source      string            `json:"source"`
	Status      string            `json:"status"`
	Native      *plan.NativeEntry `json:"native,omitempty"`
	ReasonCodes []string          `json:"reason_codes"`
}

type ProvenancePlacement struct {
	Event      string      `json:"event,omitempty"`
	StaticSlot string      `json:"static_slot,omitempty"`
	Action     string      `json:"action"`
	Status     plan.Status `json:"status"`
}

func BuildProvenance(request Request, result Result, indexBytes []byte) ([]byte, error) {
	p := Provenance{Format: 2, IndexSHA256: fmt.Sprintf("sha256:%x", sha256.Sum256(indexBytes)), Harnesses: map[string][]ProvenanceRule{}, NativeHooks: map[string][]ProvenanceNativeHook{}}
	for harness := range result.Index.Harnesses {
		p.Harnesses[harness] = []ProvenanceRule{}
	}
	for _, pkg := range request.Packages {
		for _, report := range pkg.Plans {
			if p.Harnesses[report.Adapter] == nil {
				p.Harnesses[report.Adapter] = []ProvenanceRule{}
			}
			for _, rule := range report.Rules {
				row := ProvenanceRule{Harness: report.Adapter, Rule: report.Package + "/" + rule.ID, Source: rule.Source, Target: rule.Target, Action: rule.Action, Status: string(rule.Status), Placements: []ProvenancePlacement{}, ReasonCodes: []string{}}
				row.Refresh = rule.Refresh
				for _, placement := range rule.Placements {
					row.Placements = append(row.Placements, ProvenancePlacement{placement.Event, placement.StaticSlot, placement.Action, placement.Status})
				}
				for _, fold := range result.Deduplicated {
					if fold.Harness == row.Harness && fold.Rule == row.Rule {
						row.Status = "compile-deduplicated"
						row.DeduplicatedInto = fold.Into
						row.ReasonCodes = append(row.ReasonCodes, "cross-package-equivalent")
					}
				}
				if rule.Status == plan.Skipped || rule.Status == plan.Never {
					row.ReasonCodes = append(row.ReasonCodes, "plan-"+string(rule.Status))
				}
				p.Harnesses[report.Adapter] = append(p.Harnesses[report.Adapter], row)
			}
			if p.NativeHooks[report.Adapter] == nil {
				p.NativeHooks[report.Adapter] = []ProvenanceNativeHook{}
			}
			for _, hook := range report.NativeHooks {
				row := ProvenanceNativeHook{Harness: report.Adapter, Rule: report.Package + "/" + hook.ID, Source: hook.Source, Status: string(hook.Status), ReasonCodes: []string{}}
				if hook.Native != nil {
					v := *hook.Native
					row.Native = &v
				}
				if hook.Status == plan.Skipped {
					row.ReasonCodes = append(row.ReasonCodes, "plan-skipped")
				}
				p.NativeHooks[report.Adapter] = append(p.NativeHooks[report.Adapter], row)
			}
		}
	}
	if err := p.Validate(result.Index, indexBytes); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(data)+1 > 16<<20 {
		return nil, fmt.Errorf("compile: provenance exceeds limit")
	}
	return append(data, '\n'), nil
}

func DecodeProvenance(data, indexBytes []byte, index Index) (Provenance, error) {
	var p Provenance
	if len(data) > 16<<20 || !utf8.Valid(data) {
		return p, fmt.Errorf("provenance exceeds limit")
	}
	if err := decodeStrict(data, &p); err != nil {
		return p, err
	}
	return p, p.Validate(index, indexBytes)
}

func (p Provenance) Validate(index Index, indexBytes []byte) error {
	if (p.Format != 1 && p.Format != 2) || p.IndexSHA256 != fmt.Sprintf("sha256:%x", sha256.Sum256(indexBytes)) || p.Harnesses == nil {
		return fmt.Errorf("provenance index binding mismatch")
	}
	if p.Format == 2 && p.NativeHooks == nil {
		return fmt.Errorf("provenance native hook metadata is missing")
	}
	if p.Format == 1 && len(p.NativeHooks) > 0 {
		return fmt.Errorf("invalid legacy provenance native hooks")
	}
	seen := map[string]bool{}
	covered := map[string]bool{}
	for harness, rows := range p.Harnesses {
		if harness == "" || rows == nil {
			return fmt.Errorf("invalid provenance harness")
		}
		for _, row := range rows {
			if row.Native != nil || row.Target == "native" {
				return fmt.Errorf("native-format-removed: migrate to native_hooks and reinstall the package")
			}

			if row.Refresh != nil && (row.Refresh.Interval < 1 || row.Refresh.Interval > 1000000 || !reflect.DeepEqual(row.Refresh, plan.DescribeRefresh(row.Refresh.Interval))) {
				return fmt.Errorf("invalid provenance refresh metadata")
			}
			key := row.Harness + "\x00" + row.Rule
			if row.Rule == "" || row.Harness != harness || seen[key] || row.Placements == nil || row.ReasonCodes == nil {
				return fmt.Errorf("invalid provenance rule")
			}
			seen[key] = true
			if row.Status != "compile-deduplicated" && !knownStatus(plan.Status(row.Status)) {
				return fmt.Errorf("invalid provenance status")
			}
			if row.DeduplicatedInto != "" && row.Status != "compile-deduplicated" || row.Status == "compile-deduplicated" && (row.DeduplicatedInto == "" || row.DeduplicatedInto == row.Rule) {
				return fmt.Errorf("invalid provenance folding")
			}
			reference := row.Rule
			if row.DeduplicatedInto != "" {
				reference = row.DeduplicatedInto
			}
			for _, placement := range row.Placements {
				if !knownStatus(placement.Status) || !knownAction(placement.Action) {
					return fmt.Errorf("invalid provenance placement")
				}
				if !activePlacement(plan.Placement{Status: placement.Status}) {
					continue
				}
				found := false
				if placement.StaticSlot != "" {
					for _, entry := range index.Static[row.Harness] {
						if entry.Rule == reference && entry.Slot == placement.StaticSlot {
							found = true
							covered[harness+"\x00static\x00"+entry.Rule+"\x00"+entry.Slot] = true
						}
					}
				} else {
					for _, entry := range index.Harnesses[row.Harness][placement.Event] {
						if entry.Rule == reference && entry.Action == placement.Action {
							interval := 0
							if row.Refresh != nil {
								interval = row.Refresh.Interval
							}
							if interval != entry.Refresh {
								return fmt.Errorf("provenance refresh policy mismatch")
							}
							found = true
							covered[harness+"\x00runtime\x00"+entry.Entry] = true
						}
					}
				}
				if !found {
					return fmt.Errorf("provenance references absent index placement")
				}
			}
		}
	}
	for harness, rows := range p.NativeHooks {
		if harness == "" || rows == nil {
			return fmt.Errorf("invalid provenance native harness")
		}
		for _, row := range rows {
			key := harness + "\x00" + row.Rule
			if row.Rule == "" || row.Source == "" || row.Harness != harness || seen[key] || row.ReasonCodes == nil || (row.Status != string(plan.NativeHook) && row.Status != string(plan.Skipped)) || (row.Status == string(plan.NativeHook)) != (row.Native != nil) {
				return fmt.Errorf("invalid provenance native hook")
			}
			if row.Native != nil && (row.Native.Event == "" || row.Native.Timeout < 1 || row.Native.Timeout > 600) {
				return fmt.Errorf("invalid provenance native mapping")
			}
			seen[key] = true
		}
	}
	for harness, entries := range index.Static {
		for _, entry := range entries {
			if !covered[harness+"\x00static\x00"+entry.Rule+"\x00"+entry.Slot] {
				return fmt.Errorf("provenance is missing an index static placement")
			}
		}
	}
	for harness, events := range index.Harnesses {
		for _, entries := range events {
			for _, entry := range entries {
				if !covered[harness+"\x00runtime\x00"+entry.Entry] {
					return fmt.Errorf("provenance is missing an index runtime placement")
				}
			}
		}
	}
	return nil
}
