# M2-A5 stats fixture

Clock: `2026-10-16T00:00:00Z`; default window starts `2026-10-09T00:00:00Z`.
`alpha`/`beta` each contain main and child agent records; no installed packages or index are required.
The lower/upper boundary records count, while the adjacent one-nanosecond records do not.
All hashes are synthetic metadata, never bodies.

Expected totals: outputs 11, blocks 3, dedup 1, cap_drops 1, estimated_tokens 96, unestimated 3.
Rule order: codex/pkg/check (37), claude-code/pkg/check (27), codex/pkg/stop (12),
codex/pkg/boundary (10), codex/pkg/else (10), codex/removed/legacy (0), retired-harness/removed/free (0).
Session order: codex/alpha (47), claude-code/alpha (27), codex/beta (22), retired-harness/alpha (0).
Doctor counts 2 legacy outputs/blocks lacking tokens and hash plus the hashless warn (3 total).
The codex/pkg/check estimate has five costed emissions and one hash, so repeated_body is true.
