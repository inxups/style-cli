"""Read `style --json` stdout across binary versions.

Current binaries print one envelope: {format: 2, command, status, scope,
diagnostics, result}. Older binaries print the report itself. `report` checks
the envelope strictly and returns the report shape older binaries printed, so
acceptance scripts can compare old and new installations directly.
"""

import json

ENVELOPE_KEYS = ["format", "command", "status", "scope", "diagnostics", "result"]


def envelope(text):
    value = json.loads(text)
    assert isinstance(value, dict) and list(value) == ENVELOPE_KEYS, value
    assert value["format"] == 2 and value["command"] and value["status"] in (
        "ok", "error", "needs-trust", "blocked"), value
    assert isinstance(value["scope"], dict) and value["scope"]["kind"] in ("user", "project"), value
    assert isinstance(value["diagnostics"], list), value
    return value


def report(text):
    value = json.loads(text)
    if not (isinstance(value, dict) and value.get("format") == 2):
        return value  # an older binary's report
    value = envelope(text)
    result = value["result"]
    if value["command"] == "validate" and not (result or {}).get("source"):
        return value["diagnostics"]
    if result is None:
        result = {"status": value["status"]}
    result = {**result, "diagnostics": value["diagnostics"]}
    if value["scope"]["kind"] == "project":
        result["scope"] = value["scope"]
    return result
