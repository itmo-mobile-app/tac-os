"""Compile real programs before running the in-process C++ concurrency test."""
import pathlib
import subprocess
import sys
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
    subprocess.run([runner, *bytecode], check=True, timeout=45)
