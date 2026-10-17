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

## Planned Camera API (architectural decision; not yet implemented)

Analyzer uses the public Runtime function conceptually named:

```text
camera.detectMarkers() -> detections
```

The logical return value is a collection with an `id` (integer ArUco marker id),
`relativeAngle` (degrees from the camera axis, positive to the right), and `distance`
(meters). A private Go detector process performs camera capture and OpenCV-based ArUco
recognition. The C++ Runtime communicates with it using a local Unix-domain socket,
then exposes the detections to the Analyzer on the VM.

Application `.tc` code does not use detector IPC or OpenCV directly. Analyzer uses device
position and orientation to convert detections into `Observation` records. Detector IPC
is distinct from Analyzer-to-Map IPC in `spec/ipc.md`, which is unchanged.

This records an API **design boundary**, not an executable call signature or approved wire
protocol. The current VM does not yet specify a return ABI for a collection of floating-point
records. Return encoding, helper IPC framing, exact socket path, failure behavior, and process
lifecycle must be agreed with the Runtime and VM owners before implementation; see
`docs/ai-context/02-open-questions.md` → "Runtime".

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Runtime":

- the remaining Runtime API surface (threads, IPC, location, orientation, networking, files, time,
  Activity lifecycle, WebView bridge, Window Manager access) — added incrementally, one concrete
  scenario at a time;
- error handling for a Runtime call that fails (e.g. camera unavailable).
