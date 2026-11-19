"""Real two-process tacc/tacvm Analyzer -> Map IPC acceptance test (#138).

The Analyzer executes its own geographic calculation, serializes the resulting
Observation fields, and calls the production Runtime IPC transport. No prebuilt
Observation payload or host-language sender is used.
"""
import json
import math
import os
from pathlib import Path
import socket
import stat
import subprocess
import sys
import tempfile
import time

HERE = Path(__file__).resolve().parent
SOCKET = Path("/tmp/tac-os-analyzer-map.sock")
EXPECTED_FIELDS = {
    "type", "targetId", "observerId", "latitude", "longitude", "confidence", "timestamp"
}


def compile_tc(tacc, source, output):
    completed = subprocess.run(
        [tacc, str(source), str(output)], text=True, capture_output=True, timeout=60
    )
    assert completed.returncode == 0, (
        f"tacc failed for {source}:\n{completed.stderr}"
    )


def wait_for_listener(process):
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        if process.poll() is not None:
            out, err = process.communicate(timeout=2)
            raise AssertionError(f"Map quit before listening: {out}\n{err}")
        try:
            if stat.S_ISSOCK(SOCKET.stat().st_mode):
                # bind() precedes listen(); allow that transition to finish.
                time.sleep(0.05)
                return
            raise AssertionError("Map IPC path is not a socket")
        except FileNotFoundError:
            time.sleep(0.02)
    raise AssertionError("Map did not create its IPC socket")


def run_program(tacvm, bytecode, timeout=8):
    return subprocess.run(
        [tacvm, str(bytecode)], text=True, capture_output=True, timeout=timeout
    )


def validate_messages(stdout):
    messages = [json.loads(line) for line in stdout.splitlines()]
    assert len(messages) == 4, f"expected 4 actual IPC messages, got {messages}"
    assert [m["targetId"] for m in messages] == ["17", "17", "23", "23"]
    assert [m["observerId"] for m in messages] == [
        "device-1", "device-2", "device-2", "device-3"
    ]
    assert [m["timestamp"] for m in messages] == [
        1800000000100, 9007199254740993, 9007199254740993, 1800000000101
    ]
    assert all(type(m["timestamp"]) is int for m in messages)
    expected = [
        (59.9505, 30.311179595644, 0.642857142857),
        (59.950528750901, 30.311002506833, 0.797872340426),
        (59.950548640648, 30.310967687161, 0.732899022801),
        (59.950548640648, 30.310967687161, 0.732899022801),
    ]
    for observation, (lat, lon, confidence) in zip(messages, expected):
        assert set(observation) == EXPECTED_FIELDS, observation
        assert observation["type"] == "OBSERVATION"
        assert type(observation["latitude"]) is float, observation
        assert type(observation["longitude"]) is float, observation
        assert type(observation["confidence"]) is float, observation
        assert math.isclose(observation["latitude"], lat, abs_tol=1e-9)
        assert math.isclose(observation["longitude"], lon, abs_tol=1e-9)
        assert math.isclose(observation["confidence"], confidence, abs_tol=1e-12)
        assert 0.0 < observation["confidence"] < 1.0


def main():
    tacc, tacvm, source = sys.argv[1:]
    assert not os.path.lexists(SOCKET), (
        f"IPC path already exists: {SOCKET}; do not unlink a potentially live Map socket"
    )

    with tempfile.TemporaryDirectory() as directory:
        directory = Path(directory)
        analyzer_bc = directory / "analyzer.bc"
        receiver_bc = directory / "receiver.bc"
        compile_tc(tacc, Path(source), analyzer_bc)
        compile_tc(tacc, HERE / "receiver.tc", receiver_bc)

        # No listener: the same calculated messages are dropped via the
        # approved -1 status, and the Analyzer stays alive without hanging.
        absent = run_program(tacvm, analyzer_bc)
        assert absent.returncode == 0, absent.stderr
        expected_output = (HERE / "expected.txt").read_text(encoding="utf-8")
        assert absent.stdout == expected_output, absent.stdout
        assert absent.stdout.splitlines()[-1] == "analyzer-ipc-unavailable"
        assert "IPC Map unavailable" in absent.stderr, absent.stderr
        assert not os.path.lexists(SOCKET)

        # Real Map and Analyzer: two independent OS processes of the real VM.
        listener = subprocess.Popen(
            [tacvm, str(receiver_bc)],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True
        )
        try:
            wait_for_listener(listener)
            sender = run_program(tacvm, analyzer_bc)
            assert sender.returncode == 0, sender.stderr
            assert sender.stdout.splitlines()[-1] == "analyzer-ipc-sent", sender.stdout
            assert "analyzer-observation-ok" in sender.stdout
            out, err = listener.communicate(timeout=8)
            assert listener.returncode == 0, err
            validate_messages(out)
        finally:
            if listener.poll() is None:
                listener.kill()
                listener.communicate(timeout=3)

        assert not os.path.lexists(SOCKET), (
            "Map must remove its IPC socket file after normal shutdown"
        )
    print("analyzer_tc_ipc_e2e passed: 0/1/2, invalid skip, unavailable Map")


if __name__ == "__main__":
    main()
