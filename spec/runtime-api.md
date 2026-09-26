# Runtime API specification

Status: draft — proposed by Architecture & Integration ahead of Sprint 0, pending review and approval by
the Runtime owner (see `docs/ai-context/03-team-ownership.md`) in a follow-up Pull Request.

This document defines how the VM calls into the Runtime, and the first Runtime function used by the
Compiler → VM integration test. It does not cover IPC (see `spec/ipc.md`) or the Broker (see
`spec/broker-protocol.md`).

## Call mechanism

- The VM executes `CALL_RUNTIME` (see `spec/bytecode.md`) with a constant-pool string naming the
  Runtime function as `<namespace>.<function>` (e.g. `runtime.log`).
- Arguments are popped off the operand stack in reverse order (last pushed = first argument) before
  the call.
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
