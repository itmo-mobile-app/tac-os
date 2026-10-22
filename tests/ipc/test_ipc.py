
import json
import os
import socket
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import time


SOCKET_PATH = Path("/tmp/tac-os-analyzer-map.sock")

EXPECTED = {
    "type": "OBSERVATION",
    "targetId": "17",
    "observerId": "device-1",
    "latitude": 59.9505,
    "longitude": 30.311,
    "confidence": 0.85,
    "timestamp": 1792255200000,
}


def compile_program(tacc, source, output):
    result = subprocess.run(
        [tacc, str(source), str(output)],
        capture_output=True,
        text=True,
        timeout=20,
    )
    assert result.returncode == 0, (
        f"Compilation failed: {source}\n{result.stderr}"
    )


def wait_for_map(process):
    deadline = time.monotonic() + 5

    while time.monotonic() < deadline:
        if process.poll() is not None:
            stdout, stderr = process.communicate()
            raise AssertionError(
                f"Map exited early:\n{stdout}\n{stderr}"
            )

        try:
            if stat.S_ISSOCK(SOCKET_PATH.stat().st_mode):
                # bind() creates the path before listen().
                time.sleep(0.05)
                return
            raise AssertionError("IPC path is not a socket")
        except FileNotFoundError:
            time.sleep(0.02)

    raise AssertionError("Map socket was not created")


def check_rejected_message(
    tacc, tacvm, map_bc, directory,
    name, raw_json, expected_error
):
    source = directory / f"{name}.tc"
    bytecode = directory / f"{name}.bc"

    # Создаём Analyzer с неправильным сообщением.
    source.write_text(
        'fun main() {\n'
        f'    runtime.ipc_send({json.dumps(raw_json)});\n'
        '}\n',
        encoding="utf-8",
    )

    compile_program(tacc, source, bytecode)

    assert not os.path.lexists(SOCKET_PATH), (
        f"{name}: socket already exists"
    )

    # Запускаем Map в отдельном процессе.
    map_process = subprocess.Popen(
        [tacvm, str(map_bc)],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )

    try:
        wait_for_map(map_process)

        # Передаём некорректное сообщение.
        analyzer = subprocess.run(
            [tacvm, str(bytecode)],
            capture_output=True,
            text=True,
            timeout=5,
        )

        assert analyzer.returncode == 0, (
            f"{name}: Analyzer failed:\n"
            f"{analyzer.stderr}"
        )

        stdout, stderr = map_process.communicate(
            timeout=8
        )

        # Map обязан отклонить сообщение.
        assert map_process.returncode == 1, (
            f"{name}: Map accepted invalid JSON\n"
            f"stdout: {stdout}\nstderr: {stderr}"
        )

        assert expected_error in stderr, (
            f"{name}: unexpected error:\n{stderr}"
        )

        # Некорректное сообщение не должно быть
        # возвращено VM и выведено runtime.log().
        assert stdout.strip() == (
            "Map: waiting for Analyzer"
        ), f"{name}: unexpected Map output: {stdout}"

    finally:
        if map_process.poll() is None:
            map_process.kill()
            map_process.communicate(timeout=3)

    assert not os.path.lexists(SOCKET_PATH), (
        f"{name}: socket was not cleaned"
    )

    print(f"PASS: {name}")

def check_persistent_listener(tacc, tacvm, directory):
    source = directory / "persistent-map.tc"
    bytecode = directory / "persistent-map.bc"
    source.write_text(
        'fun main() {\n'
        '    runtime.ipc_listen();\n'
        '    runtime.log(runtime.ipc_receive());\n'
        '    runtime.log(runtime.ipc_receive());\n'
        '}\n',
        encoding="utf-8",
    )
    compile_program(tacc, source, bytecode)
    process = subprocess.Popen(
        [tacvm, str(bytecode)], stdout=subprocess.PIPE,
        stderr=subprocess.PIPE, text=True,
    )
    try:
        wait_for_map(process)
        # The duplicate must fail without killing or unlinking the first Map.
        duplicate = subprocess.run(
            [tacvm, str(bytecode)], capture_output=True, text=True, timeout=5,
        )
        assert duplicate.returncode == 1, duplicate.stderr
        assert "IPC socket is already in use" in duplicate.stderr
        assert SOCKET_PATH.exists(), "Duplicate Map removed the live socket"

        message = (json.dumps(EXPECTED) + "\n").encode("utf-8")
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as client:
            client.settimeout(3)
            client.connect(str(SOCKET_PATH))
            client.sendall(message)
            # Keep the same connection open beyond the old five-second timeout.
            time.sleep(6)
            assert process.poll() is None, (
                "Map exited after an empty probe or an idle interval"
            )
            client.sendall(message)
        stdout, stderr = process.communicate(timeout=5)
        assert process.returncode == 0, stderr
        assert [json.loads(line) for line in stdout.splitlines()] == [EXPECTED, EXPECTED]
    finally:
        if process.poll() is None:
            process.kill()
            process.communicate(timeout=3)
    assert not os.path.lexists(SOCKET_PATH), "Listener socket was not cleaned"
    print("PASS: duplicate listener and idle persistent connection")


