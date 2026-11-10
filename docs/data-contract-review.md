# Observation and Target data contract review (#127)

Status: proposal, no owner approvals recorded. This is a review checklist,
not a claim that issue #127 is complete. The proposed calls are not implemented.

## Review scope

The existing data model, IPC and detector contracts are unchanged. The proposal
in `spec/language.md`, `spec/bytecode.md` and `spec/runtime-api.md` describes
scalar locals and nine String-based helpers, without a new grammar, object
model, bytecode version or ABI. Only JSON decoding/encoding and formatting
would run in Runtime. Geometry and local confidence remain in Analyzer `.tc`;
projection remains in Map `.tc`; aggregation remains in Target Service.

| Required reviewer | Area | Evidence/status |
| --- | --- | --- |
| @pypynyaa | VM kinds, calls, generated String lifetime and resource limits | Pending |
| @pateWY | Runtime helper surface, parsing, errors and alignment with #128 | Pending |
| @slutswarming | Detector extraction and Observation assembly in Analyzer | Pending |
| @mattthewww | Target extraction, invalid-update handling and Map needs | Pending |
| @TheGeniusOfEternity | Architecture boundaries and minimum sufficient scope | Pending |

Record actual review links and decisions here before changing status to
approved. Required unresolved decisions are listed under "Errors and approval
gates" in `spec/runtime-api.md`: exact-field-set validation, recoverable errors,
bounded String reclamation, limits and helper selection. A document merge alone
must not be used as evidence of approval by all five owners.

## Example expectations

The complete `.tc` sketches in `docs/examples/data-contract/` use existing
syntax and the proposed signatures. They are documentation inputs, not tests
of the current toolchain. Current Compiler must reject unregistered helpers;
registering fake helpers in production to compile these docs is not intended.

- `analyzer.tc`: zero detections produce no sends. For marker 17, zero relative
  angle, 10 meters, device coordinates 0/0 and heading north, the resulting
  latitude is approximately `0.00008993216059187306`, longitude `0.0`, confidence
  approximately `0.6428571428571429`, timestamp `1800000000100`, observerId
  `device-1` and targetId `17` as a String. The wire object has exactly those
  six fields plus `type: "OBSERVATION"`. The transport appends one newline.
  Nonzero angles are explicitly unsupported by this fixture, not by Analyzer's
  general calculation contract. The example does not supply location/time APIs.
- `map.tc`: the five-field Target yields x approximately `480.0`, y approximately
  `160.0`, probability `0.85`, unchanged targetId and exact Int lastSeenAt.
  Pixels are still Float; rendering/rounding is deliberately owned by #128/#136.
  No window or Broker connection is opened by this sketch.
- Map forwarding uses the same six-field serializer with `forIpc = false`
  after extracting the validated Observation fields. Removing IPC `type` must
  not add fields, recompute coordinates or change numeric values. Textual JSON
  key order/number spelling may differ; decoded values must remain equal.

## Implementation acceptance vectors for #132/#133

These are requirements for subsequent implementation tests, not results of
running helpers that do not exist yet.

| Case | Expected |
| --- | --- |
| Detector arrays with 0/1/2 objects | 0/1/2 iterations; preserve detector order |
| Object keys reordered, supported surrounding JSON whitespace | Same extracted values |
| ID 17 and timestamp 9007199254740993 | Exact Int values; ID formats to String `17` |
| Int timestamp limits -9223372036854775808 and 9223372036854775807 | Exact parse/format round trip |
| Missing key, duplicate key, wrong type, null, nested object or trailing bytes | RuntimeError, no fabricated defaults |
| Index -1 or equal to array length | RuntimeError |
| Malformed escape, invalid UTF-8 or unpaired surrogate | RuntimeError |
| `getInt` on `17.0`, `17e0` or an overflowing integer | RuntimeError |
| `getFloat` on `1`, `1.25` or `1.25e1` | Float 1.0, 1.25 or 12.5 |
| Numeric overflow such as `1e9999`; non-finite formatting | RuntimeError |
| observerId containing quote, backslash, newline and Unicode | Quote/decode round trip; no raw CR/LF in wire JSON |
| Float 59.95050012345678 | JSON round trip preserves binary64, without fixed precision truncation |
| Input/output exactly 65,536 bytes; then one byte more | At bound accepted if otherwise valid; above bound fails |
| Quoting expands output beyond bound; array with 73 objects | RuntimeError before unbounded allocation |
| IPC message with seven fields; Broker Observation with six; Target with five | Exact approved field sets, no detector-only fields |
| Wrong arity/type, mixed Float/Int, String `+`, unknown helper | Compile-time error |
| Two VM executions repeatedly parse/format independent messages | No shared state, dangling results or unbounded retained Strings |
| Older Runtime loads bytecode naming an unknown helper | Load-time error |
| Existing hello, Float, IPC and map projection scenarios | Unchanged behavior |

Production Map must reject invalid/extra-field updates and continue displaying
the previous state. The error/validation mechanism to support this remains an
approval gate coordinated with #128; the current fatal helper-error proposal
alone cannot provide this guarantee.

## Handoff

- #132: register approved calls/return types and camera signature in Compiler;
  add positive/negative type checks without introducing new syntax.
- #133 with Runtime owner: implement approved primitives and memory ownership,
  validate bytecode/runtime kinds and run real Compiler-to-VM acceptance cases.
- #128/#134–#136: separately agree device/time, transport, errors and WM calls.
- #138/#139: turn the sketches into real application increments only after the
  required operations and validation path are approved and implemented.
- Request the five reviews on the PR; keep #127 open until approval evidence
  and required CI are present. No reviews have been requested by this document.
