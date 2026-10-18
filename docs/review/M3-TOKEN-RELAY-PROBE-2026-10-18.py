"""Private, isolated count endpoint probe; credentials never enter the report."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request


def digest(value):
    return hashlib.sha256(value).hexdigest()


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def error_category(body):
    text = body.decode("utf-8", errors="replace").lower()
    patterns = [
        ("cloudflare_1010", r"error code: 1010"),
        ("count_endpoint_unsupported", r"count.?tokens.{0,80}(?:unsupported|not supported|disabled|not available)"),
        ("authentication", r"unauthorized|invalid.{0,12}(?:api.?key|credential)|authentication"),
        ("model_unavailable", r"(?:model.{0,60}(?:not found|not supported|invalid|unavailable))"),
        ("quota_or_balance", r"insufficient|quota|credit|balance"),
        ("upstream", r"upstream|no available accounts"),
    ]
    return next((name for name, pattern in patterns if re.search(pattern, text)), "unspecified")


def run(config_path, output):
    config = json.loads(config_path.read_text())
    base = config["base_url"].strip().rstrip("/")
    model = config["model"].strip()
    key = config["api_key"].strip()
    url = urllib.parse.urlsplit(base)
    if url.scheme != "https" or not url.hostname or url.username or url.password or url.query or url.fragment:
        raise ValueError("HTTPS base URL without credentials, query, or fragment required")
    if not model or len(model) > 256 or any(ord(ch) < 32 for ch in model):
        raise ValueError("invalid model configuration")
    if not key or len(key) > 4096 or any(ord(ch) < 33 or ord(ch) > 126 for ch in key):
        raise ValueError("invalid key configuration")
    path = url.path.rstrip("/")
    if path.endswith("/v1/messages/count_tokens"):
        endpoint = base
    elif path.endswith("/v1"):
        endpoint = base + "/messages/count_tokens"
    elif path.endswith("/messages"):
        endpoint = base + "/count_tokens"
    else:
        endpoint = base + "/v1/messages/count_tokens"
    output.mkdir(mode=0o700, parents=True, exist_ok=True)
    tls = ssl.create_default_context()
    ca_source = "python-default"
    if tls.cert_store_stats()["x509_ca"] == 0:
        tls = ssl.create_default_context(cafile="/etc/ssl/cert.pem")
        ca_source = "system-pem"
    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({}), NoRedirect(),
        urllib.request.HTTPSHandler(context=tls),
    )
    short = "Please answer concisely."
    samples = [
        ("english_short", short),
        ("chinese_short", "请用简体中文回答，并保持简洁。"),
        ("mixed_long", short + "\n" + ("Keep every source citation accurate. 请逐条核对事实与证据。\n" * 128)),
        ("english_repeat", short),
    ]
    report = {
        "format": 1,
        "task": "M3-REAL-03 / M3-TOKEN-01 relay compatibility probe",
        "started_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "endpoint_sha256": digest(endpoint.encode()),
        "model_sha256": digest(model.encode()),
        "api_version": "2023-06-01",
        "request_envelope": "one user message with string content",
        "transport": {"tls_verification": True, "redirects": False, "retries": 0,
                      "timeout_seconds": 15, "response_limit_bytes": 1048576,
                      "ambient_proxy": False, "ca_source": ca_source,
                      "user_agent": "style-cli-count-probe/1.0",
                      "trusted_ca_count": tls.cert_store_stats()["x509_ca"]},
        "probe_sha256": digest(Path(__file__).read_bytes()),
        "production_cli": False,
        "official_upstream_verified": False,
        "generation_requests": 0,
        "cases": [],
    }
    for name, text in samples:
        payload = json.dumps({"model": model, "messages": [{"role": "user", "content": text}]},
                             ensure_ascii=False, separators=(",", ":")).encode()
        record = {"case": name, "text_utf8_bytes": len(text.encode()),
                  "text_sha256": digest(text.encode()), "request_sha256": digest(payload)}
        req = urllib.request.Request(endpoint, data=payload, method="POST", headers={
            "Content-Type": "application/json", "anthropic-version": "2023-06-01", "x-api-key": key,
            "User-Agent": "style-cli-count-probe/1.0",
        })
        started = time.monotonic()
        body = b""
        try:
            try:
                response = opener.open(req, timeout=15)
            except urllib.error.HTTPError as exc:
                response = exc
            with response:
                record["http_status"] = response.status
                body = response.read(1048577)
            record["response_bytes"] = len(body)
            record["response_sha256"] = digest(body)
            (output / (name + ".response")).write_bytes(body)
            if len(body) > 1048576:
                record["outcome"] = "response_too_large"
            elif not 200 <= record["http_status"] < 300:
                record["outcome"] = "http_error"
                record["error_category"] = error_category(body)
            else:
                parsed = json.loads(body, object_pairs_hook=strict_object)
                tokens = parsed.get("input_tokens") if isinstance(parsed, dict) else None
                valid = type(tokens) is int and 0 <= tokens <= 9223372036854775807
                record["outcome"] = "valid_count" if valid else "invalid_count"
                if valid:
                    record["input_tokens"] = tokens
        except Exception as exc:
            record["outcome"] = "transport_or_decode_error"
            record["error_class"] = type(exc).__name__
            reason = getattr(exc, "reason", None)
            if reason is not None:
                record["reason_class"] = type(reason).__name__
        record["elapsed_ms"] = round((time.monotonic() - started) * 1000, 3)
        report["cases"].append(record)
        (output / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps(record, ensure_ascii=False), flush=True)
        if record["outcome"] != "valid_count":
            break
    counts = [row.get("input_tokens") for row in report["cases"]]
    complete = len(counts) == len(samples) and all(type(value) is int for value in counts)
    report["checks"] = {"all_four_valid": complete,
                        "nonempty_text_positive": complete and all(value > 0 for value in counts),
                        "longer_text_increases_count": complete and counts[2] > max(counts[0], counts[1]),
                        "identical_request_stable": complete and counts[0] == counts[3]}
    report["compatible"] = all(report["checks"].values())
    report["finished_at"] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    (output / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"checks": report["checks"], "compatible": report["compatible"]}), flush=True)


if __name__ == "__main__":
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("config", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    run(args.config, args.output)
