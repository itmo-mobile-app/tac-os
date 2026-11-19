"""Offline safety tests; these never download photos or call OpenRouter."""
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import vision_samples as samples


class SampleFixtureTests(unittest.TestCase):
    def test_manifest_has_useful_positive_and_negative_cases(self):
        self.assertEqual(len(samples.SAMPLES), 7)
        self.assertEqual(samples.SAMPLES["C1"]["expect"], "present")
        self.assertEqual(samples.SAMPLES["N1"]["expect"], "absent")
        self.assertIsNone(samples.SAMPLES["N3"]["expect"])
        for item in samples.SAMPLES.values():
            self.assertTrue(item["url"].startswith(
                "https://upload.wikimedia.org/wikipedia/commons/"))

    def test_ids_are_validated_without_network(self):
        self.assertEqual(samples.selected_ids("p1,c1,n1"), ["P1", "C1", "N1"])
        for value in ("P1,P1", "P9", "P1,", ""):
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    samples.selected_ids(value)

    def test_only_complete_bounded_jpeg_is_accepted(self):
        samples.validate_jpeg(b"\xff\xd8\xffpicture\xff\xd9")
        for raw in (b"", b"\xff\xd8\xfftruncated", b"not-jpeg",
                    b"\xff\xd8\xff" + b"a" * (samples.MAX_BYTES + 1) + b"\xff\xd9"):
            with self.subTest(size=len(raw)):
                with self.assertRaises(ValueError):
                    samples.validate_jpeg(raw)

    def test_fetch_one_writes_only_checked_jpeg(self):
        valid_data = b"\xff\xd8\xffpixels\xff\xd9"
        url = samples.SAMPLES["P1"]["url"]

        class FakeReply(io.BytesIO):
            def geturl(self):
                return url

            def __enter__(self):
                return self

            def __exit__(self, *unused):
                self.close()

        with tempfile.TemporaryDirectory() as root:
            with patch.object(samples, "PHOTOS", Path(root)):
                with patch.object(samples, "urlopen",
                                  return_value=FakeReply(valid_data)) as downloader:
                    samples.fetch_one("P1")
                    self.assertEqual(samples.image_path("P1").read_bytes(), valid_data)
                    samples.fetch_one("P1")  # Already downloaded; no second fetch.
                    self.assertEqual(downloader.call_count, 1)

    def test_bad_image_is_never_written(self):
        with tempfile.TemporaryDirectory() as root:
            with patch.object(samples, "PHOTOS", Path(root)):
                with patch.object(samples, "urlopen") as mock_fetch:
                    class FakeReply(io.BytesIO):
                        def geturl(self):
                            return samples.SAMPLES["N1"]["url"]
                        def __enter__(self):
                            return self
                        def __exit__(self, *unused):
                            self.close()
                    mock_fetch.return_value = FakeReply(b"not an image")
                    with self.assertRaises(ValueError):
                        samples.fetch_one("N1")
                    self.assertFalse(samples.image_path("N1").exists())


if __name__ == "__main__":
    unittest.main()
