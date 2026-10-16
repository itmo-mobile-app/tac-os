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

- receiving marker detections from the marker detector (see "Marker recognition");
- estimating detection confidence;
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

## Marker recognition

Marker recognition is performed by a separate program, the marker detector, not by `.tc` code on the VM:
processing camera frames in bytecode is expected to be too slow. The VM and the Runtime are written in C++;
the detector is written in Go.

```text
Camera frame
   ↓
Marker detector (Go, OpenCV): marker id, angle, distance
   ↓
Analyzer (.tc code): Detection → Observation
```

For every game marker in a frame the detector reports a detection:

| Field | Type | Notes |
| --- | --- | --- |
| `id` | Int | Marker id, see "Marker format" |
| `relativeAngle` | Float | Degrees; `0` is the center of the frame, positive is to the right |
| `distance` | Float | Meters from the camera to the marker center |

The Analyzer `.tc` code only combines these values with the device position and orientation and builds an
`Observation`. How the detections reach the Analyzer is not decided yet, see `02-open-questions.md` →
"Runtime".

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

The exact Linux IPC primitive and serialization format are not fixed yet.

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
