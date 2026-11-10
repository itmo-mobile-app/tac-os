# Runtime API specification

Status: approved

This document defines how the VM calls into the Runtime, and the first Runtime function used by the
Compiler → VM integration test. It does not cover IPC (see `spec/ipc.md`) or the Broker (see
`spec/broker-protocol.md`).

## Call mechanism

- The VM executes `CALL_RUNTIME` (see `spec/bytecode.md`) with a constant-pool string naming the
  Runtime function as `<namespace>.<function>` (e.g. `runtime.log`).
- Arguments are pushed onto the operand stack by the caller from left to right.
  Before the call, the VM pops them in reverse stack order and passes them to
  the Runtime function in signature order.
  For `foo(a, b)`, the caller pushes `a` and then `b`; the VM pops `b` and then
  `a`, and invokes the Runtime function as `foo(a, b)`.
- Every Runtime function returns exactly one value, which the VM pushes onto the operand stack after the
  call. A function that conceptually returns nothing pushes `Int 0`. This matches the return convention
  for bytecode functions in `spec/bytecode.md`, so the compiler handles calls to bytecode functions and
  to Runtime functions the same way.
- Calling an unknown `<namespace>.<function>` name is a load-time error (checked when the VM resolves
  Runtime calls after loading the bytecode, not at CALL_RUNTIME execution time).

## First Runtime function

```text
runtime.log(message: String)
```

Writes `message` followed by a newline (`\n`) to the process's standard output and returns `Int 0` (it
has no meaningful result).
This is the minimal function needed for the first Compiler → VM → Runtime integration test.

## Camera API contract

The initial VM-facing call is:

```text
camera.detectMarkers() -> String
```

The call takes zero arguments and returns exactly one `String` value, consistent
with the existing VM/Runtime call ABI. On success, that String is a compact
JSON array of detections: `id` (Int, ArUco ID 0..71), `relativeAngle`
(finite number in degrees; right positive), and `distance` (finite number
in meters, greater than 0). Example:

```json
[{"id":17,"relativeAngle":12.5,"distance":3.2}]
```

No recognized marker is the successful return value `"[]"`. Camera failure,
timeout, malformed helper response or terminated helper is a Runtime error,
not `"[]"` and not a JSON error object returned to application code.

This is a **temporary String ABI**, not the eventual typed detection API.
Current `.tc` and VM have no `Float` or collection representation and the
Compiler cannot yet call every zero-argument Runtime function in an expression.
The final `Detection[]` interface, geographic calculations in `.tc`, and
VM-level recoverable error handling require later language/VM work. Sprint 2
may verify the C++ Runtime directly with synthetic detector responses;
no `.tc` JSON parser is introduced by this decision.

The Runtime implementation is a client of the private Go/OpenCV detector over
a Unix-domain stream socket; Runtime starts and reuses the helper process.
The complete private wire format (`DETECT`, JSONL framing, success/error
envelopes, socket path, timeouts and lifecycle) is defined in
`spec/camera-detector-protocol.md`. Application code cannot open this socket
directly. Analyzer will eventually combine detector results with device
position and orientation to create `Observation` records sent to Map via
the **separate** `spec/ipc.md` contract.

`camera.detectMarkers` is registered in the C++ Runtime with arity zero.
Successful calls return `ValueKind::String`; all camera, helper, protocol and
transport failures throw `tacos::runtime::RuntimeError`. A valid helper error
(`CAMERA_UNAVAILABLE`, `DETECTION_FAILED` or `BAD_REQUEST`) preserves the
connection for a later call. A transport/protocol failure stops the owned helper
and makes later camera calls on that Runtime fail without an automatic restart.

The existing three-argument C++ Runtime constructor remains available. An
additional overload accepts `CameraOptions` as its fourth argument:

```cpp
CameraOptions camera;
camera.helper_path = "/path/to/marker-detect"; // default: marker-detect on PATH
camera.fake_mode = "two"; // default: empty, meaning the real camera
Runtime runtime(output, "/tmp/tacos-wm.sock", "/tmp/tac-os-analyzer-map.sock", camera);
```

The executable is launched directly, without a shell, with `-socket` and,
when configured, `-fake`. Host options are not `.tc` arguments and do not change
the VM value layout. They are validated on the first camera call; supported
fake modes are `two`, `empty`, `camera-unavailable` and `detection-failed`.
The helper executable must implement `spec/camera-detector-protocol.md`.

