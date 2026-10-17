#!/usr/bin/env python3
"""M3-D3 isolated investigation: Claude Code project hooks and local plugin marketplace.

Each case uses a fresh HOME, CLAUDE_CONFIG_DIR and Git project under --scratch;
the user's real ~/.claude is never read or written. The model service is a
loopback Anthropic Messages stub; ANTHROPIC_API_KEY is a fixed placeholder that
only the stub sees. Raw logs stay in --scratch; --output gets whitelisted facts.
Investigation evidence for the D3 draft, not an M3-A6/A8/C2/C4 acceptance.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import pty
import re
import select
import shutil
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


PLACEHOLDER_KEY = "sk-ant-m3d3-local-stub-placeholder-0000"
PROMPT = "M3_D3_PROBE reply ok"
MODEL = "claude-sonnet-5-5"
VOLATILE = (".claude.json", "projects", "sessions", "session-env", "backups", "statsig", "todos",
            "shell-snapshots", "debug", "telemetry", "cache", "paste-cache", "file-history", "plans")
EVENTS = {"SessionStart": "ss", "UserPromptSubmit": "ups"}


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


class Stub:
    """Anthropic Messages endpoint answering "ok"; records only method/path."""

    def __init__(self):
        self.paths = []
        owner = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def do_GET(self):
                owner.paths.append("GET " + self.path.split("?")[0])
                self.send_response(404)
                self.end_headers()

            def do_POST(self):
                body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
                path = self.path.split("?")[0]
                owner.paths.append("POST " + path)
                if path != "/v1/messages":
                    self.reply("application/json", b'{"input_tokens":1}')
                    return
                request = json.loads(body)
                message = {"id": "msg_m3d3", "type": "message", "role": "assistant", "model": request.get("model", MODEL),
                           "content": [], "stop_reason": None, "stop_sequence": None,
                           "usage": {"input_tokens": 1, "output_tokens": 1}}
                if not request.get("stream"):
                    self.reply("application/json", json.dumps(dict(message, content=[{"type": "text", "text": "ok"}],
                                                                   stop_reason="end_turn")).encode())
                    return
                events = [("message_start", {"type": "message_start", "message": message}),
                          ("content_block_start", {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}),
                          ("content_block_delta", {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "ok"}}),
                          ("content_block_stop", {"type": "content_block_stop", "index": 0}),
                          ("message_delta", {"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence": None},
                                             "usage": {"output_tokens": 1}}),
                          ("message_stop", {"type": "message_stop"})]
                self.reply("text/event-stream", "".join(f"event: {e}\ndata: {json.dumps(d)}\n\n" for e, d in events).encode())

            def reply(self, kind, data):
                self.send_response(200)
                self.send_header("Content-Type", kind)
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.url = f"http://127.0.0.1:{self.server.server_address[1]}"

    def close(self):
        self.server.shutdown()


class Case:
    def __init__(self, root, name, stub, claude):
        self.base = root / name
        self.home, self.cfg, self.marks, self.project = (self.base / n for n in ("home", "cfg", "marks", "project"))
        for path in (self.home, self.cfg, self.marks, self.project / ".claude"):
            path.mkdir(parents=True, mode=0o700)
        subprocess.run(["git", "init", "-q", str(self.project)], check=True, env={"PATH": os.environ["PATH"], "HOME": str(self.home)})
        self.mark = self.base / "mark.sh"
        self.mark.write_text(f'#!/bin/sh\nf=$(mktemp "{self.marks}/$1.XXXXXX")\n'
                             '{ printf "%s\\n%s\\n" "${CLAUDE_PROJECT_DIR-}" "${CLAUDE_PLUGIN_ROOT-}"; cat; } > "$f"\n')
        self.mark.chmod(0o755)
        self.stub, self.claude, self.runs = stub, claude, 0
        self.env = {"PATH": os.environ["PATH"], "HOME": str(self.home), "CLAUDE_CONFIG_DIR": str(self.cfg),
                    "ANTHROPIC_BASE_URL": stub.url, "ANTHROPIC_API_KEY": PLACEHOLDER_KEY, "LANG": "C.UTF-8",
                    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1", "DISABLE_AUTOUPDATER": "1", "TERM": "xterm-256color"}

    def clean(self, text):
        return text.replace(str(self.base), "<case>") if text else text

    def handler(self, form, tag, extra=(), script=None):
        script = script or str(self.mark)
        if form == "shell":
            return {"type": "command", "command": f"/bin/sh {script} {tag}"}
        return {"type": "command", "command": "/bin/sh", "args": [script, tag, *extra]}

    def hooks(self, form, label, extra=(), script=None):
        return {"hooks": {event: [{"hooks": [self.handler(form, f"{label}-{short}", extra, script)]}]
                          for event, short in EVENTS.items()}}

    def write(self, path, document):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(document, indent=2) + "\n")

    def seed(self, trusted_paths=()):
        state = {"hasCompletedOnboarding": True, "theme": "dark",
                 "customApiKeyResponses": {"approved": [PLACEHOLDER_KEY[-20:]], "rejected": []},
                 "projects": {str(p): {"hasTrustDialogAccepted": True} for p in trusted_paths}}
        self.write(self.cfg / ".claude.json", state)

    def fired(self):
        counts, env = {}, {}
        for mark in sorted(self.marks.iterdir()):
            tag = mark.name.rsplit(".", 1)[0]
            counts[tag] = counts.get(tag, 0) + 1
            lines = mark.read_text().split("\n", 2)
            env.setdefault(tag, {"CLAUDE_PROJECT_DIR": self.clean(lines[0]), "CLAUDE_PLUGIN_ROOT": self.clean(lines[1])})
        return counts, env

    def reset_marks(self):
        for mark in self.marks.iterdir():
            mark.unlink()

    def run_print(self, cwd=None):
        self.runs += 1
        log = self.base / f"run{self.runs}"
        with open(f"{log}.stdout", "w") as out, open(f"{log}.stderr", "w") as err:
            code = subprocess.run([self.claude, "-p", PROMPT, "--model", MODEL], cwd=cwd or self.project, env=self.env,
                                  stdin=subprocess.DEVNULL, stdout=out, stderr=err, timeout=180).returncode
        return code

    def run_tty(self, cwd=None, wait=15.0):
        """Interactive session on a pseudo-terminal; never answers dialogs."""
        self.runs += 1
        master, slave = pty.openpty()
        process = subprocess.Popen([self.claude, PROMPT, "--model", MODEL], cwd=cwd or self.project, env=self.env,
                                   stdin=slave, stdout=slave, stderr=slave, close_fds=True)
        os.close(slave)
        screen, deadline = b"", time.time() + wait
        while time.time() < deadline:
            ready, _, _ = select.select([master], [], [], 0.5)
            if ready:
                try:
                    screen += os.read(master, 65536)
                except OSError:
                    break
        for key in (b"\x1b", b"\x03", b"\x03"):
            try:
                os.write(master, key)
            except OSError:
                pass
            time.sleep(0.4)
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
        os.close(master)
        (self.base / f"run{self.runs}.screen").write_bytes(screen)
        text = re.sub(r"\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07|\x1b.", " ", screen.decode(errors="replace"))
        text = re.sub(r"\s+", " ", text).lower()
        return {"trust_dialog_text_seen": "trust" in text and ("folder" in text or "files" in text),
                "api_key_dialog_text_seen": "api key" in text, "onboarding_text_seen": "choose the text style" in text}

    def snapshot(self, root=None):
        root = root or self.cfg
        result = {}
        for path in sorted(root.rglob("*")):
            relative = path.relative_to(root)
            if relative.parts[0] in VOLATILE or not path.is_file():
                continue
            result[str(relative)] = digest(path.read_bytes())
        return result


def diff(before, after):
    return {"added": sorted(set(after) - set(before)), "removed": sorted(set(before) - set(after)),
            "changed": sorted(k for k in set(before) & set(after) if before[k] != after[k])}


def result(case, **extra):
    counts, env = case.fired()
    return {"fired": counts, "hook_env": env, **extra}


def project_cases(root, stub, claude):
    out = {}

    case = Case(root, "project-print-untrusted", stub, claude)
    case.seed()
    case.write(case.project / ".claude/settings.json", case.hooks("shell", "project"))
    out[case.base.name] = result(case, exit_code=case.run_print())

    for name, trusted in (("project-tty-untrusted", False), ("project-tty-trusted", True)):
        case = Case(root, name, stub, claude)
        case.seed([case.project.resolve()] if trusted else ())
        case.write(case.project / ".claude/settings.json", case.hooks("shell", "project"))
        out[name] = result(case, screen=case.run_tty())

    duplicates = {
        "dup-user-project-shell": (("cfg", "shell", ()), ("project", "shell", ())),
        "dup-user-project-exec": (("cfg", "exec", ()), ("project", "exec", ())),
        "dup-user-project-exec-args-differ": (("cfg", "exec", ("--style-home=/a",)), ("project", "exec", ("--style-home=/b",))),
        "dup-project-and-local-shell": (("project", "shell", ()), ("local", "shell", ())),
    }
    for name, layers in duplicates.items():
        case = Case(root, name, stub, claude)
        case.seed()
        for layer, form, extra in layers:
            target = {"cfg": case.cfg / "settings.json", "project": case.project / ".claude/settings.json",
                      "local": case.project / ".claude/settings.local.json"}[layer]
            case.write(target, case.hooks(form, "dup", extra))
        out[name] = result(case, exit_code=case.run_print(), layers=[layer for layer, _, _ in layers])

    # ${CLAUDE_PROJECT_DIR} substitution in exec-form args, from the root,
    # a nested directory and a symlinked spelling of the project.
    for name in ("projdir-root", "projdir-subdir", "projdir-symlink"):
        case = Case(root, name, stub, claude)
        case.seed()
        shutil.copy2(case.mark, case.project / ".claude/mark.sh")
        case.write(case.project / ".claude/settings.json",
                   case.hooks("exec", "projdir", script="${CLAUDE_PROJECT_DIR}/.claude/mark.sh"))
        cwd = case.project
        if name == "projdir-subdir":
            cwd = case.project / "nested/dir"
            cwd.mkdir(parents=True)
        if name == "projdir-symlink":
            cwd = case.base / "link"
            cwd.symlink_to(case.project)
        out[name] = result(case, exit_code=case.run_print(cwd), cwd=case.clean(str(cwd)))

    # Discovery from a nested directory with absolute script paths, so a
    # missing hook means the project file was not loaded (not a bad path).
    case = Case(root, "subdir-discovery", stub, claude)
    case.seed()
    case.write(case.project / ".claude/settings.json", case.hooks("exec", "project"))
    case.write(case.cfg / "settings.json", case.hooks("exec", "user"))
    cwd = case.project / "nested/dir"
    cwd.mkdir(parents=True)
    out[case.base.name] = result(case, exit_code=case.run_print(cwd), cwd=case.clean(str(cwd)))
    return out


def plugin_flow(root, stub, claude):
    case = Case(root, "plugin-local-marketplace", stub, claude)
    case.seed()
    market = case.base / "market"
    plugin = market / "plugins/style-runtime"
    (plugin / "scripts").mkdir(parents=True)
    shutil.copy2(case.mark, plugin / "scripts/mark.sh")
    case.write(market / ".claude-plugin/marketplace.json", {"name": "style-m3d3", "owner": {"name": "STYLE M3-D3 probe"},
               "plugins": [{"name": "style-runtime", "source": "./plugins/style-runtime", "description": "probe"}]})
    case.write(plugin / ".claude-plugin/plugin.json", {"name": "style-runtime", "version": "0.1.0", "description": "STYLE runtime hooks"})

    def plugin_hooks(label):
        document = case.hooks("exec", label, script="${CLAUDE_PLUGIN_ROOT}/scripts/mark.sh")
        # Second UserPromptSubmit handler identical to one placed in user settings.
        document["hooks"]["UserPromptSubmit"].append({"hooks": [case.handler("exec", "dupcheck")]})
        case.write(plugin / "hooks/hooks.json", document)

    plugin_hooks("plugin")
    steps, report = [], {}
    baseline = case.snapshot()

    def cli(label, *args, cwd=None):
        case.runs += 1
        before = case.snapshot()
        project_before = case.snapshot(case.project / ".claude")
        completed = subprocess.run([claude, "plugin", *args], cwd=cwd or case.base, env=case.env, stdin=subprocess.DEVNULL,
                                   capture_output=True, text=True, timeout=120)
        (case.base / f"run{case.runs}.cli").write_text(completed.stdout + "\n--stderr--\n" + completed.stderr)
        steps.append({"step": label, "args": [case.clean(a) for a in args], "exit_code": completed.returncode,
                      "config_diff": diff(before, case.snapshot()),
                      "project_claude_diff": diff(project_before, case.snapshot(case.project / ".claude"))})
        return completed

    def settings_keys():
        path = case.cfg / "settings.json"
        return sorted(json.loads(path.read_text()).keys()) if path.exists() else None

    validate = cli("validate", "validate", str(plugin))
    report["validate_passed"] = validate.returncode == 0
    cli("marketplace-add", "marketplace", "add", str(market))
    report["user_settings_keys_after_marketplace_add"] = settings_keys()
    cli("install-user", "install", "style-runtime@style-m3d3")
    report["user_settings_keys_after_install"] = settings_keys()
    installed = case.cfg / "plugins/installed_plugins.json"
    if installed.exists():
        report["installed_plugins"] = json.loads(case.clean(installed.read_text()).replace(str(case.cfg), "<cfg>"))
    known = case.cfg / "plugins/known_marketplaces.json"
    if known.exists():
        report["known_marketplaces"] = json.loads(case.clean(known.read_text()))

    report["run_after_install"] = result(case, exit_code=case.run_print())
    case.reset_marks()

    plugin_hooks("plugin-v2")  # edit the source in place without bumping version
    report["run_after_source_edit"] = result(case, exit_code=case.run_print())
    case.reset_marks()

    user_settings = case.cfg / "settings.json"
    document = json.loads(user_settings.read_text()) if user_settings.exists() else {}
    document["hooks"] = {"UserPromptSubmit": [{"hooks": [case.handler("exec", "dupcheck")]}]}
    case.write(user_settings, document)
    report["run_with_identical_user_handler"] = result(case, exit_code=case.run_print())
    case.reset_marks()
    document.pop("hooks")
    case.write(user_settings, document)

    cli("uninstall-user", "uninstall", "style-runtime@style-m3d3")
    report["run_after_uninstall"] = result(case, exit_code=case.run_print())
    case.reset_marks()

    cli("install-project-scope", "install", "style-runtime@style-m3d3", "--scope", "project", cwd=case.project)
    project_settings = case.project / ".claude/settings.json"
    report["project_settings_after_project_install"] = json.loads(project_settings.read_text()) if project_settings.exists() else None
    report["run_after_project_install"] = result(case, exit_code=case.run_print())
    case.reset_marks()
    cli("uninstall-project-scope", "uninstall", "style-runtime@style-m3d3", "--scope", "project", cwd=case.project)
    report["project_settings_after_project_uninstall"] = project_settings.read_text() if project_settings.exists() else None

    cli("marketplace-remove", "marketplace", "remove", "style-m3d3")
    final = case.snapshot()
    report["residue_vs_baseline"] = diff(baseline, final)
    report["user_settings_after_all"] = user_settings.read_text() if user_settings.exists() else None
    report["steps"] = steps
    return {case.base.name: report}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scratch", required=True, help="empty private directory for raw logs")
    parser.add_argument("--output", required=True, help="whitelisted JSON result")
    parser.add_argument("--claude", default="claude")
    parser.add_argument("--part", action="append", choices=("project", "plugin"))
    args = parser.parse_args()
    root = Path(args.scratch).resolve()
    if root.exists() and any(root.iterdir()):
        raise SystemExit("scratch directory must be empty")
    root.mkdir(parents=True, exist_ok=True, mode=0o700)
    version = subprocess.run([args.claude, "--version"], capture_output=True, text=True, check=True).stdout.strip()
    stub = Stub()
    results = {}
    try:
        parts = args.part or ("project", "plugin")
        if "project" in parts:
            results.update(project_cases(root, stub, args.claude))
        if "plugin" in parts:
            results.update(plugin_flow(root, stub, args.claude))
    finally:
        stub.close()
    report = {"probe": "M3-D3 claude project hooks and local plugin marketplace", "claude_version": version,
              "model_service": "loopback Anthropic Messages stub, placeholder key, no inference",
              "stub_paths": sorted(set(stub.paths)), "results": results}
    text = json.dumps(report, indent=2, ensure_ascii=False).replace(str(root), "<scratch>")
    Path(args.output).write_text(text + "\n")
    for name, row in results.items():
        print(name, json.dumps({k: row[k] for k in ("fired", "exit_code", "screen") if k in row}, ensure_ascii=False))


if __name__ == "__main__":
    main()
