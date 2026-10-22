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
