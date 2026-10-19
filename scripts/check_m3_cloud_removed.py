#!/usr/bin/env python3
"""M3-A19: removed cloud commands and retained local project behavior.

Version stubs and configured-hook calls are offline fixtures, not native
harness delivery. No cloud account, model or credentials are used.
"""
import argparse
import copy
import json
from pathlib import Path
import shutil
import tempfile

from check_m3_project import invoke_hook
from check_m3_update import fixture, manifest, record, run, sha, snapshot
from style_json import report as style_report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--report", required=True, type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    cases = []
    with tempfile.TemporaryDirectory(prefix="style-m3-cloud-removed-") as temp:
        root = Path(temp).resolve()
        env, base, codex, claude, stub = fixture(root)
        (stub / "style").symlink_to(binary)
        project = root / "project ' space"
        project.mkdir()
        original = snapshot(project, codex, claude)
        help_text = run(binary, env, "--help").stdout
        assert "style export" not in help_text and "claude-cloud" not in help_text
        record(cases, "removed", "help-does-not-advertise-export")
        before = snapshot(root)
        for command in (("export", "--help"),
                        ("export", "--target=claude-cloud", "--yes", "--json"),
                        ("export", "--target=claude-cloud", "--project", project, "--yes", "--json")):
            # Unknown commands are usage errors (exit 2).
            result = run(binary, env, *command, want=2)
            assert result.stderr and snapshot(root) == before
        record(cases, "removed", "export-rejected-without-writes", commands=3)
        source = root / "source"
        source.mkdir()
        (source / "STYLE.yaml").write_text(manifest("local", "Local route still works."))
        installed = style_report(run(binary, env, "install", source, "--project", project,
                                   "--harness=claude-code", "--json", confirm=True).stdout)
        home = Path(installed["scope"]["home"])
        assert not (project / ".style/cloud").exists()
        assert not (home / "cloud-export.json").exists()
        settings = json.loads((project / ".claude/settings.json").read_text())
        hook = next(h for group in settings["hooks"]["PreToolUse"] for h in group["hooks"]
                    if "agenthooks" in str(h))
        record(cases, "claude-project", "local-install-without-cloud-artifacts")
        payload = json.dumps({"hook_event_name": "PreToolUse", "session_id": "removal",
                              "tool_name": "Bash", "tool_input": {"command": "true"}})
        for remote in ("", "false"):
            local_payload = json.dumps({**json.loads(payload), "session_id": "local-" + remote})
            result = invoke_hook(hook, {**env, "CLAUDE_CODE_REMOTE": remote}, project, local_payload)
            assert result.returncode == 0 and "Local route still works." in result.stdout
        record(cases, "claude-project", "local-configured-hook-unchanged", calls=2)
        clone = root / "unbound clone"
        shutil.copytree(project, clone)
        for cwd, name in ((project, "bound"), (clone, "unbound-clone")):
            for closed in (False, True):
                selected = copy.deepcopy(hook)
                if closed:
                    if selected.get("args"):
                        selected["args"].append("--fail-closed")
                    else:
                        selected["command"] += " --fail-closed"
                before = snapshot(root)
                result = invoke_hook(selected, {**env, "CLAUDE_CODE_REMOTE": "true"}, cwd, payload)
                assert result.returncode == (1 if closed else 0), result
                assert "no longer supported" in result.stderr
                assert "Local route still works." not in result.stdout
                assert snapshot(root) == before
                record(cases, "removed", name + ("-closed" if closed else "-open"),
                       zero_writes=True, no_rule_delivery=True)
        result = invoke_hook(hook, env, clone, payload)
        assert result.returncode == 0 and "no local binding" in result.stderr
        assert "Local route still works." not in result.stdout
        record(cases, "claude-project", "unbound-local-clone-keeps-binding-requirement")
        before = snapshot(root)
        run(binary, env, "uninstall", "--all", "--project", project, "--json", want=1)
        assert snapshot(root) == before
        record(cases, "claude-project", "uninstall-still-requires-confirmation")
        removed = style_report(run(binary, env, "uninstall", "--all", "--project", project, "--json", confirm=True).stdout)
        assert removed["changed"] and "cloud_files" not in removed
        assert "awaiting_confirmation" not in removed
        assert snapshot(project, codex, claude) == original
        record(cases, "claude-project", "uninstall-restores-local-files-without-cloud-fields")
    args.report.write_text(json.dumps({"format": 1, "passed": True,
                                      "binary_sha256": sha(binary), "real_harness": False,
                                      "cases": cases}, indent=2) + "\n")
    print(json.dumps({"passed": True, "checks": len(cases)}))


if __name__ == "__main__":
    main()
