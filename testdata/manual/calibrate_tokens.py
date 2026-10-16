#!/usr/bin/env python3
"""Count a fixed corpus with official APIs; missing credentials remain pending."""

import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import urllib.error
import urllib.request


CORPUS = Path(__file__).with_name("token-calibration-40.json")
ENDPOINTS = {"openai": "https://api.openai.com/v1/responses/input_tokens", "anthropic": "https://api.anthropic.com/v1/messages/count_tokens"}


def encoded(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load_corpus(path):
    corpus = json.loads(path.read_text())
    assert len(corpus["rules"]) == 40
    assert {language: sum(rule["language"] == language for rule in corpus["rules"]) for language in ("en", "zh")} == {"en": 20, "zh": 20}
    assert len({rule["id"] for rule in corpus["rules"]}) == 40
    for rule in corpus["rules"]:
        assert sha(rule["text"].encode()) == rule["sha256"]
    return corpus


def count_request(provider, model, corpus, text):
    instructions = corpus["baseline_prefix"] + text
    if provider == "openai":
        return {"model": model, "instructions": instructions, "input": corpus["user_message"]}
    return {"model": model, "system": instructions, "messages": [{"role": "user", "content": corpus["user_message"]}]}


def official_count(provider, body, key):
    headers = {"Content-Type": "application/json"}
    if provider == "openai":
        headers["Authorization"] = "Bearer " + key
    else:
        headers.update({"x-api-key": key, "anthropic-version": "2023-06-01"})
    request = urllib.request.Request(ENDPOINTS[provider], encoded(body), headers)
    with urllib.request.urlopen(request, timeout=45) as response:
        value = json.load(response)
        count = value["input_tokens"]
        if type(count) is not int or count < 0:
            raise ValueError("invalid input_tokens")
        return {"input_tokens": count, "request_id": response.headers.get("request-id", response.headers.get("x-request-id"))}


def production_estimates(style, corpus):
    with tempfile.TemporaryDirectory(prefix="style-token-corpus-") as directory:
        package = {"spec": "style/v0.3", "name": "token-calibration", "version": "0.1.0", "description": "Fixed forty-rule calibration corpus",
                   "rules": [{"id": row["id"], "target": "event", "event": "session.start", "text": row["text"]} for row in corpus["rules"]]}
        (Path(directory) / "STYLE.yaml").write_text(json.dumps(package, ensure_ascii=False))
        environment = {key: value for key, value in os.environ.items() if key not in ("CODEX_HOME", "CODEX_SANDBOX", "STYLE_HOME", "CLAUDE_CONFIG_DIR")}
        environment.update(STYLE_HOME=str(Path(directory) / "style"), CODEX_HOME=str(Path(directory) / "codex"), CLAUDE_CONFIG_DIR=str(Path(directory) / "claude"))
        result = subprocess.run([style, "plan", directory, "--harness", "codex", "--json"], env=environment, capture_output=True, text=True, check=True)
        report = json.loads(result.stdout)["reports"][0]["tokens"]
        return {row["rule"]: row["estimate"] for row in report["rules"]}


def calibrate(style, models):
    corpus = load_corpus(CORPUS)
    estimates = production_estimates(style, corpus)
    report = {"item": "M1-FINAL-09", "at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
              "corpus_sha256": sha(CORPUS.read_bytes()), "style_binary_sha256": sha(Path(style).read_bytes()),
              "measurement": "Official counter delta = same-shape rule request input_tokens minus fixed baseline input_tokens. STYLE estimates come from the real CLI, not an alternate tokenizer.",
              "providers": {}}
    for provider, model in models.items():
        baseline = count_request(provider, model, corpus, "")
        record = {"model": model, "endpoint": ENDPOINTS[provider], "baseline": baseline,
                  "baseline_request_sha256": sha(encoded(baseline)), "status": "pending", "rules": []}
        report["providers"][provider] = record
        for rule in corpus["rules"]:
            request = count_request(provider, model, corpus, rule["text"])
            record["rules"].append({**rule, "style_estimate": estimates[rule["id"]], "request_sha256": sha(encoded(request))})
        key = os.environ.get("OPENAI_API_KEY" if provider == "openai" else "ANTHROPIC_API_KEY")
        if not key:
            record.update(status="blocked-missing-official-api-key", completed=0)
            continue
        try:
            record["baseline_count"] = official_count(provider, baseline, key)
            for rule, result in zip(corpus["rules"], record["rules"]):
                counted = official_count(provider, count_request(provider, model, corpus, rule["text"]), key)
                delta = counted["input_tokens"] - record["baseline_count"]["input_tokens"]
                result.update(official_count=counted, incremental_input_tokens=delta,
                              relative_error=(result["style_estimate"]["tokens"] - delta) / delta if delta > 0 else None)
            record.update(status="counted", completed=40)
        except (OSError, ValueError, KeyError) as error:
            record.update(status="blocked-counter-service", error_type=type(error).__name__, http_status=getattr(error, "code", None),
                          completed=sum("official_count" in rule for rule in record["rules"]))
    report["passed"] = all(record["status"] == "counted" for record in report["providers"].values())
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--style", required=True)
    parser.add_argument("--openai-model", required=True)
    parser.add_argument("--anthropic-model", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    report = calibrate(args.style, {"openai": args.openai_model, "anthropic": args.anthropic_model})
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"passed": report["passed"], "providers": {name: {key: row.get(key) for key in ("model", "status", "completed", "http_status")}
                                                             for name, row in report["providers"].items()}}, indent=2))
    return 0 if report["passed"] else 2


if __name__ == "__main__":
    raise SystemExit(main())