def check_send_status(tacc, tacvm, directory):
    source = directory / "send-status.tc"
    bytecode = directory / "send-status.bc"
    source.write_text(
        'fun main() {\n'
        f'    let status = runtime.ipc_send({json.dumps(json.dumps(EXPECTED))});\n'
        '    if (status == -1) { runtime.log("send failed; continuing"); }\n'
        '    else { runtime.log("unexpected success"); }\n'
        '    runtime.log("Analyzer still running");\n'
        '}\n',
        encoding="utf-8",
    )
    compile_program(tacc, source, bytecode)
    result = subprocess.run(
        [tacvm, str(bytecode)], capture_output=True, text=True, timeout=4,
    )
    assert result.returncode == 0, result.stderr
    assert result.stdout.splitlines() == [
        "send failed; continuing", "Analyzer still running",
    ]
    assert "IPC Map unavailable" in result.stderr, result.stderr
    print("PASS: unavailable Map returns a status and Analyzer continues")


def main():
    tacc, tacvm = sys.argv[1:3]
    source_dir = Path(__file__).resolve().parent

    assert not os.path.lexists(SOCKET_PATH), (
        "IPC socket already exists. Stop any running Map "
        "and remove only its stale socket before testing."
    )

    with tempfile.TemporaryDirectory() as directory:
        directory = Path(directory)
        map_bc = directory / "map.bc"
        analyzer_bc = directory / "analyzer.bc"

        compile_program(
            tacc, source_dir / "map.tc", map_bc
        )
        compile_program(
            tacc, source_dir / "analyzer.tc", analyzer_bc
        )

        # Start Map in its own OS process.
        map_process = subprocess.Popen(
            [tacvm, str(map_bc)],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )

        try:
            wait_for_map(map_process)

            # Start Analyzer in a second OS process.
            analyzer = subprocess.run(
                [tacvm, str(analyzer_bc)],
                capture_output=True,
                text=True,
                timeout=8,
            )

            assert analyzer.returncode == 0, (
                f"Analyzer failed:\n{analyzer.stderr}"
            )
            assert "Analyzer: message sent" in analyzer.stdout

            map_output, map_error = (
                map_process.communicate(timeout=8)
            )

            assert map_process.returncode == 0, (
                f"Map failed:\n{map_error}"
            )

            lines = map_output.strip().splitlines()
            assert len(lines) == 2, (
                f"Unexpected Map output: {map_output!r}"
            )
            assert lines[0] == "Map: waiting for Analyzer"

            observation = json.loads(lines[1])

            assert isinstance(observation, dict)
            assert set(observation) == set(EXPECTED)
            assert observation == EXPECTED
            assert type(observation["timestamp"]) is int
            assert type(observation["confidence"]) is float

        finally:
            if map_process.poll() is None:
                map_process.kill()
                map_process.communicate(timeout=3)

        # Normal Map shutdown must remove the socket.
        assert not os.path.lexists(SOCKET_PATH), (
            "Map did not clean up its IPC socket"
        )

        # No Map: report the failed send without terminating Analyzer.
        unavailable = subprocess.run(
            [tacvm, str(analyzer_bc)],
            capture_output=True,
            text=True,
            timeout=4,
        )

        assert unavailable.returncode == 0, (
            "Analyzer terminated instead of returning a send status"
        )
        assert "IPC Map unavailable" in unavailable.stderr, (
            f"Unexpected error: {unavailable.stderr}"
        )

        check_send_status(tacc, tacvm, directory)
        check_persistent_listener(tacc, tacvm, directory)

        
        invalid_cases = [
            (
                "wrong_type",
                json.dumps({
                    **EXPECTED,
                    "type": "TARGET_DETECTED"
                }),
                "IPC invalid message type",
            ),
            (
                "confidence_too_high",
                json.dumps({
                    **EXPECTED,
                    "confidence": 1.2
                }),
                "IPC invalid confidence",
            ),
            (
                "confidence_string",
                json.dumps({
                    **EXPECTED,
                    "confidence": "0.85"
                }),
                "IPC invalid confidence",
            ),
            (
                "latitude_out_of_range",
                json.dumps({
                    **EXPECTED,
                    "latitude": 91
                }),
                "IPC invalid latitude",
            ),
            (
                "longitude_out_of_range",
                json.dumps({
                    **EXPECTED,
                    "longitude": -181
                }),
                "IPC invalid longitude",
            ),
            (
                "empty_target_id",
                json.dumps({
                    **EXPECTED,
                    "targetId": ""
                }),
                "IPC invalid targetId",
            ),
            (
                "missing_observer",
                json.dumps({
                    k: v for k, v in EXPECTED.items()
                    if k != "observerId"
                }),
                "IPC invalid Observation fields",
            ),
            (
                "extra_distance",
                json.dumps({
                    **EXPECTED,
                    "distance": 3.2
                }),
                "IPC invalid Observation fields",
            ),
            (
                "invalid_json",
                '{"type":',
                "IPC invalid JSON",
            ),
        ]

        for name, raw_json, error in invalid_cases:
            check_rejected_message(
                tacc,
                tacvm,
                map_bc,
                directory,
                name,
                raw_json,
                error,
            )


    print("IPC integration test passed")


if __name__ == "__main__":
    main()

