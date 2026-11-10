# Runtime API specification

Status: existing call ABI, Camera and IPC contracts approved; Sprint 4 additions
below are a proposal for owner review in [#128](https://github.com/itmo-mobile-app/tac-os/issues/128).

This document defines the VM-facing Runtime boundary. Wire formats remain in
[`ipc.md`](ipc.md), [`camera-detector-protocol.md`](camera-detector-protocol.md),
[`broker-protocol.md`](broker-protocol.md) and [`window-manager.md`](window-manager.md).
The Sprint 4 proposal adds only operations needed by Analyzer and Map; it does
not approve an implementation or change those wire formats.

## Implementation baseline

Checked against `main` at `3877600` on 2026-11-11, including the actual Compiler
signature table, Runtime dispatch table and VM value conversions:

| Function / capability | C++ Runtime / VM | Current `.tc` Compiler |
| --- | --- | --- |
| `runtime.log`, `runtime.ipc_listen`, `runtime.ipc_receive`, `runtime.ipc_send` | Implemented | Registered |
| `camera.detectMarkers` | Implemented, zero arguments, String result | Not registered yet |
| `window.create`, `window.show`, `window.close`, `window.quit` | Implemented | Not registered yet |
| Float arithmetic and map projection | VM v2 implemented | Implemented; see `app/map/tc/projection.tc` |
| Float arguments/results across VM ↔ Runtime | Not implemented: Runtime `Value` is Int/String; VM rejects Float arguments | New signatures alone are insufficient |
| Device/time, Broker, WM drawing/events and bounded IPC polling below | Proposed, not implemented | Not registered |

Source: `runtime/src/runtime.cpp`, `runtime/include/tacos/runtime/runtime.hpp`,
`vm/include/tacos/vm/vm.hpp`, `compiler/src/main/kotlin/tacc/Checker.kt`.
The separate String/JSON data-access proposal is
[#127 / PR #152](https://github.com/itmo-mobile-app/tac-os/pull/152), inspected at
`271b381`. Its helpers and recoverable validation remain unapproved; this
document does not copy or replace that proposal.

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
Compiler and VM now support scalar `Float`, but the Runtime value boundary
remains Int/String and there is no collection representation. The Compiler
still needs to register this zero-argument function with a usable String result.
Typed `Detection[]`, detection-field extraction and recoverable validation
require separately reviewed work (#127, #132, #133). The String result and
camera error behavior remain unchanged; no `.tc` JSON parser is introduced here.

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

## Existing Window API

These functions are already in the Runtime dispatch table. This table documents
their existing signatures; it does not claim Compiler registration or bounded I/O.

| Signature | Success | Existing failure behavior |
| --- | --- | --- |
| `window.create(title: String, width: Int, height: Int) -> Int` | Sends `CREATE` for window `map`; returns 0 | Nonpositive dimensions, wrong types, transport or unsuccessful WM response: RuntimeError |
| `window.show() -> Int` | Sends `SHOW` for `map`; returns 0 | RuntimeError |
| `window.close() -> Int` | Sends `CLOSE` for `map`; returns 0 | RuntimeError |
| `window.quit() -> Int` | Sends `QUIT`; returns 0 | RuntimeError |

`window.quit` is a legacy administrative operation. Analyzer/Map shutdown must
not call it: [`activity-lifecycle.md`](activity-lifecycle.md) requires leaving
the independent WM process running. The existing WM client lacks deadlines and
event subscription; the Sprint 4 requirements below must be implemented before
claiming that a stalled WM cannot hang the VM.

## Sprint 4 minimum API (#128, proposal)

**Pending review; no owner approval is inferred.** All signatures, status codes
and new deadlines in this section are candidates for agreement. Existing
Camera/IPC signatures, results, errors and wire schemas above are preserved.

### Values and recoverable failures

- Keep `CALL_RUNTIME`, argument order, arity checks and the single-result rule.
  Add finite binary64 Float to Runtime values and both VM conversion directions
  for the proposed sensor getters; preserve existing Int/String behavior. No
  new opcode, bytecode version, struct, collection or handle is proposed.
- Every proposed status-returning call uses Int: `0` success, `-1` unavailable
  source/service or disconnected transport, `-2` operation deadline exceeded,
  `-3` invalid external data/protocol response. Polling calls additionally use
  `1` for a complete available item and `0` for no item before the idle deadline.
  Diagnostics identify the function and reason on stderr. No payload is returned
  as a fake success after a negative status.
- Wrong argument types/arity, invalid timeout/range arguments and reading a
  snapshot without a successful acquisition are programming errors:
  RuntimeError, caught by the host or reported by `tacvm` with a nonzero exit.
  Sensor loss and Broker disconnect use the recoverable statuses instead.
- This limited status model does not change `camera.detectMarkers`, the three
  existing IPC calls or existing Window failures. It also does not resolve the
  #127 gate for rejecting malformed domain JSON without terminating Map.

### Device position, orientation and time (#134)

| Proposed signature | Result and meaning |
| --- | --- |
| `location.read(timeoutMs: Int) -> Int` | Acquire and validate one position snapshot; 0 or a negative status |
| `location.latitude() -> Float` | Snapshot WGS84 latitude, finite degrees in `[-90, 90]` |
| `location.longitude() -> Float` | Same snapshot's WGS84 longitude, finite degrees in `[-180, 180]` |
| `orientation.read(timeoutMs: Int) -> Int` | Acquire and validate one heading snapshot; 0 or a negative status |
| `orientation.heading() -> Float` | Finite camera-forward azimuth in `[0, 360)`, clockwise from true north |
| `time.nowMs() -> Int` | Signed i64 Unix UTC milliseconds from the wall clock |

`read` invalidates that source's previous snapshot before attempting acquisition.
After failure its getters raise RuntimeError; they never supply stale data,
zero coordinates, NaN or a fabricated heading. After success getters are local,
nonblocking reads of the same snapshot until the next `read` or Runtime destruction.
Reading latitude and longitude must not trigger two independent GPS acquisitions.
Location and heading snapshots are independent; simultaneous capture is not claimed.

Heading conventions match the inputs of [`analyzer-observation.md`](analyzer-observation.md):
0 north, 90 east, 180 south, 270 west. Provider-side normalization may convert
360 to 0, but invalid/non-finite readings must fail. Magnetic declination and
camera/device-axis alignment belong to the provider, not Analyzer. A configured
position/heading on a sensorless Linux laptop is an explicit source, not an
automatic fallback when a real sensor fails. Accuracy, maximum sample age and
allowed camera/pose skew require owner agreement; no accuracy is promised here.

Analyzer keeps geographic calculations, confidence and Observation assembly in
`.tc`. Runtime must not turn detections into target coordinates. Analyzer records
`time.nowMs()` immediately after receiving detections, as required by #98.
Wall-clock time may move backwards; all I/O deadlines use a monotonic clock.
Clock acquisition is local; OS clock failure is RuntimeError, never timestamp 0.

### Broker transport (#135)

One Runtime owns at most one TCP Broker connection. The API is topic/payload
transport; it does not interpret Observation/Target fields or calculate probability.

| Proposed signature | Result / wire mapping |
| --- | --- |
| `broker.connect(address: String, port: Int, timeoutMs: Int) -> Int` | Connect to an explicit numeric IP and port `1..65535`; 0 or negative status |
| `broker.subscribe(topic: String) -> Int` | Write `SUB <topic>\n`; 0 or negative status |
| `broker.publish(topic: String, payload: String) -> Int` | Write `PUB <topic> <payload>\n`; 0 or negative status |
| `broker.receive(timeoutMs: Int) -> Int` | Read one `MSG`: 1 item, 0 idle, or negative status |
| `broker.topic() -> String` | Topic of the last successfully received item |
| `broker.payload() -> String` | Its JSON object text, without `MSG`, topic or newline |
| `broker.close() -> Int` | Close only the owned connection, discard buffered bytes/snapshot; idempotent, returns 0 |

Topic must be nonempty and contain no whitespace; application text cannot inject
CR/LF into a frame. Payload must be one valid UTF-8 single-line JSON object.
Runtime validates the transport envelope and JSON syntax, leaving domain field
sets, ranges and policy to `.tc` under #127. Proposed local line limit: 16 KiB
including verb/topic/newline, sufficient for one current domain record and below
the current Broker's default scanner limit. This does not enlarge the wire protocol.

`receive` invalidates its previous message snapshot at entry; topic/payload getters
are valid only after result 1 and return independent VM-owned String values.
Idle result 0 can retain an incomplete input frame internally. A partial frame
has a separate absolute 1000 ms assembly deadline across polling calls; trickled
bytes do not extend it. Buffered complete frames are delivered one per call.
EOF, write failure, malformed/oversized incoming frames or a partial-frame timeout
close the connection and discard its buffers before returning a negative status.
Local invalid arguments, including malformed outgoing JSON, raise RuntimeError
before I/O. Calls on a disconnected Broker return -1. A send/connect timeout
also closes the affected connection; a failed publication is never retried.

Connect on an already connected Runtime is a programming error. Explicit
`close` followed by `connect` is allowed, but there is no automatic reconnect,
resubscription, acknowledgement, replay or offline queue. Subscribe/publish
success means local transmission only, never Broker acceptance or delivery.
In particular, `SUB` has no acknowledgement to wait for. The existing custom
Broker remains the network peer; no ready-made server replaces it.

Map publishes only the six Observation domain fields on `observations`, removing
the IPC `type` in application code, and subscribes to `targets.updated`. The
five-field Target payload is unchanged. A Broker failure leaves Map running
with its last visible state and a local diagnostic, matching the limited #99
prototype behavior; a complete stale-data UI policy remains an application question.

### Window drawing and native close (#136)

Reuse window ID `map`, one window and the exact commands/fields of
[`map-rendering.md`](map-rendering.md) and [`window-manager.md`](window-manager.md).
Keep existing `create/show/close` signatures and their 0/RuntimeError convention.

| Proposed signature | Result / wire mapping |
| --- | --- |
| `window.beginFrame(r: Int, g: Int, b: Int, a: Int) -> Int` | `BEGIN_FRAME` with RGBA `color`; returns 0 |
| `window.drawImage(path: String, x: Int, y: Int, width: Int, height: Int) -> Int` | `DRAW_IMAGE` with `image_path` and integer geometry; returns 0 |
| `window.drawCircle(x: Int, y: Int, radius: Int, r: Int, g: Int, b: Int, a: Int) -> Int` | `DRAW_CIRCLE` with integer geometry and RGBA `color`; returns 0 |
| `window.present() -> Int` | `PRESENT`; returns 0 |
| `window.pollEvent(timeoutMs: Int) -> String` | `"window_close"` for `map`, or empty String on idle timeout |

RGBA components are `0..255`; dimensions and circle radius are positive Ints.
WM clips drawings. Path resolution is local to WM, as in the existing protocol;
Runtime does not download imagery. Validate Int geometry before narrowing to the
WM implementation's native integer representation. Runtime does not project
coordinates or derive marker opacity; Map owns those calculations and rounding.

`create` first connects to the event socket and validates its subscription
response, then sends `CREATE`. Every command gets its own command connection;
Runtime checks a complete JSON response and boolean `ok`. A malformed response,
`ok:false`, WM loss or deadline expiry raises RuntimeError with bounded cleanup.
Failed CREATE/SHOW cannot make an Activity ACTIVE. Frame order is CREATE,
BEGIN_FRAME, DRAW_IMAGE, DRAW_CIRCLE(s), PRESENT, SHOW, as in the Go reference.

Runtime retains the event connection. On `window_close`, mark the owned window
closed before returning the event; WM has already destroyed it. The application
stops its loop and Runtime performs remaining cleanup without a second CLOSE.
On explicit shutdown, close the still-owned window with CLOSE, never QUIT.
A command timeout has an uncertain server-side outcome: do not retry CREATE or
drawing commands as though failure proved that WM did nothing.
Event reads validate the response envelope and window ID; malformed events or
unexpected EOF are RuntimeError. Retain partial event bytes across polls with
a 64 KiB frame bound and an absolute 1000 ms partial-frame deadline, so idle
polling cannot conceal a broken WM stream. The initial event subscription and
CREATE each have their own deadline; together `create` takes at most 2000 ms
before any bounded failure cleanup.

### Bounded local IPC for the Map loop (proposal requiring IPC review)

The existing `runtime.ipc_receive()` deliberately waits indefinitely while idle;
calling it in Map's only event loop would prevent Broker polling and native close
handling. Changing that existing signature/behavior is not proposed. The concrete
single-threaded Map scenario needs these additive candidates:

| Proposed signature | Result |
| --- | --- |
| `runtime.ipc_poll(timeoutMs: Int) -> Int` | After `ipc_listen`: 1 validated Observation, 0 idle, negative status for external failure |
| `runtime.ipc_message() -> String` | Last successfully polled IPC JSON object, including `type`, without newline |

Use the existing socket, exact schema, 64 KiB bound and one-way at-most-once
semantics. Each poll invalidates its message snapshot. Empty connection probes
are discarded; malformed frames or partial EOF discard only the offending
connection and return -3. Retain partial bytes between polls, enforcing the
existing five-second partial-message inactivity timeout across calls. A polling
timeout is idle result 0 and never extends that inactivity deadline by itself.
Do not mix `ipc_poll` and `ipc_receive` on one Runtime; this is a programming error.
No acknowledgement, new wire message, extra listener or automatic resend is added.
This is a proposed addition for owner review, not a claim of current IPC support.

### Deadlines, ownership and shutdown

| Operation | Proposed bound / preserved behavior |
| --- | --- |
| Sensor `read`, Broker `connect` | Explicit `timeoutMs` in `1..1000`; entire operation |
| New IPC/Broker/WM polling | `timeoutMs` in `0..1000`; 0 is nonblocking; no item is normal idle |
| Broker subscribe/publish; every WM command and event-subscription handshake | Absolute 1000 ms per operation, including connect/write/read |
| Broker partial input frame | Absolute 1000 ms from its first byte, across polls |
| Existing IPC send | Preserve 1000 ms and Int -1 on send/framing failure |
| Existing IPC receive | Preserve indefinite idle wait and five-second partial-message inactivity timeout |
| Camera startup / DETECT | Preserve separate five-second readiness and request deadlines; a cold call may incur both |
| Proposed WM cleanup | At most 1000 ms total for best-effort CLOSE; release local descriptors even on failure |

Deadlines are monotonic absolute deadlines, including EINTR handling, partial
reads/writes and an unresponsive peer. The new command deadlines are requirements
for follow-up code; current `window_manager_client.cpp` does not meet them.
Polling uses bounded buffers; no unbounded message queue is introduced.

Each Analyzer/Map process owns a distinct Runtime. One Runtime is used by one
thread, consistent with [`vm-execution.md`](vm-execution.md). Runtime owns its
sensor snapshots/providers, IPC descriptors/listener pathname, Broker connection,
WM client descriptors and its private detector child. It removes only its owned
listener path and terminates/reaps only its own helper. It never unlinks WM
sockets, kills WM/Broker or shares connections across VM executions.

On normal return or exception, proposed cleanup closes Broker without a final
publication, closes a still-owned window within the cleanup deadline, releases
IPC and sensor resources and performs the existing camera-helper shutdown.
Destructors must not throw or wait indefinitely. Native close suppresses duplicate
CLOSE; programmatic close releases the event connection. Crash/SIGKILL cleanup
and a general process supervisor are outside this increment.

### Deterministic sources and checks

Test selection belongs to the host/harness before VM execution; no new `.tc`
fake-mode or sensor-setting functions are proposed. The exact provider injection
configuration is a #134 decision. Invalid configuration must fail explicitly;
never silently replace a failed real source with test data.

| Fixture / fault | Required outcome |
| --- | --- |
| Fixed position `(59.9505, 30.311)`, heading `90.0`, time `1800000000100` | Same Float values and exact i64 timestamp in `.tc`; one position snapshot |
| GPS/heading unavailable, invalid/non-finite, out of geographic range, delayed provider | -1/-3/-2 as applicable; no Observation from unavailable data; no hang |
| Latitude ±90, longitude ±180, headings 0/90/180/270 | Accepted sensor values with documented axes; Analyzer applies its stricter ±80 latitude limit separately |
| Failed read after a successful read; successive successful reads | Old snapshot unusable after failure; both coordinates update together on success |
| Camera fake modes `two`, `empty`, `camera-unavailable`, `detection-failed` | Existing `CameraOptions` and helper wire fixtures unchanged; empty success differs from RuntimeError |
| Real custom Broker, absent peer, TCP reset, silent/slow/oversized peer | Correct SUB/PUB/MSG framing, bounded status failures, no retry, Map remains closeable |
| No IPC observations while Broker updates arrive or user closes | Bounded IPC polls do not starve other event sources |
| WM success, rejection, silence, malformed response, native close during setup | Bounded success/failure; cleanup, no duplicate CLOSE after event, no QUIT |
| Repeated VM creation/destruction and long-running message polling | No descriptor/helper/window leaks; bounded buffers and reclamation of generated Strings |

Use the real custom Broker and WM/headless for integration; controlled test peers
only inject transport faults. Existing Camera/IPC/WM tests must continue passing.
Follow-up acceptance requires real Compiler → VM → Runtime calls, not only C++
or a Go application substituting for Analyzer/Map. Existing verification references:
`docs/testing/camera-runtime.md`, `docs/testing/map-wm.md`, `tests/ipc/`,
`tests/map-tc/`, `tests/system-smoke/README.md`.

### `.tc` call examples and implementation gates

- [`Analyzer example`](../docs/examples/runtime-api/analyzer.tc) acquires guarded
  sensor snapshots, calls Camera and reads time. It sends a separately labelled
  prebuilt Observation fixture through the existing IPC; its JSON is deliberately
  not claimed to be computed from the sampled pose.
- [`Map example`](../docs/examples/runtime-api/map.tc) creates/draws/shows one
  window, publishes a six-field fixture and polls IPC, Broker and native close.
  It logs incoming payloads; it does not pretend to implement domain parsing or
  dynamic target drawing before #127 is agreed.

Both files are proposed API usage sketches, **not executable examples on current
main**: new names are unregistered and Runtime Float results are unsupported.
They intentionally use precomputed pixels for the first-marker scenario.
`app/map/tc/projection.tc` is the existing executable coordinate-only reference.
Production JSON extraction/assembly, exact-field-set validation and String
reclamation belong to the linked #127 proposal. Trigonometry for general Analyzer
bearings and Float-to-Int rounding for WM coordinates remain explicit approval
gates, not invented helper names in the examples.

## Open questions and required review

See [`02-open-questions.md`](../docs/ai-context/02-open-questions.md#runtime).
Before calling #128 approved, record decisions on:

| Gate | Required review / decision |
| --- | --- |
| Sensor surface and conventions | Runtime, Analyzer and architecture: snapshot getters vs #127 representation; provider configuration, calibration, age/skew |
| Scalar Float at VM ↔ Runtime | Compiler and VM with Runtime: value conversions, result kinds, existing v1/v2 regression |
| Status and polling model | VM, Runtime and Map: negative statuses, deadlines, additive IPC polling and getter validity |
| Broker behavior | Broker/Target Service, Runtime and Map: line limit, transport validation, no ACK/retry, explicit reconnect policy |
| WM lifecycle and drawing | WM, Map and Runtime: event subscription, Int geometry, bounded failure/cleanup, native close and Activity compatibility |
| #127 data bridge | Compiler, VM, Analyzer, Map and Runtime: recoverable domain validation, exact field sets, generated String reclamation |
| Required scalar math | Compiler, Runtime, Analyzer and Map: minimal sin/cos and pixel rounding/conversion approach |

The #128 reviewer set includes `@khamitova-prog`, `@mattthewww`, `@slutswarming`,
`@m4shustik-s`, `@pypynyaa` and architecture; Broker behavior also needs its owner.
No review or approval has been requested or recorded by this local draft.
Threads, generic files/networking, Activity callbacks, WebView, multiple windows,
camera schema changes and a general exception system are outside this proposal.
