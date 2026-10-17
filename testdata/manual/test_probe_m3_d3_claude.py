import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
import probe_m3_d3_claude as probe


SCRIPT = Path(probe.__file__).resolve()

# Minimal stand-in for the Claude Code facts the probe measures: project files
# load only from cwd, identical handlers across settings files run once,
# interactive sessions wait for trust, ${CLAUDE_PROJECT_DIR} is substituted.
FAKE_CLAUDE = r"""#!/usr/bin/env python3
import json, os, subprocess, sys, time
if sys.argv[1:] == ["--version"]:
    print("0.0.0 (Fake Claude)"); sys.exit(0)
cwd = os.path.realpath(os.getcwd())
cfg = os.environ["CLAUDE_CONFIG_DIR"]
state = json.load(open(os.path.join(cfg, ".claude.json")))
interactive = sys.argv[1] != "-p"
if interactive and not state.get("projects", {}).get(cwd, {}).get("hasTrustDialogAccepted"):
    print("Do you trust the files in this folder?", flush=True)
    sys.stdin.read(1); sys.exit(0)
seen, handlers = set(), []
for path in (os.path.join(cfg, "settings.json"), os.path.join(cwd, ".claude/settings.json"),
             os.path.join(cwd, ".claude/settings.local.json")):
    if os.path.exists(path):
        for event, groups in json.load(open(path)).get("hooks", {}).items():
            for group in groups:
                for handler in group["hooks"]:
                    key = json.dumps([event, handler], sort_keys=True)
                    if key not in seen:
                        seen.add(key); handlers.append(handler)
env = dict(os.environ, CLAUDE_PROJECT_DIR=cwd)
for handler in handlers:
    if "args" in handler:
        argv = [handler["command"]] + [a.replace("${CLAUDE_PROJECT_DIR}", cwd) for a in handler["args"]]
        subprocess.run(argv, input="{}", text=True, env=env)
    else:
        subprocess.run(["/bin/sh", "-c", handler["command"]], input="{}", text=True, env=env)
print("ok", flush=True)
if interactive:
    sys.stdin.read(1)
"""


class HelperTest(unittest.TestCase):
    def test_diff_reports_added_removed_changed(self):
        self.assertEqual(probe.diff({"a": "1", "b": "1"}, {"b": "2", "c": "1"}),
                         {"added": ["c"], "removed": ["a"], "changed": ["b"]})

    def test_api_key_is_a_labelled_placeholder(self):
        self.assertIn("placeholder", probe.PLACEHOLDER_KEY)


class EndToEndFakeTest(unittest.TestCase):
    def test_project_part_counts_dedup_trust_and_hides_scratch(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw).resolve()
            fake = root / "fake-claude"
            fake.write_text(FAKE_CLAUDE)
            fake.chmod(0o755)
            output = root / "out.json"
            env = {k: v for k, v in os.environ.items() if k not in ("CODEX_HOME", "CODEX_SANDBOX", "CLAUDE_CONFIG_DIR")}
            subprocess.run([sys.executable, str(SCRIPT), "--scratch", str(root / "scratch"), "--output", str(output),
                            "--claude", str(fake), "--part", "project"], check=True, capture_output=True, text=True,
                           env=env, timeout=300)
            text = output.read_text()
            rows = json.loads(text)["results"]
            self.assertEqual(rows["project-print-untrusted"]["fired"], {"project-ss": 1, "project-ups": 1})
            self.assertEqual(rows["project-tty-untrusted"]["fired"], {})
            self.assertTrue(rows["project-tty-untrusted"]["screen"]["trust_dialog_text_seen"])
            self.assertEqual(rows["project-tty-trusted"]["fired"], {"project-ss": 1, "project-ups": 1})
            self.assertEqual(rows["dup-user-project-exec"]["fired"], {"dup-ss": 1, "dup-ups": 1})
            self.assertEqual(rows["dup-user-project-exec-args-differ"]["fired"], {"dup-ss": 2, "dup-ups": 2})
            self.assertEqual(rows["projdir-root"]["hook_env"]["projdir-ss"]["CLAUDE_PROJECT_DIR"], "<case>/project")
            self.assertEqual(rows["subdir-discovery"]["fired"], {"user-ss": 1, "user-ups": 1})
            self.assertNotIn(str(root), text)
            self.assertNotIn(probe.PLACEHOLDER_KEY, text)


if __name__ == "__main__":
    unittest.main()
