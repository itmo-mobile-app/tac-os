"""Black-box tests of the registered C++ Camera API and owned helper lifecycle."""

import json
import os
from pathlib import Path
import signal
import socket
import stat
import subprocess
import sys
import tempfile
import time
import unittest


CLIENT = str(Path(sys.argv.pop(1)).resolve())
GO_HELPER = str(Path(sys.argv.pop(1)).resolve())
TWO = [{"id": 17, "relativeAngle": 12.5, "distance": 3.2},
       {"id": 23, "relativeAngle": -8.4, "distance": 5.7}]


def process_start(path):
    status = path.read_text()
    return status[status.rfind(")") + 2:].split()[19]


class RuntimeCameraTest(unittest.TestCase):
    def run_client(self, scenario="reply", mode="two", count=2, response=None,
                   unwind=False, before_start=None, foreign_path=False, allow_socket=False,
                   helper_override=None):
        with tempfile.TemporaryDirectory(prefix="tacos-runtime-camera-") as directory:
            helper = Path(directory) / "helper with spaces"
            source = Path(__file__).with_name("fault_helper.py").read_text()
            # The temporary executable needs no Git executable bit or OpenCV.
            helper.write_text("#!" + sys.executable + "\n" + source.split("\n", 1)[1])
            helper.chmod(0o700)
            trace = Path(directory) / "trace.jsonl"
            env = dict(os.environ, TACOS_CAMERA_TEST_SCENARIO=scenario,
                       TACOS_CAMERA_TEST_TRACE=str(trace),
                       TACOS_CAMERA_TEST_GO_HELPER=GO_HELPER)
            if response is not None:
                response_file = Path(directory) / "response.bin"
                response_file.write_bytes(response)
                env["TACOS_CAMERA_TEST_RESPONSE_FILE"] = str(response_file)
            started = time.monotonic()
            process = subprocess.Popen(
                [CLIENT, helper_override or str(helper), mode, str(count),
                 "unwind" if unwind else "normal"], env=env,
                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                start_new_session=True,
            )
            socket_path = Path(f"/tmp/tac-os-detector-{process.pid}.sock")
            records = []
            leaked = []
            leftover = False
            try:
                if before_start is not None:
                    before_start(socket_path)
                try:
                    stdout, stderr = process.communicate(input=b"\n", timeout=20)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.communicate()
                    self.fail("Runtime hung; its process group was killed")
                if trace.exists():
                    records = [json.loads(line) for line in trace.read_text().splitlines()]
                for record in records:
                    pid = record["pid"]
                    identity = Path(f"/proc/{pid}/stat")
                    if identity.exists() and process_start(identity) == record["start"]:
                        leaked.append(pid)
                leftover = socket_path.exists()
                self.assertEqual(process.returncode, 0, stderr.decode(errors="replace"))
                self.assertFalse(leaked, f"helper processes still exist: {leaked}")
                if not (foreign_path or allow_socket):
                    self.assertFalse(leftover, "helper socket remained after Runtime cleanup")
                return stdout.decode().splitlines(), records, time.monotonic() - started
            finally:
                if process.poll() is None:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.communicate()
                # Test-only cleanup for an intentionally uncooperative child or failed assertion.
                if trace.exists():
                    for record in [json.loads(line) for line in trace.read_text().splitlines()]:
                        identity = Path(f"/proc/{record['pid']}/stat")
                        try:
                            if process_start(identity) == record["start"]:
                                os.kill(record["pid"], signal.SIGKILL)
                        except (FileNotFoundError, ProcessLookupError):
                            pass
                if not foreign_path:
                    try:
                        info = socket_path.lstat()
                        if stat.S_ISSOCK(info.st_mode) and info.st_uid == os.geteuid():
                            socket_path.unlink()
                    except FileNotFoundError:
                        pass

    def assert_errors(self, lines, count=2):
        self.assertEqual(len(lines), count)
        self.assertTrue(all(line.startswith("ERROR camera.detectMarkers:") for line in lines), lines)

    def test_lazy_construction(self):
        lines, records, _ = self.run_client(count=0)
        self.assertEqual(lines, [])
        self.assertEqual(records, [])

    def test_go_shared_fixtures_and_shutdown(self):
        for mode, expected in (("two", TWO), ("empty", []),
                               ("camera-unavailable", "CAMERA_UNAVAILABLE"),
                               ("detection-failed", "DETECTION_FAILED")):
            with self.subTest(mode=mode):
                lines, records, _ = self.run_client("go", mode)
                self.assertEqual(len(records), 1, "helper must be launched only once")
                if isinstance(expected, list):
                    self.assertEqual([json.loads(line.removeprefix("OK ")) for line in lines],
                                     [expected, expected])
                    self.assertTrue(all(line.startswith("OK [") for line in lines))
                else:
                    self.assert_errors(lines)
                    self.assertTrue(all(expected in line for line in lines))

    def test_one_process_one_connection_two_requests(self):
        lines, records, _ = self.run_client()
        self.assertEqual(lines, ["OK []", "OK []"])
        self.assertEqual([item["event"] for item in records],
                         ["start", "accept", "detect", "detect"])

    def test_wire_errors_keep_connection(self):
        for code in ("CAMERA_UNAVAILABLE", "DETECTION_FAILED", "BAD_REQUEST"):
            with self.subTest(code=code):
                response = (json.dumps({"ok": False, "error": code}) + "\n").encode()
                lines, records, _ = self.run_client(response=response)
                self.assert_errors(lines)
                self.assertTrue(all(code in line for line in lines))
                self.assertEqual([item["event"] for item in records].count("accept"), 1)
                self.assertEqual([item["event"] for item in records].count("detect"), 2)

    def test_malformed_response_is_terminal(self):
        invalid = [
            b'not JSON', b'[]', b'{"ok":1,"detections":[]}',
            b'{"ok":true}', b'{"ok":true,"detections":{},"extra":1}',
            b'{"ok":true,"ok":true,"detections":[]}',
            b'{"ok":false,"error":12}', b'{"ok":false,"error":"UNKNOWN"}',
            b'{"ok":true,"detections":[],"error":"CAMERA_UNAVAILABLE"}',
            b'{"ok":true,"detections":[]} trailing',
            b'{"ok":true,"detections":[]}\x00', b'\xff',
            b'{"ok":true,"detections":[' + b'[' * 20 + b']' * 20 + b']}',
        ]
        for detection in (
            {}, {"id": 1, "relativeAngle": 0},
            {"id": 1, "relativeAngle": 0, "distance": 1, "extra": 1},
            *({"id": value, "relativeAngle": 0, "distance": 1}
              for value in (-1, 72, 1.0, True, "1", 18446744073709551615)),
            *({"id": 1, "relativeAngle": value, "distance": 1}
              for value in (None, True, "0", float("inf"))),
            *({"id": 1, "relativeAngle": 0, "distance": value}
              for value in (0, -1, None, True, "1", float("nan"))),
        ):
            invalid.append(json.dumps({"ok": True, "detections": [detection]}).encode())
        invalid.extend([
            b'{"ok":true,"detections":[{"id":1,"id":2,"relativeAngle":0,"distance":1}]}',
            b'{"ok":true,"detections":[{"id":1,"relativeAngle":1e999,"distance":1}]}',
            json.dumps({"ok": True, "detections": [TWO[1], TWO[0]]}).encode(),
            json.dumps({"ok": True, "detections": [TWO[0], TWO[0]]}).encode(),
        ])
        for frame in invalid:
            with self.subTest(frame=frame):
                lines, records, _ = self.run_client(response=frame + b"\n")
                self.assert_errors(lines)
                self.assertIn("restart is disabled", lines[1])
                self.assertEqual([item["event"] for item in records].count("start"), 1)

    def test_fragmented_response(self):
        lines, _, _ = self.run_client("fragmented")
        self.assertEqual(lines, ["OK []", "OK []"])

    def test_frame_limit(self):
        frame = b'{"ok":true,"detections":[]}'
        exact = frame + b" " * (65535 - len(frame)) + b"\n"
        lines, _, _ = self.run_client(response=exact)
        self.assertEqual(lines, ["OK []", "OK []"])
        lines, _, _ = self.run_client(response=exact[:-1] + b" \n")
        self.assert_errors(lines)
        self.assertIn("64 KiB", lines[0])

    def test_incomplete_and_extra_frames(self):
        lines, _, _ = self.run_client("exit-request")
        self.assert_errors(lines)
        lines, _, _ = self.run_client("partial-eof")
        self.assert_errors(lines)
        lines, _, _ = self.run_client("close-after-response")
        self.assertEqual(lines[0], "OK []")
        self.assertTrue(lines[1].startswith("ERROR "))
        lines, _, _ = self.run_client(response=b'{"ok":true,"detections":[]}\n{}\n')
        self.assert_errors(lines)

    def test_missing_executable_and_invalid_configuration(self):
        lines, records, _ = self.run_client(helper_override="/no/such/marker-detect")
        self.assert_errors(lines)
        self.assertEqual(records, [])
        lines, records, _ = self.run_client(mode="not-a-mode")
        self.assert_errors(lines)
        self.assertEqual(records, [])

    def test_early_helper_exit(self):
        lines, records, _ = self.run_client("exit-start")
        self.assert_errors(lines)
        self.assertIn("status 17", lines[0])
        self.assertEqual(len(records), 1)

    def test_startup_and_response_deadlines(self):
        for scenario in ("no-ready", "no-response", "trickle"):
            with self.subTest(scenario=scenario):
                lines, records, elapsed = self.run_client(scenario)
                self.assert_errors(lines)
                self.assertIn("timeout", lines[0])
                self.assertGreaterEqual(elapsed, 4.8)
                self.assertLess(elapsed, 9)
                self.assertEqual([item["event"] for item in records].count("start"), 1)

    def test_shutdown_during_stack_unwind(self):
        lines, _, _ = self.run_client("go", unwind=True)
        self.assertEqual(len(lines), 1)
        self.assertEqual(json.loads(lines[0].removeprefix("OK ")), TWO)

    def test_uncooperative_helper_is_killed_and_reaped(self):
        lines, _, elapsed = self.run_client("stubborn", allow_socket=True)
        self.assertEqual(lines, ["OK []", "OK []"])
        self.assertLess(elapsed, 7)

    def test_foreign_regular_file_is_preserved(self):
        paths = []

        def occupy(path):
            path.write_text("do not delete")
            paths.append(path)

        try:
            lines, records, _ = self.run_client(before_start=occupy, foreign_path=True)
            self.assert_errors(lines)
            self.assertEqual(records, [])
            self.assertEqual(paths[0].read_text(), "do not delete")
        finally:
            for path in paths:
                path.unlink()

    def test_foreign_active_socket_is_preserved(self):
        listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        paths = []

        def occupy(path):
            listener.bind(str(path))
            listener.listen(4)
            paths.append((path, path.stat().st_ino))

        try:
            lines, _, _ = self.run_client("go", before_start=occupy, foreign_path=True)
            self.assert_errors(lines)
            self.assertEqual(paths[0][0].stat().st_ino, paths[0][1])
        finally:
            listener.close()
            for path, _ in paths:
                path.unlink()


if __name__ == "__main__":
    unittest.main(verbosity=2)
