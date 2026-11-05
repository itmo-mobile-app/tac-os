# Float VM verification (#108)

## Scope

This branch implements the bytecode-v2 VM side of the contract merged in #111.
No Compiler, Runtime, IPC, Map, Broker, DBMS or detector source is changed.
Float has a VM-owned kind and binary64 payload. Calls to the existing Runtime
convert only Int/String values; Float arguments to the current Runtime API are
rejected. Existing Runtime signatures and bytecode-v1 instructions are unchanged.

## Reproduce on Linux

With JDK 17, Go 1.22+, CMake and a C++17 compiler:

```sh
(cd compiler && ./gradlew test installDist)
cmake -S . -B build -DCMAKE_BUILD_TYPE=Debug -DTACOS_SANITIZE=ON
cmake --build build --parallel
ctest --test-dir build --output-on-failure
```

`vm_instructions` executes independent v1/v2 bytecode fixtures, including all
Float operations, arithmetic/type errors, non-finite constants/results,
truncated Float payloads, invalid versions, signed zero, finite subnormals,
rounding ties, local slots, function parameters/returns and the coordinate
formula. The old Int/Bool/String suite is also executed on v2.

## Recorded local results

- macOS arm64, AppleClang 15, JDK 17, ASan/UBSan.
- Compiler regression suite: 59 tests passed; Compiler source is unchanged.
- Final VM/regression run: 23/23 CTest checks passed with the existing
  socket-dependent `map_wm_e2e` and `ipc_vm_e2e` excluded.
- Both socket-dependent checks passed in an earlier run before the final
  VM-owned value-boundary adjustment. They need a fresh Linux CI run on the
  final commit; the later local permission request was not granted.
- The current Runtime uses Linux-only socket flags and `accept4`. Local macOS
  builds used an external, uncommitted compatibility include mapping those
  calls to socket/accept + fcntl. This is not a native Linux verification.

## Explicit dependency on Compiler #107

The unchanged main-branch compiler currently rejects `Float` in
`tests/float/coordinates.tc`. Therefore the real Float source-to-VM criterion
is still pending Compiler #107, not passed by the hand-assembled fixtures.

A real E2E test is already registered behind an explicit option:

```sh
cmake -S . -B build -DTACOS_SANITIZE=ON -DTACOS_VM_FLOAT_E2E=ON \
  -DTACOS_TACC=/absolute/path/to/float-compiler/bin/tacc
cmake --build build --parallel
ctest --test-dir build -R float_coordinates_e2e --output-on-failure
```

The result must be `float-ok`. Keep this dependency visible in the PR and do
not close the complete #108 acceptance checklist until this test has passed.
