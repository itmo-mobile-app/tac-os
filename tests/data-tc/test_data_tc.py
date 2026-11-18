"""tacc -> tacvm E2E for the Observation/Target String/JSON helpers (#132, #133, #144).

Usage: test_data_tc.py <tacc> <tacvm>
"""

import json
import math
import subprocess
import sys
import tempfile
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

HERE = Path(__file__).resolve().parent
OBSERVATION_FIELDS = {"targetId", "observerId", "latitude", "longitude", "confidence", "timestamp"}
TARGET_PAYLOAD = (
    '{"targetId":"17","latitude":59.9505,"longitude":30.311,'
    '"probability":0.85,"lastSeenAt":1800000000100}'
)


def compile_tc(tacc, source, output):
    return subprocess.run([tacc, str(source), str(output)], capture_output=True, text=True, timeout=60)


def run_bc(tacvm, bytecode):
    return subprocess.run([tacvm, str(bytecode)], capture_output=True, text=True, timeout=30)


def build_and_run(tacc, tacvm, source, workdir):
    bytecode = Path(workdir) / (Path(source).stem + ".bc")
    compiled = compile_tc(tacc, source, bytecode)
    assert compiled.returncode == 0, f"tacc failed on {source}:\n{compiled.stderr}"
    result = run_bc(tacvm, bytecode)
    assert result.returncode == 0, f"tacvm failed on {source}:\n{result.stderr}"
    return bytecode, result.stdout.splitlines()


def expected_observation(target_id, observer_id, distance, timestamp):
    return {
        "targetId": target_id,
        "observerId": observer_id,
        "latitude": distance / 6371000.0 * 180.0 / 3.141592653589793,
        "longitude": 0.0,
        "confidence": 0.9 / (1.0 + distance / 25.0),
        "timestamp": timestamp,
    }


def check_observations(lines):
    assert lines[0] == "zero count=0", lines
    assert lines[1] == "one count=1", lines
    assert lines[3] == "two count=2", lines
    observer = 'obs "q" \\ é'
    cases = [
        (lines[2], expected_observation("17", "device-1", 10.0, 1800000000100)),
        (lines[4], expected_observation("3", observer, 25.0, 9007199254740993)),
        (lines[5], expected_observation("71", observer, 100.0, 9007199254740993)),
    ]
    for line, expected in cases:
        message = json.loads(line)
        assert set(message) == OBSERVATION_FIELDS | {"type"}, message
        assert message.pop("type") == "OBSERVATION"
        # json.loads keeps integers exact and parses floats to the nearest binary64.
        assert message == expected, (message, expected)
        assert isinstance(message["timestamp"], int)

    assert abs(json.loads(lines[2])["latitude"] - 0.00008993216059187306) < 1e-18
    assert abs(json.loads(lines[2])["confidence"] - 0.6428571428571429) < 1e-15

    broker = json.loads(lines[6])
    assert set(broker) == OBSERVATION_FIELDS, broker
    assert broker["latitude"] == 59.95050012345678, "no fixed-precision truncation"
    assert len(lines) == 7, lines


def check_targets(lines):
    assert len(lines) == 3, lines
    target_id, x, y, probability, last_seen = lines[0].split(" ")
    assert target_id == "17"
    assert float(x) == (30.311 - 30.308) / (30.314 - 30.308) * 960.0
    assert float(y) == (59.951 - 59.9505) / (59.951 - 59.949) * 640.0
    assert math.isclose(float(x), 480.0, abs_tol=1e-6) and math.isclose(float(y), 160.0, abs_tol=1e-6)
    assert probability == "0.85" and last_seen == "1800000000100"
    assert lines[1] == "A-1 0.0 0.0 1.0 9007199254740993", lines[1]
    assert lines[2] == "far outside", lines[2]


def tc_string(text):
    return '"' + text.replace("\\", "\\\\").replace('"', '\\"') + '"'


# Malformed inputs: a RuntimeError ends tacvm with a nonzero status, the message names the
# helper, and nothing is logged after the failing call (no fabricated defaults).
RUNTIME_ERRORS = [
    ("json.getInt", '{"id":17.0}', 'json.getInt(p, "id")', "not an integer"),
    ("json.getInt", '{"targetId":"1"}', 'json.getInt(p, "id")', "missing key 'id'"),
    ("json.getFloat", '{"latitude":"59"}', 'json.getFloat(p, "latitude")', "not a number"),
    ("json.getString", '{"targetId":"1","targetId":"2"}', 'json.getString(p, "targetId")', "duplicate key"),
    ("json.getString", '{"targetId":"1"} trailing', 'json.getString(p, "targetId")', "malformed JSON"),
    ("json.getString", '{"targetId":null}', 'json.getString(p, "targetId")', "field 'targetId'"),
    ("json.arrayObject", '[{"id":1}]', "json.arrayObject(p, 1)", "index 1"),
    ("json.arrayLength", '{"id":1}', "json.arrayLength(p)", "not a JSON array"),
]


def check_runtime_errors(tacc, tacvm, workdir):
    for index, (helper, payload, call, detail) in enumerate(RUNTIME_ERRORS):
        source = Path(workdir) / f"bad_{index}.tc"
        source.write_text(
            "fun main() {\n"
            f"    const p = {tc_string(payload)};\n"
            '    runtime.log("before");\n'
            f"    const value = {call};\n"
            '    runtime.log("after");\n'
            "}\n",
            encoding="utf-8",
        )
        bytecode = source.with_suffix(".bc")
        compiled = compile_tc(tacc, source, bytecode)
        assert compiled.returncode == 0, compiled.stderr
        result = run_bc(tacvm, bytecode)
        assert result.returncode != 0, f"{helper} accepted {payload}"
        assert helper in result.stderr and detail in result.stderr, result.stderr
        assert result.stdout == "before\n", result.stdout


COMPILE_ERRORS = [
    ('let s = text.concat("a");', "expects 2 argument(s)"),
    ('let s = text.fromFloat(1);', "must be Float, found Int"),
    ('let x = json.getFloat("{}", "k") + 1;', "found Float and Int"),
    ('let s = "a" + "b";', "found String and String"),
    ('let s = json.parse("{}");', "unknown Runtime function 'json.parse'"),
]


def check_compile_errors(tacc, workdir):
    for index, (statement, detail) in enumerate(COMPILE_ERRORS):
        source = Path(workdir) / f"type_{index}.tc"
        source.write_text(f"fun main() {{\n    {statement}\n}}\n", encoding="utf-8")
        result = compile_tc(tacc, source, source.with_suffix(".bc"))
        assert result.returncode != 0, f"tacc accepted: {statement}"
        assert detail in result.stderr, result.stderr


def check_independent_processes(tacvm, bytecode, expected):
    with ThreadPoolExecutor(max_workers=2) as pool:
        results = list(pool.map(lambda _: run_bc(tacvm, bytecode), range(2)))
    for result in results:
        assert result.returncode == 0, result.stderr
        assert result.stdout.splitlines() == expected


def main():
    tacc, tacvm = sys.argv[1], sys.argv[2]
    with tempfile.TemporaryDirectory() as workdir:
        observation_bc, observations = build_and_run(tacc, tacvm, HERE / "observation.tc", workdir)
        check_observations(observations)
        _, targets = build_and_run(tacc, tacvm, HERE / "target.tc", workdir)
        check_targets(targets)
        check_runtime_errors(tacc, tacvm, workdir)
        check_compile_errors(tacc, workdir)
        check_independent_processes(tacvm, observation_bc, observations)
    print("data_tc_e2e passed")


if __name__ == "__main__":
    main()
