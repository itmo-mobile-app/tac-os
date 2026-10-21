# Open questions

Status: active

This document contains architectural and technical decisions that are intentionally not fixed yet.

AI agents must not silently resolve these questions. If implementation work requires one of these decisions, it should be discussed, documented, and then moved to the appropriate architecture or specification file.

## Language

A minimal syntax is defined in `spec/language.md` (reviewed and approved by the Language and
Compiler owner; compilation notes confirmed against `spec/bytecode.md` by the VM Core owner).
Remaining open:

- Minimal type system.
- Whether classes are required.
- Whether inheritance is required.
- Exact representation of structs or objects.
- Error handling model.
- Standard library scope.

## Compiler

- Exact intermediate representation, if any.
- Semantic analysis rules.
- Error reporting format.
- Exact mapping from language constructs to bytecode.

## Bytecode

The file format, constant pool, function representation, minimal instruction set, and encoding of
Runtime calls are defined in `spec/bytecode.md` (approved by the VM Core owner). Remaining open:

- Instructions for heap objects (structs/objects), once the language needs them.
- Metadata format (debug info, source locations).
- Versioning/compatibility strategy beyond the `version` field.

## VM

- Exact execution model.
- Exact stack layout.
- Heap representation.
- Memory management strategy.
- Garbage collection requirements.
- Object representation.
- String interning and string object lifetime.
- Approval of VM error handling (stack limits/underflow, integer overflow and `DIV` by zero):
  issue #62 implements controlled failures; the proposed rules are documented in
  `spec/bytecode.md` and still require Compiler owner review.

## Runtime

The Runtime call ABI and the first function (`runtime.log`) are defined in `spec/runtime-api.md`; the IPC
mechanism and message format for the Analyzer↔Map scenario are defined in `spec/ipc.md` (both approved by
the Runtime owner). Remaining open:

- Exact Runtime API (beyond `runtime.log`).
- Linux threading primitive.
- Thread synchronization primitives.
The camera architecture and the **initial** `camera.detectMarkers() -> String`
API are fixed in `spec/runtime-api.md`; the private Unix-domain socket
protocol, timeout and helper lifecycle are fixed in
`spec/camera-detector-protocol.md`. Still unresolved:

- Adding native `Float` and collections/`Detection[]` in the language, compiler,
  bytecode and VM; the initial JSON `String` is temporary.
- A recoverable error mechanism visible to `.tc` beyond an initial Runtime failure.
- Production camera configuration and device-calibration strategy; initial
  defaults are only for the prototype.
- Approval from the course instructor for OpenCV/gocv usage in the detector.
- Location API.
- Orientation API.
- Networking API.
- File API.
- Time API.

## Window Manager

- Whether the Window Manager runs permanently as a system service.
- Exact startup mechanism.

## Activity framework

- Exact Activity lifecycle.
- Whether multiple Activities may exist simultaneously.
- Navigation model between Activities.
- State persistence requirements.
- Relationship between Activities and windows.

## WebView

- Exact WebView engine or library.
- WebView initialization model.
- JavaScript bridge API.
- Direction of communication between `.tc` code and JavaScript.
- Whether a future UI scenario beyond the native Tactical Map prototype needs WebView.

## IPC scenario

The intended primary IPC scenario is:

```text
Analyzer process
   ↓
IPC
   ↓
Map process
```

The transport, `OBSERVATION` message (finished coordinates), validation,
and delivery behavior are fixed in `spec/ipc.md`. Still unresolved:

- reconnection behavior after the Map process restarts.

Additional message types are deferred until a concrete scenario requires them.

## Broker

The transport, wire protocol, and command set are defined in `spec/broker-protocol.md` (approved by the
Broker and Target Service owner). Remaining open:

- Reconnection behavior.
- Whether message persistence is required.

Initial logical topics are expected to include:

- `observations`;
- `targets.updated`.

## Target Service

`Observation` and `Target` fields are defined in `spec/data-model.md` (approved by the Broker and Target
Service, Analyzer, and Tactical Map owners). Remaining open:

- Aggregation algorithm.
- Coordinate aggregation strategy.
- Probability calculation formula.
- Time decay formula.
- Handling of duplicate observations.
- Handling of stale observations.

## DBMS

The minimal SQL subset is defined in `spec/sql.md` (approved by the SQL DBMS owner). Remaining open:

- Storage format.
- Table representation.
- Whether indexes are required.
- Whether transactions are required.
- The client protocol is specified in `spec/dbms-protocol.md`: Unix-domain
  socket, four-byte little-endian length framing, raw UTF-8 SQL requests and
  JSON responses with typed SELECT columns and structured errors. Its network
  server and Target Service client are not yet implemented.
- How stale Observation records are removed or expired, since `DELETE` is not
  included in the minimal SQL subset.

## Domain model

- Exact observation identifier format.
- Device/client identifier format.
- Probability representation.
- Confidence representation, including where the detection `confidence` comes from (the ArUco detector
  does not provide a score).
- Physical marker size and camera parameters (field of view or calibration) as detector configuration.
- Direction and azimuth calculation details.
- Coordinate precision requirements.

## Application

- Exact "tag"/elimination rule (how many independent observations, within what time, confirm a hit).
- What happens to a player after being tagged (eliminated for the round vs. respawn after a cooldown).
- Whether the match history (who observed whom and when) must be kept and viewable after the match.
- Number of teams in a match and whether each team runs its own central node or all teams share one.
- Exact UI structure.
- Exact division between Analyzer and Map responsibilities.
- Whether Analyzer has its own visible Activity.
- Whether analysis continues while the Map Activity is active.
- Behavior when the central server is unavailable.

The Map prototype's north-up coordinate conversion and PNG/JSON asset format are defined in
`spec/map-rendering.md` and `01-architecture.md` → "Map prototype for Sprint 1". The real playground
image and its geographic calibration still need to be supplied for deployment. Exact drawing-command
alignment and native integration with the WM owner's implementation remain pending.

## Deployment

- Exact client Linux environment.
- Exact server Linux environment.
- Startup order of system components.
- Process supervision.
- Configuration format.
- Port assignments.
- Packaging format for the application.
- Whether a custom application bundle format is required.

## Testing

The build and the integration test approach for the Compiler, the VM, and the Runtime (Gradle for the
Compiler, CMake for the VM and the Runtime, CTest, comparison of program output with an expected file,
sanitizer build of the C++ code in CI) are described in
`01-architecture.md` → "Implementation and build". Remaining open:

- Unit-test frameworks for components other than DBMS (DBMS uses Go's standard testing package).
- Choice of Go or Rust for the Window Manager, Broker, and Target Service.
  The DBMS uses Go, as documented in `01-architecture.md` → "Implementation and build".
- Whether the compiler must use ANTLR4 (the course stack names "Kotlin (ANTLR4)"; the current compiler has
  a hand-written lexer and parser).
  
- End-to-end environment.
- Mock strategy for camera, GPS, and network.
- Required CI checks before merge.

## Team process

- Final ownership assignment for all 9 directions.
- Required number of reviewers per Pull Request.
- CODEOWNERS rules.
- Branch protection rules.
- CI requirements for `main`.
