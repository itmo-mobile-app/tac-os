# Runtime API specification

Status: approved

This document defines how the VM calls into the Runtime, and the first Runtime function used by the
Compiler → VM integration test. It does not cover IPC (see `spec/ipc.md`) or the Broker (see
`spec/broker-protocol.md`).

## Call mechanism

- The VM executes `CALL_RUNTIME` (see `spec/bytecode.md`) with a constant-pool string naming the
  Runtime function as `<namespace>.<function>` (e.g. `runtime.log`).
- Arguments are pushed onto the operand stack by the caller from left to right.
  Before the call, the VM pops them in reverse stack order and passes them to
  the Runtime function in signature order.
  For `foo(a, b)`, the caller pushes `a` and then `b`; the VM pops `b` and then
  `a`, and invokes the Runtime function as `foo(a, b)`.
- Every Runtime function returns exactly one value, which the VM pushes onto the operand stack after the
  call. A function that conceptually returns nothing pushes `Int 0`. This matches the return convention
  for bytecode functions in `spec/bytecode.md`, so the compiler handles calls to bytecode functions and
  to Runtime functions the same way.
- Calling an unknown `<namespace>.<function>` name is a load-time error (checked when the VM resolves
  Runtime calls after loading the bytecode, not at CALL_RUNTIME execution time).

## First Runtime function

```text
runtime.log(message: String)
```

Writes `message` followed by a newline (`\n`) to the process's standard output and returns `Int 0` (it
has no meaningful result).
This is the minimal function needed for the first Compiler → VM → Runtime integration test.

## Camera API contract (approved design; implementation pending)

The initial VM-facing call is:

```text
camera.detectMarkers() -> String
```

The call takes zero arguments and returns exactly one `String` value, consistent
with the existing VM/Runtime call ABI. On success, that String is a compact
JSON array of detections: `id` (Int, ArUco ID 0..71), `relativeAngle`
(finite number in degrees; right positive), and `distance` (finite number
in meters, greater than 0). Example:

```json
[{"id":17,"relativeAngle":12.5,"distance":3.2}]
```

No recognized marker is the successful return value `"[]"`. Camera failure,
timeout, malformed helper response or terminated helper is a Runtime error,
not `"[]"` and not a JSON error object returned to application code.

This is a **temporary String ABI**, not the eventual typed detection API.
Current `.tc` and VM have no `Float` or collection representation and the
Compiler cannot yet call every zero-argument Runtime function in an expression.
The final `Detection[]` interface, geographic calculations in `.tc`, and
VM-level recoverable error handling require later language/VM work. Sprint 2
may verify the C++ Runtime directly with synthetic detector responses;
no `.tc` JSON parser is introduced by this decision.

The Runtime implementation is a client of the private Go/OpenCV detector over
a Unix-domain stream socket; Runtime starts and reuses the helper process.
The complete private wire format (`DETECT`, JSONL framing, success/error
envelopes, socket path, timeouts and lifecycle) is defined in
`spec/camera-detector-protocol.md`. Application code cannot open this socket
directly. Analyzer will eventually combine detector results with device
position and orientation to create `Observation` records sent to Map via
the **separate** `spec/ipc.md` contract.

This section fixes the Camera API design but does not claim that
`camera.detectMarkers` is already registered or executable in the current
C++ Runtime or compiled `.tc` applications.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Runtime":

- the remaining Runtime API surface (threads, IPC, location, orientation, networking, files, time,
  Activity lifecycle, WebView bridge, Window Manager access) — added incrementally, one concrete
  scenario at a time;
- how recoverable Runtime call errors are represented inside `.tc` programs beyond the initial failure-reporting behavior.
