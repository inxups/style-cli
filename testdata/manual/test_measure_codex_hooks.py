"""Guard the OTLP parser used to decide real B11 timing acceptance."""

import json
import unittest

from measure_codex_hooks import collect


def span(duration_ms, event="PreToolUse", source="user", outcome="completed", start_ms=0, name="codex.hooks.command", handler="command"):
    start = 1_792_000_000_000_000_000 + int(start_ms * 1e6)
    return {
        "name": name,
        "traceId": "t",
        "startTimeUnixNano": str(start),
        "endTimeUnixNano": str(start + int(duration_ms * 1e6)),
        "attributes": [
            {"key": "hook.event_name", "value": {"stringValue": event}},
            {"key": "hook.source", "value": {"stringValue": source}},
            {"key": "hook.handler_type", "value": {"stringValue": handler}},
            {"key": "hook.command_outcome", "value": {"stringValue": outcome}},
        ],
    }


def export(*spans):
    body = {"resourceSpans": [{"scopeSpans": [{"spans": list(spans)}]}]}
    return (json.dumps({"path": "/v1/traces", "body": body}) + "\n").encode()


def sequential(durations, **kwargs):
    return [span(value, start_ms=index * 1000, **kwargs) for index, value in enumerate(durations)]


class CodexTimingEvidenceTest(unittest.TestCase):
    def test_gate_uses_pre_tool_use_and_keeps_slow_samples(self):
        raw = export(*sequential([10] * 18 + [150, 900]), span(5000, event="SessionStart", start_ms=-5000))
        report = collect(raw, "user", "PreToolUse", 20, 150)
        self.assertEqual(report["status"], "over-budget")
        self.assertEqual(report["gate"]["p95_ms"], 150)
        self.assertEqual(report["gate"]["max_ms"], 900)
        self.assertEqual(report["by_event"]["SessionStart"]["count"], 1)

    def test_filters_other_sources_handlers_and_spans(self):
        raw = export(
            span(20), span(999, source="plugin", start_ms=10), span(999, handler="mcp_tool", start_ms=20),
            span(999, name="run_hooks_and_record_inputs", start_ms=30),
        )
        report = collect(raw, "user", "PreToolUse", 1, 150)
        self.assertEqual(report["status"], "within-budget")
        self.assertEqual(report["overall"]["count"], 1)

    def test_time_window_and_overlap_count(self):
        raw = export(span(50, start_ms=0), span(50, start_ms=10), span(50, start_ms=5000))
        start = 1_792_000_000_000_000_000
        report = collect(raw, "user", "PreToolUse", 1, 150, until=start + int(1000 * 1e6))
        self.assertEqual(report["gate"]["count"], 2)
        self.assertEqual(report["gate_overlapping_pairs"], 1)

    def test_failed_hook_is_not_a_performance_pass(self):
        report = collect(export(span(1, outcome="failed")), "user", "PreToolUse", 1, 150)
        self.assertEqual(report["status"], "hook-errors")

    def test_malformed_export_is_not_silently_dropped(self):
        report = collect(b"not json\n" + export(span(1)), "user", "PreToolUse", 1, 150)
        self.assertEqual(report["status"], "malformed-export")

    def test_empty_export_is_not_a_performance_pass(self):
        report = collect(b"", "user", "PreToolUse", 100, 150)
        self.assertEqual(report["status"], "insufficient-samples")
        self.assertIsNone(report["gate"]["p95_ms"])


if __name__ == "__main__":
    unittest.main()
