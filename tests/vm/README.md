# VM arithmetic and control flow

`vm_instructions` runs black-box tests against `tacvm`, using independent Python
bytecode fixtures. It covers arithmetic (including signed division), locals,
comparisons, branches and loops, function argument order, recursive calls, caller
stack/local isolation, invalid instructions/operands/indices/targets, uninitialized
locals, type errors, integer overflow, division by zero and stack limits.
Python 3 is required for this test. The normal sanitizer CMake build instruments
all VM executions, including malformed-bytecode cases.

The full language scenarios require Compiler #63 (PR #82), since the compiler in
main only supports the initial Runtime-call scenario. To verify that compiler
without copying its implementation into the VM branch:

```sh
cmake -S . -B build-vm -DTACOS_SANITIZE=ON \
  -DTACOS_VM_LANGUAGE_E2E=ON \
  -DTACOS_TACC=/absolute/path/to/compiler/build/install/tacc/bin/tacc
cmake --build build-vm
ctest --test-dir build-vm --output-on-failure
```

`control-flow.tc` and `language-example.tc` mirror the integration fixtures in
Compiler #63. `recursion.tc` additionally checks recursive frames, argument order,
caller locals and string equality. The advanced tests run by default now that Compiler #63 and VM #62 are integrated;
`hello_e2e` and `vm_instructions` also remain enabled.
Safety semantics proposed for Compiler owner review are in `spec/bytecode.md`.

## Verified integration snapshot

The VM was verified against Compiler #63 commit `7c7ae59` with all three language
scenarios enabled. The complete CTest suite passed (19/19) with AppleClang 15,
ASan/UBSan, JDK 17 and Go 1.27.3 on macOS arm64. The compiler's 58 tests passed.
The main-branch compiler's hello scenario was also checked separately. Linux
sanitizer verification remains the responsibility of the existing Build CI job.

## Float v2 (#108)

The Python suite now executes the original regression tests on both bytecode
versions and checks Float constants, all seven Float instructions, mixed-type
errors, locals/arguments/returns, signed zero, subnormals, ties-to-even rounding,
overflow, non-finite constants and truncated data. v1 rejects Float instructions
even when unreachable. Float values are owned by the VM and share numeric 64-bit storage with Int,
but have a distinct kind. Existing Int/String values are converted at the
Runtime boundary. Float arguments are rejected because the current Runtime
signatures define no Float parameters; Runtime sources and ABI are unchanged.

The VM sets round-to-nearest for each run and restores the embedding thread's
rounding mode on success or error. Build without fast-math; the VM target disables
FP contraction and enables rounding-aware code generation on GCC/Clang.

The real coordinate-source fixture is `tests/float/coordinates.tc`. Enable it
only with a Float-capable compiler from #107:

```sh
cmake -S . -B build -DTACOS_SANITIZE=ON -DTACOS_VM_FLOAT_E2E=ON \
  -DTACOS_TACC=/absolute/path/to/float-compiler/bin/tacc
cmake --build build
ctest --test-dir build --output-on-failure
```

The default compiler on main has no Float support. A prepared E2E fixture is
not evidence of a passing Float source-to-VM integration until #107 is available.
