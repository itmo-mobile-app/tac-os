# Private Camera Detector protocol

Status: approved design for issue #60; the Go detector side is implemented (issue #69), the C++ Runtime
client is pending.

This is the **private** interface between the C++ Runtime in the Analyzer VM process and
its Go/OpenCV marker-detector helper. It is not an application IPC interface, a
public service, or a replacement for Analyzer. The separate Analyzer-to-Map
`OBSERVATION` contract remains in `spec/ipc.md`.

## Roles and connection

- The C++ Runtime is the client and manages the helper's lifecycle.
- The Go marker detector is the Unix-domain stream socket server and owns camera
  capture, ArUco recognition, and camera-relative angle/distance estimation.
- Runtime starts one private helper lazily on the first `camera.detectMarkers()` call
  and reuses the same process and connection for later calls. Never start a new
  helper for each frame.
- The helper listens at `/tmp/tac-os-detector-<analyzer-pid>.sock`, where
  `<analyzer-pid>` is replaced with the decimal PID of the Analyzer VM process.
  Runtime passes the concrete path when starting the helper (e.g. a `-socket`
  argument). Only the helper removes its owned socket file on shutdown; it must
  not delete an unrelated active socket. Restrict access to the current user.
- The API accepts only one outstanding request per Runtime/helper connection;
  requests and responses are matched by order. No multiplexing or request IDs
  are needed for this first scenario.

The detector may run in an explicitly configured test mode that provides
deterministic synthetic detections without opening a physical camera. This mode
uses **the same** request and response format.

## Helper command line

Runtime starts the helper as

```text
marker-detect -socket /tmp/tac-os-detector-<analyzer-pid>.sock [-camera N] [-hfov DEG] [-marker-size M]
```

- `-socket PATH` selects this server mode. The helper creates the socket, accessible only to the
  current user, and listens at once. It opens the camera on the first `DETECT` and not at startup, so a missing
  camera is reported as `CAMERA_UNAVAILABLE` and the next `DETECT` tries to open it again. A leftover socket
  file of a dead process is replaced. A socket that another process still listens on, and a file that is not
  a socket, are never removed: the helper exits with status 1.
- `-camera`, `-hfov` and `-marker-size` are the prototype camera parameters from "Timeouts, failure handling
  and lifecycle" (defaults: camera 0, 60 degrees, 0.10 meters).
- Diagnostics go to standard error; standard output is not used.
- On `SIGTERM` or `SIGINT` the helper stops accepting connections, closes the open ones, releases the camera,
  removes its socket file and exits with status 0. It waits at most 3 seconds for a request in progress.
- Exit status: 0 after a signal, 1 on a startup or runtime failure, 2 for an invalid command line.

### Test mode

`-fake MODE` (only together with `-socket`) answers every `DETECT` without opening a camera. The modes are the
shared fixtures for the Go and C++ contract tests:

| Mode | Response line |
| --- | --- |
| `two` | `{"ok":true,"detections":[{"id":17,"relativeAngle":12.5,"distance":3.2},{"id":23,"relativeAngle":-8.4,"distance":5.7}]}` |
| `empty` | `{"ok":true,"detections":[]}` |
| `camera-unavailable` | `{"ok":false,"error":"CAMERA_UNAVAILABLE"}` |
| `detection-failed` | `{"ok":false,"error":"DETECTION_FAILED"}` |

## Framing

- Each request and response is one UTF-8 JSON object on one line, terminated by
  `\n`. Newlines inside a JSON object must be escaped.
- The persistent socket connection carries repeated request/response pairs.
- A frame may not exceed 64 KiB, including the terminating newline.
- Reject malformed JSON, missing/unknown fields and wrong field types. Non-finite
  numeric values are invalid. JSON keys and command strings are case-sensitive.
- No version field is needed for this single private protocol increment. Change
  the specification before making an incompatible wire change.

## Request: `DETECT`

```json
{"command":"DETECT"}
```

The request contains exactly one field, `command`, whose value must be
`"DETECT"`. The detector captures and analyzes **one current camera frame**
per request and returns at most one result per recognized game marker. Raw
camera frames never traverse this socket.

