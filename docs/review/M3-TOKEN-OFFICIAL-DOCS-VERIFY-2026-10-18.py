"""Verify official-count closure, current links and protected historical evidence.

Usage: python3 docs/review/M3-TOKEN-OFFICIAL-DOCS-VERIFY-2026-10-18.py /tmp/m3-official-docs.json
No API requests, harness launches or Go builds are performed.
"""
import ast
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import unicodedata

root = Path(__file__).resolve().parents[2]
baseline = "730fe4ca6115cfe4c6c223c4b664da1b49364c4f"


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
    "M3-T31": "-", "M3-CARRY-01": "?", "M3-REAL-04": "?"}
assert {k: v for k, v in tests.items() if v != "x"} == {
    "M3-A7": "-", "M3-C3": "-", "M3-CARRY-01": "?", "M3-W1": "?"}
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
for path in (root / "scripts/check_m3_cloud_removed.py", Path(__file__), root / "docs/review/M3-TOKEN-OFFICIAL-VERIFY-2026-10-18.py"):
    ast.parse(path.read_text(), filename=str(path))
acceptance = json.loads((root / "docs/review/M3-TOKEN-OFFICIAL-2026-10-18.json").read_text())
assert acceptance["passed"] and acceptance["official_count_acceptance_verified"]
assert acceptance["closes"] == ["M3-REAL-03", "M3-TOKEN-01"]
assert acceptance["source_commit"] == baseline
assert acceptance["native_cli"]["model"] == "claude-haiku-5-5"
assert acceptance["native_cli"]["effort"] == "low"
assert acceptance["native_cli"]["exact_marker_reply"]
assert acceptance["official_count_requests"] == 7 and len(acceptance["cases"]) == 7
assert all(row["passed"] for row in acceptance["cases"])
assert acceptance["calibration_results"] == {"anthropic": "0/40", "openai": "0/40"}
assert acceptance["verification_source_sha256"] == hashlib.sha256((root / "docs/review/M3-TOKEN-OFFICIAL-VERIFY-2026-10-18.py").read_bytes()).hexdigest()
built = json.loads((root / "docs/review/M3-REMOVE-01-2026-10-18.json").read_text())["binary"]
assert acceptance["binary_sha256"] == built["sha256"]
for path, digest in built["source_inputs"].items():
    assert "sha256:" + hashlib.sha256((root / path).read_bytes()).hexdigest() == digest, path

subprocess.run(["git", "diff", "--check"], cwd=root, check=True)
report = {"format": 1, "passed": True, "baseline_commit": baseline,
          "task_ids": len(tasks), "test_ids": len(tests), "all_tests_mapped": True,
          "cancelled_tasks": ["M3-T31"], "cancelled_tests": ["M3-A7", "M3-C3"],
          "archives_unchanged": len(archives), "historical_review_files_unchanged": len(reports),
          "goldens_unchanged": len(goldens), "progress_append_only": True,
          "progress_prefix_sha256": hashlib.sha256(progress).hexdigest(),
          "removed_code_and_cleanup_absent": True, "documents": len(documents),
          "local_links_and_anchors": links, "fence_blocks": fences, "python_syntax": True,
          "git_diff_check": True, "official_count_closures": ["M3-REAL-03", "M3-TOKEN-01"], "native_cli_model_effort_verified": True, "build_source_inputs_verified": len(built["source_inputs"]), "remaining_pending_tasks": ["M3-CARRY-01", "M3-REAL-04"], "remaining_pending_tests": ["M3-CARRY-01", "M3-W1"]}
Path(sys.argv[1]).write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
print(json.dumps(report, ensure_ascii=False))
