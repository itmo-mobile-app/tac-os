# Independent VM context verification (#93)

## Scope and owner review

Execution was extracted into `tacos_vm`; the CLI still delegates to the same
loader/interpreter. Concurrent calls use independent stacks, frames, locals,
Runtime instances and output streams. Runtime and Compiler sources are unchanged.
The proposed ownership restrictions are in `spec/vm-execution.md` and still
require Runtime owner review. Shared IPC/window/camera usage is not approved.

## Reproduce on Linux

```sh
(cd compiler && ./gradlew test installDist)
cmake -S . -B build -DCMAKE_BUILD_TYPE=Debug -DTACOS_SANITIZE=ON
cmake --build build --parallel
ctest --test-dir build --output-on-failure
```

For the dedicated ThreadSanitizer run, use a different build directory:

```sh
cmake -S . -B build/tsan -DCMAKE_BUILD_TYPE=Debug -DTACOS_THREAD_SANITIZE=ON
cmake --build build/tsan --target vm_context_tests --parallel
ctest --test-dir build/tsan -R '^vm_contexts$' --output-on-failure
```

ASan/UBSan and TSAN cannot be enabled in the same build. The test compiles
three real `.tc` files, starts two workers together, and checks their separate
return values and exact outputs over twelve repetitions. A failing context
runs alongside a successful one; a subsequent invocation verifies recovery.

## Recorded local results

- macOS arm64, AppleClang 15, JDK 17.
- ASan/UBSan: 24/24 non-socket CTest checks passed, including `vm_contexts` and
  existing Compiler-to-VM regressions.
- ThreadSanitizer: `vm_contexts` passed, no race diagnostics.
- The two existing socket-dependent checks (`map_wm_e2e`, `ipc_vm_e2e`) were
  excluded here because permission for another unrestricted full-suite run
  was not granted. They remain enabled for Linux CI.
- Linux-only Runtime socket calls required an external, uncommitted macOS
  compatibility include for local compilation. Native Linux CI is still needed.
- This branch is based on main independently of Float #108, as required by
  #93's separate-PR scope. Reconcile the interpreter extraction with #108
  when the first branch merges; do not overwrite either implementation.
