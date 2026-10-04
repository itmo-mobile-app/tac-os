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
- The Runtime function's return value, if any, is pushed back onto the operand stack; a function with
  no return value pushes nothing.
- Calling an unknown `<namespace>.<function>` name is a load-time error (checked when the VM resolves
  Runtime calls after loading the bytecode, not at CALL_RUNTIME execution time).

## First Runtime function

```text
runtime.log(message: String) -> Void
```

Writes `message` to the process's standard output. This is the minimal function needed for the first
Compiler → VM → Runtime integration test.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Runtime":

- the full Runtime API surface (threads, IPC, camera, location, orientation, networking, files, time,
  Activity lifecycle, WebView bridge, Window Manager access) — added incrementally, one concrete
  scenario at a time;
- error handling for a Runtime call that fails (e.g. camera unavailable).
