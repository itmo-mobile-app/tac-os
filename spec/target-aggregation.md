# Target aggregation specification

Status: proposed for review in issue #91 (Sprint 3).

This specification defines how Target Service derives the five-field `Target` from
six-field `Observation` messages in `spec/data-model.md`. The algorithm is a
small, deterministic heuristic for an airsoft map, **not** a statistically
calibrated posterior probability. It may be refined after field measurements.

## Ownership and interfaces

- Analyzer assigns `Observation.confidence` and geographic coordinates.
  The method for assigning local confidence is a separate decision (#98).
- Map forwards the six Observation fields unchanged to Broker topic
  `observations`; Broker transports messages without interpreting them.
- Target Service validates and aggregates observations per `targetId`, computes
  `Target.probability`, `latitude`, `longitude`, and `lastSeenAt`, and
  publishes the five-field Target on `targets.updated`.
- DBMS executes SQL and stores data without knowing the aggregation formula.
  No new fields or topics are introduced by this specification.

## Initial constants

| Name | Value | Meaning |
| --- | --- | --- |
| `HALF_LIFE_MS` | `30000` | A local observation loses half of its weight every 30 seconds |
| `MAX_AGE_MS` | `120000` | Evidence expires at age >= 120 seconds |
| `FUTURE_TOLERANCE_MS` | `5000` | Maximum allowed device clock lead on arrival |
| `CLUSTER_RADIUS_M` | `20.0` | Maximum distance from a candidate anchor to a group member |
| `RECALC_INTERVAL_MS` | `5000` | Recalculate active targets for time decay |

Constants are initial prototype choices, not measured GPS/camera accuracy
guarantees. The same constants must be used in tests.

## Validation and per-observer state

On receiving a message at server time `nowMs`:

1. Parse exactly the six `Observation` fields from `spec/data-model.md`.
   Require nonempty `targetId` and `observerId`, finite numeric coordinates
   (`-90 <= latitude <= 90`, `-180 <= longitude <= 180`), and finite
   `0 <= confidence <= 1`. `timestamp` must be an integer Unix millisecond
   value representable by signed 64-bit integer. Reject malformed values.
2. Reject `timestamp > nowMs + FUTURE_TOLERANCE_MS`. Set the internal
   normalized observation time to `min(timestamp, nowMs)`. This avoids
   negative ages for small clock skew; it does not rewrite the wire message.
3. Reject a message if its normalized age is >= `MAX_AGE_MS`.
4. Keep one latest accepted observation per pair
   `(targetId, observerId)`. If the incoming normalized timestamp is less
   than or **equal to** the stored timestamp, ignore it, even when its
   coordinates or confidence differ. Otherwise replace the old value.
   Thus one device never counts as several independent confirmations.
5. Discard expired entries from active in-memory state during processing and
   periodic recalculation. `confidence = 0` is valid but carries no evidence.

Server time is the trusted clock for decay; `lastSeenAt` comes from accepted
normalized observation timestamps. Messages dropped by Broker are not retried
by this algorithm. Observer IDs identify devices only; true statistical
independence and clock synchronization are assumptions, not guaranteed here.

## Time decay and independent evidence

For each unexpired observation `i` at server time `t`:

```text
ageMs = max(0, t - normalizedTimestamp_i)
weight_i = confidence_i * 2^(-ageMs / HALF_LIFE_MS)
```

Exclude weight-zero records from candidate groups and from location estimates.
For a group `G` containing distinct observers, compute:

```text
probability(G) = 1 - product(1 - weight_i for i in G)
```

The result is in `[0, 1]`. Do **not** sum confidences or count repeated
reports from the same `observerId`. In particular, two colocated observations
with confidence 0.8 and 0.6 yield `1 - (0.2 * 0.4) = 0.92`, not 1.4.

These inputs are heuristic confidence scores: `Target.probability` expresses
relative trust in the **reported location**, not a calibrated probability that
a particular enemy exists or a proof of observer independence.

## Coordinate agreement and group selection

Use great-circle distance in meters (haversine, Earth radius `6371000` m)
between latitude/longitude pairs. Do not compare raw degree differences to a
threshold in meters.

For a given `targetId`, after filtering expired and zero-weight records:

1. Treat every remaining observation as a candidate anchor.
2. For each anchor, form a candidate group with all active observations no
   farther than `CLUSTER_RADIUS_M` from that anchor (including the anchor).
   This is **anchor-radius** grouping; two members may be up to 40 m apart.
3. Score each candidate using `probability(G)`; choose the largest score.
   For exact score ties, prefer the group whose maximum normalized timestamp
   is later; if still tied, choose the lexicographically smallest
   `observerId` of its anchor (bytewise UTF-8 order). Evaluate anchors in
   sorted observer ID order for reproducibility.
4. Compute the selected group's latitude and longitude using weighted
   arithmetic means, `sum(weight_i * coordinate_i) / sum(weight_i)`.
   This approximation is intended for a small local map away from the
   antimeridian and poles; it is not a global geodesic estimator.
5. Set `lastSeenAt` to the greatest normalized timestamp among **contributing**
   observations in the selected group, not the recalculation time.
6. Publish a Target with the winning group's location and probability.

A geographically incompatible observation does not increase the probability
of the winning group. No motion tracking, Kalman filtering, per-target
multi-cluster history or observer reputation is included in this increment.
A moving target may temporarily retain an older, more strongly supported
position until fresh evidence outweighs it.

## Periodic refresh, expiry, and persistence

Recompute active targets every `RECALC_INTERVAL_MS`, even if no message arrives,
so their published probability can decrease. Publish the updated Target with
the existing five fields. When all positive-weight observations expire, keep
the last published coordinates and `lastSeenAt` and publish **one** transition
to `probability = 0.0` (stale); do not emit repeated zero updates until new
evidence arrives. A target never observed with positive weight need not be
published. No extra `expired` field is introduced.

The calculation and deduplication state may initially be kept in Target
Service memory. Once its DBMS client is available, Target records can be
stored through the already specified SQL interface. The exact history schema,
recovery across process restarts, SQL cleanup and durable storage are **not**
defined by this contract; the current SQL subset has no `DELETE` or
transactions. An in-memory implementation must not claim restart persistence.

## Deterministic acceptance vectors

Each row starts with empty active state unless it explicitly says otherwise.
Use `t0 = 1800000000000` milliseconds. All test coordinates are in degrees.
`A` and `B` are distinct `observerId` values observing `targetId = "17"`.
Unless specified, the coordinates are `(0.0, 0.0)` and the observation time
is `t0`. Compare floating-point results within `1e-9`.

| Scenario | Observations / evaluation time | Expected |
| --- | --- | --- |
| One observer | A: 0.8; at t0 | P = 0.8, lat/lon = 0, lastSeenAt = t0 |
| Two independent observers | A: 0.8, B: 0.6; at t0 | P = 0.92, lat/lon = 0, lastSeenAt = t0 |
| Duplicate | Previous pair, replay A with the same timestamp; at t0 | P still 0.92, no extra evidence |
| Older report | A: 0.8 at t0, then A: 0.9 at t0 - 1; at t0 | P still 0.8 |
| Newer replacement | A: 0.8 at t0, then A: 0.5 at t0 + 1000; evaluate at t0 + 1000 | P = 0.5, lastSeenAt = t0 + 1000 |
| Half-life | A: 0.8, B: 0.6 at t0; evaluate at t0 + 30000 | Weights 0.4 and 0.3; P = 0.58, lastSeenAt = t0 |
| Conflicting location | A: 0.8, B: 0.6 at (0,0), C: 0.9 at (0,0.001); at t0 | Winning group A+B; P = 0.92, position (0,0) |
| Expired | A: 0.8, B: 0.6 at t0; evaluate at t0 + 120000 | P = 0; keep last position and lastSeenAt = t0 |
| Invalid confidence | A: 1.2 at t0 | Rejected; no Target emitted |
| Future timestamp | A: 0.8 at t0 + 5001; received at t0 | Rejected; no Target emitted |

Also cover zero confidence (does not increase P), out-of-range coordinates,
non-finite numbers, malformed JSON, and stable deterministic tie breaking.

## Reproduction

The Go implementation in Target Service should provide a deterministic pure
aggregation function accepting observations and `nowMs`; no wall-clock reads
inside it. A table-driven `go test` using the vectors above is the acceptance
test when implementation is added. Until then this document is a reviewed
design contract, **not** evidence that aggregation or DBMS persistence has
already been implemented.
