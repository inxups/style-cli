#!/usr/bin/env python3
"""Run B1-B9 independently on one isolated native Codex route, with real inference.

Requires the user's existing custom Responses provider. Logs and copied config
remain in --scratch (private, outside Git); --output contains safe evidence.
No current installation is changed. Force pushes target only a local bare repo.
"""

import argparse
import json
from pathlib import Path
import shutil
import subprocess
import time

from codex_probe_support import AppServer, NativeProbe, digest


REPO = Path(__file__).resolve().parents[2]


def output_count(call, rule):
    return sum(row["entry"].split("@")[0] == rule and row["result"] == "output" for row in call["stats"])


def request_text(call):
    return json.dumps(call["requests"], ensure_ascii=False)


def verify_effort_delivery(call, tier):
    model = call["state"]["model"]
    bodies = {
        "terse-reviewer/session-info": f"Current session: model {model}, effort {tier or 'unknown'}.\n",
        "m1b-probes/session-info": f"M1B_SESSION model={model} effort={tier or 'unknown'}.",
    }
    assert call["hook_context"], "native rollout has no hook context"
    for rule, body in bodies.items():
        emitted = [row for row in call["stats"] if row["entry"].split("@")[0] == rule and row["result"] == "output"]
        assert len(emitted) == 1 and emitted[0]["body_sha256"] == "sha256:" + digest(body.encode()), rule
        assert body.rstrip() in request_text(call), rule


def register_plugin(probe):
    directory = probe.store / ".agents/plugins"
    directory.mkdir(parents=True, exist_ok=True)
    (directory / "marketplace.json").write_text(json.dumps({"name": "style", "interface": {"displayName": "STYLE M1 acceptance"},
        "plugins": [{"name": "style-runtime", "source": {"source": "local", "path": "./adapters/codex/plugin"},
                     "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"}, "category": "Productivity"}]}))
    return [probe.native_command("plugin", "marketplace", "add", str(probe.store), "--json"),
            probe.native_command("plugin", "add", "style-runtime@style", "--json")]


def unregister_plugin(probe):
    return [probe.native_command("plugin", "remove", "style-runtime@style", "--json"),
            probe.native_command("plugin", "marketplace", "remove", "style")]


def finish_configuration_roundtrip(config, baseline, work):
    current = config.read_bytes()
    native_trust = ('[projects.' + json.dumps(str(work.resolve())) + ']\ntrust_level = "trusted"\n\n').encode()
    native_write = current == baseline + native_trust
    if native_write:
        # app-server persists its own project trust while starting a thread.
        # Assert that this is the ONLY foreign change, then remove the probe's
        # native registration separately from STYLE's uninstall transaction.
        config.write_bytes(current[:-len(native_trust)])
    assert config.read_bytes() == baseline, "unexpected configuration residue after uninstall"
    return {"configuration_byte_roundtrip": True, "native_project_trust_added_and_cleaned_by_probe": native_write,
            "baseline_sha256": digest(baseline)}


