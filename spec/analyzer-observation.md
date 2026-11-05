# Analyzer observation specification

Status: proposed for review in issue #98 (Sprint 3). The review includes the agreement of the
local confidence policy by the Target Service and architecture owners.

This specification defines how Analyzer turns one detector result and the device pose into an
`Observation` from `spec/data-model.md`: the coordinate calculation, the local
`Observation.confidence` policy, and the boundary between ArUco detection and the required AI
recognition (C-13). Like `spec/target-aggregation.md`, it is a small, deterministic heuristic for
an airsoft map, **not** a calibrated measurement model. It may be refined after field tests.

## Ownership and interfaces

- Analyzer (`.tc` code) performs the calculation and assigns `Observation.confidence`.
- The Go marker detector exposes only `id`, `relativeAngle` and `distance`
  (`spec/camera-detector-protocol.md`). It never calculates coordinates or confidence.
- Map forwards the six `Observation` fields unchanged; Target Service aggregates them
  (`spec/ipc.md`, `spec/target-aggregation.md`).
- This specification adds no field, command or message. `Observation`, the `DETECT` protocol,
  `camera.detectMarkers()` and the Analyzer-to-Map IPC keep their approved shape.

## Inputs

For every `camera.detectMarkers()` result Analyzer needs the following values.

| Input | Unit and range | Source |
| --- | --- | --- |
| `id` | Integer `0..71`, ArUco marker id | Detection (`spec/runtime-api.md`) |
| `relativeAngle` | Degrees, `0` is the center of the frame, **positive to the right** | Detection |
| `distance` | Meters from the camera to the marker center, greater than 0 | Detection |
| device `latitude`, `longitude` | WGS84 degrees | Runtime location API (not specified yet) |
| device `heading` | Degrees in `[0, 360)`, measured **clockwise from true north**; the direction in which the **camera looks** | Runtime orientation API (not specified yet) |
| `timestamp` | Unix time in milliseconds when the detections were received | Runtime time API (not specified yet) |
| `observerId` | Device identifier | Not specified yet (`02-open-questions.md` → "Domain model") |

This document fixes the values and their units. The Runtime APIs that supply them are a separate
decision of the Runtime owner. On a laptop without sensors the position and heading may come from
configuration.

Assumptions of the calculation:

- the camera is held roughly level, so `distance` is used as the horizontal distance and pitch and
  roll are ignored;
- `heading` is the direction of the camera axis. A camera that is not aligned with the device must
  be corrected by the orientation API, not by Analyzer;
- `heading` refers to true north. If a sensor delivers magnetic north, the declination correction
  belongs to the orientation API.

## Calculation

The axes match the Map: north is up, east is right (`spec/map-rendering.md`). Angles grow clockwise.

Constants:

| Name | Value | Meaning |
| --- | --- | --- |
| `EARTH_RADIUS_M` | `6371000` | The radius also used by `spec/target-aggregation.md` |
| `MAX_RANGE_M` | `100` | Largest accepted `distance`; also the limit of the approximation below |
| `MAX_ABS_LATITUDE` | `80` | Largest accepted `abs(latitude)` of the device |
| `MAX_ABS_RELATIVE_ANGLE` | `90` | `abs(relativeAngle)` must be **less** than this |

Steps, with all angles converted to radians for `sin` and `cos`:

```text
bearing   = (heading + relativeAngle) mod 360        -- degrees clockwise from true north
north     = distance * cos(bearing)                  -- meters
east      = distance * sin(bearing)                  -- meters
latitude  = deviceLatitude  + degrees(north / EARTH_RADIUS_M)
longitude = deviceLongitude + degrees(east / (EARTH_RADIUS_M * cos(deviceLatitude)))
```

This is the local tangent plane approximation. It needs only `sin`, `cos` and arithmetic, which is
the minimum for `.tc` code. For `distance <= MAX_RANGE_M` it differs from the exact great-circle
destination by about `distance^2 / (2 * EARTH_RADIUS_M)`, that is at most one millimeter, which is
irrelevant next to the errors below.

Accuracy: the result is only as good as its inputs. Device GPS error is typically several meters.
A heading error of 5 degrees moves the target by about 1.7 m at 20 m. The distance comes from a
pinhole camera with default parameters (60 degrees horizontal field of view, 0.10 m marker) that are
not calibrated (`spec/camera-detector-protocol.md`), so its error is not measured yet. No accuracy
better than meters may be claimed for an `Observation`.

Precision: arithmetic uses `Float` (IEEE 754 binary64). `latitude` and `longitude` must keep at least
six decimal places (about 0.1 m) when they are serialized; rounding to fewer places is not allowed.

## Validation

