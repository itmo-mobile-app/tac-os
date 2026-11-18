"""Explicit cross-branch check against the real Runtime #162, in a temp copy.

Does not edit either branch or inject Compiler signatures. Requires CMake,
Go, the main compiler, and optional normal CMake -D arguments for local deps.
Usage: python check_device.py <runtime-git-ref> <cmake-executable> [cmake args...]
"""
import io
import pathlib
import shutil
import struct
import subprocess
import sys
import tarfile
import tempfile

repository = pathlib.Path(__file__).resolve().parents[2]
ref, cmake = sys.argv[1:3]


def archive(revision, target, *paths):
    raw = subprocess.check_output(['git', 'archive', revision, *paths], cwd=repository)
    with tarfile.open(fileobj=io.BytesIO(raw)) as tar:
        tar.extractall(target, filter='data')


def bytecode(path, constants, code, locals_=0):
    data = b'TACB' + struct.pack('<HH', 2, len(constants))
    for value in constants:
        if isinstance(value, int):
            data += b'\0' + struct.pack('<q', value)
        else:
            raw = value.encode()
            data += b'\1' + struct.pack('<I', len(raw)) + raw
    data += struct.pack('<HI', 1, 1) + b'f' + bytes([0, locals_])
    data += struct.pack('<I', len(code)) + code + struct.pack('<H', 0)
    path.write_bytes(data)


with tempfile.TemporaryDirectory(prefix='tacos133-device-') as temp:
    target = pathlib.Path(temp)
    archive('HEAD', target)
    # Replace only the temporary Runtime with its actual implementation.
    shutil.rmtree(target / 'runtime')
    archive(ref, target, 'runtime')
    shutil.rmtree(target / 'vm')
    shutil.copytree(repository / 'vm', target / 'vm')
    shutil.copyfile(repository / 'tests/vm-data/device_runtime.cpp', target / 'device.cpp')
    with (target / 'CMakeLists.txt').open('a') as file:
        file.write('\nadd_executable(vm_device_compat device.cpp)\n')
        file.write('target_link_libraries(vm_device_compat PRIVATE tacos_vm)\n')
    subprocess.run([cmake, '-S', str(target), '-B', str(target / 'build'),
                    '-DTACOS_SANITIZE=ON', *sys.argv[3:]], check=True)
    subprocess.run([cmake, '--build', str(target / 'build'), '--target', 'vm_device_compat',
                    '--parallel', '4'], check=True)
    constants = [100, 'location.read', 'location.latitude',
                 'orientation.read', 'orientation.heading', 'time.nowMs']
    # read position -> Float result to a local; read heading -> add local -> RET.
    floating = bytes([1, 0, 0, 13, 1, 0, 1, 15, 13, 2, 0, 0, 3, 0,
                      1, 0, 0, 13, 3, 0, 1, 15, 13, 4, 0, 0, 2, 0, 17, 14])
    float_path, time_path = target / 'float.bc', target / 'time.bc'
    bytecode(float_path, constants, floating, locals_=1)
    bytecode(time_path, constants, bytes([13, 5, 0, 0, 14]))
    subprocess.run([str(target / 'build/vm_device_compat'), str(float_path), str(time_path)], check=True)
