#!/usr/bin/env python3
"""Run opt-in OpenRouter Vision checks against JPEG/WebP photos committed to Git.

No photo downloads or network requests happen except deliberate
OpenRouter calls triggered by run --send-images.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

HERE = Path(__file__).resolve().parent
PHOTOS = HERE / "photos"
MANIFEST = HERE / "expected.json"
MAX_BYTES = 2 << 20


def load_manifest():
    data = json.loads(MANIFEST.read_text(encoding="utf-8"))
    if not isinstance(data, dict) or not data:
        raise ValueError("expected.json must contain image cases")
    for sample_id, entry in data.items():
        if not isinstance(entry, dict) or entry.get("expect") not in (
            "present", "absent", None
        ):
            raise ValueError(f"{sample_id}: invalid expected status")
        if entry.get("file") not in (sample_id + ".jpg", sample_id + ".webp"):
            raise ValueError(f"{sample_id}: invalid image filename")
        if not isinstance(entry.get("git_blob_sha1"), str) or len(entry["git_blob_sha1"]) != 40:
            raise ValueError(f"{sample_id}: missing Git blob SHA-1")
    return data


def selected_ids(value, cases):
    names = [name.strip().upper() for name in value.split(",")]
    if not names or not all(name in cases for name in names):
        raise ValueError("invalid sample ID; available: " + ", ".join(cases))
    if len(names) != len(set(names)):
        raise ValueError("duplicate image ID")
    return names


def image_path(sample_id, cases=None):
    cases = cases if cases is not None else load_manifest()
    return PHOTOS / cases[sample_id]["file"]


def validate_image(path, expected_sha1):
    # Verify the exact committed image used to define the test expectation.
    if not path.is_file():
        raise ValueError(f"missing committed image: {path}")
    data = path.read_bytes()
    if not 1 <= len(data) <= MAX_BYTES:
        raise ValueError(f"{path.name}: image size exceeds 2 MiB or is empty")
    if path.suffix.lower() == ".jpg":
        valid_format = data.startswith(b"\xff\xd8\xff") and data.endswith(b"\xff\xd9")
    elif path.suffix.lower() == ".webp":
        valid_format = data.startswith(b"RIFF") and data[8:12] == b"WEBP"
    else:
        valid_format = False
    if not valid_format:
        raise ValueError(f"{path.name}: invalid JPEG/WebP contents")
    git_blob = b"blob " + str(len(data)).encode("ascii") + b"\x00" + data
    if hashlib.sha1(git_blob).hexdigest() != expected_sha1:
        raise ValueError(f"{path.name}: Git blob hash differs from expected.json")


def run_one(sample_id, cases, timeout):
    path = image_path(sample_id)
    validate_image(path, cases[sample_id]["git_blob_sha1"])
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
        print(f"{sample_id}: ERROR: {finished.stderr.strip()}", file=sys.stderr)
        return False
    try:
        result = json.loads(finished.stdout)
    except json.JSONDecodeError:
        print(f"{sample_id}: ERROR: malformed CLI result", file=sys.stderr)
        return False
    actual = result.get("status")
    if actual not in ("present", "absent", "uncertain"):
        print(f"{sample_id}: ERROR: unexpected status", file=sys.stderr)
        return False
    expected = cases[sample_id]["expect"]
    # A valid normalized bbox is NOT proof of correct pixel localization.
    label = "REVIEW" if expected is None else (
        "PASS" if actual == expected else "FAIL"
    )
    print(f"{sample_id}: {label} expected={expected or 'informational'}"
          f" actual={actual} bbox={result.get('bbox')}")
    return expected is None or actual == expected


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("list", "check", "run"),
                        help="list/check are offline; run invokes the model")
    parser.add_argument("--ids", default="",
                        help="comma-separated fixture IDs (default: all)")
    parser.add_argument("--send-images", action="store_true",
                        help="explicit permission to upload selected photos")
    parser.add_argument("--timeout", type=int, default=60,
                        help="per-photo model timeout 1..60 seconds")
    args = parser.parse_args()
    try:
        cases = load_manifest()
        ids = selected_ids(args.ids or ",".join(cases), cases)
        if args.action == "run":
            if not args.send_images:
                parser.error("run requires --send-images")
            if not os.getenv("OPENROUTER_API_KEY") or not os.getenv("OPENROUTER_MODEL"):
                parser.error("set OPENROUTER_API_KEY and OPENROUTER_MODEL")
            if not 1 <= args.timeout <= 60:
                parser.error("--timeout must be 1..60 seconds")
        success = True
        for sample_id in ids:
            entry = cases[sample_id]
            if args.action == "list":
                print(f"{sample_id}: {entry['expect'] or 'informational'} "
                      f"{image_path(sample_id)}")
            elif args.action == "check":
                validate_image(image_path(sample_id), entry["git_blob_sha1"])
                print(f"{sample_id}: committed photo checksum OK")
            else:
                result = run_one(sample_id, cases, args.timeout)
                success = result and success
        return 0 if success else 1
    except (ValueError, OSError, json.JSONDecodeError) as exc:
        print(f"vision samples: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
