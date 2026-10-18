# Architecture

Status: draft

## Purpose

This document describes the current high-level architecture of `tac-os`.

It defines component boundaries and the main interaction model.
Implementation details belong in `spec/` or component-specific documentation.

## Platform layers

The application execution stack is:

```text
.tc source
 ↓
Compiler
 ↓
.bc bytecode
 ↓
VM
 ↓
Runtime
 ↓
Linux
```

### Compiler

The compiler transforms `.tc` source code into `.bc` bytecode.

The compiler is not required to run on the client device.

### VM

The VM executes `.bc` bytecode.

The VM owns:

- bytecode loading;
- instruction execution;
- instruction pointer;
- operand stack;
- call stack;
- heap;
- function calls;
- branching;
- Runtime calls.

The VM should not implement application business logic or Linux-specific functionality directly.

### Runtime

The Runtime provides high-level APIs to code running inside the VM.

Its role is to abstract Linux facilities and system services.

Expected Runtime areas include:

- threads;
- IPC;
- camera;
- location;
- device orientation;
- networking;
- files;
- time;
- Activity lifecycle;
- WebView bridge;
- access to the Window Manager.

The Runtime may be linked into the VM executable and does not need to run as a separate process.

### Implementation and build

- The VM and the Runtime are written in C++17 and built with CMake (`CMakeLists.txt` at the root). The
  build produces `tacvm`, the VM with the Runtime linked in: `tacvm <program.bc>`.
- The Compiler is written in Kotlin (JVM 17) and built with Gradle in `compiler/`. `./gradlew installDist`
  produces the `tacc` launcher: `tacc <input.tc> <output.bc>`.
- The DBMS is written in Go (1.22+) in `server/dbms/`, matching the course stack. It uses
  only the standard library. `go build ./cmd/tacdb` builds its CLI; `go test ./...` runs
  its unit tests. CMake also builds this executable and registers DBMS tests with CTest.
  Issue #31 is an in-memory increment; persistence and the client protocol are deferred.
- The Window Manager, Broker, Target Service, and DBMS are written in Go or Rust, as required by the course
  technology stack (`docs/requirements.md`, C-11). Each component owner picks one of the two; the choice is
  recorded here.
- Integration tests live in `tests/` and run through CTest: a test runs a program and compares its
  standard output with an expected file.
- On every Pull Request, CI builds the Compiler with Gradle, builds the VM and the Runtime with
  AddressSanitizer and UndefinedBehaviorSanitizer (these sanitizers apply only to the C++ code), and runs
  CTest. CI also checks Go formatting, runs `go vet`, and tests the DBMS with the Go race detector.
- The marker detector is written in Go (a separate module, built with `go build`) and uses OpenCV through
  gocv, so building it needs cgo, OpenCV and `pkg-config`; the gocv version must match the OpenCV version.
  Its packages that do not use OpenCV are built and tested with plain `go test`.

## Client application

The user interacts with a single Tactical App.

Internally, the application is split into at least two processes:

```text
Tactical App
├── Analyzer process
└── Map process
```

Each process runs its own VM instance.

### Analyzer process

Responsible for:

- requesting marker detections through the Runtime Camera API (not accessing detector IPC directly);
- deriving observation confidence using a policy that still needs to be specified;
- reading device position and orientation through Runtime APIs;
- calculating target coordinates;
- creating an `Observation`;
- sending local results to the Map process through IPC.

### Map process

Responsible for:

- displaying the application UI;
- displaying a simple 2D tactical map;
- displaying local and remote targets;
- displaying target probability;
- receiving local observations from Analyzer through IPC;
- receiving aggregated target updates from the central system.

### Map prototype for Sprint 1

The Map owner selected native rendering for #35. `app/map/` is a standalone Go prototype outside the
VM; the final Map application remains `.tc` code using Runtime. The prototype converts a hardcoded
list of `Target` records into drawing commands for the existing Go/SDL2 Window Manager.
WM receives image and circle commands, never geographic coordinates or target business logic.
The drawing commands are a Map-side contract awaiting the WM owner's implementation, as documented
in `spec/map-rendering.md`; existing WM window-lifecycle commands are unchanged.

The prototype uses a local 960x640 PNG and a version-1 JSON metadata file containing the image path,
dimensions and latitude/longitude bounds. North is up and east is right. For the small test rectangle,
coordinates are interpolated linearly, rounded to integer pixels, with inside east/south boundary
points placed in the last column/row. Outside targets are omitted. Geographic assumptions, formulas
and the asset format are specified in `spec/map-rendering.md`.

