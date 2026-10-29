# Data model

Status: draft

This document describes the current shared domain model used by the `tac-os` application and server components.

The exact field types and serialization formats are not fixed here unless explicitly stated.
Formal shared structures should later be defined in `spec/data-model.md`.

## Detection

`Detection` is produced by the private Go marker detector, received through
Runtime and converted by Analyzer into a shared `Observation`.

The current detector returns:

- `id` — integer ArUco marker ID; Analyzer converts it to the decimal-string `targetId`;
- `relativeAngle` — angle relative to the camera center, in degrees, positive to the right;
- `distance` — estimated distance to the marker, in meters.

The prototype does not return `confidence`. Analyzer supplies `Observation.confidence`
according to a policy still to be decided, and creates the observation timestamp.

A `Detection` is local to the client and does not represent the final shared target state.

## Observation

`Observation` represents a single client's report about a target.

It is produced after combining a local `Detection` with device position and orientation.

Conceptually it contains:

- `targetId` — identifier of the observed target;
- `observerId` — identifier of the client/device that created the observation;
- `latitude` — calculated target latitude;
- `longitude` — calculated target longitude;
- `confidence` — confidence of the local detection;
- `timestamp` — observation time.

The exact field set is already defined in `spec/data-model.md`.

## Target

`Target` represents the aggregated shared state of a physical target.

Conceptually it contains:

- `targetId` — stable identifier of the physical target;
- `latitude` — aggregated target latitude;
- `longitude` — aggregated target longitude;
- `probability` — system confidence that the target is currently at the reported location;
- `lastSeenAt` — time of the most recent relevant observation.

`Target` is calculated from one or more `Observation` records.

## Target identity

Targets are assumed to have recognizable identifiers or markers.

The initial design intentionally avoids complex target matching by spatial similarity.

If several clients report the same `targetId`, the system treats those reports as observations of the same physical target.

The identifier of a target is the id of its ArUco marker (see `01-architecture.md` → "Marker recognition").

## Probability

`probability` and local detection `confidence` are different concepts.

### Detection confidence

Represents the reliability of one local Observation, assigned by Analyzer; it is not an aggregated score across devices. Its estimation policy is not yet defined.

### Target probability

Represents how confident the whole system is in the current shared target state.

The intended behavior is:

- fresh observations increase target probability;
- observations from multiple independent clients increase confidence more than repeated observations from one client;
- old observations lose weight over time;
- probability decreases when no fresh confirmations arrive.

The initial probability formula and spatial grouping rules are defined in
`spec/target-aggregation.md`.

## Time decay

Target confidence must decrease over time when no new observations are received.

The initial half-life, expiry, timestamp and duplicate rules are defined in
`spec/target-aggregation.md`.

## Data flow

The intended flow is:

```text
Camera frame
   ↓
Go Marker Detector → Detection
   ↓ Runtime Camera API
Analyzer: device position + orientation
   ↓ compute global coordinates
Observation
   ↓ IPC
Map
   ↓ publish Observation
Broker
   ↓
Target Service
   ↓
Target
```

The resulting `Target` state is persisted through the DBMS and published back to clients.

## Ownership of data

### Marker Detector

Creates local `Detection` records.

### Analyzer

Consumes detections through Runtime and creates `Observation` records.

### Map

Consumes:

- local observations through IPC;
- aggregated `Target` updates from the central system.

### Target Service

Consumes:

- `Observation`.

Produces:

- aggregated `Target`.

### DBMS

Stores domain data but does not interpret its business meaning.

## Rules

- Shared structures must not be changed silently.
- When exact fields are fixed, move the formal contract into `spec/data-model.md`.
- If a field is added because one component needs it, update every affected producer and consumer.
- Do not add fields for hypothetical future use.
- Keep `confidence` and `probability` semantically distinct.
