# IPC specification

Status: approved for the initial Analyzer-to-Map Observation message (issue #59).

This defines application IPC between two separate VM processes: Analyzer sends a finished
`Observation` to Map. The private Runtime-to-Go detector IPC is a different contract (issue #60).

## Transport

- Unix-domain stream socket at `/tmp/tac-os-analyzer-map.sock` on the same Linux device.
- Map listens; Analyzer connects and reuses the connection for successive messages.
- Map removes a stale socket file before binding and its own socket file on normal shutdown.
- Communication is one-way from Analyzer to Map; there is no acknowledgement or retry.

## Framing and encoding

- Each message is one UTF-8 JSON object on one line, terminated by `\n`.
- Each flat IPC object contains exactly `type` and the six fields of `Observation` from
  `spec/data-model.md`. All seven fields are required. Unknown fields are rejected.
- The only supported `type` is `"OBSERVATION"`; other types are rejected.
  Additional message types are not part of Sprint 2.
- JSON keys are case-sensitive. Numeric fields must be JSON numbers (not strings);
  non-finite numbers are invalid.

## Message: `OBSERVATION`

Analyzer computes geographic coordinates from detector angle/distance and device
position/orientation before producing an `Observation`. Map does not recalculate coordinates.

```json
{
  "type": "OBSERVATION",
  "targetId": "17",
  "observerId": "device-1",
  "latitude": 59.9505,
  "longitude": 30.311,
  "confidence": 0.85,
  "timestamp": 1792255200000
}
```

The on-wire representation must be serialized on one line, followed by `\n`.

| Field | JSON type | Validation |
| --- | --- | --- |
| `type` | string | Exactly `"OBSERVATION"` |
| `targetId` | string | Nonempty, decimal ArUco marker ID string |
| `observerId` | string | Nonempty client/device ID; ID allocation remains unspecified |
| `latitude` | number | Finite WGS84 degrees, -90 to 90 inclusive |
| `longitude` | number | Finite WGS84 degrees, -180 to 180 inclusive |
| `confidence` | number | Finite value 0.0 to 1.0 inclusive |
| `timestamp` | integer | Unix milliseconds |

`Observation.confidence` represents the reliability of **one local observation**, as
assigned by Analyzer. The Go detector prototype does not emit a confidence score;
the production policy for this value is a separate open question. A synthetic
test fixture may supply a valid example value but does not define that policy.

`Target.probability` is different: Target Service will aggregate independent
observations of the same target, accounting for coordinate agreement and age.
The aggregation/time-decay formula is not specified in this IPC contract.

`relativeAngle` and `distance` belong to the local detector/Analyzer workflow and
are not sent to Map; global coordinates take their place.

## Map-to-Broker forwarding

Map forwards the same six-field domain `Observation` to Broker topic `observations`
using `spec/broker-protocol.md`, stripping only the IPC `type` field:

```text
PUB observations {"targetId":"17","observerId":"device-1","latitude":59.9505,"longitude":30.311,"confidence":0.85,"timestamp":1792255200000}
```

The broker JSON payload must contain exactly the six `Observation` fields from
`spec/data-model.md`, with unchanged values.

## Delivery and validation behavior

- At-most-once, no acknowledgement, no resend and no offline queue.
- If Map is unavailable, Analyzer must not crash or wait indefinitely; it may
  drop that message and report the send failure locally.
- Map rejects malformed JSON, missing/extra fields, unknown `type`, incorrect
  data types or out-of-range values; rejected messages are not forwarded to Broker.
- Map-restart reconnection behavior is not specified in this increment.

## Contract checks

The valid IPC example above, when serialized as a single line, must be accepted;
the Broker payload must equal the six-field object after removing `type`.
The following modifications to that example must be rejected:

- `type` set to `"TARGET_DETECTED"` or any other unrecognized value;
- `confidence` set to `1.2` or `"0.85"`;
- `latitude` set to `91` or `longitude` to `-181`;
- empty `targetId`, missing `observerId`, or added `distance`;
- invalid JSON.

This is a **wire contract**. The current `.tc` language/VM cannot yet compute
floating-point geographic coordinates or store structured detections. Sprint 2
may test IPC by sending prebuilt JSON Strings from separate processes; native
`Float` arithmetic and collection ABI are deferred.

## Open questions

See `docs/ai-context/02-open-questions.md` → "IPC scenario" for reconnection
after Map restarts. New message types can be specified if future scenarios need them.
