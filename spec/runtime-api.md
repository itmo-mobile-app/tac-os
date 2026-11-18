# Runtime API specification

Status: approved for the existing call ABI, Camera and IPC. The device API below implements the
[#128 proposal](https://github.com/itmo-mobile-app/tac-os/issues/128) for owner review in #134.

This document defines how the VM calls into the Runtime and documents the VM-facing operations.
The IPC and Broker wire formats are specified in [ipc.md](ipc.md) and
[broker-protocol.md](broker-protocol.md).

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

The Camera result uses the **temporary String ABI**. The language has scalar
`Float` but still needs a collection representation for a typed `Detection[]`
result. Camera tests use the C++ Runtime with synthetic detector responses.

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

## Device position, orientation and time (#134)

| Signature | Result |
| --- | --- |
| `location.read(timeoutMs: Int) -> Int` | Acquire one position snapshot and return a status |
| `location.latitude() -> Float` | Latitude in WGS84 degrees, from −90 through +90. Positive is north |
| `location.longitude() -> Float` | Longitude in WGS84 degrees, from −180 through +180. Positive is east |
| `orientation.read(timeoutMs: Int) -> Int` | Acquire one heading snapshot and return a status |
| `orientation.heading() -> Float` | Azimuth in degrees clockwise from true north along the camera's forward axis. Valid range is [0, 360) |
| `time.nowMs() -> Int` | UTC wall-clock time measured in Unix milliseconds as a signed 64-bit integer |

Azimuths 0, 90, 180 and 270 indicate north, east, south and west. The source is responsible for
magnetic declination and camera-axis alignment. A value of 360 degrees is outside the accepted range.
Coordinate and heading getters return finite binary64 values through the existing call ABI.
The Compiler selects bytecode version 2 for programs using these Float results.

Both reads take a timeout in `1..1000` ms measured against a monotonic clock. They return
`0` for success, `-1` for an unavailable source, `-2` for an expired deadline and `-3`
for a non-finite or out-of-range reading. A negative status writes the reason to stderr.
Incorrect argument types, arity and timeouts raise `RuntimeError`.

Runtime owns one position and one heading snapshot each. A read clears the corresponding
snapshot before starting. After a successful read, the getters access that snapshot locally
without sampling again. After a failed read or before the first successful read, the
corresponding getters raise `RuntimeError`. Reads of one source leave the other snapshot
available. Snapshots and configured sources are released when their Runtime is destroyed.

The host selects fixed sources explicitly by passing `DeviceOptions` to the C++ `Runtime` constructor.
With default Linux configuration, location and orientation report unavailable.
`time.nowMs()` reads `CLOCK_REALTIME` unless the host supplies a fixed test timestamp.
The wall clock may move backwards. A clock read failure or an unrepresentable timestamp
raises `RuntimeError`. Runtime provides device measurements. Analyzer owns calculation of
target coordinates.

### Linux verification

```sh
(cd compiler && ./gradlew --no-daemon installDist)
cmake -S . -B build
cmake --build build --parallel
ctest --test-dir build -R '^device_vm_e2e$' --output-on-failure
```

The test compiles [`tests/device/device.tc`](../tests/device/device.tc) and uses the real VM
with host-injected position, azimuth and time. It also covers missing sources, geographic
limits, non-finite readings, deadlines, invalidated snapshots and the default Linux wall clock.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Runtime":

- the remaining Runtime API surface (threads, IPC, location, orientation, networking, files, time,
  Activity lifecycle, WebView bridge, Window Manager access) — added incrementally, one concrete
  scenario at a time;
- how recoverable Runtime call errors are represented inside `.tc` programs beyond the initial failure-reporting behavior.
