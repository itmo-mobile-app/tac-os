# Runtime camera integration tests (#94)

## Build and verify

The production `cmd/marker-detect` imports gocv even when run with `-fake`.
For Runtime tests, `prototypes/marker-detect/internal/runtimehelper` is a small
test-only launcher around the existing `ipc.Listen`, `ipc.NewServer` and
`ipc.NewFakeSource` implementations from #86. It accepts `-socket PATH -fake MODE`
and stops on SIGTERM or SIGINT. It contains no detector algorithm and cannot
open a camera. CMake builds it with cgo and module downloads disabled.

On Linux, build the Compiler and run the complete sanitizer suite:

```bash
(cd compiler && bash ./gradlew --no-daemon installDist test)
cmake -S . -B build -DCMAKE_BUILD_TYPE=Debug -DTACOS_SANITIZE=ON
cmake --build build --parallel
ctest --test-dir build --output-on-failure
```

To run only the camera tests after building:

```bash
ctest --test-dir build -R 'camera_' --output-on-failure
```

The Go helper alone can also be checked before building the Compiler or Runtime:

```bash
cmake -S runtime/tests/camera -B build/camera-helper
cmake --build build/camera-helper
ctest --test-dir build/camera-helper --output-on-failure
```

## Coverage

`camera_helper_contract` exercises the real Go server with the shared `two`,
`empty`, `camera-unavailable` and `detection-failed` fixtures. Each case sends
two DETECT requests on one connection, checks socket mode 0600 and verifies
normal shutdown, a reaped child and removal of the helper's socket.

`camera_runtime_contract` calls the registered C++ Runtime function. It verifies:

- Zero-argument function lookup, wrong arity and a String result without the
  private success envelope. Construction and function lookup launch no child.
- The actual Go helper in all four shared fake modes, repeated calls and cleanup.
- One process, one accepted connection and two DETECT requests, recorded by an
  instrumented test server. Valid wire errors keep that connection usable.
- Invalid JSON/UTF-8, duplicate keys, unknown/missing fields, wrong types,
  out-of-range/non-integer IDs, duplicate or unsorted IDs, invalid distances,
  non-finite numbers, excessive nesting and unsolicited extra frames.
- Fragmented responses, the exact 64-KiB frame boundary and an oversized frame.
- An unavailable socket, a missing executable, startup exit, EOF, partial EOF,
  disconnect and deadlines for startup, silent responses and slowly arriving bytes.
- No automatic restart after a transport/protocol failure.
- Normal destruction, stack unwinding and a child that ignores SIGTERM.
- Preservation of an unrelated regular file and an active socket at the default path.

Each Runtime test runs in its own process, giving it a distinct default socket
pathname. The C++ driver checks `waitpid` for remaining children, including
zombies. The Python harness also checks recorded PID/start-time identities,
then cleans up only its own test processes and socket paths after a failure.

Runtime first closes its connection and sends SIGTERM to its child. It allows
3.5 seconds for graceful shutdown, then uses SIGKILL and reaps the child.
Only the helper removes its socket, as required by the private protocol.
An intentionally uncooperative test helper cannot unlink after SIGKILL;
the harness removes that test-owned stale socket after verifying child reaping.
Likewise, a real helper killed externally may leave a stale socket, which the
existing Go startup code reclaims on a later run. Runtime never unlinks an
unrelated endpoint.

## Runtime ownership and review

Camera state belongs to one Runtime instance; there is no global mutable camera
state and no shared Runtime locking. Use a Runtime from one thread at a time.
The default pathname contains the process PID, so simultaneous camera owners
inside one process are not supported. This preserves the restrictions proposed
in `origin/codex/vm-contexts-93:spec/vm-execution.md`; it does not approve concurrent
access to one camera or change the VM's execution API.

The VM-facing ABI and private wire format remain the approved contracts in
`spec/runtime-api.md` and `spec/camera-detector-protocol.md`. Before merging #94,
request detector/Analyzer owner review for helper integration and VM owner review
for compatibility with #93. Architecture/integration review covers the test and
documentation changes. Activity lifecycle proposal #95 is a separate task.
