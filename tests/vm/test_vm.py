"""Black-box VM instruction and malformed-bytecode regression tests."""
import pathlib
import struct
import subprocess
import sys
import tempfile
import unittest

VM = sys.argv.pop(1)


def ins(op, value=None, argc=None):
    data = bytes([op])
    if op in (1, 12, 13):
        data += struct.pack('<H', value)
    elif op in (2, 3):
        data += bytes([value])
    elif op in (10, 11):
        data += struct.pack('<I', value)
    if argc is not None:
        data += bytes([argc])
    return data


def module(constants, functions, entry=0):
    data = b'TACB' + struct.pack('<HH', 1, len(constants))
    for value in constants:
        if isinstance(value, int):
            data += b'\0' + struct.pack('<q', value)
        else:
            raw = value.encode()
            data += b'\1' + struct.pack('<I', len(raw)) + raw
    data += struct.pack('<H', len(functions))
    for params, locals_, code in functions:
        data += struct.pack('<I', 1) + b'f' + bytes([params, locals_])
        data += struct.pack('<I', len(code)) + code
    return data + struct.pack('<H', entry)


class VmTests(unittest.TestCase):
    def run_bc(self, constants, code, locals_=0, functions=None, error=None, output=''):
        data = module(constants, functions or [(0, locals_, code)])
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / 'test.bc'
            path.write_bytes(data)
            result = subprocess.run([VM, str(path)], capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 1 if error else 0, result.stderr)
        if error:
            self.assertIn(error, result.stderr)
        else:
            self.assertEqual(result.stdout, output)
        self.assertNotIn('Sanitizer', result.stderr)
        self.assertNotIn('runtime error:', result.stderr)

    def assert_value(self, expression, constants, expected):
        # Failed comparisons select "wrong"; successful ones print "ok".
        pool = constants + [expected, 'ok', 'wrong', 'runtime.log', 0]
        n = len(constants)
        prefix = expression + ins(1, n) + ins(8)
        yes = ins(1, n + 1) + ins(13, n + 3, 1) + ins(15)
        no = ins(1, n + 2) + ins(13, n + 3, 1) + ins(15)
        target = len(prefix) + 5 + len(yes) + 5
        code = prefix + ins(11, target) + yes + ins(10, target + len(no))
        code += no + ins(1, n + 4) + ins(14)
        self.run_bc(pool, code, output='ok\n')

    def test_arithmetic_and_comparisons(self):
        for op, expected in [(4, 10), (5, 4), (6, 21), (7, 2), (8, 0), (9, 0)]:
            with self.subTest(op=op):
                self.assert_value(ins(1, 0) + ins(1, 1) + ins(op), [7, 3], expected)
        self.assert_value(ins(1, 0) + ins(1, 1) + ins(7), [-7, 3], -2)
        self.assert_value(ins(1, 0) + ins(16), [0], 1)
        self.assert_value(ins(1, 0) + ins(16), [1], 0)
        self.assert_value(ins(1, 0) + ins(1, 1) + ins(8), ['same', 'same'], 1)
        self.assert_value(ins(1, 0) + ins(1, 1) + ins(8), ['a', 'b'], 0)
        self.assert_value(ins(1, 0) + ins(1, 1) + ins(9), [-2, 1], 1)

    def test_loop_locals(self):
        # while (x < 3) x = x + 1; then log the chosen branch.
        code = ins(1, 0) + ins(3, 0)
        loop = len(code)
        condition = ins(2, 0) + ins(1, 1) + ins(9)
        body = ins(2, 0) + ins(1, 2) + ins(4) + ins(3, 0) + ins(10, loop)
        end = len(code) + len(condition) + 5 + len(body)
        code += condition + ins(11, end) + body
        code += ins(1, 3) + ins(13, 4, 1) + ins(15) + ins(1, 0) + ins(14)
        self.run_bc([0, 3, 1, 'done', 'runtime.log'], code, locals_=1, output='done\n')

    def test_call_order_and_caller_stack(self):
        # Keep 100 below arguments, subtract(9, 4), then add => 105.
        expression = ins(1, 0) + ins(1, 1) + ins(1, 2) + ins(12, 1, 2) + ins(4)
        code = expression + ins(1, 3) + ins(8) + ins(16) + ins(11, len(expression) + 11)
        # A failed assertion attempts to pop an empty stack.
        code += ins(15) + ins(1, 4) + ins(14)
        callee = ins(2, 0) + ins(2, 1) + ins(5) + ins(14)
        self.run_bc([100, 9, 4, 105, 0], b'', functions=[(0, 0, code), (2, 0, callee)])

    def test_recursive_factorial_and_local_isolation(self):
        # fact(n): if n < 2 return 1; return n * fact(n - 1).
        base = ins(1, 2) + ins(14)
        condition = ins(2, 0) + ins(1, 1) + ins(9)
        recursive = (ins(2, 0) + ins(2, 0) + ins(1, 2) + ins(5)
                     + ins(12, 1, 1) + ins(6) + ins(14))
        callee = condition + ins(11, len(condition) + 5 + len(base)) + base + recursive
        caller = ins(1, 0) + ins(3, 0) + ins(2, 0) + ins(12, 1, 1)
        caller += ins(1, 3) + ins(8) + ins(16)
        target = len(caller) + 6
        caller += ins(11, target) + ins(15)
        caller += ins(2, 0) + ins(1, 0) + ins(8) + ins(16)
        target = len(caller) + 6
        caller += ins(11, target) + ins(15) + ins(1, 4) + ins(14)
        self.run_bc([5, 2, 1, 120, 0], b'', functions=[(0, 1, caller), (1, 0, callee)])

    def test_errors(self):
        cases = [
            ([0], ins(2, 0) + ins(14), 0, 'local index'),
            ([0], ins(2, 0) + ins(14), 1, 'uninitialized'),
            ([0], ins(3, 0), 1, 'underflow'),
            ([], ins(15), 0, 'underflow'),
            ([], ins(14), 0, 'underflow'),
            ([1, 0], ins(1, 0) + ins(1, 1) + ins(7), 0, 'division by zero'),
            ([2**63-1, 1], ins(1, 0) + ins(1, 1) + ins(4), 0, 'overflow'),
            ([-2**63, 1], ins(1, 0) + ins(1, 1) + ins(5), 0, 'overflow'),
            ([2**62, 4], ins(1, 0) + ins(1, 1) + ins(6), 0, 'overflow'),
            ([-2**63, -1], ins(1, 0) + ins(1, 1) + ins(7), 0, 'overflow'),
            (['x', 1], ins(1, 0) + ins(1, 1) + ins(4), 0, 'expected Int'),
            (['x', 1], ins(1, 0) + ins(1, 1) + ins(8), 0, 'matching types'),
            ([2], ins(1, 0) + ins(16), 0, 'Bool'),
            ([2], ins(1, 0) + ins(11, 8) + ins(14), 0, 'Bool'),
            ([], ins(10, 1), 0, 'invalid jump'),
            ([], ins(10, 5), 0, 'invalid jump'),
            ([], ins(10, 999), 0, 'invalid jump'),
            ([], ins(1, 0), 0, 'constant index'),
            ([], ins(12, 9, 0), 0, 'function index'),
            ([], ins(12, 0, 1), 0, 'argument count'),
            ([], b'\xff', 0, 'unknown opcode'),
            ([], b'\x0a\x00', 0, 'unexpected end'),
            ([], b'', 0, 'without RET'),
            (['missing'], ins(1, 0) + ins(14) + ins(13, 0, 0), 0, 'unknown Runtime'),
        ]
        for constants, code, locals_, error in cases:
            with self.subTest(error=error, code=code):
                self.run_bc(constants, code, locals_, error=error)

    def test_callee_cannot_pop_callers_values(self):
        self.run_bc([42], b'', functions=[
            (0, 0, ins(1, 0) + ins(12, 1, 0) + ins(14)),
            (0, 0, ins(14)),
        ], error='underflow')

    def test_recursion_limit(self):
        self.run_bc([], ins(12, 0, 0) + ins(14), error='call stack overflow')

    def test_operand_limit(self):
        self.run_bc([0], ins(1, 0) + ins(10, 0), error='operand stack overflow')

    def test_truncated_operands(self):
        for op, width in [(1, 2), (2, 1), (3, 1), (10, 4), (11, 4), (12, 3), (13, 3)]:
            for count in range(width):
                with self.subTest(op=op, count=count):
                    self.run_bc([], bytes([op]) + bytes(count), error='unexpected end')


if __name__ == '__main__':
    unittest.main()
