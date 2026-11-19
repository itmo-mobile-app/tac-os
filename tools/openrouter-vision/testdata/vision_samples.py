#!/usr/bin/env python3
"""Opt-in Wikimedia Commons image samples for the OpenRouter Vision PoC.

Normal CI never downloads these photos or calls OpenRouter. Run this script
manually on trusted, authorized images to evaluate real model behavior.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from urllib.parse import urlparse
from urllib.request import Request, urlopen

HERE = Path(__file__).resolve().parent
PHOTOS = HERE / "photos"
MAX_BYTES = 2 << 20

# The actual files are downloaded locally, not committed to the repository.
# Attribution and licensing details are recorded in README.md.
SAMPLES = {
    "P1": {
        "url": "https://upload.wikimedia.org/wikipedia/commons/9/98/Airsoft_squad.jpg",
        "expect": "present",
    },
    "P2": {
        "url": "https://upload.wikimedia.org/wikipedia/commons/thumb/3/37/AirsoftRockField.jpg/960px-AirsoftRockField.jpg",
        "expect": "present",
    },
    "P3": {
        "url": "https://upload.wikimedia.org/wikipedia/commons/3/32/Airsoft_sniper.jpg",
        "expect": "present",
    },
    "C1": {
        "url": "https://upload.wikimedia.org/wikipedia/commons/thumb/9/91/People_walking_in_park_in_front_of_bay_and_Berkeley_shore_%287114041597%29.jpg/1280px-People_walking_in_park_in_front_of_bay_and_Berkeley_shore_%287114041597%29.jpg",
        "expect": "present",
    },
    "N1": {
        "url": "https://upload.wikimedia.org/wikipedia/commons/thumb/e/ed/Forest_nature_trail.jpg/960px-Forest_nature_trail.jpg",
        "expect": "absent",
    },
    "N2": {
        "url": "https://upload.wikimedia.org/wikipedia/commons/thumb/4/4f/Airsoft_Bushnell_rifle_scope.JPG/960px-Airsoft_Bushnell_rifle_scope.JPG",
        "expect": "absent",
    },
    "N3": {
        "url": "https://upload.wikimedia.org/wikipedia/commons/thumb/7/76/Mannequin_in_a_Singapore_Army_uniform_on_display_at_the_National_Museum_of_Singapore_-_20140223.jpg/960px-Mannequin_in_a_Singapore_Army_uniform_on_display_at_the_National_Museum_of_Singapore_-_20140223.jpg",
        "expect": None,  # Informational mannequin case; permitted false positive in Sprint 4 PoC
    },
}


def selected_ids(value):
    names = [x.strip().upper() for x in value.split(",")]
    if not all(x in SAMPLES for x in names) or len(names) != len(set(names)):
        raise ValueError("invalid or duplicate sample ID; options: " + ", ".join(SAMPLES))
    return names


def validate_jpeg(data):
    if not data.startswith(b"\xff\xd8\xff") or not data.endswith(b"\xff\xd9"):
        raise ValueError("downloaded content is not a complete JPEG")
    if len(data) > MAX_BYTES:
        raise ValueError("image exceeds the 2 MiB OpenRouter PoC limit")


def image_path(sample_id):
    return PHOTOS / (sample_id + ".jpg")


def fetch_one(sample_id):
    path = image_path(sample_id)
    if path.exists():
        try:
            validate_jpeg(path.read_bytes())
        except ValueError as exc:
            raise ValueError(f"{sample_id}: existing image is invalid: {exc}") from exc
        print(f"{sample_id}: already downloaded -> {path}")
        return
    url = SAMPLES[sample_id]["url"]
    parsed = urlparse(url)
    if parsed.scheme != "https" or parsed.hostname != "upload.wikimedia.org":
        raise ValueError("unexpected photo host")
    req = Request(url, headers={"User-Agent": "tac-os-course-image-fixtures/1.0"})
    with urlopen(req, timeout=30) as response:
        redirect = urlparse(response.geturl())
        if redirect.scheme != "https" or redirect.hostname != "upload.wikimedia.org":
            raise ValueError("unexpected download redirect")
        data = response.read(MAX_BYTES + 1)
    validate_jpeg(data)
    PHOTOS.mkdir(parents=True, exist_ok=True)
    # Only write after complete size/type validation.
    with tempfile.NamedTemporaryFile(dir=PHOTOS, delete=False) as tmp:
        tmp.write(data)
        temporary = Path(tmp.name)
    temporary.replace(path)
    print(f"{sample_id}: downloaded {len(data)} bytes -> {path}")


def run_one(sample_id, timeout):
    path = image_path(sample_id)
    if not path.is_file():
        raise ValueError(f"{sample_id}: image missing; first run 'fetch'")
    validate_jpeg(path.read_bytes())
    cmd = ["go", "run", ".", "-image", str(path), "-send-image",
           "-timeout", str(timeout) + "s"]
    try:
        finished = subprocess.run(
            cmd,
            cwd=HERE.parent,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=timeout + 90,
            check=False,
        )
    except subprocess.TimeoutExpired:
        print(f"{sample_id}: ERROR: local Go process timed out", file=sys.stderr)
        return False
    if finished.returncode:
        # CLI already sanitizes provider errors; no raw image/model response.
        print(f"{sample_id}: ERROR: {finished.stderr.strip()}", file=sys.stderr)
        return False
    try:
        result = json.loads(finished.stdout)
    except json.JSONDecodeError:
        print(f"{sample_id}: ERROR: malformed CLI output", file=sys.stderr)
        return False
    actual = result.get("status")
    if actual not in ("present", "absent", "uncertain"):
        print(f"{sample_id}: ERROR: unexpected status", file=sys.stderr)
        return False
    expected = SAMPLES[sample_id]["expect"]
    # The Go CLI validates bbox shape and bounds. It cannot check pixel accuracy.
    label = "REVIEW" if expected is None else ("PASS" if actual == expected else "FAIL")
    print(f"{sample_id}: {label} expected={expected or 'informational'} actual={actual}"
          f" bbox={result.get('bbox')}")
    return expected is None or actual == expected


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("fetch", "run"),
                        help="fetch = download local photos; run = consented live model queries")
    parser.add_argument("--ids", default=",".join(SAMPLES),
                        help="comma-separated IDs from " + ", ".join(SAMPLES))
    parser.add_argument("--send-images", action="store_true",
                        help="explicitly authorize uploading chosen photos to your model provider")
    parser.add_argument("--timeout", type=int, default=60,
                        help="per-photo model request timeout in seconds (1..60)")
    args = parser.parse_args()
    try:
        ids = selected_ids(args.ids)
        if args.action == "run":
            if not args.send_images:
                parser.error("run requires --send-images; photos are sent to an external provider")
            if not os.getenv("OPENROUTER_API_KEY"):
                parser.error("OPENROUTER_API_KEY is missing")
            if not os.getenv("OPENROUTER_MODEL"):
                parser.error("OPENROUTER_MODEL is missing")
            if not (1 <= args.timeout <= 60):
                parser.error("--timeout must be in 1..60 seconds")
        success = True
        for sample_id in ids:
            if args.action == "fetch":
                fetch_one(sample_id)
            else:
                # One call per image, no retries. Errors are not classified as absent.
                success = run_one(sample_id, args.timeout) and success
        return 0 if success else 1
    except (ValueError, OSError, TimeoutError) as exc:
        print(f"vision samples: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
