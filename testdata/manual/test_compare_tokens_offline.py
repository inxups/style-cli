import unittest

from compare_tokens_offline import compare, metrics
from token_test_support import TokenCorpusTestCase


class OfflineTokenReferenceTests(TokenCorpusTestCase):
    def test_statistics_keep_language_groups_and_per_rule_error(self):
        rows = [{"style_estimate": {"tokens": 20}, "ref": 10},
                {"style_estimate": {"tokens": 10}, "ref": 20}]
        result = metrics(rows, "ref")
        self.assertEqual(result["mean_signed_relative_error"], 0.25)
        self.assertEqual(result["mean_absolute_relative_error"], 0.75)
        self.assertEqual(result["max_absolute_relative_error"], 1)
        self.assertEqual(result["style_estimated_tokens"], 30)
        self.assertEqual(result["local_reference_tokens"], 30)

    def test_missing_rule_does_not_silently_reduce_coverage(self):
        with self.assertRaisesRegex(ValueError, "IDs"):
            compare(self.corpus, {}, [])

    def test_changed_rendering_and_invalid_counts_are_rejected(self):
        corpus = self.corpus
        for bad in ({"tokens": True, "chars": len(corpus["rules"][0]["text"])},
                    {"tokens": -1, "chars": len(corpus["rules"][0]["text"])},
                    {"tokens": 1, "chars": 0}):
            estimates = {row["id"]: {"tokens": 1, "chars": len(row["text"])} for row in corpus["rules"]}
            estimates[corpus["rules"][0]["id"]] = bad
            with self.assertRaisesRegex(ValueError, "Invalid STYLE"):
                compare(corpus, estimates, [])

    def test_nonpositive_reference_is_not_reported_as_success(self):
        class EmptyEncoding:
            def encode_ordinary(self, text):
                return []

        corpus = self.corpus
        estimates = {row["id"]: {"tokens": 1, "chars": len(row["text"])} for row in corpus["rules"]}
        with self.assertRaisesRegex(ValueError, "Nonpositive"):
            compare(corpus, estimates, [EmptyEncoding()])


if __name__ == "__main__":
    unittest.main()
