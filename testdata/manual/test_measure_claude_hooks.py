"""Guard the log parser used to decide real C9 timing acceptance."""

import unittest

from measure_claude_hooks import collect, summarize


def completion(duration, binary="/test/style", owner="style", status=0):
    return (
        "2026-10-15T14:20:00Z [DEBUG] Hooks: PreToolUse:Bash "
        f"[{binary} agenthooks run --owner={owner}] finished with status "
        f"{status} ({duration}ms)\n"
    )


class TimingEvidenceTest(unittest.TestCase):
    def test_percentile_and_even_median(self):
        values = [{"duration_ms": n} for n in range(1, 101)]
        self.assertEqual(summarize(values), {
            "count": 100, "median_ms": 50.5, "p95_ms": 95, "max_ms": 100,
        })

    def test_keeps_slow_samples_and_rejects_threshold_equality(self):
        raw = (completion(10) * 18 + completion(50) + completion(209)).encode()
        report = collect(raw, "/test/style", 20, 50)
        self.assertEqual(report["status"], "over-budget")
        self.assertEqual(report["overall"]["p95_ms"], 50)
        self.assertEqual(report["overall"]["max_ms"], 209)

    def test_filters_other_owners_binaries_and_internal_hook_timings(self):
        raw = (
            completion(21.5) + completion(999, owner="style-other")
            + completion(999, binary="/other/style")
            + "2026-10-15T14:20:00Z [DEBUG] hooks module PreToolUse settled (999ms)\n"
        ).encode()
        report = collect(raw, "/test/style", 1, 50)
        self.assertEqual(report["status"], "within-budget")
        self.assertEqual(report["overall"]["count"], 1)
        self.assertEqual(report["overall"]["p95_ms"], 21.5)

    def test_hook_error_is_not_a_performance_pass(self):
        report = collect(completion(1, status=1).encode(), None, 1, 50)
        self.assertEqual(report["status"], "hook-errors")
        self.assertEqual(report["hook_errors"], 1)

    def test_unknown_completion_format_is_not_silently_dropped(self):
        raw = completion("unknown").encode()
        report = collect(raw, None, 1, 50)
        self.assertEqual(report["status"], "unparsed-completions")
        self.assertEqual(report["unparsed_STYLE_completions"], 1)

    def test_empty_log_is_not_a_performance_pass(self):
        report = collect(b"", None, 100, 50)
        self.assertEqual(report["status"], "insufficient-samples")
        self.assertIsNone(report["overall"]["p95_ms"])


if __name__ == "__main__":
    unittest.main()