One Runtime must not be called concurrently from two threads. Each camera
client owns only its child and connection. Simultaneous camera owners inside
one process are unsupported because the default socket pathname contains the
Analyzer PID. No new `.tc` types, JSON parser or language support are added.
Direct C++ contract tests and shared Go fixtures are documented in
`docs/testing/camera-runtime.md`.

## IPC API: Analyzer to Map

The Runtime provides IPC between two independent VM
processes using a Unix-domain stream socket.

The Runtime is embedded in each VM process.
No separate Runtime process is required.

### Functions

```text
runtime.ipc_listen() -> Int
runtime.ipc_receive() -> String
runtime.ipc_send(message: String) -> Int
```

### runtime.ipc_listen()

Creates a Unix-domain stream socket at:

`/tmp/tac-os-analyzer-map.sock`

Map uses this function to start listening for
Analyzer connections.

Returns Int 0 on success.
Throws RuntimeError on failure.

### runtime.ipc_receive()

Accepts an Analyzer connection if necessary and
receives one newline-terminated message.

Returns the message as a String without the
trailing newline.

The current implementation supports messages
up to 64 KiB.

Connection state is retained for subsequent
messages.

Waiting for a connection or the next message has no idle timeout: observations
may arrive infrequently. Once a partial message has arrived, five seconds
without further bytes causes a RuntimeError. A connection closed between
messages (including an empty listener probe) is discarded and the listener
continues waiting. EOF within an incomplete message remains an error.

### runtime.ipc_send(message: String)

Connects to Map if not already connected.

Sends the given String as UTF-8 JSONL with a
trailing newline.

Returns Int 0 on successful local transmission.

Returns Int -1 and writes a diagnostic to standard error if Map is unavailable,
sending fails, or message framing is invalid. Analyzer continues executing and
can inspect the result; no message is retried automatically. A wrong argument
type remains a RuntimeError. Successful local transmission does not guarantee
that Map accepted the message.

The current send deadline is 1000 milliseconds.

### Lifecycle

Runtime owns the socket descriptors.

The Runtime destructor closes open descriptors
and removes the listener socket file on
normal shutdown.

### Delivery semantics

Messages use at-most-once delivery.

There is no acknowledgement, automatic retry,
or offline queue.

The wire schema is defined in spec/ipc.md.

## Open questions

### Minimal String data helpers (#127, proposal)

