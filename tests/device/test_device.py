import subprocess
import sys
import tempfile
import time
from pathlib import Path


NOMINAL = ("59.9505", "30.311", "90.0", "1800000000100")
STATUS_SOURCE = """
fun statusName(status: Int): String {
    if (status == 0) { return "ok"; }
    if (status == -1) { return "unavailable"; }
    if (status == -2) { return "timeout"; }
    if (status == -3) { return "invalid"; }
    return "unknown";
}
fun main() {
    runtime.log(statusName(location.read(100)));
    runtime.log(statusName(orientation.read(100)));
}
"""


def compile_source(tacc, directory, name, source):
    path = directory / f"{name}.tc"
    path.write_text(source, encoding="utf-8")
    program = path.with_suffix(".bc")
    result = subprocess.run([tacc, path, program], capture_output=True, text=True, timeout=15)
    assert result.returncode == 0, result.stderr
    return program


def run(args, expected, error=None):
    result = subprocess.run(args, capture_output=True, text=True, timeout=4)
    assert result.returncode == (1 if error else 0), result
    assert result.stdout.splitlines() == expected, result
    if error:
        assert error in result.stderr, result


def check_samples(tacc, fixture_vm, directory):
    source = Path(__file__).with_name("device.tc").read_text(encoding="utf-8")
    samples = [
        (NOMINAL, "1800000000100"),
        (("-90.0", "180.0", "0.0", "-9223372036854775808"), "(-9223372036854775807 - 1)"),
        (("90.0", "-180.0", "359.5", "9223372036854775807"), "9223372036854775807"),
    ]
    for index, (sample, time_literal) in enumerate(samples):
        tc = source
        for variable, original, value in zip(
                ("latitude", "longitude", "heading", "now"), NOMINAL, (*sample[:3], time_literal)):
            tc = tc.replace(f"({variable} != {original})", f"({variable} != {value})")
        program = compile_source(tacc, directory, f"device-{index}", tc)
        assert program.read_bytes()[4:6] == b"\x02\x00"
        run([fixture_vm, program, *sample, "0"], ["device sample ok"])


def check_statuses(tacc, tacvm, fixture_vm, directory):
    program = compile_source(tacc, directory, "statuses", STATUS_SOURCE)
    assert program.read_bytes()[4:6] == b"\x01\x00"
    run([tacvm, program], ["unavailable", "unavailable"])
    cases = [
        (("-", "-", "90.0"), ["unavailable", "ok"]),
        (("59.0", "30.0", "-"), ["ok", "unavailable"]),
        (("-90.01", "30.0", "360.0"), ["invalid", "invalid"]),
        (("59.0", "180.01", "-0.01"), ["invalid", "invalid"]),
        (("nan", "30.0", "inf"), ["invalid", "invalid"]),
        (("59.0", "30.0", "90.0"), ["timeout", "timeout"]),
    ]
    for shape, expected in cases:
        delay = "10000" if expected[0] == "timeout" else "0"
        run([fixture_vm, program, *shape, NOMINAL[3], delay], expected)
    run([fixture_vm, program, *NOMINAL, "-1"], [], "device source delay must be nonnegative")


def check_snapshots(tacc, fixture_vm, directory):
    location_failure = """
fun main() {
    if (location.read(1000) != 0) { return; }
    if (orientation.read(1000) != 0) { return; }
    if (location.read(5) != -2) { return; }
    if (orientation.heading() == 90.0) { runtime.log("heading retained"); }
    location.longitude();
}
"""
    program = compile_source(tacc, directory, "stale-position", location_failure)
    run([fixture_vm, program, *NOMINAL, "40"], ["heading retained"], "no current position")

    orientation_failure = """
fun main() {
    if (orientation.read(1000) != 0) { return; }
    if (orientation.read(5) != -2) { return; }
    orientation.heading();
}
"""
    program = compile_source(tacc, directory, "stale-heading", orientation_failure)
    run([fixture_vm, program, *NOMINAL, "40"], [], "no current heading")

    program = compile_source(
        tacc, directory, "before-read", "fun main() { location.latitude(); }"
    )
    run([fixture_vm, program, *NOMINAL, "0"], [], "no current position")


def check_timeout_range_and_v1(tacc, tacvm, fixture_vm, directory):
    for index, call in enumerate(("location.read(0)", "orientation.read(1001)")):
        program = compile_source(tacc, directory, f"bad-timeout-{index}",
                                 "fun main() { " + call + "; }")
        run([tacvm, program], [], "expected timeout 1..1000 ms")

    source = """
fun main() {
    if (location.read(100) == 0) {
        location.latitude();
        runtime.log("received");
    }
}
"""
    program = compile_source(tacc, directory, "discard-float", source)
    original = program.read_bytes()
    assert original[4:6] == b"\x02\x00"
    run([fixture_vm, program, *NOMINAL, "0"], ["received"])
    program.write_bytes(original[:4] + b"\x01\x00" + original[6:])
    run([fixture_vm, program, *NOMINAL, "0"], [], "Float Runtime result requires bytecode version 2")


def check_system_time(tacc, tacvm, directory):
    # The default CLI uses host wall time. Allow for compiler startup and clock precision.
    lower = time.time_ns() // 1_000_000 - 1000
    upper = lower + 30_000
    source = f"""
fun main() {{
    let now = time.nowMs();
    if (now < {lower}) {{ return; }}
    if (now > {upper}) {{ return; }}
    runtime.log("system time ok");
}}
"""
    program = compile_source(tacc, directory, "system-time", source)
    run([tacvm, program], ["system time ok"])


def main():
    tacc, tacvm, fixture_vm = sys.argv[1:]
    with tempfile.TemporaryDirectory() as directory:
        directory = Path(directory)
        check_samples(tacc, fixture_vm, directory)
        check_statuses(tacc, tacvm, fixture_vm, directory)
        check_snapshots(tacc, fixture_vm, directory)
        check_timeout_range_and_v1(tacc, tacvm, fixture_vm, directory)
        check_system_time(tacc, tacvm, directory)


if __name__ == "__main__":
    main()
