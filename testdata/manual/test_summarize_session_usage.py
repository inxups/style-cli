import json
from pathlib import Path
import tempfile
import unittest

from summarize_session_usage import claude_report, codex_report, codex_usage


def stat(result="output"):
    return {"entry": "package/rule@session.start", "action": "inject", "result": result,
            "est_tokens": 5, "body_sha256": "fixture-body-hash"}


class SessionUsageTests(unittest.TestCase):
    def test_resume_uses_final_cumulative_usage_and_stats_only_once(self):
        calls = []
        for number, usage, stats in ((1, 100, [stat()]), (2, 250, [stat(), stat("dedup")])):
            calls.append({"session_id": "same-session", "name": str(number), "exit": 0,
                          "events": [{"type": "turn.completed", "usage": {"input_tokens": usage, "output_tokens": number}}],
                          "requests": [{"model": "fixture-model"}], "stats": stats})
        with tempfile.TemporaryDirectory() as scratch:
            path = Path(scratch) / "report.json"
            path.write_text(json.dumps({"route": "codex-user", "calls": calls}))
            report = codex_report(path)
            self.assertEqual(report["session_count"], 1)
            self.assertEqual(report["observed_http_requests"], 2)
            self.assertEqual(report["reported_usage"], {"input_tokens": 250, "output_tokens": 2})
            self.assertEqual(report["style"]["rows"], 2)
            self.assertEqual(report["style"]["inject_outputs"], 1)
            self.assertEqual(report["style"]["dedup_fraction_of_output_or_dedup"], 0.5)
            calls[1]["stats"] = [stat("dedup")]
            path.write_text(json.dumps({"route": "codex-user", "calls": calls}))
            with self.assertRaisesRegex(AssertionError, "append-only"):
                codex_report(path)

    def test_app_server_cumulative_snapshots_are_not_summed(self):
        events = [{"method": "thread/tokenUsage/updated", "params": {"tokenUsage": {"total": {"inputTokens": value}}}} for value in (100, 250)]
        self.assertEqual(codex_usage({"events": events})["usage"], {"input_tokens": 250})

    def test_partial_native_total_field_is_not_presented_as_aggregate(self):
        calls = [{"session_id": str(n), "name": str(n), "exit": 0, "requests": [], "stats": [],
                  "events": [{"type": "turn.completed", "usage": usage}]} for n, usage in
                 enumerate(({"input_tokens": 100}, {"input_tokens": 200, "total_tokens": 220}))]
        with tempfile.TemporaryDirectory() as scratch:
            path = Path(scratch) / "report.json"
            path.write_text(json.dumps({"route": "codex-user", "calls": calls}))
            report = codex_report(path)
        self.assertEqual(report["reported_usage"], {"input_tokens": 300})
        self.assertEqual(report["usage_field_session_counts"]["total_tokens"], 1)

    def test_claude_duplicate_blocks_and_cache_ttl_subsets_are_not_added_twice(self):
        def assistant(output):
            return {"type": "assistant", "requestId": "request-1", "timestamp": "fixture-time", "message": {
                "id": "message-1", "model": "fixture-model", "content": "PRIVATE-PROMPT",
                "usage": {"input_tokens": 2, "cache_creation_input_tokens": 10, "cache_read_input_tokens": 30,
                          "output_tokens": output, "cache_creation": {"ephemeral_1h_input_tokens": 10}}}}
        with tempfile.TemporaryDirectory() as scratch:
            transcript, stats = Path(scratch) / "transcript.jsonl", Path(scratch) / "stats.jsonl"
            transcript.write_text("\n".join(json.dumps(row) for row in [assistant(3), assistant(5), {"type": "user", "message": "PRIVATE-PROMPT"}]))
            stats.write_text(json.dumps(stat()) + "\n")
            report = claude_report(transcript, stats)
        self.assertEqual(report["observed_usage_response_requests"], 1)
        self.assertEqual(report["duplicate_snapshot_rows_removed"], 1)
        self.assertEqual(report["reported_usage"]["output_tokens"], 5)
        self.assertEqual(report["reported_usage"]["total_input_including_cache"], 42)
        self.assertEqual(report["reported_usage"]["ephemeral_1h_input_tokens"], 10)
        self.assertNotIn("PRIVATE-PROMPT", json.dumps(report))


if __name__ == "__main__":
    unittest.main()
