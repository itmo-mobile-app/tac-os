# Data model specification

Status: approved by the Broker and Target Service owner and the Tactical Map owner; pending approval by the Analyzer owner.

This document formalizes the shared structures introduced conceptually in
`docs/ai-context/04-data-model.md`, for use by `spec/ipc.md` and `spec/broker-protocol.md`.

## Observation

Produced by Analyzer, sent to Map over IPC, and published by Map to the Broker.

| Field | Type | Notes |
| --- | --- | --- |
| `targetId` | String | Identifier of the observed target |
| `observerId` | String | Identifier of the reporting client/device |
| `latitude` | Float | Calculated target latitude |
| `longitude` | Float | Calculated target longitude |
| `confidence` | Float | `0.0`–`1.0`, from local detection |
| `timestamp` | Int | Unix time in milliseconds |

## Target

Produced by Target Service, published on `targets.updated`.

| Field | Type | Notes |
| --- | --- | --- |
| `targetId` | String | Stable identifier of the physical target |
| `latitude` | Float | Aggregated target latitude |
| `longitude` | Float | Aggregated target longitude |
| `probability` | Float | `0.0`–`1.0`, system confidence in current location |
| `lastSeenAt` | Int | Unix time in milliseconds of the most recent relevant observation |

## JSON encoding

Both structures are encoded as single-line JSON objects with exactly the fields above, using the same
field names, for use as `spec/ipc.md` and `spec/broker-protocol.md` payloads.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Domain model" and "Target Service":

- probability calculation formula and time decay formula;
- duplicate/stale observation handling;
- device/observer identifier format.
