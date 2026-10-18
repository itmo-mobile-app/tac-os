# Data model specification

Status: approved by the Broker and Target Service, Analyzer, and Tactical Map owners.

This document formalizes the shared structures introduced conceptually in
`docs/ai-context/04-data-model.md`, for use by `spec/ipc.md` and `spec/broker-protocol.md`.

## Observation

Produced by Analyzer, sent to Map over IPC, and published by Map to the Broker.

| Field | Type | Notes |
| --- | --- | --- |
| `targetId` | String | Identifier of the observed target: the marker id as a decimal string, e.g. `"17"` |
| `observerId` | String | Identifier of the reporting client/device |
| `latitude` | Float | Calculated target latitude |
| `longitude` | Float | Calculated target longitude |
| `confidence` | Float | `0.0`–`1.0`, reliability of this single local Observation |
| `timestamp` | Int | Unix time in milliseconds |

The six fields above are the entire shared `Observation` object. Analyzer-to-Map
IPC adds only an outer `"type":"OBSERVATION"` field; Map publishes the original
six fields to Broker unchanged. See `spec/ipc.md`. Detector angles and distances
are not part of this shared object. The production method for estimating local
confidence is not yet defined.

## Target

Produced by Target Service, published on `targets.updated`.

| Field | Type | Notes |
| --- | --- | --- |
| `targetId` | String | Stable identifier of the physical target |
| `latitude` | Float | Aggregated target latitude |
| `longitude` | Float | Aggregated target longitude |
| `probability` | Float | `0.0`–`1.0`, aggregated target-location confidence computed by Target Service |
| `lastSeenAt` | Int | Unix time in milliseconds of the most recent relevant observation |

`Target.probability` is separate from `Observation.confidence` and depends on
evidence from multiple observers, coordinate agreement and freshness. Its exact
formula is not specified here.

## JSON encoding

Shared domain objects are single-line JSON objects with exactly their respective
table fields. The Analyzer-to-Map IPC adds only its transport `type` field; the
Broker's payload remains the six-field `Observation` domain object.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Domain model" and "Target Service":

- probability calculation formula and time decay formula;
- duplicate/stale observation handling;
- device/observer identifier format.