def run(probe, route, report):
    example = str(REPO / "testdata/packages/terse-reviewer")
    probes = str(REPO / "testdata/manual/m1b-probes")
    config = probe.home / "config.toml"
    baseline = config.read_bytes()
    report["checks"] = checks = {}

    def passed(name, evidence):
        checks[name] = {"passed": True, **evidence}
        print(f"{route} {name}: passed", flush=True)

    def install(source=example, selected=route):
        return probe.style_command("install", source, "--harness", "codex", "--route", f"codex={selected}", "--yes")

    install()
    if route == "codex-plugin":
        report["registration"] = register_plugin(probe)
    initial_config = config.read_bytes()
    install(probes)
    probe.style_command("sync")
    probe.style_command("uninstall", "m1b-probes", "--yes")
    probe.style_command("sync")
    assert config.read_bytes() == initial_config
    install(probes)
    assert config.read_bytes() == initial_config
    passed("B1", {"second_package_configuration_unchanged": True,
                  "trust_scope": "native exec accepts installed hook trust; no TUI workspace prompt claimed"})

    effort_cases = [("low", "low", [], "low"), ("minimal", "minimal", ["-m", "gpt-6-sol"], "minimal"),
                    ("high", "high", [], "high"), ("unset", None, [], ""),
                    ("unknown", "future-tier", [], ""), ("profile", "low", ["-p", "probe"], "high"),
                    ("override", "low", ["-m", "gpt-6-sol", "-p", "probe", "-c", 'model_reasoning_effort="minimal"'], "minimal")]
    (probe.home / "probe.config.toml").write_text('model_reasoning_effort="high"\n')
    effort_results = []
    for name, raw, flags, tier in effort_cases:
        probe.set_effort(raw)
        call = probe.invoke(f"b2-{name}", "Reply only STYLE_EFFORT_CHECK. Do not use tools.", flags)
        assert call["state"] is not None and call["state"].get("effort", "") == tier, name
        assert call["requests"], name
        assert call["requests"][0]["static_rule_present"], name
        verify_effort_delivery(call, tier)
        native = call["requests"][0].get("reasoning", {}).get("effort")
        if name != "unset":
            expected_raw = "high" if name == "profile" else "minimal" if name == "override" else raw
            assert native == expected_raw, (name, native)
        assert f"effort {tier or 'unknown'}." in request_text(call), name
        guard = "Use a concise review format for low-effort runs." in request_text(call)
        assert guard == (tier in ("minimal", "low")), name
        if name != "unknown":
            assert call["exit"] == 0, name
        effort_results.append({"case": name, "native_effort": native, "style_effort": tier,
                               "session_id": call["session_id"], "model_turn_completed": call["exit"] == 0,
                               "guard_delivered": guard})
        print(f"{route} B2 {name}: measured", flush=True)
    probe.set_effort("low")
    for trust in ("trusted", "untrusted"):
        project = probe.work / ".codex"
        project.mkdir(exist_ok=True)
        (project / "config.toml").write_text('model_reasoning_effort="high"\n')
        saved = config.read_text()
        config.write_text(saved + f'\n[projects.{json.dumps(str(probe.work.resolve()))}]\ntrust_level={json.dumps(trust)}\n')
        call = probe.invoke(f"b2-{trust}", "Reply only STYLE_TRUST_CHECK. Do not use tools.")
        native = call["requests"][0]["reasoning"]["effort"]
        assert native == ("high" if trust == "trusted" else "low")
        assert call["exit"] == 0 and call["state"]["effort"] == "low"
        doctor = probe.style_command("doctor")
        assert "project layer skipped" in json.dumps(doctor)
        effort_results.append({"case": trust, "native_effort": native, "style_effort": "low",
                               "session_id": call["session_id"], "reason": "production trust unknown: project layer skipped and doctor explains"})
        config.write_text(saved)
        (project / "config.toml").unlink()
    passed("B2-U10", {"cases": effort_results, "unknown_never_reuses_known_tier": True})

    # Native skill exists independently of STYLE's rules.
    skill = probe.work / ".agents/skills/code-review"
    skill.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(REPO / "testdata/manual/claude-skills/code-review/SKILL.md", skill / "SKILL.md")
    explicit = probe.invoke("b4-explicit", "$code-review\nFollow this acceptance skill. Do not run a review.")
    assert explicit["exit"] == 0 and output_count(explicit, "terse-reviewer/review-skill") == 1
    implicit = probe.invoke("b4-cat", "Run exactly cat .agents/skills/code-review/SKILL.md once, then reply DONE.", resume=explicit["session_id"])
    assert implicit["exit"] == 0 and output_count(implicit, "terse-reviewer/review-skill") == 1
    reverse = probe.invoke("b4-sed", "Use the shell tool to run sed -n '1,20p' .agents/skills/code-review/SKILL.md, then head -n 5 .agents/skills/code-review/SKILL.md. Do not call skills.read. Then reply DONE.")
    assert reverse["exit"] == 0 and output_count(reverse, "terse-reviewer/review-skill") == 1
    reverse_explicit = probe.invoke("b4-explicit-second", "$code-review\nFollow this acceptance skill.", resume=reverse["session_id"])
    assert reverse_explicit["exit"] == 0 and output_count(reverse_explicit, "terse-reviewer/review-skill") == 1
    assert "When using the code-review skill, report at most 10 findings." in request_text(explicit) + request_text(reverse)
    passed("B4", {"sessions": [explicit["session_id"], reverse["session_id"]], "output_per_session": 1})

    def git(*args):
        return subprocess.check_output(["git", *args], cwd=probe.work, env=probe.env, text=True, stderr=subprocess.DEVNULL).strip()

    git("init", "-q")
    git("config", "user.name", "STYLE Probe")
    git("config", "user.email", "style-probe@example.invalid")
    (probe.work / "README.md").write_text("Local acceptance fixture.\n")
    git("add", "README.md")
    git("commit", "-qm", "Local acceptance baseline")
    remote = probe.root / "remote.git"
    subprocess.run(["git", "init", "--bare", "-q", str(remote)], check=True)
    git("remote", "add", "origin", str(remote))
    assert git("remote", "get-url", "origin") == str(remote)
    git("push", "-qu", "origin", "HEAD")
    head_before = git("rev-parse", "HEAD")
    block = probe.invoke("b5-force", "This fixture has ONLY a local bare remote. Call the shell tool to execute git push --force exactly once to test the installed hook. If denied, stop; do not bypass or retry. Then execute git push --force-with-lease once as the allowed control.")
    assert block["exit"] == 0 and any(row["result"] == "blocked" and "no-force-push@" in row["entry"] for row in block["stats"])
    assert "--force-with-lease" in json.dumps(block["events"]) and "Everything up-to-date" in json.dumps(block["events"])
    assert git("rev-parse", "HEAD") == head_before
    passed("B5", {"session_id": block["session_id"], "remote": "local bare only", "head_unchanged": True})

    edit = probe.invoke("b6-patch", "Use apply_patch to create acceptance-note.txt containing one line '// TODO acceptance probe'. After the tool result, use apply_patch again to replace it with '// acceptance complete'. This temporary fixture tests a warning; complete both operations and leave no TODO text. Then reply DONE.")
    assert edit["exit"] == 0 and output_count(edit, "terse-reviewer/no-todo-comments") == 1
    assert "TODO" not in (probe.work / "acceptance-note.txt").read_text()
    passed("B6", {"session_id": edit["session_id"], "warn_outputs": 1, "final_fixture_has_no_unfinished_comment": True})

    mcp = probe.invoke("b7-mcp", "Invoke github/create_pull_request exactly once with title STYLE acceptance. It is a local stub and cannot create a real PR. Then reply DONE.",
                       ["-c", 'mcp_servers.github.command="python3"', "-c", "mcp_servers.github.args=" + json.dumps(["-I", str(REPO / "testdata/manual/github_mcp_stub.py")])])
    assert mcp["exit"] == 0 and output_count(mcp, "terse-reviewer/before-create-pr") == 1
    assert "STYLE_M1B_MCP_STUB" in json.dumps(mcp["events"])
    assert "Before creating a pull request, confirm the branch and test status." in request_text(mcp)
    passed("B7", {"session_id": mcp["session_id"], "transport": "existing local stdio stub", "outputs": 1})

    manifest = probe.store / "packages/terse-reviewer/prompts/session-info.md"
    old_body = manifest.read_text()
    old_index = (probe.store / "build/index.json").read_bytes()
    manifest.write_text(old_body.replace("Current session:", "Current session: M1_FINAL_B9"))
    assert probe.style_command("sync", expected=3)["needs_trust"] == ["terse-reviewer"]
    preview = probe.style_command("trust", "terse-reviewer", expected=1)
    assert preview["awaiting_confirmation"] and (probe.store / "build/index.json").read_bytes() == old_index
    old = probe.invoke("b9-before-trust", "Reply only STYLE_BEFORE_TRUST. Do not use tools.")
    assert old["exit"] == 0 and "M1_FINAL_B9" not in request_text(old)
    probe.style_command("trust", "terse-reviewer", "--yes")
    new = probe.invoke("b9-after-trust", "Reply only STYLE_AFTER_TRUST. Do not use tools.")
    assert new["exit"] == 0 and "M1_FINAL_B9" in request_text(new)
    manifest.write_text(old_body)
    probe.style_command("trust", "terse-reviewer", "--yes")
    passed("B9", {"before_session": old["session_id"], "after_session": new["session_id"], "preview_read_only": True})

    server = AppServer(probe)
    try:
        start = len(probe.requests)
        thread = server.rpc("thread/start", {"cwd": str(probe.work), "approvalPolicy": "never", "sandbox": "danger-full-access"})["thread"]["id"]
        server.turn(thread, "Reply only STYLE_BEFORE_COMPACT. Do not use tools.")
        server.rpc("thread/compact/start", {"threadId": thread})
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            message = server.next()
            if message.get("method") == "turn/completed":
                break
        else:
            raise RuntimeError("native compact did not complete")
        server.turn(thread, "Reply only STYLE_AFTER_COMPACT_1. Do not use tools.")
        server.turn(thread, "Reply only STYLE_AFTER_COMPACT_2. Do not use tools.")
        compact = probe.capture("b3-compact", thread, server.events, start)
        assert output_count(compact, "terse-reviewer/session-info") == 2
        assert output_count(compact, "m1b-probes/session-info") == 2
        scope = json.loads((probe.store / "state" / thread / "main/scope.json").read_text())
        assert scope["epoch"] == 1
        passed("B3", {"session_id": thread, "epoch": 1, "outputs_each_dynamic_system_rule": 2,
                      "native_notifications": [event["method"] for event in server.events if "compact" in event["method"].lower()]})
    finally:
        report["compact_notifications"] = server.events
        server.close()

    # Registration is a separate Codex lifecycle; remove it before STYLE.
    if route == "codex-plugin":
        report["unregistration"] = unregister_plugin(probe)
    probe.style_command("uninstall", "--all", "--yes")
    roundtrip = finish_configuration_roundtrip(config, baseline, probe.work)
    assert not (probe.home / "hooks.json").exists()
    passed("B8", {**roundtrip, "no_user_hooks_file": True})
    report["passed"] = all(checks.get(key, {}).get("passed") for key in ("B1", "B2-U10", "B3", "B4", "B5", "B6", "B7", "B8", "B9"))


