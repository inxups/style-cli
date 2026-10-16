import unittest

from collect_claude_c9 import workload


def pair(identity, start, end, command="printf 'STYLE_M1B_C9\\n'"):
    return [{"type": "assistant", "timestamp": start, "message": {"id": identity, "content": [
                {"type": "tool_use", "id": identity, "name": "Bash", "input": {"command": command}}]}},
            {"type": "user", "timestamp": end, "message": {"content": [
                {"type": "tool_result", "tool_use_id": identity, "content": "STYLE_M1B_C9", "is_error": False}]}}]


class NativeC9WorkloadTests(unittest.TestCase):
    def test_snapshots_and_duplicate_rows_are_not_extra_executions(self):
        rows = pair("one", "01", "02") + pair("two", "03", "04")
        snapshot = {**rows[1], "type": "api-request-blob"}
        snapshot.pop("timestamp")
        observed = workload(rows + rows + [snapshot], expected=2)
        self.assertTrue(observed["passed"])
        self.assertEqual(observed["unique_calls"], 2)
        self.assertEqual(observed["ignored_non_timed_snapshots"], 1)

    def test_overlapping_calls_fail_even_when_every_result_succeeds(self):
        observed = workload(pair("one", "01", "04") + pair("two", "02", "05"), expected=2)
        self.assertFalse(observed["passed"])
        self.assertEqual(len(observed["overlaps"]), 1)

    def test_loop_missing_result_and_error_do_not_pass(self):
        self.assertFalse(workload(pair("one", "01", "02", "for i in 1 2; do printf STYLE_M1B_C9; done"), expected=1)["passed"])
        self.assertFalse(workload(pair("one", "01", "02")[:1], expected=1)["passed"])
        rows = pair("one", "01", "02")
        rows[1]["message"]["content"][0]["is_error"] = True
        self.assertFalse(workload(rows, expected=1)["passed"])


if __name__ == "__main__":
    unittest.main()