**Pending VM, Runtime, Analyzer, Map and architecture owner review.** The
approved API above is unchanged. These names must not be implemented or
treated as approved solely because they appear in this subsection.
Selection rationale and source examples are in
[`language.md`](language.md#observation-and-target-data-access-127-proposal).

| Proposed signature | Result and concrete use |
| --- | --- |
| `json.arrayLength(source: String) -> Int` | Number of detector objects; `[]` gives 0 |
| `json.arrayObject(source: String, index: Int) -> String` | One flat detection object, zero-based index |
| `json.getInt(source: String, key: String) -> Int` | Exact i64 `id`, `timestamp` or `lastSeenAt` |
| `json.getFloat(source: String, key: String) -> Float` | Finite binary64 angle/distance, coordinates or confidence/probability |
| `json.getString(source: String, key: String) -> String` | Decoded `targetId` or `observerId` |
| `json.quote(value: String) -> String` | One JSON string token, including quotes and required escaping |
| `text.concat(left: String, right: String) -> String` | Concatenated text for an application-built JSON object |
| `text.fromInt(value: Int) -> String` | Base-10 i64 token without quotes, grouping or leading `+` |
| `text.fromFloat(value: Float) -> String` | Locale-independent finite JSON number token |

#### Supported JSON subset

- Input must be valid UTF-8 JSON with exactly one root value; JSON whitespace
  around it is allowed. Trailing garbage, duplicate decoded keys, malformed
  escapes, unpaired surrogate escapes and invalid UTF-8 are rejected.
- Object readers accept a flat object of String and JSON number values only.
  Arrays are accepted only by `arrayLength`/`arrayObject`, and contain only
  those flat objects. Empty arrays are valid. Nested objects/arrays, booleans
  and null are outside this increment. The entire source is validated on each
  read, including unrequested fields or elements. Object key order is irrelevant.
- Keys are matched by decoded, case-sensitive content. Missing keys and wrong
  field types are errors, never empty String, zero or absent-value sentinels.
  These readers do not enforce domain field sets or ranges: `.tc` still owns
  schema/range validation according to `data-model.md` and `ipc.md`.
  Support for exact-key-set validation without a DOM is an open review question
  below; unknown domain fields must not silently become accepted wire data.
- `getInt` accepts integer lexical tokens only, in signed i64 range. `17.0`,
  `17e0`, `"17"` and overflowing integers are errors. It does not pass through
  binary64, so `9007199254740993` remains exact. `-0` yields Int 0.
- `getFloat` accepts JSON number syntax (including integer/exponent tokens),
  converted directly to binary64 with round to nearest, ties to even.
  Overflow to a non-finite value is an error. Finite subnormals, signed zero
  and underflow to zero are allowed. This explicit JSON conversion does not
  introduce an implicit `.tc` Int-to-Float conversion.
- `arrayObject` returns an independent immutable String containing the selected
  object (original substring or equivalent compact JSON). Its lifetime does not
  depend on retaining the source. It is not a borrowed pointer or object handle.

#### Output and resource rules

- `quote` escapes quotes, backslashes and U+0000–U+001F, including CR/LF.
  It preserves decoded text and emits valid UTF-8 JSON without raw newlines.
  It is for **values**, never quoting an already serialized object. Strings
  constructed internally may contain controls; JSON encoding must escape them.
- `fromFloat` emits a shortest decimal that round-trips to the same binary64
  value, with `.` as the decimal separator; exponent notation is allowed.
  It never rounds coordinates to a fixed low number of decimal places.
  Both signed zeros are legal JSON numbers. Non-finite input is an error.
- All helpers are pure with respect to external services. They do no camera,
  IPC, network or WM I/O, and never add a newline. `runtime.ipc_send` remains
  responsible for the one transport newline. Application JSON assembly selects
  exactly six domain fields or those same six plus IPC `type: "OBSERVATION"`.
- Proposed bounds: 65,536 UTF-8 bytes per input/output String, excluding any
  transport newline, and at most 72 array elements (the detector marker range).
  Bounds are checked before allocation/concatenation; an escape-expanded
  `quote` result may exceed the bound even when its input fits.
  This does not enlarge any downstream transport's existing limit.
- No parser state, cached document handle or String storage is shared between
  independent VM executions. Results use the existing String runtime kind;
  numeric results use existing Int or Float kinds.

#### Errors and approval gates

Wrong argument runtime kinds, malformed/unsupported JSON, missing/wrong-type
fields, numeric overflow, negative/out-of-range array indices, byte/element
limit violations and allocation failure produce a RuntimeError. Diagnostics
identify the helper and failing key/index or limit without logging the entire
payload. A declared call with a wrong argument type is rejected earlier by
Compiler; invalid bytecode is still checked by VM/Runtime.

The existing uncaught Runtime error path terminates execution with a nonzero
status. This is an explicit limitation of this proposal, **not** the production
Map policy. In contrast, the existing `runtime.ipc_send` still returns -1 on
transport/framing failure; helpers do not change that approved behavior.

Before #132/#133 proceed, owners must decide:

1. Whether these nine helpers are sufficient, or narrow schema-aware codecs
   would reduce total complexity. Such codecs may validate/encode fields only;
   they must never compute coordinates, confidence, probability or map pixels.
2. How `.tc` can reject malformed, wrong-type, missing/extra-field domain
   messages and continue without a fatal RuntimeError. A bounded validation
   API versus recoverable errors needs explicit agreement with #128; neither
   a general exception model nor a JSON DOM is approved here.
3. The VM ownership/reclamation rule for generated immutable Strings. Per-call
   size bounds do not bound a long-running process. No GC, global String arena,
   leaked lifetime or new pointer ABI is implicitly approved.
4. Agreement on the proposed byte/element limits with existing camera/IPC and
   future Broker limits. Parser implementation/library selection is not fixed.

See [`data-contract-review.md`](../docs/data-contract-review.md) for required
reviews and acceptance vectors. #128 owns device/time, networking and WM calls;
this section adds no signatures for those services and changes neither the
Camera String result nor the private detector protocol.

Not decided here; see `docs/ai-context/02-open-questions.md` → "Runtime":

- the remaining Runtime API surface (threads, IPC, location, orientation, networking, files, time,
  Activity lifecycle, WebView bridge, Window Manager access) — added incrementally, one concrete
  scenario at a time;
- how recoverable Runtime call errors are represented inside `.tc` programs beyond the initial failure-reporting behavior.