def run_switches(probe, report):
    example = str(REPO / "testdata/packages/terse-reviewer")
    config = probe.home / "config.toml"
    baseline = config.read_bytes()

    def install(route):
        probe.style_command("install", example, "--harness", "codex", "--route", f"codex={route}", "--yes")

    install("codex-user")
    user_config, user_hooks = config.read_bytes(), (probe.home / "hooks.json").read_bytes()
    report["switches"] = []
    for route in ("codex-plugin", "codex-user", "codex-plugin", "codex-user"):
        if route == "codex-plugin":
            install(route)
            register_plugin(probe)
            assert not (probe.home / "hooks.json").exists()
        else:
            unregister_plugin(probe)
            install(route)
            assert config.read_bytes() == user_config
            assert (probe.home / "hooks.json").read_bytes() == user_hooks
        call = probe.invoke("switch-" + str(len(report["switches"])), "Run printf STYLE_ROUTE_SWITCH exactly once using the shell tool, then reply DONE.")
        assert call["exit"] == 0
        spans = call["hook_spans"]
        source = "plugin" if route == "codex-plugin" else "user"
        assert spans and all(span["hook.source"] == source for span in spans)
        for event in ("SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse"):
            assert sum(span["hook.event_name"] == event for span in spans) == 1, (route, event, spans)
        assert output_count(call, "terse-reviewer/session-info") == 1
        report["switches"].append({"route": route, "session_id": call["session_id"],
                                   "native_hook_spans": spans, "no_duplicate_injection": True})
        print(f"B10 switch to {route}: passed", flush=True)
    probe.style_command("uninstall", "--all", "--yes")
    assert config.read_bytes() == baseline and not (probe.home / "hooks.json").exists()
    report.update(passed=True, configuration_byte_roundtrip=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--style", required=True)
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--route", required=True, choices=("codex-user", "codex-plugin"))
    parser.add_argument("--switches-only", action="store_true")
    parser.add_argument("--scratch", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    probe = NativeProbe(args.scratch, args.style, args.codex)
    report = {"date": "2026-10-16", "route": args.route,
              "harness_version": subprocess.check_output([args.codex, "--version"], text=True).strip(),
              "style_binary_sha256": digest(Path(args.style).read_bytes()),
              "configuration_baseline_sha256": probe.baseline_config_sha256, "passed": False}
    try:
        if args.switches_only:
            run_switches(probe, report)
        else:
            run(probe, args.route, report)
    except Exception as error:
        report["failure"] = str(error)
        raise
    finally:
        report["calls"] = probe.calls
        args.output.write_text(json.dumps(probe.sanitize(report), ensure_ascii=False, indent=2) + "\n")
        probe.close()


if __name__ == "__main__":
    main()
