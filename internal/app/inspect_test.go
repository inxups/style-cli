package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/codex"
)

func inspectionFixture(t *testing.T) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "style")
	service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(home, "adapters", "codex", "plugin"))
	_, err := service.Install(context.Background(), InstallRequest{Home: home,
		Source: filepath.Join(repositoryRoot(t), "testdata", "packages", "terse-reviewer"), Yes: true, Harnesses: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	return service, home
}

func TestListReportsInstalledHashDriftAndMissingSources(t *testing.T) {
	service, home := inspectionFixture(t)
	ctx := context.Background()
	result, err := service.List(ctx, ListRequest{Home: home})
	if err != nil || len(result.Packages) != 1 || !result.Packages[0].HashMatches || result.Packages[0].Sequence != 1 {
		t.Fatalf("List = %+v, %v", result, err)
	}
	manifest := filepath.Join(home, "packages", "terse-reviewer", "STYLE.yaml")
	file, err := os.OpenFile(manifest, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("\n# edited\n")
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.List(ctx, ListRequest{Home: home})
	if err != nil || result.Packages[0].HashMatches || strings.Join(result.Packages[0].ChangedFiles, ",") != "STYLE.yaml" {
		t.Fatalf("drift = %+v, %v", result, err)
	}
	if err := os.RemoveAll(filepath.Dir(manifest)); err != nil {
		t.Fatal(err)
	}
	result, err = service.List(ctx, ListRequest{Home: home})
	if err != nil || result.Packages[0].Error == "" || result.Packages[0].HashMatches {
		t.Fatalf("missing = %+v, %v", result, err)
	}
}

func TestListSingleFileAndDevelopmentLink(t *testing.T) {
	for _, link := range []bool{false, true} {
		t.Run(map[bool]string{false: "stored", true: "link"}[link], func(t *testing.T) {
			root := t.TempDir()
			service, err := New()
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Init(context.Background(), InitRequest{OutputDir: root, Name: "single", Single: true})
			if err != nil {
				t.Fatal(err)
			}
			home := filepath.Join(root, "store")
			_, err = service.Install(context.Background(), InstallRequest{Home: home, Source: filepath.Join(root, "single.style.md"), Link: link, Yes: true})
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.List(context.Background(), ListRequest{Home: home})
			if err != nil || len(result.Packages) != 1 || !result.Packages[0].HashMatches || (result.Packages[0].Link == "dev") != link {
				t.Fatalf("List = %+v, %v", result, err)
			}
		})
	}
}

type inspectionDetector struct{ err error }

func (detector inspectionDetector) Detect(context.Context) (adapter.Installation, bool, error) {
	return adapter.Installation{}, false, detector.err
}

type inspectionRoute struct {
	adapter.Route
	available, status *int
}

func (route inspectionRoute) Available(context.Context, adapter.Installation) (bool, string, error) {
	*route.available++
	return false, "not available", nil
}

func (route inspectionRoute) Status(context.Context, adapter.Installation) (adapter.RouteStatus, error) {
	*route.status++
	return adapter.RouteStatus{}, errors.New("status failed")
}

func TestAdaptersInspectsEveryRouteAndReportsFailures(t *testing.T) {
	definition := codex.DefinitionForInstallation(adapter.Installation{Provider: "codex", ConfigDir: t.TempDir()})
	definition.Detector = inspectionDetector{err: errors.New("detect failed")}
	available, status := 0, 0
	for i, route := range definition.Routes {
		definition.Routes[i] = inspectionRoute{route, &available, &status}
	}
	service, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Adapters(context.Background())
	if err != nil || len(result.Adapters) != 1 || available != 2 || status != 2 {
		t.Fatalf("Adapters = %+v, %v; calls %d/%d", result, err, available, status)
	}
	report := result.Adapters[0]
	if report.Error == "" || len(report.Capabilities) != 21 || len(report.Fields) == 0 || report.Limit.Value != 2500 || len(report.Static) == 0 {
		t.Fatalf("incomplete inventory: %+v", report)
	}
	for _, route := range report.Routes {
		if route.Error != "status failed" || route.Trust == "" || route.Available || route.Trusted {
			t.Fatalf("route = %+v", route)
		}
	}
}

func TestRunInspectionCommandsJSONHelpAndErrors(t *testing.T) {
	service, home := inspectionFixture(t)
	t.Setenv("STYLE_HOME", home)
	t.Setenv("SHELL", "/bin/sh")
	for _, command := range []string{"list", "doctor"} {
		var stdout, stderr bytes.Buffer
		code := RunWithService(context.Background(), service, t.TempDir(), []string{command, "--json"}, &stdout, &stderr)
		if code != 0 || stderr.Len() != 0 || !json.Valid(stdout.Bytes()) {
			t.Fatalf("%s exit=%d stdout=%s stderr=%s", command, code, stdout.String(), stderr.String())
		}
		stdout.Reset()
		if code := RunWithService(context.Background(), service, "", []string{command, "--help"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "style "+command) {
			t.Fatalf("help %s = %d %s", command, code, stdout.String())
		}
		if code := RunWithService(context.Background(), service, "", []string{command, "--bad"}, &stdout, &stderr); code != 1 {
			t.Fatalf("bad option %s = %d", command, code)
		}
	}
	var doctorText, removed bytes.Buffer
	if RunWithService(context.Background(), service, t.TempDir(), []string{"doctor"}, &doctorText, &bytes.Buffer{}); !strings.Contains(doctorText.String(), "  limit: ") {
		t.Fatalf("doctor text omits the adapter inventory: %s", doctorText.String())
	}
	if code := RunWithService(context.Background(), service, t.TempDir(), []string{"adapters", "--json"}, &bytes.Buffer{}, &removed); code != ExitUsage || !strings.Contains(removed.String(), `unknown command "adapters"`) {
		t.Fatalf("removed adapters command = %d %s", code, removed.String())
	}
	if err := os.WriteFile(filepath.Join(home, "build", "index.json"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := RunWithService(context.Background(), service, "", []string{"doctor", "--json"}, &out, nil); code != 1 || !bytes.Contains(out.Bytes(), []byte(`"code": "index-invalid"`)) {
		t.Fatalf("broken index exit=%d stdout=%s", code, out.String())
	}
}
