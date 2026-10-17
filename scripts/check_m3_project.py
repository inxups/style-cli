#!/usr/bin/env python3
"""M3-A6 built CLI, portable entry and old-program refusal, entirely offline.

Version stubs and direct configured-hook invocations are fixtures, never real
harness delivery or workspace trust evidence.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

from check_m3_update import fixture, manifest, record, run, sha, snapshot


def invoke_hook(hook, env, cwd, payload):
    if hook.get("args"):
        command = [hook["command"], *[arg.replace("${CLAUDE_PROJECT_DIR}", str(cwd))
                                      for arg in hook["args"]]]
    else:
        command = ["/bin/sh", "-c", hook["command"]]
    return subprocess.run(command, cwd=cwd, env={**env, "PWD": str(cwd),
                          "CLAUDE_PROJECT_DIR": str(cwd)}, input=payload,
                          text=True, capture_output=True, timeout=15)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--old-binary", required=True, type=Path)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    new, old = args.binary.resolve(), args.old_binary.resolve()
    cases = []
    for provider in ("codex", "claude-code"):
        route = "codex-project" if provider == "codex" else "claude-project"
        with tempfile.TemporaryDirectory(prefix="style-m3-project-") as temporary:
            root = Path(temporary).resolve()
            env, base, codex, claude, stub = fixture(root)
            (stub / "style").symlink_to(new)
            project = root / "project ' space"
            project.mkdir()
            source = root / "source"
            source.mkdir()
            (source / "STYLE.yaml").write_text(manifest("project-rules", "Project.", native=True))
            before = snapshot(base, codex, claude, project)
            preview = json.loads(run(new, env, "install", source, "--project", project,
                                     "--harness", provider, "--json", want=1).stdout)
            assert preview["scope"]["kind"] == "project" and preview["native_hooks"]
            assert snapshot(base, codex, claude, project) == before
            record(cases, route, "preview-zero-write")
            installed = json.loads(run(new, env, "install", source, "--project", project,
                                       "--harness", provider, "--yes", "--json").stdout)
            home = Path(installed["scope"]["home"])
            assert home.parent == base / "projects"
            assert "format: 3" in (home / "style.lock.yaml").read_text()
            assert json.loads((home / "build/index.json").read_text())["format"] == 1
            declaration = json.loads((project / ".style/project.json").read_text())
            assert declaration["effects_sha256"].startswith("sha256:")
            assert str(root) not in json.dumps(declaration)
            record(cases, route, "install-format3-private-scope")
            for command in (("list",), ("sync",), ("trust", "project-rules", "--yes"),
                            ("update", "--all", "--yes"), ("install", "--frozen", "--yes"),
                            ("doctor",), ("stats",)):
                report = json.loads(run(new, env, *command, "--project", project, "--json").stdout)
                assert report["scope"]["home"] == str(home)
            record(cases, route, "management-scope-consistency", commands=7)
            before = snapshot(base, codex, claude, project)
            for command in (("list",), ("sync",), ("trust", "project-rules", "--yes"),
                            ("uninstall", "--all", "--yes"), ("update", "--all", "--yes"),
                            ("install", "--frozen", "--yes")):
                result = run(old, {**env, "STYLE_HOME": str(home)}, *command, want=1)
                message = result.stdout + result.stderr
                assert any(code in message for code in ("unsupported lock format 3", "lock-invalid", "frozen-lock-invalid", "frozen-metadata-incomplete")), (command, message)
                assert snapshot(base, codex, claude, project) == before
            record(cases, route, "old-binary-format3-refusal", commands=6,
                   bytes_mode_mtime_inode=True)
            native_path = project / (".codex/hooks.json" if provider == "codex" else ".claude/settings.json")
            groups = json.loads(native_path.read_text())["hooks"]["PreToolUse"]
            handlers = [hook for group in groups for hook in group["hooks"]]
            dynamic = next(hook for hook in handlers if "agenthooks" in str(hook))
            raw = next(hook for hook in handlers if hook["command"].startswith(": --owner=style"))
            payload = json.dumps({"hook_event_name": "PreToolUse", "session_id": "same-session",
                                  "tool_name": "Bash", "tool_input": {"command": "true"},
                                  "cwd": "/forged-payload-root"})
            result = invoke_hook(dynamic, env, project, payload)
            assert result.returncode == 0 and "Project." in result.stdout, (result.stdout, result.stderr)
            assert (home / "state").is_dir() and not (base / "state").exists()
            assert "NATIVE-" not in result.stdout
            record(cases, route, "configured-entry-runtime-wire", real_harness=False)
            # Only this known fixture's printf command is executed, from a child
            # directory. Native author commands retain native cwd semantics.
            child = project / "child"
            child.mkdir()
            result = invoke_hook(raw, env, child, payload)
            assert result.returncode == 0 and result.stdout == "NATIVE-Project."
            record(cases, route, "native-fixture-command-unchanged")
            clone = root / "clone"
            git_env = {**env, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
                       "GIT_TERMINAL_PROMPT": "0"}
            for command in (("init", "-q", str(project)),
                            ("-C", str(project), "add", "--all"),
                            ("-C", str(project), "-c", "user.name=Fixture", "-c",
                             "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false",
                             "commit", "-q", "-m", "Portable project fixture"),
                            ("-C", str(project), "worktree", "add", "-q", "--detach", str(clone), "HEAD")):
                completed = subprocess.run(["git", *command], env=git_env,
                                           text=True, capture_output=True, timeout=15)
                assert completed.returncode == 0, completed.stderr
            result = invoke_hook(dynamic, env, clone, payload)
            assert result.returncode == 0 and "Project." not in result.stdout and "no local binding" in result.stderr
            assert not (base / "state").exists()
            record(cases, route, "unbound-clone-no-user-fallback")
            adopted = json.loads(run(new, env, "install", source, "--project", clone,
                                     "--harness", provider, "--yes", "--json").stdout)
            assert adopted["scope"]["home"] != str(home)
            assert adopted["project_adoption"] is True
            result = invoke_hook(dynamic, env, clone, payload)
            assert result.returncode == 0 and "Project." in result.stdout
            record(cases, route, "clone-explicit-activation-independent-state", real_git_worktree=True)
            run(new, env, "uninstall", "--all", "--project", clone, "--yes")
            assert not (clone / ".style/project.json").exists()
            assert not (clone / ".style/hooks/style").exists()
            clone_native = clone / native_path.relative_to(project)
            if clone_native.exists():
                assert "--owner=style" not in clone_native.read_text()
            original = json.loads(run(new, env, "list", "--project", project, "--json").stdout)
            assert original["packages"][0]["name"] == "project-rules"
            record(cases, route, "clone-uninstall-original-preserved")
    report = {"format": 1, "binary_sha256": sha(new), "old_binary_sha256": sha(old),
              "status": "passed", "real_harness": False, "cases": cases}
    if args.report:
        args.report.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"status": "passed", "cases": len(cases)}))


if __name__ == "__main__":
    main()
