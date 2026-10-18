package spec

import (
	"fmt"
	"strings"
	"testing"
)

func TestM3A16NativeExtensionsRejectAcrossFormatsAndCatalogs(t *testing.T) {
	extensions := []string{"async: true", "async: false", "async: 17", "asyncRewake: true", "onFailure: block", "onFailure: continue", "onFailure: false", "type: command", "type: http", "type: prompt", "type: agent", "type: mcp_tool", "args: [SECRET_VALUE]", "shell: SECRET_VALUE", "url: SECRET_VALUE", "headers: {Authorization: SECRET_VALUE}", "allowedEnvVars: [SECRET_VALUE]", "prompt: SECRET_VALUE", "model: SECRET_VALUE", "server: SECRET_VALUE", "input: {secret: SECRET_VALUE}"}
	for _, harness := range []string{"codex", "claude-code", "future-harness"} {
		for _, format := range []string{"directory", "snapshot"} {
			for _, extension := range extensions {
				t.Run(harness+"/"+format+"/"+extension, func(t *testing.T) {
					pkg := loadNativeHook(t, format, fmt.Sprintf("fallback: skip\n%s: {event: Stop, command: notify, %s}\n", harness, extension), "")
					catalog := NativeCatalog{harness: fakeNativeHarness{events: map[string]bool{"Stop": false}}}
					ds := validateWithNative(pkg, catalog)
					found := false
					for _, d := range ds {
						if d.Code == "native-field" {
							found = true
							if !strings.Contains(d.Message, "not supported;") || strings.Contains(d.Message, "SECRET_VALUE") {
								t.Fatal(d)
							}
						}
					}
					if !found {
						t.Fatal(ds)
					}
				})
			}
		}
	}
	pkg := loadNativeHook(t, "directory", "codex: {event: Stop, command: notify, futureField: SECRET_VALUE}\n", "")
	ds := validateWithNative(pkg, testNativeCatalog)
	if len(ds) != 1 || ds[0].Message != `native codex hook field "futureField" is not supported` {
		t.Fatal(ds)
	}
}
