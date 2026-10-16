import tempfile
import unittest
from pathlib import Path

from probe_codex_overflow import native_context_observation


class NativeOverflowEvidenceTests(unittest.TestCase):
    def test_head_and_tail_markers_do_not_imply_full_delivery(self):
        body = "BEGIN" + " omitted-middle " * 100 + "END"
        with tempfile.TemporaryDirectory() as root:
            saved = Path(root) / "hook.txt"
            saved.write_text(body)
            call = {"hook_context": [{"payload": {"content": [{"text":
                "BEGIN ... END\nFull hook output saved to: " + str(saved)}]}}]}
            observation = native_context_observation(call, body)
            self.assertEqual(observation["outcome"], "file-preview")
            self.assertFalse(observation["full_body_in_hook_context"])
            self.assertTrue(observation["saved_files"][0]["equals_raw_body"])

    def test_full_delivery_requires_exact_body(self):
        call = {"hook_context": [{"payload": {"content": [{"text": "complete body"}]}}]}
        self.assertEqual(native_context_observation(call, "complete body")["outcome"], "accepted-full")
        self.assertEqual(native_context_observation(call, "missing body")["outcome"], "not-found-or-other")


if __name__ == "__main__":
    unittest.main()
