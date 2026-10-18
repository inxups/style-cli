"""Verify Claude calibration arithmetic, production estimates and current documentation.

Usage: python3 docs/review/M3-CARRY-01-VERIFY-2026-10-18.py /tmp/m3-calibration-check.json
No API requests, native harness launches or Go builds are performed.
"""
import ast
from collections import Counter
import math
import statistics
import tempfile
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import unicodedata

sys.dont_write_bytecode = True
root = Path(__file__).resolve().parents[2]
baseline = "d42fdb8f4f4e07affa23c78a324e089c584dd6a9"


def git(*args):
    return subprocess.check_output(["git", *args], cwd=root)


def declared(path):
    rows = re.findall(r"^- \[([x ~!?-])\] \*\*(M3-[A-Z0-9-]+)", path.read_text(), re.M)
    assert len(rows) == len({name for _, name in rows}), path
    return {name: state for state, name in rows}


def anchors(path):
    text = path.read_text()
    found = set(re.findall(r'<a\s+(?:name|id)=[\'"]([^\'"]+)', text))
    used = {}
    for title in re.findall(r"^#{1,6}\s+(.+?)\s*#*\s*$", text, re.M):
        title = re.sub(r"\[([^]]+)\]\([^)]*\)", r"\1", title)
        title = re.sub(r"<[^>]+>", "", title).strip().lower()
        slug = "".join(c for c in title if c.isalnum() or c in "_- " or unicodedata.category(c).startswith("M")).replace(" ", "-")
        count = used.get(slug, 0)
        found.add(slug + ("-" + str(count) if count else ""))
        used[slug] = count + 1
    return found


tasks = declared(root / "docs/STYLE-M3-TASKS.md")
tests = declared(root / "docs/STYLE-M3-TESTS.md")
assert len(tasks) == 45 and len(tests) == 40
assert {k: v for k, v in tasks.items() if v != "x"} == {
    "M3-T31": "-", "M3-CARRY-01": "~", "M3-REAL-04": "?"}
assert {k: v for k, v in tests.items() if v != "x"} == {
    "M3-A7": "-", "M3-C3": "-", "M3-CARRY-01": "~", "M3-W1": "?"}
covered = set()
for row in (root / "docs/STYLE-M3-TASKS.md").read_text().splitlines():
    if not re.match(r"^- \[[x ~!?-]\] \*\*M3-", row):
        continue
    reference = re.search(r"测试：([^。]+)", row)
    assert reference, row
    expanded = re.sub(r"M3-([A-Z]+)(\d+)–([A-Z]+)(\d+)", lambda m: "、".join("M3-" + m[1] + str(i) for i in range(int(m[2]), int(m[4]) + 1)) if m[1] == m[3] else m[0], reference[1])
    ids = set(re.findall(r"M3-[A-Z0-9-]+", expanded))
    assert ids and ids <= tests.keys(), ids - tests.keys()
    covered.update(ids)
assert covered == tests.keys(), tests.keys() - covered

originals = git("ls-tree", "-r", "--name-only", baseline).decode().splitlines()
archives = [p for p in originals if re.match(r"docs/(?:STYLE-M[12]B?(?:-|\.)|review/M[12]B?[-.])", p)]
reports = [p for p in originals if p.startswith("docs/review/")]
goldens = [p for p in originals if p.startswith(("testdata/", "agenthooks/")) and ("golden" in p or p.endswith(".golden"))]
for path in set(archives + reports + goldens):
    assert (root / path).read_bytes() == git("show", baseline + ":" + path), path
progress = git("show", baseline + ":docs/PROGRESS.md")
assert (root / "docs/PROGRESS.md").read_bytes().startswith(progress)
for path in ("internal/app/cloud.go", "internal/app/cloud_command.go", "internal/app/cloud_test.go", "scripts/check_m3_cloud.py", "internal/app/legacy_cloud.go"):
    assert not (root / path).exists(), path
for path in (root / "internal/app").glob("*.go"):
    if path.name.endswith("_test.go"):
        continue
    text = path.read_text()
    assert not re.search(r"runCloudHook|ExportRequest|ExportResult|CloudManifest|cloud_files|cloud-state|cloud-export|removeCloudExport|checkCloudExport", text), path

changed = set(git("diff", "--name-only", baseline).decode().splitlines())
changed.update(git("ls-files", "--others", "--exclude-standard").decode().splitlines())
documents = [root / p for p in sorted(changed) if p.endswith(".md") and (root / p).exists()]
links = fences = 0
for path in documents:
    text = path.read_text()
    opened = None
    for row in text.splitlines():
        match = re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", row)
        if not match:
            continue
        marker, rest = match.groups()
        if opened:
            if marker[0] == opened[0] and len(marker) >= len(opened) and not rest.strip():
                opened = None
        else:
            opened = marker
            fences += 1
    assert opened is None, path
    for target in re.findall(r"\]\(([^)]+)\)", text):
        if "://" in target:
            continue
        name, _, anchor = target.partition("#")
        destination = path.parent / name if name else path
        assert destination.exists(), (path, target)
        if anchor and destination.suffix == ".md":
            assert anchor in anchors(destination), (path, target)
        links += 1
