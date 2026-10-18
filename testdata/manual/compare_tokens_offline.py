#!/usr/bin/env python3
"""Compare STYLE estimates with named local encodings, never official counters."""

import argparse
import hashlib
import importlib.metadata
import json
from pathlib import Path
import struct

from calibrate_tokens import load_corpus, production_estimates, sha


ENCODINGS = ("o200k_base", "cl100k_base")


def fingerprint(encoding):
    """Hash the public token-ID/bytes mapping, including special tokens."""
    digest = hashlib.sha256()
    defined = 0
    for token_id in range(encoding.n_vocab):
        try:
            value = encoding.decode_single_token_bytes(token_id)
        except KeyError:
            continue
        digest.update(struct.pack(">II", token_id, len(value)))
        digest.update(value)
        defined += 1
    return {"name": encoding.name, "n_vocab": encoding.n_vocab,
            "defined_token_ids": defined, "token_id_bytes_sha256": digest.hexdigest()}


def metrics(rows, reference):
    errors = [(row["style_estimate"]["tokens"] - row[reference]) / row[reference] for row in rows]
    return {"rules": len(rows), "style_estimated_tokens": sum(row["style_estimate"]["tokens"] for row in rows),
            "local_reference_tokens": sum(row[reference] for row in rows),
            "mean_signed_relative_error": sum(errors) / len(errors),
            "mean_absolute_relative_error": sum(abs(value) for value in errors) / len(errors),
            "max_absolute_relative_error": max(abs(value) for value in errors)}


def compare(corpus, estimates, encodings):
    rules = corpus["rules"]
    if set(estimates) != {row["id"] for row in rules}:
        raise ValueError("STYLE estimate IDs do not match the corpus")
    for row in rules:
        estimate = estimates[row["id"]]
        if (type(estimate.get("tokens")) is not int or estimate["tokens"] < 0
                or estimate.get("chars") != len(row["text"])):
            raise ValueError("Invalid STYLE estimate or changed rendered body")
    results = []
    for encoding in encodings:
        prefix_tokens = len(encoding.encode_ordinary(corpus["baseline_prefix"]))
        rows = []
        for rule in rules:
            body = len(encoding.encode_ordinary(rule["text"]))
            delta = len(encoding.encode_ordinary(corpus["baseline_prefix"] + rule["text"])) - prefix_tokens
            if body <= 0 or delta <= 0:
                raise ValueError("Nonpositive local reference; no calibration result emitted")
            rows.append({"id": rule["id"], "language": rule["language"], "body_sha256": rule["sha256"],
                         "style_estimate": estimates[rule["id"]], "local_body_tokens": body,
                         "local_prefixed_text_delta": delta})
        summary = {}
        for field in ("local_body_tokens", "local_prefixed_text_delta"):
            summary[field] = {language: metrics([row for row in rows if language == "all" or row["language"] == language], field)
                              for language in ("all", "en", "zh")}
        results.append({"encoding": fingerprint(encoding), "local_prefix_tokens": prefix_tokens,
                        "rules": rows, "summary": summary})
    return results


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--style", required=True, type=Path)
    parser.add_argument("--corpus", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    # Optional manual dependency only; the normal CLI and CI do not require it.
    import tiktoken

    corpus = load_corpus(args.corpus)
    report = {"item": "M2-ALT-01", "format": 1,
              "measurement": "Local text-only encoding comparison; no model mapping or API request accounting is assumed.",
              "corpus_sha256": sha(args.corpus.read_bytes()), "style_binary_sha256": sha(args.style.read_bytes()),
              "collector_sha256": sha(Path(__file__).read_bytes()),
              "tiktoken_version": importlib.metadata.version("tiktoken"),
              "encodings": compare(corpus, production_estimates(str(args.style.resolve()), corpus),
                                   [tiktoken.get_encoding(name) for name in ENCODINGS]),
              "official_calibration": {"status": "pending", "completed_per_provider": 0, "required_per_provider": 40},
              "limits": ["Local encodings are not the official OpenAI or Claude request counter.",
                         "No Claude tokenizer equivalence or model-specific accuracy is claimed.",
                         "Message framing, tools, hidden instructions and cache costs are excluded.",
                         "No error threshold is asserted and production coefficients are unchanged.",
                         "Initial encoding-table setup can download public data; warm-cache runs need no network."],
              "completed": True}
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    for result in report["encodings"]:
        print(result["encoding"]["name"], json.dumps(result["summary"]["local_body_tokens"], ensure_ascii=False))


if __name__ == "__main__":
    main()
