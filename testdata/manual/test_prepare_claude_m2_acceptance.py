import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

from prepare_claude_m2_acceptance import NATIVE_BODIES, observation, prepare, verify_restored


class M2ClaudeAcceptancePreparationTests(unittest.TestCase):
    def test_observer_retains_only_loading_and_matcher_facts(self):
        row = observation({"hook_event_name": "PreToolUse", "session_id": "s", "tool_name": "Bash",
                           "tool_input": {"command": "PRIVATE"}, "prompt": "PRIVATE", "api_key": "SECRET"}, "shell")
        self.assertEqual(row, {"fixture": "shell", "fields": {
            "hook_event_name": "PreToolUse", "session_id": "s", "tool_name": "Bash"}})
        self.assertNotIn("PRIVATE", json.dumps(row))
        self.assertNotIn("SECRET", json.dumps(row))

    def test_nonempty_directory_is_preserved(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "evidence").write_text("keep")
            with self.assertRaisesRegex(ValueError, "scratch must be empty"):
                prepare(root, "/missing/style", "/missing/claude")
            self.assertEqual((root / "evidence").read_text(), "keep")

    def test_launchers_pin_identity_discard_credentials_and_do_not_start_sessions(self):
        with tempfile.TemporaryDirectory(prefix="m2 'acceptance ") as directory:
            parent = Path(directory).resolve()
            root, style, claude, home = (parent / name for name in ("scratch", "style", "claude", "real-home"))
            home.mkdir()
            executable = "#!" + sys.executable + "\nimport os,json,sys\nprint(json.dumps({'env':dict(os.environ),'args':sys.argv[1:],'cwd':os.getcwd()}))\n"
            for path in (style, claude):
                path.write_text(executable)
                path.chmod(0o700)
            with patch.dict(os.environ, {"HOME": str(home), "USER": "real-user", "LOGNAME": "real-login",
                    "ANTHROPIC_API_KEY": "SECRET", "CLAUDE_CODE_OAUTH_TOKEN": "SECRET", "CODEX_HOME": "SECRET"}), \
                    patch("prepare_claude_m2_acceptance.subprocess.run", side_effect=[
                        SimpleNamespace(stdout="2.1.fixture\n"), SimpleNamespace(stdout='{"changed":true}'),
                        SimpleNamespace(stdout='{"changed":true}')]) as run:
                report = prepare(root, style, claude)
            self.assertEqual(run.call_count, 3)
            self.assertEqual(run.call_args_list[0].args[0], [str(claude), "--version"])
            self.assertTrue(all(call.args[0][1] == "install" for call in run.call_args_list[1:]))
            self.assertFalse(report["acceptance"])
            self.assertEqual(root.stat().st_mode & 0o777, 0o700)
            for name in ("launch.sh", "login.sh", "style.sh"):
                launcher = root / name
                self.assertEqual(launcher.stat().st_mode & 0o777, 0o700)
                subprocess.run(["sh", "-n", str(launcher)], check=True)
                call = subprocess.run([str(launcher)], env={**os.environ, "ANTHROPIC_API_KEY": "SECRET", "HOME": "/wrong"},
                                      capture_output=True, text=True, check=True)
                facts = json.loads(call.stdout)
                self.assertEqual(facts["env"]["HOME"], str(home))
                self.assertEqual(facts["env"]["USER"], "real-user")
                self.assertEqual(facts["env"]["CLAUDE_CONFIG_DIR"], str(root / "claude"))
                self.assertEqual(facts["env"]["STYLE_HOME"], str(root / "style"))
                self.assertEqual(facts["cwd"], str(root / "work"))
                self.assertNotIn("SECRET", call.stdout)
            # Run the pinned observer and restoration checker, not just inspect source.
            observer = root / "prepare_claude_m2_acceptance.py"
            call = subprocess.run([sys.executable, str(observer), "--observe", str(root / "native.jsonl"), "shell"],
                                  input=json.dumps({"hook_event_name": "PreToolUse", "session_id": "s", "tool_name": "Bash", "tool_input": "PRIVATE"}),
                                  capture_output=True, text=True, check=True)
            self.assertEqual(json.loads(call.stdout)["hookSpecificOutput"]["additionalContext"], NATIVE_BODIES["shell"])
            self.assertNotIn("PRIVATE", (root / "native.jsonl").read_text())
            self.assertTrue(verify_restored(root)["external_bytes_and_modes_restored"])
            rule = root / "claude/rules/style-m2-claude-first.md"
            rule.write_text("managed")
            with self.assertRaisesRegex(AssertionError, "managed rule remains"):
                verify_restored(root)
            rule.unlink()
            settings = root / "claude/settings.json"
            settings.chmod(0o644)
            with self.assertRaises(AssertionError):
                verify_restored(root)


if __name__ == "__main__":
    unittest.main()
