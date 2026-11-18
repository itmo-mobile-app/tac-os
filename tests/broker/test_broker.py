import contextlib
import socket
import struct
import subprocess
import sys
import tempfile
import time
from pathlib import Path


OBSERVATION = (
    b'{"targetId":"17","observerId":"fixture","latitude":0.5,"longitude":0.25,'
    b'"confidence":0.8,"timestamp":1800000000000}'
)
TARGETS = [
    b'{"targetId":"17", "latitude":0.5,"longitude":0.25,"probability":0.8,"lastSeenAt":1800000000000}',
    b'{"targetId":"17", "latitude":0.75,"longitude":0.25,"probability":0.9,"lastSeenAt":1800000000500}',
]
FAULT_PROBE = """
fun main() {
    if (broker.connect("127.0.0.1", BROKER_PORT, 1000) != 0) { return; }
    if (broker.subscribe("probe") != 0) { return; }
    let status = 0;
    let polls = 0;
    while (status == 0) {
        if (polls == 15) { return; }
        status = broker.receive(100);
        polls = polls + 1;
    }
    if (status == -1) { runtime.log("disconnected"); }
    if (status == -2) { runtime.log("partial timeout"); }
    if (status == -3) { runtime.log("invalid frame"); }

    if (broker.connect("127.0.0.1", BROKER_PORT, 1000) != 0) { return; }
    if (broker.receive(1000) != 1) { return; }
    runtime.log(broker.payload());
    if (broker.close() != 0) { return; }
    if (broker.close() != 0) { return; }
    if (broker.receive(0) == -1) { runtime.log("closed"); }

    if (broker.connect("127.0.0.1", BROKER_PORT, 1000) != 0) { return; }
    if (broker.receive(50) != 0) { return; }
    if (broker.subscribe("idle") != 0) { return; }
    if (broker.receive(1000) == 1) { runtime.log(broker.payload()); }
}
"""


def compile_source(tacc, directory, name, source, port):
    tc = directory / f"{name}.tc"
    bc = directory / f"{name}.bc"
    tc.write_text(source.replace("BROKER_PORT", str(port)), encoding="utf-8")
    result = subprocess.run([tacc, tc, bc], capture_output=True, text=True, timeout=15)
    assert result.returncode == 0, result.stderr
    return bc


@contextlib.contextmanager
def process(args):
    child = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        yield child
    finally:
        if child.poll() is None:
            child.kill()
        child.communicate(timeout=3)


def collect(child):
    stdout, stderr = child.communicate(timeout=5)
    assert child.returncode == 0, stderr
    return stdout.splitlines()


@contextlib.contextmanager
def bound_socket():
    with socket.socket() as listener:
        listener.settimeout(5)
        listener.bind(("127.0.0.1", 0))
        yield listener


def wait_for_broker(port, broker):
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline and broker.poll() is None:
        try:
            return socket.create_connection(("127.0.0.1", port), timeout=0.2)
        except OSError:
            time.sleep(0.02)
    raise AssertionError("Go Broker did not start on the selected port")


def check_round_trip(tacc, tacvm, broker_binary, directory):
    with bound_socket() as reservation:
        port = reservation.getsockname()[1]
        source = Path(__file__).with_name("map.tc").read_text(encoding="utf-8")
        bytecode = compile_source(tacc, directory, "map", source, port)

    with process([broker_binary, str(port)]) as broker:
        with wait_for_broker(port, broker) as peer, peer.makefile("rb") as messages:
            peer.settimeout(4)
            # A same-connection round trip ensures this subscription is installed.
            peer.sendall(b'SUB observations\nPUB observations {"ready":true}\n')
            assert messages.readline() == b'MSG observations {"ready":true}\n'
            with process([tacvm, bytecode]) as map_vm:
                assert messages.readline() == b"MSG observations " + OBSERVATION + b"\n"
                peer.sendall(b"".join(b"PUB targets.updated " + value + b"\n" for value in TARGETS))
                assert collect(map_vm) == [
                    "targets.updated", TARGETS[0].decode(),
                    "targets.updated", TARGETS[1].decode(), "closed",
                ]


