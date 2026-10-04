# IPC specification

Status: approved

This document defines the minimal IPC contract for the primary local scenario: the Analyzer process
sending a detected target to the Map process (see `docs/ai-context/01-architecture.md` → "IPC").

## Transport

A Unix domain socket at `/tmp/tac-os-analyzer-map.sock`, one connection per Analyzer↔Map pair.
The Map process listens; the Analyzer process connects.

The Map process removes a stale socket file before binding and removes the socket file on normal shutdown.

Chosen for minimal implementation complexity on Linux; revisit if a concrete scenario
needs multiple listeners or cross-host IPC (out of scope — Analyzer and Map always run on the same
device).

## Framing and encoding

- One message per line (`\n`-terminated).
- Each line is a UTF-8 JSON object with a `type` field.

## Message: `TARGET_DETECTED`

Sent by Analyzer to Map when a local `Observation` (see `spec/data-model.md`) is produced.

```json
{
  "type": "TARGET_DETECTED",
  "targetId": "string",
  "confidence": 0.0,
  "relativeAngle": 0.0,
  "distance": 0.0,
  "timestamp": 0
}
```

- `confidence`: `0.0`–`1.0`.
- `relativeAngle`: degrees, relative to device heading.
- `distance`: meters.
- `timestamp`: Unix time in milliseconds.

## Delivery behavior

- No acknowledgement and no message retry: if there is no active connection to the Map process
  when an observation is produced, the Analyzer drops that message.
- No request/response — this is a one-way notification channel.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "IPC scenario":

- reconnection behavior after the Map process restarts;
- whether additional message types beyond `TARGET_DETECTED` are needed.
