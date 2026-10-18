import os
from pathlib import Path
import unittest
from unittest.mock import patch

from calibrate_tokens import calibrate, count_request, load_corpus
from token_test_support import TokenCorpusTestCase


class OfficialCalibrationPreparationTests(TokenCorpusTestCase):
    def test_corpus_has_fixed_balanced_meaningful_rules_and_verified_hashes(self):
        corpus = load_corpus(self.corpus_path)
        self.assertEqual(len(corpus["rules"]), 40)
        self.assertTrue(all(len(row["text"]) > 15 for row in corpus["rules"]))

    def test_rule_and_baseline_requests_have_identical_structure(self):
        corpus = self.corpus
        for provider, field in (("openai", "instructions"), ("anthropic", "system")):
            baseline = count_request(provider, "fixed-model", corpus, "")
            rule = count_request(provider, "fixed-model", corpus, "RULE_BODY")
            self.assertEqual(rule.pop(field), baseline.pop(field) + "RULE_BODY")
            self.assertEqual(rule, baseline)

    def test_missing_official_keys_never_calls_network_or_reports_real_counts(self):
        estimates = {row["id"]: {"tokens": 1, "chars": len(row["text"])} for row in self.corpus["rules"]}
        with patch.dict(os.environ, {}, clear=True), patch("calibrate_tokens.production_estimates", return_value=estimates), patch("calibrate_tokens.official_count") as count:
            report = calibrate(str(Path(__file__)), {"openai": "fixed-openai", "anthropic": "fixed-claude"}, self.corpus_path)
        count.assert_not_called()
        self.assertFalse(report["passed"])
        for provider in report["providers"].values():
            self.assertEqual(provider["status"], "blocked-missing-official-api-key")
            self.assertEqual(provider["completed"], 0)
            self.assertTrue(all("official_count" not in rule for rule in provider["rules"]))


if __name__ == "__main__":
    unittest.main()
