"""Real tacc + tacvm acceptance tests; no stub helpers or signature injection.

Requires approved #127 implementations and Compiler #132. These tests are
opt-in until those dependencies exist. They cannot approve the pending API.
"""
import concurrent.futures
import json
import math
import pathlib
import subprocess
import sys
import tempfile

compiler, vm = sys.argv[1:]
source = pathlib.Path(__file__).parent


def run(args):
    return subprocess.run(args, capture_output=True, text=True, timeout=30)


with tempfile.TemporaryDirectory() as temp:
    folder = pathlib.Path(temp)
    bytecode = {}
    for name in ('observation', 'target', 'strings'):
        path = folder / (name + '.bc')
        result = run([compiler, str(source / (name + '.tc')), str(path)])
        if result.returncode:
            raise AssertionError('Real Compiler #132/helpers are required: ' + result.stderr)
        bytecode[name] = path
    # Distinct VM OS processes, not merely independent objects in one thread.
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        jobs = {name: pool.submit(run, [vm, str(bytecode[name])])
                for name in ('observation', 'target')}
        outputs = {name: future.result() for name, future in jobs.items()}
    for result in outputs.values():
        assert result.returncode == 0, result.stderr
        assert 'Sanitizer' not in result.stderr, result.stderr
    observation = json.loads(outputs['observation'].stdout)
    assert set(observation) == {'targetId', 'observerId', 'latitude', 'longitude',
                                'confidence', 'timestamp'}
    assert observation['targetId'] == '17'
    assert observation['observerId'] == 'device"\\\nцель'
    assert observation['timestamp'] == 9007199254740993
    assert observation['longitude'] == 0.0
    assert math.isclose(observation['latitude'], 10.0 / 6371000.0 * 180.0 / math.pi)
    assert math.isclose(observation['confidence'], 0.9 / (1.0 + 10.0 / 25.0))
    assert outputs['target'].stdout == 'target-ok\n'
    strings = run([vm, str(bytecode['strings'])])
    assert strings.returncode == 0, strings.stderr
    assert strings.stdout == 'iteration:19999\n'

    for index, statement in enumerate((
        'runtime.log(text.fromFloat(1));',
        'runtime.log(json.getString("{}", 1));',
        'let x = "left" + "right";',
        'let x = 1 + 2.0;',
        'json.notAnApprovedHelper("{}");',
    )):
        path = folder / ('invalid' + str(index) + '.tc')
        path.write_text('fun main() { ' + statement + ' }\n')
        result = run([compiler, str(path), str(folder / 'invalid.bc')])
        assert result.returncode == 1, 'Compiler did not report a controlled type error: ' + result.stderr
    # Candidate #152 runtime errors: no fabricated defaults or silent coercions.
    invalid_expressions = (
        'text.fromInt(json.getInt("{}", "id"))',
        'text.fromInt(json.getInt("{\\"id\\":17.0}", "id"))',
        'text.fromInt(json.getInt("{\\"id\\":17e0}", "id"))',
        'text.fromInt(json.getInt("{\\"id\\":1,\\"id\\":2}", "id"))',
        'text.fromInt(json.getInt("{\\"id\\":9223372036854775808}", "id"))',
        'text.fromFloat(json.getFloat("{\\"x\\":1e9999}", "x"))',
        'json.arrayObject("[]", 0)',
        'json.arrayObject("[]", -1)',
    )
    for index, expression in enumerate(invalid_expressions):
        path = folder / ('runtime-invalid' + str(index) + '.tc')
        path.write_text('fun main() { runtime.log(' + expression + '); }\n')
        output = folder / ('runtime-invalid' + str(index) + '.bc')
        compiled = run([compiler, str(path), str(output)])
        assert compiled.returncode == 0, compiled.stderr
        executed = run([vm, str(output)])
        assert executed.returncode == 1, 'Runtime did not report a controlled error: ' + executed.stderr
        assert executed.stdout == '', 'Failed helper fabricated a printable result'
        assert executed.stderr.startswith('tacvm:'), executed.stderr
        assert 'Sanitizer' not in executed.stderr, executed.stderr

    # These bounds are from the proposal; run only once that contract is approved.
    for size, expected_success in ((65536, True), (65537, False)):
        path = folder / ('bound' + str(size) + '.tc')
        path.write_text('fun main() { runtime.log(text.concat("' + 'x' * size + '", "")); }\n')
        output = folder / ('bound' + str(size) + '.bc')
        compiled = run([compiler, str(path), str(output)])
        assert compiled.returncode == 0, compiled.stderr
        executed = run([vm, str(output)])
        if expected_success:
            assert executed.returncode == 0, executed.stderr
            assert executed.stdout == 'x' * size + '\n'
        else:
            assert executed.returncode == 1, 'Runtime did not report a controlled limit error: ' + executed.stderr
            assert executed.stderr.startswith('tacvm:'), executed.stderr
            assert 'Sanitizer' not in executed.stderr, executed.stderr
print('Observation/Target real Compiler-to-VM acceptance passed')
