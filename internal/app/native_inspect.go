package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/inxups/style-cli/internal/adapter"
)

type NativeHookCheck struct {
	NativeHookReport
	Status      string `json:"status"`
	TrustStatus string `json:"trusted_hash,omitempty"`
}

func (s *Service) checkNative(ctx context.Context, home string, lock lockDocument, approving string) ([]NativeHookCheck, error) {
	adapters, err := s.lockedAdapters(ctx, home, lock)
	if err != nil {
		return nil, err
	}
	var approved []NativeHookReport
	for _, item := range lock.Packages {
		baseline, available, err := loadApproval(home, item)
		if err != nil {
			return nil, err
		}
		if !available {
			files, err := installedSnapshot(home, item)
			if err != nil {
				return nil, err
			}
			if len(filesChanged(item, files)) > 0 {
				if item.Name == approving {
					continue
				}
				return nil, fmt.Errorf("native baseline unavailable for changed package %s", item.Name)
			}
			baseline.Files = files
		}
		native, err := s.baselineNative(item, baseline, adapters)
		if err != nil {
			return nil, err
		}
		approved = append(approved, native...)
	}
	return inspectNativeReports(ctx, adapters, approved)
}

func inspectNativeReports(ctx context.Context, adapters []installAdapter, approved []NativeHookReport) ([]NativeHookCheck, error) {
	reports := make(map[string]NativeHookReport)
	hooks := make(map[adapter.ProviderID][]adapter.HookSpec)
	for _, report := range approved {
		provider := adapter.ProviderID(report.Harness)
		reports[report.Harness+"\x00"+report.Rule] = report
		hooks[provider] = append(hooks[provider], adapter.HookSpec{NativeEvent: report.Event, Matcher: report.Matcher,
			Command: report.Command, NativeTag: report.Rule, Extra: map[string]any{"timeout_seconds": report.Timeout}})
	}
	var result []NativeHookCheck
	for _, target := range adapters {
		if len(hooks[target.provider]) == 0 {
			continue
		}
		inspector, ok := target.route.(adapter.NativeInspector)
		if !ok {
			return result, fmt.Errorf("route %s cannot inspect native hooks", target.route.Name())
		}
		statuses, err := inspector.InspectNative(ctx, hooks[target.provider])
		if err != nil {
			return result, fmt.Errorf("route %s native configuration: %w", target.route.Name(), err)
		}
		for _, status := range statuses {
			report := reports[string(target.provider)+"\x00"+status.Tag]
			result = append(result, NativeHookCheck{NativeHookReport: report, Status: status.State, TrustStatus: status.Trust})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Harness+"\x00"+result[i].Rule < result[j].Harness+"\x00"+result[j].Rule
	})
	return result, nil
}

func requireNativeRepairs(result *SyncResult, checks []NativeHookCheck, lock lockDocument, target string) {
	for _, check := range checks {
		if check.Status == "ok" && (check.TrustStatus == "" || check.TrustStatus == "match") {
			continue
		}
		name, _, _ := strings.Cut(check.Rule, "/")
		found := false
		for i := range result.Changes {
			if result.Changes[i].Package != name {
				continue
			}
			result.Changes[i].NeedsTrust, result.Changes[i].NativeChanged = true, true
			found = true
		}
		if !found {
			item, _ := findLockedPackage(lock, name)
			result.Changes = append(result.Changes, PackageChange{Package: name, Link: item.Link, Files: []string{}, NeedsTrust: true, NativeChanged: true})
		}
		if name != target {
			present := false
			for _, existing := range result.NeedsTrust {
				present = present || existing == name
			}
			if !present {
				result.NeedsTrust = append(result.NeedsTrust, name)
			}
		}
	}
	sort.Strings(result.NeedsTrust)
}