Analyzer creates **no** `Observation` for an input that breaks a rule. It never guesses a missing
value; it reports the problem locally (for example with `runtime.log`) and goes on with the next
frame. Invalid detections do not affect the valid ones of the same result.

| Reason | Rule | Applies to |
| --- | --- | --- |
| `INVALID_POSITION` | Device position unavailable, not finite, `abs(latitude) > MAX_ABS_LATITUDE`, or `longitude` outside `[-180, 180]` | The whole result |
| `INVALID_HEADING` | Device heading unavailable, not finite, or outside `[0, 360)` | The whole result |
| `INVALID_ID` | `id` is not an integer in `0..71` | One detection |
| `INVALID_ANGLE` | `relativeAngle` not finite or `abs(relativeAngle) >= MAX_ABS_RELATIVE_ANGLE` | One detection |
| `INVALID_DISTANCE` | `distance` not finite, `<= 0`, or `> MAX_RANGE_M` | One detection |
| `INVALID_RESULT` | The calculated `longitude` is outside `[-180, 180]` (the antimeridian is not supported) | One detection |

An empty detection list is a successful result: no `Observation` and nothing to report.
The reason names only label the cases in the test vectors; they are not a wire value.

## Observation assembly

For every valid detection Analyzer creates one `Observation` with exactly the six fields of
`spec/data-model.md`:

| Field | Value |
| --- | --- |
| `targetId` | The marker `id` as a decimal string, e.g. `"17"` |
| `observerId` | The device identifier |
| `latitude`, `longitude` | The result of "Calculation" |
| `confidence` | The local confidence below |
| `timestamp` | The `timestamp` input |

Observations follow the ascending `id` order of the detections. Team affiliation is not part of an
`Observation`; Analyzer derives it from the marker id (`team = id / 12`) for its own use. Analyzer
then sends each `Observation` to Map as specified in `spec/ipc.md`.

## Local confidence policy

`Observation.confidence` is the reliability of **one local report of the target location**.
It is not a measure of the ArUco decoding, because the detector provides no score. A recognized
game marker is treated as a reliable identification, and the uncertainty comes from the geometry.
It is different from `Target.probability`, which Target Service computes from several observations.

```text
confidence = CONFIDENCE_MAX / (1 + distance / CONFIDENCE_DISTANCE_SCALE_M)
```

| Name | Value | Meaning |
| --- | --- | --- |
| `CONFIDENCE_MAX` | `0.9` | Upper bound: GPS and heading errors always remain, so the value is never `1.0` |
| `CONFIDENCE_DISTANCE_SCALE_M` | `25` | The distance at which the confidence has fallen to half of `CONFIDENCE_MAX` |

| `distance` (m) | 2 | 10 | 25 | 50 | 100 |
| --- | --- | --- | --- | --- | --- |
| `confidence` | 0.8333 | 0.6429 | 0.45 | 0.3 | 0.18 |

Reasons for this shape:

- the position error caused by a heading error grows with the distance, so a farther target is less
  reliable;
- the value stays above 0 for every accepted detection, because a `confidence` of 0 carries no
  evidence in `spec/target-aggregation.md`;
- one scale constant keeps the policy easy to explain and to tune.

For example, one observation at 10 m gives a target probability of 0.6429 in Target Service, and two
independent observers at 10 m give `1 - (1 - 0.6429)^2 = 0.8724`.

Deliberately **not** used, because the inputs do not exist yet: the GPS and heading accuracy, the
position of the marker near the edge of the frame (lens distortion is not corrected), a calibrated
camera, and AI recognition. The constants are initial prototype choices to be tuned in field tests,
not measured accuracy. The same constants must be used in tests.

## AI recognition boundary

The instructor requires a demonstration of AI-based recognition of players and targets (C-13,
FR-2). OpenRouter was suggested as a possible vision-model provider; neither it nor any model is
approved, and the conflict with NFR-1 and C-8 is unresolved (`docs/requirements.md`). Follow-up
issue #104 prepares the proof of concept and the contract. This document only fixes the boundary
that the Analyzer calculation needs.

