package agenthooks

import (
	"reflect"
	"testing"
)

func TestM3A11CodexLaunchProviderAllowlistAndLastOverride(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		provider string
		set      bool
	}{
		{"quoted", []string{"codex", "-c", `model_provider="Azure"`, "-c", "api_key=PRIVATE_CANARY"}, "Azure", true},
		{"last", []string{"codex", "--config=model_provider=first", "-cmodel_provider=last"}, "last", true},
		{"empty last", []string{"codex", "-c", "model_provider=first", "-c", "model_provider="}, "", true},
		{"absent", []string{"codex", "-c", "model_providers.local.api_key=PRIVATE_CANARY"}, "", false},
		{"after separator ignored", []string{"codex", "--", "-c", "model_provider=prompt"}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := parseCodexLaunchArgs(tc.args, "/work").styleSettings()
			if settings.ModelProvider != tc.provider || settings.ModelProviderSet != tc.set {
				t.Fatal(settings)
			}
			typ := reflect.TypeOf(settings)
			if typ.NumField() != 5 {
				t.Fatal("review allowlist field expansion", typ)
			}
		})
	}
}
