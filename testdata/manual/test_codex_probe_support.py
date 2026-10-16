import json
import unittest

from codex_probe_support import marker_locations


class MarkerEvidenceTests(unittest.TestCase):
    def test_retention_records_non_developer_summary_without_copying_content(self):
        body = {"input": [
            {"role": "developer", "content": "MARKER first MARKER"},
            {"role": "user", "content": "private summary with MARKER"},
            {"type": "function_call_output", "output": "unrelated private output"},
        ]}
        found = marker_locations(body, ("MARKER", "MISSING"))
        self.assertEqual([row["role"] for row in found["MARKER"]], ["developer", "user"])
        self.assertEqual([row["occurrences"] for row in found["MARKER"]], [2, 1])
        self.assertEqual(found["MISSING"], [])
        self.assertNotIn("private", json.dumps(found))

    def test_marker_in_tool_result_is_distinguished_from_hook_instruction(self):
        found = marker_locations({"input": [{"type": "function_call_output", "output": "MARKER"}]}, ("MARKER",))
        self.assertEqual(found["MARKER"][0]["type"], "function_call_output")
        self.assertIsNone(found["MARKER"][0]["role"])


if __name__ == "__main__":
    unittest.main()