| Stays with ArUco | Expected from AI recognition (proposal for #104, not approved) |
| --- | --- |
| `targetId` (the marker id), the angle and the distance of an `Observation` | Show that a player or target is visible in the camera image |
| Works without internet access (NFR-1) | The result form is to be defined: presence, class and a region in the frame, versus a stable player identity |
| Calculation and confidence in this document | May later cross-check or adjust the local confidence through a separately reviewed contract |

Rules:

1. The ArUco detection remains the only source of `targetId`, `relativeAngle` and `distance`.
   A model answer is never turned into a marker id, a distance or a coordinate without tests that
   show it is trustworthy. An answer without a marker id cannot create an `Observation`, because
   `targetId` is required.
2. The AI result does not change this calculation, the `confidence` formula, the six `Observation`
   fields or the `DETECT` protocol. It must not use the `DETECT` channel; camera frames never
   travel over it (`spec/camera-detector-protocol.md`).
3. If the AI recognition is unavailable (no internet, timeout, rate limit, invalid answer), Analyzer
   keeps producing ArUco observations (NFR-1, NFR-6).
4. Target Service keeps the aggregation and Map keeps the display; Analyzer remains the owner of
   world coordinates.

Questions handed to #104, with the full list in `docs/ai-context/02-open-questions.md` →
"AI target recognition":

- what exactly the model must recognize, and the measurable criterion;
- which process captures the image and calls the model, and the separate interface for it;
- how the result relates to ArUco detections, `targetId` and `confidence`;
- the decision about the offline requirement NFR-1 and constraint C-8, and the instructor's approval;
- the limits: images and consent, the API key kept outside git, latency, cost, failures and the
  offline fallback.

## Changes needed outside this document

These are **not** approved here; each needs its owner's decision.

1. `Float` in the language, bytecode and VM, proposed in #106 and not implemented yet (#107, #108).
2. `sin` and `cos`, or equivalent functions, for `.tc` code: not part of the language or of the
   Runtime API (Runtime and Language owners).
3. Runtime location, orientation and time APIs with the units of "Inputs" (Runtime owner).
4. Access to the fields of the detections. `camera.detectMarkers()` returns a JSON `String`, and
   `.tc` has neither a JSON parser nor collections (`spec/runtime-api.md`).
5. Formatting `Float` values into the JSON `String` that `runtime.ipc_send` sends.

Until they exist, this document and its vectors are a reviewed design contract, **not** evidence
that the Analyzer calculation is implemented in `.tc`.

## Test vectors

`spec/analyzer-observation-vectors.json` holds the vectors. Each one has an `input` (observer id,
timestamp, device position, device heading and the detections) and the `expected` result: the list of
`Observation` objects, and the rejected inputs with their reason. A position or heading that is
`null` means "unavailable". Floating-point values are compared within `1e-9`, as in
`spec/target-aggregation.md`. Every expected `Observation` has exactly the six fields of
`spec/data-model.md` and is valid for `spec/ipc.md` after adding `"type":"OBSERVATION"`.

| Vector | Expected |
| --- | --- |
| `no-detections` | Success, no `Observation` |
| `single-east` | One `Observation`: 10 m east of the device |
| `two-detections-heading-wraps` | Two `Observation` objects; the detections are those of the detector test mode `two`; heading 350 wraps the bearing |
| `southwest-in-southern-western-hemisphere` | One `Observation`; checks the signs of latitude and longitude |
| `maximum-range-north` | One `Observation` at `distance = MAX_RANGE_M` |
| `position-unavailable` | `INVALID_POSITION` |
| `latitude-out-of-range` | `INVALID_POSITION` (latitude 91) |
| `latitude-beyond-supported` | `INVALID_POSITION` (latitude 80.5) |
| `heading-unavailable` | `INVALID_HEADING` |
| `heading-360` | `INVALID_HEADING` |
| `id-out-of-range` | `INVALID_ID` (id 72) |
| `angle-out-of-range` | `INVALID_ANGLE` (90 degrees) |
| `zero-distance` | `INVALID_DISTANCE` |
| `beyond-maximum-range` | `INVALID_DISTANCE` (100.01 m) |
| `invalid-detection-does-not-hide-valid-one` | One `Observation` for the valid detection, one rejected |
| `antimeridian` | `INVALID_RESULT` |

For example, the vector `single-east`:

```json
{
  "input": {
    "observerId": "device-1",
    "timestamp": 1800000000100,
    "position": {"latitude": 59.9505, "longitude": 30.311},
    "heading": 90.0,
    "detections": [{"id": 17, "relativeAngle": 0.0, "distance": 10.0}]
  },
  "expected": {
    "observations": [
      {"targetId": "17", "observerId": "device-1", "latitude": 59.9505,
       "longitude": 30.311179595644, "confidence": 0.642857142857, "timestamp": 1800000000100}
    ],
    "rejected": []
  }
}
```

The expected values were calculated with the formulas above in binary64 and checked against an
independent great-circle calculation (destination point, haversine distance and initial bearing).
The positions differ by less than 0.05 mm, the distances by less than 0.02 mm and the bearings by
less than 0.0001 degrees.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md`:

- the Runtime location, orientation and time APIs, including true versus magnetic north and any
  accuracy values that could improve the confidence;
- the device identifier format;
- calibration of the camera and of the confidence constants with field measurements;
- the AI recognition design (#104).