def check_unavailable_and_arguments(tacc, tacvm, directory):
    with bound_socket() as reservation:
        port = reservation.getsockname()[1]
        source = """
fun main() {
    if (broker.connect("127.0.0.1", BROKER_PORT, 200) == -1) {
        if (broker.receive(0) == -1) { runtime.log("unavailable"); }
    }
}
"""
        bytecode = compile_source(tacc, directory, "unavailable", source, port)
        with process([tacvm, bytecode]) as vm:
            assert collect(vm) == ["unavailable"]

        cases = [
            ('broker.publish("demo", "[]");', "broker.publish: invalid"),
            ('broker.subscribe("demo\\nnext");', "broker.subscribe: invalid"),
            ('broker.connect("localhost", BROKER_PORT, 100);', "numeric IP address"),
            ('broker.receive(1001);', "broker.receive: expected timeout"),
            ('runtime.log(broker.payload());', "broker.payload: no current message"),
        ]
        for index, (call, error) in enumerate(cases):
            bytecode = compile_source(
                tacc, directory, f"invalid-{index}", "fun main() { " + call + " }", port
            )
            result = subprocess.run([tacvm, bytecode], capture_output=True, text=True, timeout=4)
            assert result.returncode == 1 and error in result.stderr, result.stderr


def check_connection_faults(tacc, tacvm, directory):
    cases = [
        (None, "disconnected"),
        (b"PUB topic {}\n", "invalid frame"),
        (b"MSG topic []\n", "invalid frame"),
        (b'MSG topic {"field":"\xff"}\n', "invalid frame"),
        (b"MSG topic ".ljust(16384), "invalid frame"),
        (b"MSG topic {", "partial timeout"),
    ]
    with bound_socket() as listener:
        listener.listen()
        port = listener.getsockname()[1]
        bytecode = compile_source(tacc, directory, "fault-probe", FAULT_PROBE, port)
        for data, expected in cases:
            with process([tacvm, bytecode]) as vm:
                first, _ = listener.accept()
                with first, first.makefile("rb") as reader:
                    first.settimeout(3)
                    assert reader.readline() == b"SUB probe\n"
                    if data is None:
                        first.setsockopt(socket.SOL_SOCKET, socket.SO_LINGER, struct.pack("ii", 1, 0))
                    else:
                        first.sendall(data)
                        assert reader.read(1) == b"", "Failure left the first connection open"

                second, _ = listener.accept()
                with second:
                    second.settimeout(3)
                    second.sendall(b'MSG demo {"reconnected":true}\n')
                    assert second.recv(1) == b"", "Explicit close left the connection open"

                third, _ = listener.accept()
                with third, third.makefile("rb") as reader:
                    third.settimeout(3)
                    third.sendall(b"MSG demo {")
                    assert reader.readline() == b"SUB idle\n"
                    third.sendall(b"}\n")
                    assert collect(vm) == [expected, '{"reconnected":true}', "closed", "{}"]
                    assert reader.read(1) == b""


def check_send_deadline(tacc, tacvm, directory):
    source = r"""
fun main() {
    if (broker.connect("127.0.0.1", BROKER_PORT, 1000) != 0) { return; }
    let status = 0;
    let sent = 0;
    let payload = "{\"fixture\":\"PADDING\"}";
    while (status == 0) {
        if (sent == 1024) { return; }
        status = broker.publish("demo", payload);
        sent = sent + 1;
    }
    if (status == -2) {
        if (broker.publish("demo", "{}") == -1) { runtime.log("send timeout"); }
    }
}
"""
    with bound_socket() as listener:
        # Keep the peer's TCP receive window small, then leave it unread.
        listener.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 4096)
        listener.listen()
        port = listener.getsockname()[1]
        bytecode = compile_source(tacc, directory, "send-deadline", source.replace("PADDING", "x" * 16000), port)
        with process([tacvm, bytecode]) as vm:
            peer, _ = listener.accept()
            with peer:
                assert collect(vm) == ["send timeout"]


def main():
    tacc, tacvm, broker = sys.argv[1:]
    with tempfile.TemporaryDirectory() as directory:
        directory = Path(directory)
        check_round_trip(tacc, tacvm, broker, directory)
        check_unavailable_and_arguments(tacc, tacvm, directory)
        check_connection_faults(tacc, tacvm, directory)
        check_send_deadline(tacc, tacvm, directory)


if __name__ == "__main__":
    main()