Probability affects marker opacity; it is not calculated by Map. `spec/data-model.md` is already
approved by the Tactical Map and Analyzer owners and remains unchanged.
The prototype has no Analyzer IPC, Broker subscription, WebView or `.tc` code. It includes an offline
PNG preview and coordinate/frame tests; those do not establish native WM integration.

## Marker recognition

**Architecture decision for #34:** `camera.detectMarkers()` is the public Runtime API for the Analyzer,
while the marker detector is a private implementation detail of that API. The detector runs as
a helper Linux process in Go because camera-frame processing does not belong in `.tc` bytecode.
It is not an independently exposed application service.

The two Tactical App processes remain Analyzer and Map, each running `.tc` on its own VM. The
detector is an additional internal helper process, not a replacement for Analyzer.

```text
Linux camera -> Marker Detector (Go + OpenCV)
                         |
                         | compact detections over private Unix-domain socket
                         v
            Runtime Camera API (C++)
                         |
                         | camera.detectMarkers() (conceptual call)
                         v
                  Analyzer (.tc / VM)
                         |
                         | computes coordinates and builds Observation
                         v
                  Map (.tc / VM) via application IPC
```

The Analyzer calls `camera.detectMarkers()` through the normal VM-to-Runtime call mechanism. Runtime
owns local IPC to the detector and presents its results without exposing its process or OpenCV
types to `.tc` code. The detector handles camera capture, ArUco identification, and relative
angle/distance estimation; raw camera frames are not passed over this IPC channel.

Runtime starts one private Go helper lazily and reuses a persistent Unix-domain socket
connection. The initial protocol is newline-delimited JSON: `{"command":"DETECT"}`
requests one frame, and responses contain either an array of detections or
an explicit error. Runtime uses a bounded startup/request timeout and owns
helper shutdown; the initial implementation has no automatic retry.
See `spec/camera-detector-protocol.md` for the exact wire format and lifecycle.

At the VM boundary, `camera.detectMarkers()` initially returns a `String` containing
only the JSON detection array (or reports a Runtime error). This temporary
representation works with the current String-capable ABI; a typed `Detection[]`
result awaits later `Float`/collection support. No raw frames cross the IPC
boundary. This decision does not change Analyzer-to-Map IPC in `spec/ipc.md`.

The Analyzer retains application logic: it obtains device position and orientation, interprets
team affiliation, computes global target coordinates, builds `Observation` records, and sends
them to Map. The detector does not know about geolocation, `Observation`, Map, Broker, or
target aggregation.

For each visible game marker the detector returns a logical detection with these fields:

| Field | Type | Notes |
| --- | --- | --- |
| `id` | Int | Marker id, see "Marker format" |
| `relativeAngle` | Float | Degrees; `0` is the center of the frame, positive is to the right |
| `distance` | Float | Meters from the camera to the marker center |

The detector prototype does not supply `confidence`; the policy for
`Observation.confidence` remains open. The initial VM representation for
detections is JSON in a `String`, as specified above. Native `Float` values,
typed collections and an appropriate final `Detection[]` ABI are deferred.

### Marker format

- ArUco markers, dictionary `DICT_4X4_100`.
- 6 teams with 12 players each: `id = team * 12 + player`, team `0..5`, player `0..11`, so valid ids are
  `0..71`. Markers with other ids are ignored.
- `targetId` of a `Detection` and an `Observation` is the marker `id` as a decimal string.

### Implementation

The detector uses the ArUco detector of OpenCV. The distance is estimated from the four corners of the
square marker of known physical size, assuming a pinhole camera. The marker size and the camera parameters
are detector configuration.

## IPC

IPC is used for communication between separate application processes.

Primary local scenario:

```text
Analyzer process
   ↓
Runtime IPC API
   ↓
Linux IPC mechanism
   ↓
Runtime IPC API
   ↓
Map process
```

The Analyzer-to-Map contract is a Unix-domain stream socket carrying
newline-delimited JSON `OBSERVATION` messages with completed geographic
coordinates. Map forwards their six domain fields to Broker without the
IPC `type` envelope. See `spec/ipc.md` and `spec/data-model.md`.
The private Go detector-to-Runtime IPC is a separate interface.

## Threads

Threads are exposed through the Runtime.

They are used for concurrent work inside a single process and are distinct from IPC.

