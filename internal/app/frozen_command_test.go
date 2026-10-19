package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/tokens"
)

func a5Golden(t *testing.T, name, output, root, digest string) {
	if runtime.GOOS == "windows" {
		t.Run("POSIX-golden/"+name, func(t *testing.T) {
			t.Skip("D11: POSIX native plan and path golden; Windows command semantics asserted by parent and native CLI checks")
		})
		return
	}

	t.Helper()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{canonical, root} {
		output = strings.ReplaceAll(output, path, "$FIXTURE")
	}
	// Absolute lock bindings make its digest fixture-specific; verify it against
	// the actual bytes before normalizing only that field in the wire golden.
	if digest != "" {
		output = strings.ReplaceAll(output, digest, "$LOCK_SHA256")
	}
	path := filepath.Join("..", "..", "testdata", "frozen", name)
	if os.Getenv("STYLE_M3_A5_RECORD") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if string(readSyncFile(t, path)) != output {
		t.Fatalf("%s differs\n%s", name, output)
	}
}

func TestM3A5CommandGoldenAndSingleJSON(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "json"}[jsonOutput], func(t *testing.T) {
			s, home, config, _ := a4Setup(t, "codex-user", nativePackageYAML("first"), false)
			t.Setenv("STYLE_HOME", home)
			a5Offline(t, s)
			a5Remove(t, filepath.Join(home, "build/index.json"))
			suffix := "txt"
			if jsonOutput {
				suffix = "json"
			}
			for _, phase := range []string{"preview", "apply", "unchanged", "error"} {
				args := []string{"restore"}
				if jsonOutput {
					args = append(args, "--json")
				}
				var input io.Reader
				if phase == "apply" {
					input = approve()
				}
				digest := ""
				if phase == "error" {
					a5Remove(t, filepath.Join(home, "style.lock.yaml"))
				} else {
					digest = hashBytes(readSyncFile(t, filepath.Join(home, "style.lock.yaml")))
				}
				before := a4State(t, home, config)
				var out, errOut bytes.Buffer
				exit := RunWithServiceInput(context.Background(), s, t.TempDir(), args, input, &out, &errOut)
				want := ExitOK
				if phase == "preview" || phase == "error" {
					want = ExitInvalid
				}
				if exit != want {
					t.Fatalf("%s exit=%d stdout=%s stderr=%s", phase, exit, &out, &errOut)
				}
				if phase != "apply" {
					a4AssertState(t, before, home, config)
				}
				if jsonOutput {
					decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
					var result FrozenResult
					if err := decoder.Decode(new(any)); err != nil {
						t.Fatal(err)
					}
					if err := decodeReport(out.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if decoder.Decode(new(any)) != io.EOF || result.Format != 1 || result.Command != "restore" || !result.LockedSource || result.LockSHA256 != digest {
						t.Fatalf("JSON=%s stderr=%s", &out, &errOut)
					}
					if phase != "error" && len(result.Native) != fixtureNativeCount(2) {
						t.Fatal("native disclosure omitted")
					}
				} else if phase != "error" && !strings.Contains(out.String(), digest) {
					t.Fatal("actual lock digest missing")
				}
				a5Golden(t, phase+".stdout."+suffix, out.String(), filepath.Dir(home), digest)
				a5Golden(t, phase+".stderr."+suffix, errOut.String(), filepath.Dir(home), digest)
			}
		})
	}
}

func TestM3A5NativeReplaceAndBlockedCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX author native/login shell fixture; Windows rejects those capabilities")
	}
	for _, gate := range []string{"replace", "input-ceiling", "fallback-error"} {
		t.Run(gate, func(t *testing.T) {
			manifest := a4Manifest("first", "Locked instructions.")
			if gate == "replace" {
				manifest = strings.ReplaceAll(manifest, "target: system", "target: system, mode: replace")
			}
			if gate == "fallback-error" {
				manifest = "spec: style/v0.3\nname: first\nversion: 1.0.0\ndescription: blocked\nrules: []\nnative_hooks:\n  - {id: native, fallback: error, codex: {event: Stop, command: 'printf PRIVATE-COMMAND'}}\n"
			}
			s, home, config, origin, _ := nativeInstallFixture(t, "codex-user")
			writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), manifest)
			if _, err := s.Install(context.Background(), InstallRequest{Home: home, Source: origin, Yes: true, AllowReplace: true, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}}); err != nil {
				t.Fatal(err)
			}
			d := s.definitions["codex"]
			if gate == "input-ceiling" {
				d.Limits.Input = tokens.InputLimit{Value: 1, Unit: tokens.CharsUnit}
			}
			if gate == "fallback-error" {
				d.Facts = nonNativeFacts{d.Facts}
			}
			s.definitions["codex"] = d
			a5Offline(t, s)
			t.Setenv("STYLE_HOME", home)
			before := a4State(t, home, config)
			check := a5Lock(t, home)
			var out, errOut bytes.Buffer
			exit := RunWithServiceInput(context.Background(), s, t.TempDir(), []string{"restore", "--json"}, approve(), &out, &errOut)
			want := ExitBlocked
			if gate == "replace" {
				want = ExitInvalid
			}
			var result FrozenResult
			if err := decodeReport(out.Bytes(), &result); err != nil || exit != want || result.Status != "error" || result.Changed || errOut.Len() != 0 {
				t.Fatalf("%s exit=%d stdout=%s stderr=%s err=%v", gate, exit, &out, &errOut, err)
			}
			a4AssertState(t, before, home, config)
			check()
			if gate == "replace" {
				result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true, AllowReplace: true})
				if err != nil || result.Status != "unchanged" || !result.Changes[0].After.Replace {
					t.Fatalf("%+v %v", result, err)
				}
				a4AssertState(t, before, home, config)
				check()
			}
		})
	}
}

func TestM3A5WholeLockRejectsBeforeAnyRecovery(t *testing.T) {
	for _, damage := range []string{"approval", "settings", "two-replace"} {
		t.Run(damage, func(t *testing.T) {
			s, home, config, origins := a5Batch(t, "codex-user")
			switch damage {
			case "approval":
				writeSyncFile(t, approvalPath(home, "second"), "invalid approval")
			case "settings":
				writeSyncFile(t, filepath.Join(home, "settings/second.yaml"), "extra: forbidden\n")
			case "two-replace":
				lock := mustA4Lock(t, home)
				for i := range lock.Packages {
					item := &lock.Packages[i]
					manifest := strings.ReplaceAll(a4Manifest(item.Name, "Locked replacement."), "target: system", "target: system, mode: replace")
					files := map[string][]byte{"STYLE.yaml": []byte(manifest)}
					item.Files, _ = hashPackageFiles(files)
					item.Replace = true
					item.Version = "1.0.0"
					approved, _ := json.Marshal(approvedSnapshot{Format: 1, Files: files})
					item.Approval = hashBytes(approved)
					writeSyncFile(t, approvalPath(home, item.Name), string(approved))
					writeSyncFile(t, filepath.Join(origins[i], "STYLE.yaml"), manifest)
				}
				a4WriteRawLock(t, home, lock)
			}
			code := "frozen-metadata-incomplete"
			if damage == "two-replace" {
				code = "frozen-build-mismatch"
			}
			a5Reject(t, s, home, config, code)
		})
	}
}

func TestM3A5JointDedupAndApprovedBatchRecovery(t *testing.T) {
	for _, route := range a4Routes {
		t.Run(route, func(t *testing.T) {
			s, home, _, first := a4Setup(t, route, a4Manifest("first", "Identical instructions."), false)
			second := writeInstallPackage(t, filepath.Join(filepath.Dir(first), "second-source"), a4Manifest("second", "Identical instructions."))
			a4Install(t, s, home, route, second, false)
			index := readSyncFile(t, filepath.Join(home, "build/index.json"))
			provenance := readSyncFile(t, filepath.Join(home, "build/explain.json"))
			a5Remove(t, first, second, filepath.Join(home, "packages/first"), filepath.Join(home, "packages/second"), filepath.Join(home, "build"))
			a5Offline(t, s)
			check := a5Lock(t, home)
			result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true})
			if err != nil || !result.Changed || len(result.Selected) != 2 {
				t.Fatalf("%+v %v", result, err)
			}
			alias := false
			for _, report := range result.Reports {
				if report.Package == "second" {
					for _, rule := range report.Plan.Rules {
						alias = alias || rule.DeduplicatedInto != ""
					}
				}
			}
			if !alias || !bytes.Equal(index, readSyncFile(t, filepath.Join(home, "build/index.json"))) || !bytes.Equal(provenance, readSyncFile(t, filepath.Join(home, "build/explain.json"))) {
				t.Fatal("joint dedup/index/provenance changed")
			}
			for _, content := range result.Contents {
				if content.Snapshot != "approved" || !content.StorageRepair {
					t.Fatal(content)
				}
			}
			check()
		})
	}
}
