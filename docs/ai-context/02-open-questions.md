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
- Error handling during bytecode execution, including stack overflow/underflow and `DIV` by zero.

## Runtime

The Runtime call ABI and the first function (`runtime.log`) are defined in `spec/runtime-api.md`; the IPC
mechanism and message format for the Analyzer↔Map scenario are defined in `spec/ipc.md` (both approved by
the Runtime owner). Remaining open:

- Exact Runtime API (beyond `runtime.log`).
- Whether `runtime.log` appends a newline after the message.
- Linux threading primitive.
- Thread synchronization primitives.
- Camera API.
- Location API.
- Orientation API.
- Networking API.
- File API.
- Time API.

## Window Manager

- Exact Linux graphics/windowing backend.
- Window lifecycle.
- Input event model.
- Communication protocol between Runtime and Window Manager.
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
- Whether WebView is used for the whole UI or only the tactical map.

## IPC scenario

The intended primary IPC scenario is:

```text
Analyzer process
   ↓
IPC
   ↓
Map process
```

The transport, message type (`TARGET_DETECTED`), payload format, and delivery behavior (one-way, no
acknowledgement, message dropped when Map is not connected) are defined in `spec/ipc.md`. Still unresolved:

- reconnection behavior after the Map process restarts;
- whether message types beyond `TARGET_DETECTED` are needed.

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
Service owner; pending approval by the Analyzer and Tactical Map owners). Remaining open:

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
- Client protocol between Target Service and DBMS.
- Whether DBMS runs as a separate process or is embedded as a library.
- How stale Observation records are removed or expired, since `DELETE` is not
  included in the minimal SQL subset.

## Domain model

- Exact target identifier format.
- Exact observation identifier format.
- Device/client identifier format.
- Probability representation.
- Confidence representation.
- Distance estimation method.
- Direction and azimuth calculation details.
- Coordinate precision requirements.

## Application

- Exact team/opponent affiliation encoding (how a marker identifies which team it belongs to, so a
  client can tell its own team apart from opponents).
- Exact "tag"/elimination rule (how many independent observations, within what time, confirm a hit).
- What happens to a player after being tagged (eliminated for the round vs. respawn after a cooldown).
- Exact UI structure.
- Exact division between Analyzer and Map responsibilities.
- Whether Analyzer has its own visible Activity.
- Whether analysis continues while the Map Activity is active.
- Exact 2D map coordinate system.
- Exact map asset format.
- Behavior when the central server is unavailable.

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

- Required unit-test framework per language.
- Implementation language for the Window Manager, DBMS, Broker, and Target Service (C++17 with the same
  CMake build is the expected default).
- End-to-end environment.
- Mock strategy for camera, GPS, and network.
- Required CI checks before merge.

## Team process

- Final ownership assignment for all 9 directions.
- Required number of reviewers per Pull Request.
- CODEOWNERS rules.
- Branch protection rules.
- CI requirements for `main`.
