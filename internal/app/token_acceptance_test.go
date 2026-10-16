package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestA18EveryTokenDiagnosticHasAFileFixture(t *testing.T) {
	root := repositoryRoot(t)
	service := testService(t)
	for _, code := range []string{"T001", "T002", "T004", "T005", "T006", "T007"} {
		t.Run(code, func(t *testing.T) {
			result, err := service.Plan(context.Background(), PlanRequest{Source: filepath.Join(root, "testdata", "tokens", code), Harnesses: []string{"codex"}})
			if err != nil {
				t.Fatal(err)
			}
			if hasErrors(result.Diagnostics) || len(result.Reports) != 1 {
				t.Fatalf("invalid fixture %s: %+v", code, result)
			}
			found := false
			for _, diagnostic := range result.Reports[0].Tokens.Diagnostics {
				if diagnostic.Code == code {
					found = true
				}
			}
			if !found {
				t.Fatalf("fixture did not trigger %s: %+v", code, result)
			}
		})
	}
}

func TestA18SecondInstallT003BlocksWithoutAnyChanges(t *testing.T) {
	root := t.TempDir()
	home, config := filepath.Join(root, "style"), filepath.Join(root, "codex")
	service := testCodexInstallService(t, config, filepath.Join(home, "adapters", "codex", "plugin"))
	t.Setenv("STYLE_HOME", home)
	fixture := filepath.Join(repositoryRoot(t), "testdata", "tokens", "T003", "STYLE.yaml")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	first := writeInstallPackage(t, filepath.Join(root, "first"), string(data))
	// M2 folds equivalent cross-package bodies. Keep the same 1002-token
	// estimate but make this fixture distinct so T003 still tests real growth.
	secondData := strings.ReplaceAll(string(data), "tokens-t003", "tokens-t003-second")
	secondData = strings.Replace(secondData, "text: a", "text: z", 1)
	second := writeInstallPackage(t, filepath.Join(root, "second"), secondData)
	for _, source := range []string{first, second} {
		planned, err := service.Plan(context.Background(), PlanRequest{Source: source, Harnesses: []string{"codex"}})
		if err != nil || len(planned.Reports) != 1 {
			t.Fatalf("plan = %+v, %v", planned, err)
		}
		if got := planned.Reports[0].Tokens.Events; len(got) != 1 || got[0].Estimate.Tokens != 1002 {
			t.Fatalf("fixed T003 estimate = %+v, want 1002", got)
		}
	}
	var out, errOut bytes.Buffer
	if code := RunWithService(context.Background(), service, root, []string{"install", first, "--harness=codex", "--yes"}, &out, &errOut); code != ExitOK {
		t.Fatalf("first exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	before := installedTree(t, home, config)
	out.Reset()
	errOut.Reset()
	if code := RunWithService(context.Background(), service, root, []string{"install", second, "--harness=codex", "--yes"}, &out, &errOut); code != ExitBlocked || !strings.Contains(errOut.String(), "T003") || !strings.Contains(errOut.String(), "installed estimate") {
		t.Fatalf("second exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !reflect.DeepEqual(before, installedTree(t, home, config)) {
		t.Fatal("T003 second install changed package/lock/index/approval/provider bytes")
	}
	lock, err := loadLock(home)
	if err != nil || len(lock.Packages) != 1 || lock.Packages[0].Name != "tokens-t003" || lock.Packages[0].Seq != 1 {
		t.Fatalf("lock = %+v, %v", lock, err)
	}
}
