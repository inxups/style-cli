import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
import probe_m3_d3_routes as probe


SCRIPT = Path(probe.__file__).resolve()


class DefinitionHashTest(unittest.TestCase):
    # Vectors produced by agenthooks/install.DefinitionHash at fd88e8d.
    def test_matches_agenthooks_vectors(self):
        self.assertEqual(probe.definition_hash("SessionStart", 'cat > "$(mktemp /x/original-session_start.XXXXXX)"', 5),
                         "sha256:00e66070bc1dc64bd44a203bdcec2fd8efd327b0772f4e4ee293fd6ed6b3360f")
        self.assertEqual(probe.definition_hash("UserPromptSubmit", "exec /opt/style agenthooks run --provider=codex --owner=style", 5),
                         "sha256:d2ef4dff467267df522032020ede3bc6f48bd69ae3ffb86b3ae5a4468909c3b5")


class TrustKeyTest(unittest.TestCase):
    def test_resolved_and_link_spellings_differ(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw).resolve()
            real = root / "real"
            real.mkdir()
            link = root / "link"
            link.symlink_to(real)
            document = probe.hooks_document(root / "marks", "t")
            resolved = probe.trust_toml(link, document)
            spelled = probe.trust_toml(link, document, resolve=False)
            self.assertIn(json.dumps(f"{real}/.codex/hooks.json:session_start:0:0"), resolved)
            self.assertIn(json.dumps(f"{link}/.codex/hooks.json:user_prompt_submit:0:0"), spelled)
            self.assertIn(json.dumps(str(real)), probe.project_trust_toml(link))
            self.assertIn(json.dumps(str(link)), probe.project_trust_toml(link, resolve=False))

    def test_every_case_uses_known_keys(self):
        known = {"project", "hooks", "repo_hooks_state", "clone", "trust_clone_project", "user_same",
                 "subdir", "symlink", "link_project", "link_hooks"}
        for name, spec in probe.CASES.items():
            self.assertLessEqual(set(spec), known, name)


FAKE_CODEX = """#!/usr/bin/env python3
import json, os, subprocess, sys
if sys.argv[1:] == ["--version"]:
    print("codex-cli 0.0.0-fake"); sys.exit(0)
work = sys.argv[sys.argv.index("-C") + 1]
config = open(os.path.join(os.environ["CODEX_HOME"], "config.toml")).read()
hooks = json.load(open(os.path.join(work, ".codex", "hooks.json")))["hooks"]
# Fire only when the user config names this exact project path as trusted.
if json.dumps(os.path.realpath(work)) in config and "trusted_hash" in config:
    for event, groups in hooks.items():
        subprocess.run(["/bin/sh", "-c", groups[0]["hooks"][0]["command"]],
                       input=json.dumps({"hook_event_name": event, "cwd": work}), text=True, check=True)
print("sandbox: read-only", file=sys.stderr)
print("hook: fake " + work, file=sys.stderr)
"""


class EndToEndFakeTest(unittest.TestCase):
    def run_probe(self, root, *cases):
        fake = root / "fake-codex"
        fake.write_text(FAKE_CODEX)
        fake.chmod(0o755)
        output = root / "out.json"
        args = [sys.executable, str(SCRIPT), "--scratch", str(root / "scratch"), "--output", str(output), "--codex", str(fake)]
        for case in cases:
            args += ["--case", case]
        env = {k: v for k, v in os.environ.items() if k not in ("CODEX_HOME", "CODEX_SANDBOX", "CLAUDE_CONFIG_DIR")}
        subprocess.run(args, check=True, capture_output=True, text=True, env=env)
        return json.loads(output.read_text()), output.read_text()

    def test_counts_invocations_and_hides_scratch_paths(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw).resolve()
            report, text = self.run_probe(root, "untrusted", "project-and-hook-trusted")
            rows = {row["case"]: row for row in report["results"]}
            self.assertEqual(rows["untrusted"]["fired"], {})
            self.assertEqual(rows["project-and-hook-trusted"]["fired"],
                             {"original-session_start": 1, "original-user_prompt_submit": 1})
            self.assertEqual(rows["project-and-hook-trusted"]["stderr_hook_lines"], ["hook: fake <case>/project"])
            self.assertEqual(rows["untrusted"]["sandbox"], "read-only")
            self.assertEqual(report["codex_version"], "codex-cli 0.0.0-fake")
            self.assertNotIn(str(root / "scratch"), text)

    def test_rejects_non_empty_scratch(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw).resolve()
            (root / "scratch").mkdir()
            (root / "scratch" / "leftover").write_text("x")
            result = subprocess.run([sys.executable, str(SCRIPT), "--scratch", str(root / "scratch"),
                                     "--output", str(root / "o.json"), "--codex", "/bin/false"],
                                    capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("must be empty", result.stderr)


if __name__ == "__main__":
    unittest.main()
