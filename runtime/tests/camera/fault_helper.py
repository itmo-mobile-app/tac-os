#!/usr/bin/env python3
"""Controllable child process for Runtime transport and cleanup tests only."""

import argparse
import json
import os
from pathlib import Path
import signal
import socket
import sys
import time


parser = argparse.ArgumentParser()
parser.add_argument("-socket", required=True)
parser.add_argument("-fake", default="")
args = parser.parse_args()
path = Path(args.socket)
scenario = os.environ["TACOS_CAMERA_TEST_SCENARIO"]


def record(event):
    status = Path(f"/proc/{os.getpid()}/stat").read_text()
    start = status[status.rfind(")") + 2:].split()[19]
    with open(os.environ["TACOS_CAMERA_TEST_TRACE"], "a") as trace:
        trace.write(json.dumps({"event": event, "pid": os.getpid(),
                                "start": start, "path": str(path)}) + "\n")


def terminate(signum, frame):
    raise SystemExit(0)


signal.signal(signal.SIGTERM, terminate)
signal.signal(signal.SIGINT, terminate)
record("start")
if scenario == "go":
    helper = os.environ["TACOS_CAMERA_TEST_GO_HELPER"]
    os.execv(helper, [helper] + sys.argv[1:])

listener = None
connection = None
owned = False
try:
    if scenario == "exit-start":
        sys.exit(17)
    if scenario == "no-ready":
        time.sleep(60)
    if scenario == "stubborn":
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
    listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    listener.bind(str(path))
    owned = True
    path.chmod(0o600)
    listener.listen(4)
    while True:
        connection, _ = listener.accept()
        record("accept")
        with connection, connection.makefile("rb") as requests:
            for request in requests:
                record("detect")
                if request != b'{"command":"DETECT"}\n':
                    raise RuntimeError("incorrect Runtime DETECT request")
                if scenario == "exit-request":
                    sys.exit(9)
                if scenario == "partial-eof":
                    connection.sendall(b'{"ok":true')
                    sys.exit(9)
                if scenario == "no-response":
                    time.sleep(60)
                response_file = os.environ.get("TACOS_CAMERA_TEST_RESPONSE_FILE")
                response = (Path(response_file).read_bytes() if response_file else
                            b'{"ok":true,"detections":[]}\n')
                if scenario in ("fragmented", "trickle"):
                    for byte in response:
                        connection.sendall(bytes([byte]))
                        time.sleep(0.6 if scenario == "trickle" else 0.001)
                else:
                    connection.sendall(response)
                if scenario == "close-after-response":
                    connection.close()
                    time.sleep(60)
finally:
    if connection is not None:
        connection.close()
    if listener is not None:
        listener.close()
    if owned:
        path.unlink(missing_ok=True)
