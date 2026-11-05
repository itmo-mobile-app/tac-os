# Independent VM executions (#93)

Status: proposed restrictions for Runtime and Compiler owner review.
This API implements isolated executions; it does not approve shared Runtime use.

## C++ entry point

The `tacos_vm` CMake target exposes `tacos/vm/vm.hpp`:

```cpp
tacos::vm::Value tacos::vm::run_file(
    const std::string& path, tacos::runtime::Runtime& runtime);
```

Each call loads its own program and owns its instruction pointers, constants,
operand stack, frames and local variables. Successful execution returns the
entry function's single typed result, including Float from #108. VM errors throw `tacos::vm::VmError`; Runtime
errors retain `tacos::runtime::RuntimeError`. The CLI catches errors, writes a
`tacvm:` diagnostic and exits nonzero, preserving its existing behavior.
No mutable execution state is shared between calls, including after a failure.
The Runtime reference is borrowed for the duration of a call, never retained.
The caller keeps it alive until execution completes and joins worker threads
before destroying their dependencies.

## Concurrency restrictions

- Concurrent calls must use different Runtime objects and different output
  streams. The embedding caller owns each Runtime and its resource lifetime.
- A Runtime object must not be used by two threads concurrently. No new locks
  or claims about existing Runtime methods being thread-safe are introduced.
- The initial verified scope is computation plus `runtime.log` into a private
  stream per context. The shared Runtime dispatch table is immutable.
- OS resources are not isolated by a VM object. IPC socket paths, listener
  ownership, camera helpers, WM window IDs and process-wide diagnostics require
  separate ownership or owner-approved synchronization before concurrent use.
  In particular, the default IPC pathname must not be used by two listeners.
- The VM error rules proposed in `spec/bytecode.md` are unchanged; the Compiler
  owner's approval remains pending. One thread's exception does not cancel or
  modify another context; the host catches each exception and joins both threads.
- No `.tc` spawn/join API, shared heap, synchronization primitive, scheduler or
  new instruction is introduced. Analyzer and Map remain separate processes.

These conservative limits still require review by the Runtime owner before
merging #93. They do not assert that prior owner agreement occurred.

## Verification

`vm_contexts` compiles two actual `.tc` programs using `tacc`, then executes them
in two `std::thread` workers with separate Runtime/output instances. It checks
return values, local variables, function calls and exact per-context output.
It repeats concurrent execution and checks that one VM's error does not prevent
another from completing. Separate Float-bytecode fixtures verify payload/kind,
signed zero and per-thread rounding restoration. All existing CLI/E2E checks remain enabled.

Run under ASan/UBSan using the normal `TACOS_SANITIZE=ON` build. For a separate
ThreadSanitizer build, use `TACOS_THREAD_SANITIZE=ON` (never combine sanitizers). See
`docs/testing/vm-contexts.md` for recorded results and platform limitations.
Runtime IPC and GUI concurrency are explicitly outside this test's claim.
