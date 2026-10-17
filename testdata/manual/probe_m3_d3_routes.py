#!/usr/bin/env python3
"""M3-D3 isolated investigation: does Codex load project-level `.codex/hooks.json`?

Every case runs in a fresh private CODEX_HOME/HOME and temporary Git project.
The model service is a local stub (no real inference, no credentials), so only
hook discovery, project trust and per-hook trusted_hash behaviour are observed.
Raw logs stay in --scratch; --output receives whitelisted facts only.
This is investigation evidence for the D3 draft, not an M3-A6/B2 acceptance.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


EVENTS = ("SessionStart", "UserPromptSubmit")
LABELS = {"SessionStart": "session_start", "UserPromptSubmit": "user_prompt_submit"}
PROMPT = "M3_D3_PROBE reply ok"


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


def definition_hash(event, command, timeout):
    """Python twin of agenthooks/install.DefinitionHash for matcher-less events."""
    identity = {"event_name": LABELS[event],
                "hooks": [{"async": False, "command": command, "timeout": timeout, "type": "command"}]}
    text = json.dumps(identity, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return "sha256:" + hashlib.sha256(text.encode()).hexdigest()


def hooks_document(marks, tag):
    hooks = {}
    for event in EVENTS:
        # One file per invocation so duplicate delivery is countable.
        command = f'cat > "$(mktemp {marks}/{tag}-{LABELS[event]}.XXXXXX)"'
        hooks[event] = [{"hooks": [{"type": "command", "command": command, "timeout": 5}]}]
    return {"hooks": hooks}


def trust_toml(project, document, resolve=True):
    source = (Path(project).resolve() if resolve else Path(project)) / ".codex" / "hooks.json"
    lines = []
    for event in EVENTS:
        command = document["hooks"][event][0]["hooks"][0]["command"]
        key = f"{source}:{LABELS[event]}:0:0"
        lines += [f"[hooks.state.{json.dumps(key)}]", f"trusted_hash = {json.dumps(definition_hash(event, command, 5))}", ""]
    return "\n".join(lines)


def project_trust_toml(project, resolve=True):
    return f"[projects.{json.dumps(str(Path(project).resolve() if resolve else Path(project)))}]\ntrust_level = \"trusted\"\n"


# Each case: which config pieces exist. "project"/"hooks" go to the user
# CODEX_HOME config.toml; "repo_hooks_state" puts hook trust inside the
# repository's own .codex/config.toml; "clone" runs from a copy at a new path
# while trust names only the original path; "user_same" also installs the
# identical command in CODEX_HOME/hooks.json; "subdir"/"symlink" change cwd.
CASES = {
    "untrusted": {},
    "project-trusted-only": {"project": True},
    "hook-hash-only": {"hooks": True},
    "project-and-hook-trusted": {"project": True, "hooks": True},
    "repo-self-declared-hook-trust": {"project": True, "repo_hooks_state": True},
    "clone-with-original-trust": {"project": True, "hooks": True, "clone": True},
    "clone-project-trusted-only": {"project": True, "hooks": True, "clone": True, "trust_clone_project": True},
    "user-and-project-same-command": {"project": True, "hooks": True, "user_same": True},
    "run-from-subdir": {"project": True, "hooks": True, "subdir": True},
    "run-via-symlink": {"project": True, "hooks": True, "symlink": True},
    # Same symlink launch, adding trust spelled with the unresolved link path.
    "symlink-link-project-and-hooks": {"project": True, "hooks": True, "symlink": True, "link_project": True, "link_hooks": True},
    "symlink-link-project-only": {"project": True, "hooks": True, "symlink": True, "link_project": True},
    "symlink-link-hooks-only": {"project": True, "hooks": True, "symlink": True, "link_hooks": True},
}


def user_trust_toml(codex_home, document):
    source = Path(codex_home).resolve() / "hooks.json"
    lines = []
    for event in EVENTS:
        command = document["hooks"][event][0]["hooks"][0]["command"]
        lines += [f"[hooks.state.{json.dumps(f'{source}:{LABELS[event]}:0:0')}]",
                  f"trusted_hash = {json.dumps(definition_hash(event, command, 5))}", ""]
    return "\n".join(lines)


class Stub:
    """Minimal Responses SSE endpoint; records request count and nothing else."""

    def __init__(self):
        self.requests = 0
        owner = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def do_POST(self):
                self.rfile.read(int(self.headers.get("Content-Length", 0)))
                owner.requests += 1
                events = [
                    {"type": "response.created", "response": {"id": "resp_m3d3"}},
                    {"type": "response.output_item.done", "item": {"type": "message", "role": "assistant", "id": "msg_m3d3",
                                                                  "content": [{"type": "output_text", "text": "ok"}]}},
                    {"type": "response.completed", "response": {"id": "resp_m3d3", "usage": {
                        "input_tokens": 1, "input_tokens_details": None, "output_tokens": 1,
                        "output_tokens_details": None, "total_tokens": 2}}},
                ]
                body = "".join(f"event: {e['type']}\ndata: {json.dumps(e)}\n\n" for e in events).encode()
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.url = f"http://127.0.0.1:{self.server.server_address[1]}/v1"

    def close(self):
        self.server.shutdown()


def git_init(path):
    subprocess.run(["git", "init", "-q", str(path)], check=True, env={"PATH": os.environ["PATH"], "HOME": str(path.parent)})


def run_case(name, spec, root, stub, codex):
    base = root / name
    home, codex_home, marks = base / "home", base / "codex", base / "marks"
    for path in (home, codex_home, marks):
        path.mkdir(parents=True, mode=0o700)
    project = base / "project"
    project.mkdir()
    git_init(project)
    document = hooks_document(marks, "original")
    (project / ".codex").mkdir()
    (project / ".codex/hooks.json").write_text(json.dumps(document, indent=2) + "\n")
    config = [f'model = "m3-d3-stub"\nmodel_provider = "stub"\n\n[model_providers.stub]\nname = "stub"\n'
              f'base_url = "{stub.url}"\nwire_api = "responses"\nrequest_max_retries = 0\nstream_max_retries = 0\n']
    if spec.get("project"):
        config.append(project_trust_toml(project))
    if spec.get("hooks"):
        config.append(trust_toml(project, document))
    if spec.get("repo_hooks_state"):
        (project / ".codex/config.toml").write_text(trust_toml(project, document))
    run_dir = project
    if spec.get("clone"):
        run_dir = base / "clone"
        shutil.copytree(project, run_dir, symlinks=True)
        clone_doc = hooks_document(marks, "clone")
        (run_dir / ".codex/hooks.json").write_text(json.dumps(clone_doc, indent=2) + "\n")
        if spec.get("trust_clone_project"):
            config.append(project_trust_toml(run_dir))
    if spec.get("user_same"):
        (codex_home / "hooks.json").write_text(json.dumps(document, indent=2) + "\n")
        config.append(user_trust_toml(codex_home, document))
    if spec.get("subdir"):
        run_dir = project / "nested" / "dir"
        run_dir.mkdir(parents=True)
    if spec.get("symlink"):
        run_dir = base / "link"
        run_dir.symlink_to(project)
        if spec.get("link_project"):
            config.append(project_trust_toml(run_dir, resolve=False))
        if spec.get("link_hooks"):
            config.append(trust_toml(run_dir, document, resolve=False))
    config_path = codex_home / "config.toml"
    config_path.write_text("\n".join(config))
    before = config_path.read_bytes()
    env = {"PATH": os.environ["PATH"], "HOME": str(home), "CODEX_HOME": str(codex_home), "LANG": "C.UTF-8",
           "OTEL_SDK_DISABLED": "true"}
    requests_before = stub.requests
    with (base / "codex.stdout").open("w") as out, (base / "codex.stderr").open("w") as err:
        code = subprocess.run([codex, "exec", "--skip-git-repo-check", "-C", str(run_dir), PROMPT],
                              env=env, cwd=run_dir, stdout=out, stderr=err, stdin=subprocess.DEVNULL, timeout=180).returncode
    after = config_path.read_bytes()
    fired = {}
    for mark in marks.iterdir():
        label = mark.name.rsplit(".", 1)[0]
        fired[label] = fired.get(label, 0) + 1
    fired = dict(sorted(fired.items()))
    payload_keys = {}
    for mark in marks.iterdir():
        try:
            payload = json.loads(mark.read_text())
            payload_keys[mark.name.rsplit(".", 1)[0]] = sorted(payload.keys())
            payload_keys[mark.name.rsplit(".", 1)[0] + ":cwd"] = payload.get("cwd", "").replace(str(base), "<case>")
        except ValueError:
            payload_keys[mark.name.rsplit(".", 1)[0]] = "unparsed"
    stderr = (base / "codex.stderr").read_text(errors="replace")
    hook_lines = sorted({line.strip()[:200].replace(str(base), "<case>") for line in stderr.splitlines()
                         if "hook" in line.lower() or "trust" in line.lower()})
    # Project trust also changes Codex's default sandbox; record that line only.
    sandbox = next((line.removeprefix("sandbox:").strip() for line in stderr.splitlines() if line.startswith("sandbox:")), None)
    return {"case": name, "spec": spec, "exit_code": code, "stub_requests": stub.requests - requests_before, "sandbox": sandbox,
            "fired": fired, "payload_keys": payload_keys,
            "user_config_changed": before != after, "user_config_before": digest(before), "user_config_after": digest(after),
            "user_config_added_text": after.decode()[len(before):].replace(str(base), "<case>") if after.startswith(before) and after != before else None,
            "stderr_hook_lines": hook_lines}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scratch", required=True, help="empty private directory for raw logs")
    parser.add_argument("--output", required=True, help="whitelisted JSON result")
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--case", action="append", choices=sorted(CASES))
    args = parser.parse_args()
    root = Path(args.scratch).resolve()
    if root.exists() and any(root.iterdir()):
        raise SystemExit("scratch directory must be empty")
    root.mkdir(parents=True, exist_ok=True, mode=0o700)
    version = subprocess.run([args.codex, "--version"], capture_output=True, text=True, check=True).stdout.strip()
    stub = Stub()
    try:
        results = [run_case(name, CASES[name], root, stub, args.codex) for name in (args.case or CASES)]
    finally:
        stub.close()
    report = {"probe": "M3-D3 codex project hooks", "codex_version": version, "model_service": "local stub, no inference",
              "results": results}
    Path(args.output).write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n")
    for row in results:
        print(f"{row['case']}: exit={row['exit_code']} fired={row['fired']} config_changed={row['user_config_changed']}")


if __name__ == "__main__":
    main()