## Successful response

```json
{"ok":true,"detections":[{"id":17,"relativeAngle":12.5,"distance":3.2},{"id":23,"relativeAngle":-8.4,"distance":5.7}]}
```

A successful result with no recognized markers is **not an error**:

```json
{"ok":true,"detections":[]}
```

The success response contains exactly `ok` (Boolean `true`) and
`detections` (array). Each detection contains exactly these fields:

| Field | JSON type | Meaning and validation |
| --- | --- | --- |
| `id` | integer | ArUco `DICT_4X4_100` game ID, 0 to 71 inclusive |
| `relativeAngle` | number | Finite horizontal angle in degrees, right positive and camera center 0 |
| `distance` | number | Finite distance in meters from camera to marker center; greater than 0 |

Detection results must have unique IDs and be sorted in ascending ID order.
There is no `confidence` or `probability`: `Observation.confidence` is assigned
later by Analyzer, and `Target.probability` is calculated by Target Service.
The detector does not provide geographic coordinates or `Observation` objects.

## Failed response

```json
{"ok":false,"error":"CAMERA_UNAVAILABLE"}
```

A failed response contains exactly `ok` (Boolean `false`) and
`error` (string). Initial wire error codes:

| Error | Meaning |
| --- | --- |
| `CAMERA_UNAVAILABLE` | Camera cannot be opened or read |
| `DETECTION_FAILED` | An acquired frame could not be processed |
| `BAD_REQUEST` | Unknown command, invalid JSON, or incorrect request schema |

The detector must not encode a camera failure as an empty detections array.
After `BAD_REQUEST`, the connection may remain open for another valid request.

## Timeouts, failure handling and lifecycle

- Runtime waits at most 5 seconds for the helper to become ready and accept a
  connection, and at most 5 seconds for an individual `DETECT` response.
  Both waits must be bounded, including a blocked read or write.
- On timeout, unexpected EOF, broken connection or unexpected helper exit,
  Runtime reports a transport/helper failure (not an empty detection list)
  and releases the broken connection. Automatic restart/retry is not
  required in this increment.
- On a wire error response Runtime maps the error code to a Runtime failure.
  The exact user-facing exception and VM-level recovery mechanism remain
  outside this transport contract; an error must not be mistaken for success.
- Runtime owns shutdown of the helper it spawned: at Analyzer termination it
  closes the connection, requests process termination, and reaps its child.
  The helper releases the camera and socket. It must never terminate the
  Analyzer VM or the unrelated Window Manager.
- Default prototype camera parameters (camera index 0, 60-degree horizontal
  field of view and 0.10-meter marker side) may be supplied at helper startup.
  Real-world calibration remains separate; these defaults do not establish
  actual measurement accuracy.

## VM-facing boundary

`spec/runtime-api.md` defines the initial public signature
`camera.detectMarkers() -> String`. Runtime validates a successful detector
response and returns **only the JSON serialization of its `detections` array**
as a `String`, without the private `ok` wrapper. Thus:

- Success with two detections -> `'[{"id":17,...},{"id":23,...}]'` (complete JSON array).
- Success with no detections -> `"[]"`.
- Camera or transport failure -> Runtime error, **not** `"[]"`.

This is an intentionally temporary VM ABI until `Float` and collections are
implemented. There is no requirement to add a JSON parser to `.tc` for Sprint 2.
The current compiler/VM may exercise this contract using a C++ Runtime test
client or synthetic fixture; the Go helper IPC implementation belongs to #69.

## Contract verification

Both Go and C++ implementations must be tested with the same fixtures:

1. `DETECT` -> two ordered, valid detections.
2. `DETECT` -> empty array.
3. `DETECT` -> `CAMERA_UNAVAILABLE`.
4. Unknown command and malformed JSON -> `BAD_REQUEST`.
5. Multiple sequential requests over one connection.
6. Helper exit, connection failure and timeout -> explicit Runtime failure,
   with no process hang.
7. Only the detections array is exposed through the VM-facing `String`.

No camera frames, confidence, global coordinates or Analyzer-to-Map messages
are transmitted through the private channel.
