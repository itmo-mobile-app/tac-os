"""Compile real programs before running the in-process C++ concurrency test."""
import pathlib
import subprocess
import sys
import struct
import tempfile

compiler, runner = sys.argv[1:]
source_dir = pathlib.Path(__file__).parent
with tempfile.TemporaryDirectory() as tmp:
    bytecode = []
    for name in ('first', 'second', 'failure'):
        target = pathlib.Path(tmp) / (name + '.bc')
        subprocess.run([compiler, str(source_dir / (name + '.tc')), str(target)],
                       check=True, timeout=30)
        bytecode.append(str(target))
    # Float fixtures test the host API without pretending Compiler #107 exists.
    for name, constants, code in [
        ('rounding', [1.0, 2**-53], bytes([1, 0, 0, 1, 1, 0, 17, 14])),
        ('negative-zero', [0.0], bytes([1, 0, 0, 21, 14])),
    ]:
        data = b'TACB' + struct.pack('<HH', 2, len(constants))
        for value in constants:
            data += b'\2' + struct.pack('<d', value)
        data += struct.pack('<HI', 1, 1) + b'f' + bytes([0, 0])
        data += struct.pack('<I', len(code)) + code + struct.pack('<H', 0)
        target = pathlib.Path(tmp) / (name + '.bc')
        target.write_bytes(data)
        bytecode.append(str(target))
    subprocess.run([runner, *bytecode], check=True, timeout=45)
