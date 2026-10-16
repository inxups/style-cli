import tempfile
import unittest
from pathlib import Path

from probe_codex_routes import finish_configuration_roundtrip


class ConfigurationRoundtripTests(unittest.TestCase):
    def test_cleans_only_the_exact_native_fixture_registration(self):
        with tempfile.TemporaryDirectory() as root:
            work = Path(root) / "work"
            baseline = b'model="fixture"\n'
            suffix = ('[projects."' + str(work.resolve()) + '"]\ntrust_level = "trusted"\n\n').encode()
            config = Path(root) / "config.toml"
            config.write_bytes(baseline + suffix)
            result = finish_configuration_roundtrip(config, baseline, work)
            self.assertTrue(result["native_project_trust_added_and_cleaned_by_probe"])
            self.assertEqual(config.read_bytes(), baseline)

    def test_preexisting_trust_is_preserved(self):
        with tempfile.TemporaryDirectory() as root:
            work = Path(root) / "work"
            baseline = b'[projects."existing"]\ntrust_level = "trusted"\n'
            config = Path(root) / "config.toml"
            config.write_bytes(baseline)
            result = finish_configuration_roundtrip(config, baseline, work)
            self.assertFalse(result["native_project_trust_added_and_cleaned_by_probe"])
            self.assertEqual(config.read_bytes(), baseline)

    def test_unexpected_residue_fails_without_rewriting_configuration(self):
        with tempfile.TemporaryDirectory() as root:
            work = Path(root) / "work"
            baseline = b'model="fixture"\n'
            unexpected = baseline + b'\n[plugins."leftover"]\nenabled=true\n'
            config = Path(root) / "config.toml"
            config.write_bytes(unexpected)
            with self.assertRaises(AssertionError):
                finish_configuration_roundtrip(config, baseline, work)
            self.assertEqual(config.read_bytes(), unexpected)


if __name__ == "__main__":
    unittest.main()
