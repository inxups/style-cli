import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
from types import SimpleNamespace
import unittest

from codex_probe_support import marker_locations
from probe_codex_m2 import BODIES, counts, native_fixture, require_delivery, verify_stats


class M2NativeEvidenceTests(unittest.TestCase):
    def call(self, body, role="developer", exit_code=0):
        return {"exit": exit_code, "requests": [{"marker_locations": marker_locations({"input": [
            {"role": role, "content": [{"text": body}]}]}, (BODIES["shared"],))}],
            "hook_context": [{"payload": {"content": [{"text": body}]}}]}

    def test_delivery_requires_complete_body_in_both_request_and_rollout(self):
        body = BODIES["shared"]
        self.assertEqual(require_delivery(self.call(body), body, 1)["rollout_occurrences"], 1)
        for call in (self.call(body.split('.')[0]), self.call(body, role="user"), self.call(body, exit_code=1), self.call(body * 2)):
            with self.subTest(call=call), self.assertRaises(AssertionError):
                require_delivery(call, body, 1)
        missing_rollout = self.call(body)
        missing_rollout["hook_context"] = []
        with self.assertRaises(AssertionError):
            require_delivery(missing_rollout, body, 1)

    def test_stats_agree_per_rule_and_total_and_reject_alias_rows(self):
        rows = [{"entry": "first/one@session.start", "result": "output", "est_tokens": 5, "body_sha256": "sha256:fixture"},
                {"entry": "first/two@tool.pre", "result": "output"},
                {"entry": "first/one@tool.pre", "result": "dedup"}]
        report = {"format": 1, "by": "rule", "totals": counts(rows), "quality": {"partial": False},
                  "rows": [{"rule": name, **counts([row for row in rows if row['entry'].split('@')[0] == name])}
                           for name in ("first/one", "first/two")]}
        verify_stats(report, rows, "rule")
        for mutate in (lambda r: r["totals"].update(outputs=8),
                       lambda r: r["rows"][0].update(estimated_tokens=99),
                       lambda r: r["rows"].append({"rule": "folded/alias", **counts([])}),
                       lambda r: r["quality"].update(partial=True)):
            changed = copy.deepcopy(report)
            mutate(changed)
            with self.assertRaises(AssertionError):
                verify_stats(changed, rows, "rule")

    def test_native_observer_records_only_whitelisted_metadata(self):
        with tempfile.TemporaryDirectory(prefix="m2 'native ") as directory:
            root = Path(directory)
            package = native_fixture(SimpleNamespace(root=root))
            definition = json.loads((package / "STYLE.yaml").read_text())
            self.assertEqual([rule['codex'].get('matcher') for rule in definition['rules'][:3]], [None, '^Bash$', '^Read$'])
            log = root / "observations.jsonl"
            result = subprocess.run([sys.executable, "-I", str(root / "native-observer.py"), str(log), "native-start", BODIES["native-start"]],
                                    input=json.dumps({"hook_event_name": "SessionStart", "session_id": "session", "prompt": "DO_NOT_RECORD", "api_key": "DO_NOT_RECORD"}),
                                    text=True, capture_output=True, check=True)
            observed = json.loads(log.read_text())
            self.assertEqual(observed, {"fixture": "native-start", "event": "SessionStart", "session_id": "session"})
            self.assertEqual(log.stat().st_mode & 0o777, 0o600)
            self.assertNotIn("DO_NOT_RECORD", result.stdout + log.read_text())
            self.assertEqual(json.loads(result.stdout)["hookSpecificOutput"]["additionalContext"], BODIES["native-start"])


if __name__ == "__main__":
    unittest.main()
