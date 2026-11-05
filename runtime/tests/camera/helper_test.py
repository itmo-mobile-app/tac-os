"""Exercise the real Go detector IPC with its existing synthetic sources."""

import contextlib
import json
import os
from pathlib import Path
import socket
import stat
import subprocess
import sys
import tempfile
import time
import unittest


HELPER = str(Path(sys.argv.pop(1)).resolve())


class CameraHelperTest(unittest.TestCase):
    @contextlib.contextmanager
    def connected_helper(self, mode):
        with tempfile.TemporaryDirectory(prefix="tacos-camera-") as directory:
            path = Path(directory) / "detector.sock"
            process = subprocess.Popen(
                [HELPER, "-socket", str(path), "-fake", mode],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            connection = None
            try:
                deadline = time.monotonic() + 5
                while connection is None:
                    self.assertIsNone(process.poll(), "helper exited during startup")
                    candidate = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                    candidate.settimeout(5)
                    try:
                        candidate.connect(str(path))
                    except (FileNotFoundError, ConnectionRefusedError):
                        candidate.close()
                        if time.monotonic() >= deadline:
                            self.fail("helper startup timed out")
                        time.sleep(0.01)
                    except BaseException:
                        candidate.close()
                        raise
                    else:
                        connection = candidate
                self.assertEqual(stat.S_IMODE(os.stat(path).st_mode), 0o600)
                yield process, connection
            finally:
                if connection is not None:
                    connection.close()
                if process.poll() is None:
                    process.terminate()
                try:
                    stdout, stderr = process.communicate(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.communicate()
                    self.fail("helper did not stop after SIGTERM; killed and reaped")
                self.assertEqual(process.returncode, 0, stderr.decode(errors="replace"))
                self.assertEqual(stdout, b"", "helper must not write to stdout")
                self.assertFalse(path.exists(), "helper left its socket after shutdown")

    def check_mode(self, mode, expected):
        with self.connected_helper(mode) as (process, connection):
            with connection.makefile("rb") as responses:
                for _ in range(2):
                    connection.sendall(b'{"command":"DETECT"}\n')
                    line = responses.readline(64 * 1024 + 1)
                    self.assertTrue(line.endswith(b"\n"), "incomplete JSONL response")
                    self.assertLessEqual(len(line), 64 * 1024)
                    self.assertEqual(json.loads(line), expected)
                    self.assertIsNone(process.poll(), "helper exited between requests")

    def test_two_requests_on_one_connection(self):
        self.check_mode("two", {"ok": True, "detections": [
            {"id": 17, "relativeAngle": 12.5, "distance": 3.2},
            {"id": 23, "relativeAngle": -8.4, "distance": 5.7},
        ]})

    def test_empty(self):
        self.check_mode("empty", {"ok": True, "detections": []})

    def test_camera_unavailable(self):
        self.check_mode("camera-unavailable", {"ok": False, "error": "CAMERA_UNAVAILABLE"})

    def test_detection_failed(self):
        self.check_mode("detection-failed", {"ok": False, "error": "DETECTION_FAILED"})


if __name__ == "__main__":
    unittest.main(verbosity=2)
