import copy
import unittest

from claude_live_probe import canonical_hash, replay_requests, request_observation


def fixture():
    shape = {"system": [{"type": "text", "text": "系统"}], "tools": []}
    one = {"role": "user", "content": [{"type": "text", "text": "HISTORY_MARKER"}]}
    two = {"role": "assistant", "content": [{"type": "text", "text": "OK"}]}
    return [{"type": "api-request-shape", "shapeHash": canonical_hash(shape), "shape": shape},
            *[{"type": "api-request-blob", "hash": canonical_hash(message), "message": message} for message in (one, two)],
            {"type": "api-request", "keep": 0, "tail": [canonical_hash(one)], "shapeHash": canonical_hash(shape),
             "params": {"model": "fixture"}, "id": "r1", "sessionId": "s", "timestamp": "01", "requestDigest": "native1"},
            {"type": "api-request", "keep": 1, "tail": [canonical_hash(two)], "shapeHash": canonical_hash(shape),
             "params": {"model": "fixture"}, "id": "r2", "sessionId": "s", "timestamp": "02", "requestDigest": "native2"}]


class NativeClaudeRequestReplayTests(unittest.TestCase):
    def test_prefix_replay_uses_verified_content_and_preserves_history(self):
        requests = replay_requests(fixture())
        self.assertEqual(len(requests[1]["body"]["messages"]), 2)
        self.assertTrue(request_observation(requests[1], ["HISTORY_MARKER"])["marker_locations"]["HISTORY_MARKER"])

    def test_missing_or_corrupt_native_content_is_not_treated_as_delivery(self):
        rows = fixture()
        rows[1]["message"]["content"][0]["text"] = "changed"
        with self.assertRaisesRegex(ValueError, "hash mismatch"):
            replay_requests(rows)
        rows = fixture()
        del rows[2]
        with self.assertRaisesRegex(ValueError, "missing content"):
            replay_requests(rows)

    def test_invalid_prefix_length_fails_instead_of_reusing_stale_request(self):
        rows = fixture()
        rows[3]["keep"] = 100
        with self.assertRaisesRegex(ValueError, "prefix"):
            replay_requests(rows)

    def test_streamed_assistant_blocks_are_matched_by_exact_request_hash(self):
        rows = fixture()
        blocks = [{"type": "thinking", "thinking": "fixture thought", "signature": "fixture signature"},
                  {"type": "tool_use", "id": "tool", "name": "Bash", "input": {"command": "printf OK"}}]
        message = {"role": "assistant", "content": blocks}
        rows[2:3] = [{"type": "assistant", "apiBlockIndex": index,
                      "message": {"id": "msg", "role": "assistant", "content": [block]}}
                     for index, block in enumerate(blocks)]
        rows[-1]["tail"] = [canonical_hash(message)]
        self.assertEqual(replay_requests(rows)[-1]["body"]["messages"][-1], message)

    def test_native_tool_result_row_can_supply_a_deduplicated_blob(self):
        rows = fixture()
        message = {"role": "user", "content": [{"type": "tool_result", "tool_use_id": "tool", "content": "OK"}]}
        rows[2] = {"type": "user", "message": message, "timestamp": "02"}
        rows[-1]["tail"] = [canonical_hash(message)]
        self.assertEqual(replay_requests(rows)[-1]["body"]["messages"][-1], message)

    def test_native_string_prompt_is_verified_against_block_form_hash(self):
        rows = fixture()
        rows[1] = {"type": "user", "message": {"role": "user", "content": "HISTORY_MARKER"}}
        self.assertEqual(replay_requests(rows)[0]["body"]["messages"][0]["content"][0]["text"], "HISTORY_MARKER")

    def test_fork_rewritten_system_blob_requires_verified_source(self):
        rows = fixture()
        source = copy.deepcopy(rows)
        rows[1]["message"] = {"role": "system", "content": "HISTORY_MARKER"}
        with self.assertRaisesRegex(ValueError, "hash mismatch"):
            replay_requests(rows)
        self.assertEqual(replay_requests(rows, source)[0]["body"]["messages"][0], source[1]["message"])


if __name__ == "__main__":
    unittest.main()
