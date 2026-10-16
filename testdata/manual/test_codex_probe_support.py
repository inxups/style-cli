import json
import unittest

from codex_probe_support import marker_locations
from probe_codex_context import child_tool_results


class MarkerEvidenceTests(unittest.TestCase):
    def test_child_result_accepts_native_code_mode_output_and_requires_exit_code(self):
        row = {"type": "response_item", "payload": {"type": "custom_tool_call_output", "output": [
            {"type": "input_text", "text": '{"exit_code":0,"output":"STYLE_CHILD_SCOPE_OK"}'},
            {"type": "input_text", "text": "Command blocked by PreToolUse hook: M1_FINAL_CHILD_BLOCKED"}]}}
        self.assertEqual(child_tool_results([row]), {"allowed_printf_results": [
            {"exit_code": 0, "output": "STYLE_CHILD_SCOPE_OK"}], "blocked_printf_hook_result": True})
        self.assertEqual(child_tool_results([{"type": "response_item", "payload": {
            "type": "function_call", "arguments": "STYLE_CHILD_SCOPE_OK"}}])["allowed_printf_results"], [])

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