for path in (root / "scripts/check_m3_cloud_removed.py", Path(__file__), root / "docs/review/M3-CARRY-01-CALIBRATE-2026-10-18.py"):
    ast.parse(path.read_text(), filename=str(path))

acceptance_path = root / "docs/review/M3-CARRY-01-ANTHROPIC-2026-10-18.json"
acceptance = json.loads(acceptance_path.read_text())
corpus_path = root / "testdata/manual/token-calibration-40.json"
corpus = json.loads(corpus_path.read_text())
digest = lambda b: hashlib.sha256(b).hexdigest()
assert acceptance["corpus_sha256"] == digest(corpus_path.read_bytes())
assert corpus_path.read_bytes() == git("show", baseline + ":testdata/manual/token-calibration-40.json")
assert acceptance["source_commit"] == baseline
assert acceptance["item"] == "M3-CARRY-01"
assert acceptance["requested_scope_completed"] and acceptance["available_provider_checks_passed"]
assert not acceptance["passed"] and not acceptance["all_providers_completed"]
assert acceptance["collector_process_exit_code"] == 2
assert acceptance["measurement_completed_for"] == ["anthropic"]
assert acceptance["accuracy_threshold"] is None and not acceptance["production_coefficients_changed"]
assert acceptance["official_settings_unchanged"] and acceptance["fixture_inputs_unchanged"]
assert acceptance["providers"]["openai"]["status"] == "deferred-by-user"
assert acceptance["providers"]["openai"]["completed"] == 0
assert acceptance["providers"]["openai"]["official_requests"] == 0
assert acceptance["collector_sha256"] == digest((root / "docs/review/M3-CARRY-01-CALIBRATE-2026-10-18.py").read_bytes())
assert acceptance["original_collector_sha256"] == digest((root / "testdata/manual/calibrate_tokens.py").read_bytes())
assert acceptance["model_selection_evidence_sha256"] == digest((root / acceptance["anthropic_model_evidence"]).read_bytes())
provider = acceptance["providers"]["anthropic"]
assert provider["model"] == "claude-haiku-5-5"
assert provider["endpoint"] == "https://api.anthropic.com/v1/messages/count_tokens"
assert acceptance["api_version"] == "2023-06-01"
assert provider["status"] == "counted" and provider["completed"] == 40
assert len(provider["rules"]) == len(corpus["rules"]) == 40
assert provider["baseline_count"]["input_tokens"] == 24

def request(text):
    return {"model": provider["model"], "system": corpus["baseline_prefix"] + text,
            "messages": [{"role": "user", "content": corpus["user_message"]}]}

