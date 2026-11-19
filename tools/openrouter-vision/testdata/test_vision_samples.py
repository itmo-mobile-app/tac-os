"""Offline safety checks for the committed Wikimedia Commons JPEG fixtures."""
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

import vision_samples as samples


class SampleFixtureTests(unittest.TestCase):
    def test_manifest_has_positive_and_negative_cases(self):
        cases = samples.load_manifest()
        self.assertEqual(len(cases), 7)
        self.assertEqual(cases["C1"]["expect"], "present")
        self.assertEqual(cases["N1"]["expect"], "absent")
        self.assertEqual(cases["N3"]["expect"], "absent")
        for sample_id, item in cases.items():
            self.assertIn(item["expect"], ("present", "absent", None))
            self.assertTrue(item["source"].startswith(
                "https://commons.wikimedia.org/wiki/File:"))
            with self.subTest(sample_id=sample_id):
                samples.validate_image(
                    samples.image_path(sample_id), item["sha256"])

    def test_sample_ids_are_validated_without_network(self):
        cases = samples.load_manifest()
        self.assertEqual(samples.selected_ids("p1,c1,n1", cases),
                         ["P1", "C1", "N1"])
        for value in ("P1,P1", "P9", "P1,", ""):
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    samples.selected_ids(value, cases)

    def test_no_invalid_or_modified_photo_is_accepted(self):
        with tempfile.TemporaryDirectory() as folder:
            image = Path(folder) / "test.jpg"
            content = b"\xff\xd8\xff" + b"photo bytes" + b"\xff\xd9"
            image.write_bytes(content)
            sha = hashlib.sha256(content).hexdigest()
            samples.validate_image(image, sha)
            with self.assertRaises(ValueError):
                samples.validate_image(image, "0" * 64)
            image.write_bytes(b"not an image")
            with self.assertRaises(ValueError):
                samples.validate_image(image, hashlib.sha256(b"not an image").hexdigest())
            image.unlink()
            with self.assertRaises(ValueError):
                samples.validate_image(image, sha)

    def test_manifest_file_is_json(self):
        raw = json.loads(samples.MANIFEST.read_text(encoding="utf-8"))
        self.assertEqual(set(raw), set(samples.load_manifest()))


if __name__ == "__main__":
    unittest.main()
