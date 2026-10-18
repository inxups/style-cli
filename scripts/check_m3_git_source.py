#!/usr/bin/env python3
"""M3-A3: real old-binary lock gate, runtime wire and invalid Git CLI inputs.

Export an isolated fixture with TestM3A3ExportFormatTwoFixture first.
Uses no real harness, credentials or remote repository.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tempfile


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def snapshot(root):
    result = {}
    for path in [root, *sorted(root.rglob("*"))]:
        info = path.lstat()
        identity = (info.st_mode, info.st_dev, info.st_ino, info.st_mtime_ns)
        if stat.S_ISLNK(info.st_mode):
            content = os.readlink(path)
        elif stat.S_ISREG(info.st_mode):
            content = digest(path)
        else:
            assert stat.S_ISDIR(info.st_mode), (path, "unexpected fixture entry")
            content = None
        result[str(path.relative_to(root))] = (identity, content)
    return result


def run(binary, env, args, payload=None):
    return subprocess.run([str(binary), *map(str, args)], input=payload, env=env,
                          capture_output=True, text=True, timeout=10)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--old-binary", type=Path, required=True)
    parser.add_argument("--fixture", type=Path, required=True)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    new, old, fixture = (path.resolve() for path in
                         (args.binary, args.old_binary, args.fixture))
    home, config = fixture / "home", fixture / "config"
    assert (home / "style.lock.yaml").is_file(), "export the test fixture first"
    env = {"STYLE_HOME": str(home), "HOME": str(fixture),
           "XDG_CONFIG_HOME": str(fixture / "xdg"), "PATH": os.defpath, "LC_ALL": "C"}
    cases = []
    with tempfile.TemporaryDirectory(prefix="style-a3-gate-") as temporary:
        private = Path(temporary)
        local = private / "local"
        local.mkdir()
        (local / "STYLE.yaml").write_text(
            "spec: style/v0.3\nname: local\nversion: 1.0.0\ndescription: local\n"
            "rules: [{id: base, target: system, text: local}]\n")
        commands = [
            ["list", "--json"], ["sync", "--json"],
            ["trust", "native-test", "--yes", "--json"],
            ["uninstall", "--all", "--yes", "--json"],
            ["install", str(local), "--harness", "codex", "--yes", "--json"],
            ["doctor", "--json"],
        ]
        for command in commands:
            before = snapshot(fixture)
            result = run(old, env, command)
            assert result.returncode == 1 and "unsupported lock format 2" in (
                result.stdout + result.stderr), (command, result)
            assert before == snapshot(fixture), (command, "format 2 installation changed")
            cases.append({"command": command[0], "exit_code": 1,
                          "unsupported_format_2": True,
                          "tree_identity_bytes_modes_mtimes_preserved": True})

        before = snapshot(fixture)
        result = run(new, env, ["list", "--json"])
        assert result.returncode == 0, result
        package = json.loads(result.stdout)["packages"][0]
        assert package["git"]["commit"] and package["hash_matches"]
        assert before == snapshot(fixture), "new list changed the installation"
        cases.append({"command": "new-list", "exit_code": 0,
                      "git_metadata_present": True, "read_only": True})

        payload = json.dumps({"session_id": "a3-real-binary", "hook_event_name": "PreToolUse",
                              "tool_name": "Bash", "tool_input": {"command": "true"},
                              "cwd": "/offline"})
        outputs = []
        for number, binary in enumerate((old, new)):
            runtime_home = private / f"runtime-{number}"
            (runtime_home / "build").mkdir(parents=True)
            shutil.copy(home / "build" / "index.json", runtime_home / "build" / "index.json")
            shutil.copy(home / "style.lock.yaml", runtime_home / "style.lock.yaml")
            command = [f"--style-home={runtime_home}", f"--harness-home={config}",
                       "--harness-version=2.1.315", "agenthooks", "run",
                       "--provider=codex", "--owner=style"]
            result = run(binary, env, command, payload)
            assert result.returncode == 0 and "Be careful." in result.stdout, (binary, result)
            outputs.append(result.stdout)
        assert outputs[0] == outputs[1], "ordinary runtime wire changed"
        cases.append({"command": "old-new-runtime", "exit_code": 0, "wire_identical": True,
                      "stdout_sha256": hashlib.sha256(outputs[0].encode()).hexdigest(),
                      "index_format": 1, "lock_format": 2})

        stub = private / "bin"
        stub.mkdir()
        marker = stub / "git-called"
        git = stub / "git"
        git.write_text('#!/bin/sh\nprintf called > "$0-called"\nexit 99\n')
        git.chmod(0o700)
        cli_home = private / "cli-home"
        cli_env = env | {"HOME": str(private), "STYLE_HOME": str(cli_home), "PATH": str(stub)}
        for command in ("validate", "plan", "install"):
            inputs = [
                ["git+https://user:CANARY@example.invalid/repo.git"],
                ["git+http://example.invalid/repo"],
                [str(local), "--ref", "refs/heads/main"],
            ]
            for source in inputs:
                result = run(new, cli_env, [command, *source, "--json"])
                output = result.stdout + result.stderr
                assert result.returncode == 1 and "source-invalid" in output, (command, source, result)
                assert "CANARY" not in output, "credentials leaked into a diagnostic"
                assert not marker.exists() and not cli_home.exists(), "invalid input reached Git or wrote home"
                cases.append({"command": command, "input_class": "invalid_remote_or_combination",
                              "exit_code": 1, "no_git_no_home_write": True, "secret_absent": True})

    report = {"status": "passed", "new_binary_sha256": digest(new),
              "old_binary_sha256": digest(old),
              "format_two_fixture": "real TestM3A3ExportFormatTwoFixture production Install output",
              "cases": cases}
    if args.report:
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": "passed", "cases": len(cases),
                      "new_binary_sha256": report["new_binary_sha256"],
                      "old_binary_sha256": report["old_binary_sha256"]}))


if __name__ == "__main__":
    main()
