"""Temporary synthetic input for offline token collector tests."""
import hashlib
import json
from pathlib import Path
import tempfile
import unittest


class TokenCorpusTestCase(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        rules = []
        for language in ("en", "zh"):
            for number in range(20):
                text = (f"Review fixture {number}: report validation errors clearly."
                        if language == "en" else
                        f"检查测试样例{number}，清晰报告校验错误并保留上下文。")
                rules.append({"id": f"{language}-fixture-{number}",
                              "language": language, "text": text,
                              "sha256": hashlib.sha256(text.encode()).hexdigest()})
        self.corpus = {"baseline_prefix": "Fixture instructions:\n",
                       "user_message": "Continue.", "rules": rules}
        self.corpus_path = Path(directory.name) / "corpus.json"
        self.corpus_path.write_text(json.dumps(self.corpus, ensure_ascii=False), encoding="utf-8")
