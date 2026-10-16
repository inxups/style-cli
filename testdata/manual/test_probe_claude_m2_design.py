import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
from types import SimpleNamespace

from probe_claude_m2_design import MARKERS, STYLES, fixture, observation, prepare_ui, request_facts, write_ui_launchers


class M2ClaudeDesignTests(unittest.TestCase):
    def test_observer_excludes_content_credentials_and_unknown_fields(self):
        observed = observation({"hook_event_name": "InstructionsLoaded", "file_path": "/scratch/claude/rules/style.md",
                                "memory_type": "User", "load_reason": "session_start", "session_id": "session",
                                "content": "PRIVATE_BODY", "prompt": "PRIVATE_PROMPT", "api_key": "SECRET", "other": "NO"}, "ui")
        self.assertEqual(set(observed["fields"]), {"hook_event_name", "file_path", "memory_type", "load_reason", "session_id"})
        self.assertNotIn("PRIVATE", json.dumps(observed))
        self.assertNotIn("SECRET", json.dumps(observed))

    def test_request_facts_count_only_system_and_messages(self):
        request = {"system": [{"text": MARKERS["keep"]}], "messages": [{"content": MARKERS["rules"] * 2}],
                   "metadata": {"secret": MARKERS["existing"]}, "api_key": "SECRET"}
        facts = request_facts(request, {"rules": MARKERS["rules"] + ". Complete body."})
        self.assertEqual(facts["messages"]["markers"]["rules"][0]["occurrences"], 2)
        self.assertTrue(facts["system"]["markers"]["keep"])
        self.assertFalse(facts["messages"]["markers"]["existing"])
        self.assertNotIn(MARKERS["rules"], json.dumps(facts))
        self.assertNotIn("SECRET", json.dumps(facts))
        self.assertFalse(facts["messages"]["full_bodies"]["rules"])

    def test_fixture_uses_user_rules_and_three_separate_output_styles(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            rule, settings = fixture(root / "claude", root)
            self.assertEqual(rule.relative_to(root).as_posix(), "claude/rules/style-m2-user-probe.md")
            self.assertEqual(rule.stat().st_mode & 0o777, 0o600)
            self.assertIn(MARKERS["rules"], rule.read_text())
            self.assertEqual(set(settings["hooks"]), {"InstructionsLoaded"})
            for name in ("existing", "replace", "keep"):
                text = (root / "claude/output-styles" / (STYLES[name] + ".md")).read_text()
                self.assertIn(MARKERS[name], text)
                self.assertIn("keep-coding-instructions: " + ("true" if name == "keep" else "false"), text)

    def test_ui_prepare_is_private_isolated_and_never_starts_a_session(self):
        with tempfile.TemporaryDirectory(prefix="m2 'design ") as directory:
            parent = Path(directory)
            root, cli = parent / "probe", parent / "fake claude"
            cli.write_text("#!/bin/sh\necho fixture\n")
            with patch.dict(os.environ, {"ANTHROPIC_API_KEY": "DO_NOT_COPY", "CODEX_HOME": "DO_NOT_COPY", "CLAUDE_CONFIG_DIR": "DO_NOT_COPY"}), patch("probe_claude_m2_design.subprocess.run", return_value=SimpleNamespace(stdout="2.1.fixture\n")) as run:
                report = prepare_ui(root, cli)
            self.assertFalse(report["acceptance"])
            self.assertEqual(run.call_count, 1)
            self.assertEqual(run.call_args.args[0], [str(cli.resolve()), "--version"])
            self.assertEqual(root.stat().st_mode & 0o777, 0o700)
            self.assertEqual(json.loads((root / "claude/settings.json").read_text())["outputStyle"], "M2Existing")
            for launcher in list(report["launchers"].values()) + [report["login"]]:
                path = Path(launcher)
                self.assertEqual(path.stat().st_mode & 0o777, 0o700)
                self.assertIn("env -i", path.read_text())
                self.assertIn("CLAUDE_CONFIG_DIR=", path.read_text())
                self.assertIn("CLAUDE_CODE_SIMPLE_SYSTEM_PROMPT=0", path.read_text())
                self.assertNotIn("DO_NOT_COPY", path.read_text())
                subprocess.run(["sh", "-n", str(path)], check=True)

    def test_nonempty_scratch_preserves_existing_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            file = root / "evidence.json"
            file.write_text("original")
            with self.assertRaisesRegex(ValueError, "scratch must be empty"):
                prepare_ui(root, "/missing/claude")
            self.assertEqual(file.read_text(), "original")

    def test_launchers_preserve_os_identity_and_isolate_config_at_execution(self):
        with tempfile.TemporaryDirectory(prefix="m2 'auth ") as directory:
            parent = Path(directory).resolve()
            root, cli, home = parent / "probe", parent / "fake claude", parent / "native home"
            home.mkdir()
            cli.write_text("#!" + sys.executable + "\nimport json, os, sys\n"
                           "print('fixture' if sys.argv[1:] == ['--version'] else "
                           "json.dumps({'env': dict(os.environ), 'cwd': os.getcwd(), 'args': sys.argv[1:]}))\n")
            cli.chmod(0o700)
            source_env = {"HOME": str(home), "USER": "fixture-user", "LOGNAME": "fixture-login",
                          "ANTHROPIC_API_KEY": "DO_NOT_COPY", "CLAUDE_CODE_OAUTH_TOKEN": "DO_NOT_COPY",
                          "ANTHROPIC_CONFIG_DIR": "DO_NOT_COPY", "XDG_CONFIG_HOME": "DO_NOT_COPY",
                          "CLAUDE_SECURESTORAGE_CONFIG_DIR": "DO_NOT_COPY", "CODEX_HOME": "DO_NOT_COPY",
                          "CODEX_SANDBOX": "DO_NOT_COPY", "CLAUDE_CONFIG_DIR": "DO_NOT_COPY"}
            with patch.dict(os.environ, source_env):
                report = prepare_ui(root, cli)
            # Execute generated scripts with different caller identity and credentials.
            # Preparation must pin the intended identity and discard these overrides.
            caller_env = {**os.environ, **source_env, "HOME": str(parent / "wrong-home"), "USER": "wrong-user"}
            for phase, launcher in {**report["launchers"], "login": report["login"]}.items():
                with self.subTest(phase=phase):
                    call = subprocess.run([launcher], env=caller_env, capture_output=True, text=True, check=True)
                    facts = json.loads(call.stdout)
                    environment = facts["env"]
                    self.assertEqual(environment["HOME"], str(home))
                    self.assertEqual(environment["USER"], "fixture-user")
                    self.assertEqual(environment["LOGNAME"], "fixture-login")
                    self.assertEqual(environment["CLAUDE_CONFIG_DIR"], str(root / "claude"))
                    self.assertEqual(environment["ANTHROPIC_CONFIG_DIR"], str(root / "anthropic"))
                    self.assertEqual(environment["XDG_CONFIG_HOME"], str(root / "xdg-config"))
                    self.assertEqual(facts["cwd"], str(root / "work"))
                    self.assertNotIn("DO_NOT_COPY", call.stdout)
                    for key in ("ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "CODEX_HOME", "CODEX_SANDBOX",
                                "CLAUDE_SECURESTORAGE_CONFIG_DIR"):
                        self.assertNotIn(key, environment)
                    if phase == "login":
                        self.assertEqual(facts["args"], ["auth", "login"])
                    else:
                        self.assertEqual(facts["args"][:2], ["--setting-sources", "user"])

    def test_rewriting_launchers_preserves_configuration_and_prior_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "work").mkdir()
            evidence = root / "instructions.jsonl"
            evidence.write_text('{"prior": "observation"}\n')
            settings = root / "existing-settings.json"
            settings.write_text('{"outputStyle": "user-selected"}\n')
            before = {path: path.read_bytes() for path in (evidence, settings)}
            launchers, login = write_ui_launchers(root, Path("/pinned/claude"))
            self.assertEqual(len(launchers), 4)
            self.assertTrue(login.is_file())
            for path, data in before.items():
                self.assertEqual(path.read_bytes(), data)


if __name__ == "__main__":
    unittest.main()