The Runtime maps the project threading API to Linux threading facilities.

## Window Manager

The Window Manager

The Window Manager is a separate system component running on the client system.

It is responsible for:

- creating windows;
- showing and hiding windows;
- closing windows and releasing their resources;
- dispatching supported window events to applications through Runtime communication channels.

Applications do not access Linux windowing facilities directly. They access the Window Manager through Runtime APIs.

The Window Manager is implemented in Go and runs as a separate Linux process. Runtime communicates with it through local Unix-domain sockets.

SDL2 is the agreed windowing library for the Window Manager. X11 and Wayland are native Linux window-system options, while SDL2 provides a higher-level API that can use the available system backend. This avoids tying the Window Manager directly to a single window-system protocol and provides a common interface for window creation, display, closure, and native window events.

The initial implementation supports one Tactical Map window with a fixed drawing area of 960 × 640 pixels, the title `Tactical Map`, and native window decorations with a close control. The Window Manager chooses the window position.

The initial event support includes notification when the user requests that the window be closed through its native close control. Mouse and keyboard event forwarding, window resizing, multiple simultaneous windows, Activity lifecycle management, and WebView integration are outside the scope of the initial implementation.

The Runtime–Window Manager protocol is documented in `spec/window-manager.md`.

## Activity model

The Activity mechanism belongs to the Runtime/framework layer.

The Runtime is responsible for lifecycle behavior such as:

- creating an Activity;
- activating it;
- hiding it;
- destroying it.

Concrete Activities belong to application code written in the custom language.

The exact Activity lifecycle is intentionally minimal and is not fixed yet.

## WebView

WebView may be used as the rendering layer for the mobile UI.

The Tactical Map prototype uses native drawing commands, not WebView. The WebView model below remains
an option for a future concrete UI scenario; it is not a dependency of #35.

The intended model is:

```text
.tc application code
   ↓
VM
   ↓
Runtime WebView API
   ↓
WebView bridge
   ↓
HTML / CSS / JavaScript
```

WebView is intended to reduce the amount of custom UI rendering code.

The exact WebView engine and bridge API are not fixed yet.

## Central node

The central server runs on a separate Linux device in the same local network.

Its main components are:

```text
Broker
   ↓
Target Service
   ↓
DBMS
   ↓
Database
```

The components are architecturally separate even if multiple components are owned by the same developer.

## Broker

The Broker implements publish/subscribe message delivery.

Initial logical topics:

- `observations`;
- `targets.updated`.

The Broker does not:

- calculate target probability;
- understand target business rules;
- execute SQL;
- directly manage database storage.

## Target Service

The Target Service contains application backend logic.

It is responsible for:

- consuming `Observation` messages;
- grouping observations by target identifier;
- aggregating observations from multiple clients;
- calculating or updating target coordinates;
- calculating target probability;
- applying time-based probability decay;
- reading and writing data through the DBMS;
- publishing `Target` updates through the Broker.

The Target Service runs as its own process on the central node. It is a client of the DBMS process.

## DBMS

The DBMS is a custom minimal SQL database management system.

It is responsible for:

- parsing the supported SQL subset;
- executing queries;
- managing tables;
- persistent storage.

The DBMS does not understand the target domain and does not interact with the Broker directly.

The DBMS runs as a separate server process on the central node. The Target Service connects to it over a
socket, sends SQL statements, and receives result rows. The DBMS stores the database in a file on the
central node's disk, so data survives a restart of either process.

## Database

The Database is the persistent data managed by the DBMS.

It is not a separate business service.

Expected domain entities include:

- `Observation`;
- `Target`.

The exact schema is defined separately and is not fixed in this document.

## Main data flow

A typical target update flow is:

```text
Analyzer
   ↓ local IPC
Map / client-side logic
   ↓ publish Observation
Broker
   ↓
Target Service
   ↓ SQL
DBMS
   ↓
Database
```

After processing:

```text
Target Service
   ↓ publish TargetUpdated
Broker
   ↓
Clients
   ↓
Map
```

## Architectural principles

- Keep components minimal.
- Keep component responsibilities narrow.
- Do not merge components only because one person owns both.
- Do not add functionality without a concrete application scenario.
- Keep VM logic independent from Linux-specific implementation details.
- Keep DBMS logic independent from target-domain business logic.
- Keep Broker logic independent from message payload semantics.
- Document public interfaces before dependent components diverge.
