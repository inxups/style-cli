import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from types import SimpleNamespace

from claude_probe_support import ClaudeProbe, body_observation


class NativeClaudeEvidenceTests(unittest.TestCase):
    def test_preview_with_markers_is_not_full_delivery(self):
        body = "BEGIN_" + "middle " * 100 + "_END"
        with tempfile.TemporaryDirectory() as root:
            saved = Path(root) / "body.txt"
            saved.write_text(body)
            text = "<persisted-output>\nFull output saved to: " + str(saved) + "\n" + body[:40] + " ... _END\n</persisted-output>"
            request = {"messages": [{"content": [{"text": text}]}]}
            observed = body_observation(request, body, root)
            self.assertEqual(observed["outcome"], "file-preview")
            self.assertFalse(observed["full_body_in_request"])
            self.assertTrue(observed["saved_files"][0]["equals_raw_body"])
            saved.write_text("different body")
            self.assertEqual(body_observation(request, body, root)["outcome"], "not-found-or-invalid-artifact")

    def test_full_delivery_requires_contiguous_body_in_model_input(self):
        body = "full body"
        request = {"messages": [{"content": [{"text": body}]}], "metadata": {"text": "elsewhere"}}
        self.assertTrue(body_observation(request, body)["full_body_in_request"])
        self.assertFalse(body_observation(request, "elsewhere")["full_body_in_request"])

    def test_stub_uses_isolated_environment_and_excludes_synthetic_usage(self):
        with tempfile.TemporaryDirectory() as root:
            with patch("claude_probe_support.shutil.which", return_value="/fixture/claude"), patch(
                    "claude_probe_support.subprocess.run", return_value=SimpleNamespace(stdout="fixture CLI")):
                probe = ClaudeProbe(root)
            try:
                self.assertEqual(probe.env["HOME"], str(Path(root).resolve() / "home"))
                self.assertNotIn("ANTHROPIC_AUTH_TOKEN", probe.env)
                self.assertNotIn("CODEX_HOME", probe.env)
                self.assertTrue(probe.env["ANTHROPIC_BASE_URL"].startswith("http://127.0.0.1:"))
                # Exercise the protocol without requiring an installed Claude
                # session or a model inference.
                import urllib.request
                url = probe.env["ANTHROPIC_BASE_URL"] + "/v1/messages"
                data = json.dumps({"model": "fixture", "messages": [], "stream": False}).encode()
                with urllib.request.urlopen(urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"})) as response:
                    self.assertEqual(json.load(response)["content"][0]["text"], "STYLE_LOCAL_STUB_DONE")
                self.assertEqual(len(probe.report_requests()), 1)
                self.assertNotIn("body", probe.report_requests()[0])
            finally:
                probe.close()


if __name__ == "__main__":
    unittest.main()
