#!/usr/bin/env python3
"""D3 static discovery investigation; loopback stub, no real model or credentials.

Reuse the reviewed isolation/response fixtures. Capture only fixed marker presence
in request fields, never request text. Exit 1 on native command/request failure;
discovery outcomes are observations, not STYLE feature acceptance.
"""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess

import probe_m3_d3_routes as codex
import probe_m3_d3_claude as claude


MARKERS = {name: "M3_D3_STATIC_" + name.upper() + "_71D2"
           for name in ("user", "project", "memory")}


def capture(stub):
    rows = []
    original = stub.server.RequestHandlerClass

    class Handler(original):
        def do_POST(self):
            body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
            value = json.loads(body)
            # Keep provenance of the field as well as presence. The fixed
            # marker names are absent from the prompt and fixture stub reply.
            rows.append({
                "path": self.path.split("?")[0],
                "markers": {name: sorted(key for key, part in value.items()
                                        if marker in json.dumps(part))
                            for name, marker in MARKERS.items()},
            })
            self.rfile = io.BytesIO(body)
            super().do_POST()

    stub.server.RequestHandlerClass = Handler
    return rows


def run_codex(root, executable):
    stub = codex.Stub()
    requests = capture(stub)
    rows = []
    try:
        for name, trusted, project_static in (
                ("codex-user-only", True, False),
                ("codex-project-trusted", True, True),
                ("codex-project-untrusted", False, True)):
            base = root / name
            home, cfg, project = (base / part for part in ("home", "config", "project"))
            for path in (home, cfg, project / ".codex"):
                path.mkdir(parents=True, mode=0o700)
            codex.git_init(project)
            config = (f'model = "m3-d3-stub"\nmodel_provider = "stub"\n'
                      f'developer_instructions = {json.dumps(MARKERS["user"])}\n'
                      f'[model_providers.stub]\nname = "stub"\nbase_url = "{stub.url}"\n'
                      'wire_api = "responses"\nrequest_max_retries = 0\nstream_max_retries = 0\n')
            if trusted:
                config += codex.project_trust_toml(project)
            (cfg / "config.toml").write_text(config)
            if project_static:
                (project / ".codex/config.toml").write_text(
                    f'developer_instructions = {json.dumps(MARKERS["project"])}\n')
            env = {"PATH": os.environ["PATH"], "HOME": str(home), "CODEX_HOME": str(cfg),
                   "LANG": "C.UTF-8"}
            before = len(requests)
            with (base / "stdout").open("w") as out, (base / "stderr").open("w") as err:
                p = subprocess.run([executable, "exec", "--skip-git-repo-check", "-C", str(project), codex.PROMPT],
                                   cwd=project, env=env, stdin=subprocess.DEVNULL,
                                   stdout=out, stderr=err, timeout=180)
            rows.append({"case": name, "exit_code": p.returncode, "requests": requests[before:]})
    finally:
        stub.close()
    return rows


def run_claude(root, executable):
    stub = claude.Stub()
    requests = capture(stub)
    rows = []
    try:
        for name, nested in (("claude-static-root", False), ("claude-static-subdir", True)):
            case = claude.Case(root, name, stub, executable)
            case.seed()
            for path, marker in ((case.cfg / "rules/user.md", MARKERS["user"]),
                                 (case.project / ".claude/rules/project.md", MARKERS["project"]),
                                 (case.project / "CLAUDE.md", MARKERS["memory"])):
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(marker + "\n")
            cwd = case.project
            if nested:
                cwd = case.project / "nested/dir"
                cwd.mkdir(parents=True)
            before = len(requests)
            code = case.run_print(cwd)
            rows.append({"case": name, "exit_code": code, "requests": requests[before:]})
    finally:
        stub.close()
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scratch", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--claude", default="claude")
    args = parser.parse_args()
    root = Path(args.scratch).resolve()
    if root.exists() and any(root.iterdir()):
        parser.error("scratch directory must be empty")
    root.mkdir(parents=True, exist_ok=True, mode=0o700)
    versions = {name: subprocess.run([exe, "--version"], check=True, capture_output=True,
                                    text=True, timeout=20).stdout.strip()
                for name, exe in (("codex", args.codex), ("claude", args.claude))}
    rows = run_codex(root, args.codex) + run_claude(root, args.claude)
    report = {"task": "M3-D3 static discovery investigation", "versions": versions,
              "model_service": "loopback stubs; no inference; no real credentials",
              "probe_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              "results": rows}
    Path(args.output).write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))
    return 0 if all(row["exit_code"] == 0 and row["requests"] for row in rows) else 1


if __name__ == "__main__":
    raise SystemExit(main())
