import json
from pathlib import Path
import tempfile
import unittest

from calibrate_tokens import sha
from summarize_claude_acceptance_usage import summarize


class PinnedAcceptanceUsageTests(unittest.TestCase):
    def fixture(self, root):
        transcript = root / "12345678-1234-1234-1234-123456789abc.jsonl"
        rows = []
        for output in (1, 9):
            rows.append({"type": "assistant", "requestId": "request-one", "message": {
                "model": "fixture-model", "content": "PRIVATE_TEXT_MUST_NOT_BE_EXPORTED",
                "usage": {"input_tokens": 2, "cache_creation_input_tokens": 3,
                          "cache_read_input_tokens": 5, "output_tokens": output,
                          "cache_creation": {"ephemeral_5m_input_tokens": 3}}}})
        transcript.write_text("\n".join(json.dumps(row) for row in rows) + "\n")
        report = root / "acceptance.json"
        report.write_text(json.dumps({"passed": True, "evidence": [{"path": str(transcript), "sha256": sha(transcript.read_bytes())}]}))
        return transcript, report

    def test_final_snapshot_cache_categories_and_whitelist(self):
        with tempfile.TemporaryDirectory() as directory:
            _, source = self.fixture(Path(directory))
            result = summarize([source, source])
        self.assertEqual(result["session_count"], 1)
        self.assertEqual(result["usage_responses"], 1)
        session = result["sessions"][0]
        self.assertEqual(session["duplicate_snapshot_rows_removed"], 1)
        self.assertEqual(session["reported_usage"]["output_tokens"], 9)
        self.assertEqual(session["reported_usage"]["total_input_including_cache"], 10)
        self.assertNotIn("PRIVATE_TEXT", json.dumps(result))

    def test_drifted_private_transcript_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            transcript, source = self.fixture(Path(directory))
            transcript.write_text(transcript.read_text() + "\n")
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                summarize([source])


if __name__ == "__main__":
    unittest.main()
