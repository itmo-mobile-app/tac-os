# Broker protocol specification

Status: draft — proposed by Architecture & Integration ahead of Sprint 0, pending review and approval by
the Broker and Target Service owner (see `docs/ai-context/03-team-ownership.md`) in a follow-up Pull
Request.

This document defines the minimal publish/subscribe protocol between clients (Map, Target Service) and
the Broker (see `docs/ai-context/01-architecture.md` → "Broker").

## Transport

Plain TCP, one connection per client. Chosen for minimal implementation complexity; revisit if a
concrete scenario needs browser clients (WebSocket) or message persistence.

## Framing and encoding

- One command per line (`\n`-terminated), UTF-8 text.
- Command format: `<VERB> <topic> [payload]`, where `payload` (if present) is a single-line JSON object.

## Commands

| Verb | Direction | Meaning |
| --- | --- | --- |
| `SUB <topic>` | client → broker | Subscribe to `<topic>` |
| `PUB <topic> <payload>` | client → broker | Publish `<payload>` to `<topic>` |
| `MSG <topic> <payload>` | broker → client | Delivered to every subscriber of `<topic>` |

## Topics

- `observations` — clients publish `Observation` (see `spec/data-model.md`); Target Service subscribes.
- `targets.updated` — Target Service publishes `Target` (see `spec/data-model.md`); clients subscribe.

## Delivery behavior

- At-most-once delivery: no persistence, no acknowledgement, no retry.
- A client that is not connected when a message is published simply does not receive it.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Broker":

- reconnection behavior;
- whether message persistence is required for a later scenario.