def request_hash(text):
    return digest(json.dumps(request(text), ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode())

assert provider["baseline"] == request("")
assert provider["baseline_request_sha256"] == request_hash("")
rows = provider["rules"]
assert Counter(row["language"] for row in rows) == {"en": 20, "zh": 20}
assert len({row["id"] for row in rows}) == 40
for source, row in zip(corpus["rules"], rows):
    assert row["id"] == source["id"] and row["language"] == source["language"]
    assert row["sha256"] == source["sha256"] == digest(source["text"].encode())
    assert row["style_estimate"]["chars"] == len(source["text"])
    assert row["request_sha256"] == request_hash(source["text"])
    assert row["official_count"]["http_status"] == 200
    delta = row["official_count"]["input_tokens"] - 24
    assert type(delta) is int and delta > 0 and row["incremental_input_tokens"] == delta
    assert math.isclose(row["relative_error"], (row["style_estimate"]["tokens"] - delta) / delta, abs_tol=1e-12)

for language in ("all", "en", "zh"):
    selected = [row for row in rows if language == "all" or row["language"] == language]
    estimates = sum(row["style_estimate"]["tokens"] for row in selected)
    official = sum(row["incremental_input_tokens"] for row in selected)
    errors = [(row["style_estimate"]["tokens"] - row["incremental_input_tokens"]) / row["incremental_input_tokens"] for row in selected]
    absolute = sorted(abs(error) for error in errors)
    expected = {"rules": len(selected), "estimated_tokens": estimates, "official_incremental_tokens": official,
                "aggregate_relative_error": (estimates - official) / official,
                "mean_signed_relative_error": statistics.mean(errors),
                "mean_absolute_relative_error": statistics.mean(absolute),
                "p95_absolute_relative_error": absolute[math.ceil(0.95 * len(selected)) - 1],
                "max_absolute_relative_error": absolute[-1],
                "underestimated_rules": len([e for e in errors if e < 0]),
                "overestimated_rules": len([e for e in errors if e > 0]),
                "exact_rules": len([e for e in errors if e == 0])}
    actual = provider["summary"][language]
    assert expected.keys() == actual.keys()
    for key, value in expected.items():
        assert math.isclose(value, actual[key], abs_tol=1e-12), (language, key)

receipts = acceptance["official_requests"]
assert acceptance["official_request_count"] == len(receipts) == 46
assert all(row["provider"] == "anthropic" and row["http_status"] == 200 and "error_type" not in row for row in receipts)
for row in receipts:
    assert all(re.fullmatch("[0-9a-f]{64}", row[k]) for k in ("request_sha256", "response_sha256", "request_id_sha256"))
    assert type(row["duration_ms"]) is int and row["duration_ms"] >= 0
repeated = ["", *[corpus["rules"][i]["text"] for i in (0, 19, 20, 39)]]
assert Counter(row["request_sha256"] for row in receipts) == Counter(map(request_hash, ["", *[row["text"] for row in corpus["rules"]], *repeated]))
for rule in rows:
    matching = [row for row in receipts if row["request_sha256"] == rule["request_sha256"]]
    assert all(row["input_tokens"] == rule["official_count"]["input_tokens"] and row["response_sha256"] == rule["official_count"]["response_sha256"] for row in matching)
checks = acceptance["repeat_checks"]
assert len(checks) == 5 and all(row["provider"] == "anthropic" and row["passed"] and row["expected"] == row["observed"] for row in checks)
assert [row["specimen"] for row in checks] == ["baseline", *[rows[i]["id"] for i in (0, 19, 20, 39)]]

built = json.loads((root / "docs/review/M3-REMOVE-01-2026-10-18.json").read_text())["binary"]
binary = Path(built["build"]["command"][4])
assert acceptance["style_binary_sha256"] == built["sha256"] == digest(binary.read_bytes())
for path, expected in built["source_inputs"].items():
    assert "sha256:" + digest((root / path).read_bytes()) == expected, path
sys.path.insert(0, str(root / "scripts"))
from check_m3_update import fixture, snapshot
with tempfile.TemporaryDirectory(prefix="style-carry-offline-") as temporary:
    env, _, _, _, _ = fixture(Path(temporary))
    source = Path(temporary) / "package"
    source.mkdir()
    package = {"spec": "style/v0.3", "name": "token-calibration", "version": "0.1.0", "description": "Fixed corpus",
               "rules": [{"id": row["id"], "target": "event", "event": "session.start", "text": row["text"]} for row in corpus["rules"]]}
    (source / "STYLE.yaml").write_text(json.dumps(package, ensure_ascii=False))
    before = snapshot(Path(temporary))
    planned = subprocess.run([str(binary), "plan", str(source), "--harness=codex,claude-code", "--json"], env=env, check=True, capture_output=True, text=True, timeout=30)
    assert not planned.stderr and snapshot(Path(temporary)) == before
    plan_reports = json.loads(planned.stdout)["reports"]
    assert len(plan_reports) == 2
    expected = {row["id"]: row["style_estimate"] for row in rows}
    for planned_report in plan_reports:
        assert {row["rule"]: row["estimate"] for row in planned_report["tokens"]["rules"]} == expected

private_settings = json.loads(Path("/Users/inxups/.claude-style-official/settings.json").read_text())
key = private_settings["env"]["ANTHROPIC_API_KEY"]
assert key
for path in changed:
    candidate = root / path
    if candidate.is_file():
        assert key.encode() not in candidate.read_bytes(), path
subprocess.run(["git", "diff", "--check"], cwd=root, check=True)
report = {"format": 1, "passed": True, "baseline_commit": baseline,
          "task_ids": len(tasks), "test_ids": len(tests), "all_tests_mapped": True,
          "archives_unchanged": len(archives), "historical_review_files_unchanged": len([p for p in originals if p.startswith("docs/review/")]),
          "goldens_unchanged": len(goldens), "progress_append_only": True,
          "progress_prefix_sha256": digest(progress), "removed_code_and_cleanup_absent": True,
          "documents": len(documents), "local_links_and_anchors": links, "fence_blocks": fences,
          "python_syntax": True, "git_diff_check": True, "credentials_absent_from_changes": True,
          "build_source_inputs_verified": len(built["source_inputs"]),
          "corpus_unchanged": True, "anthropic_measurements_verified": 40, "stable_repeats_verified": 5,
          "official_receipts_verified": 46, "summary_groups_independently_recomputed": 3,
          "fresh_production_plans_verified": 2, "fresh_plan_inputs_unchanged": True,
          "network_requests_by_this_verifier": 0, "go_rebuilt": False,
          "partial_tasks": ["M3-CARRY-01"], "remaining_pending_tasks": ["M3-REAL-04"],
          "openai_calibration": "0/40 deferred by user", "anthropic_calibration": "40/40 completed",
          "acceptance_sha256": digest(acceptance_path.read_bytes()),
          "verification_source_sha256": digest(Path(__file__).read_bytes())}
Path(sys.argv[1]).write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
print(json.dumps(report, ensure_ascii=False))
