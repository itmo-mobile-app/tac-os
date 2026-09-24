# Open questions

Status: active

This document contains architectural and technical decisions that are intentionally not fixed yet.

AI agents must not silently resolve these questions. If implementation work requires one of these decisions, it should be discussed, documented, and then moved to the appropriate architecture or specification file.

## Language

A minimal syntax is proposed in `spec/language.md` (draft, pending approval by the Language and
Compiler owner). Remaining open:

- Exact language syntax.
- Minimal type system.
- Whether classes are required.
- Whether inheritance is required.
- Exact representation of structs or objects.
- Function syntax and calling conventions.
- Error handling model.
- Standard library scope.

## Compiler

- Exact intermediate representation, if any.
- Semantic analysis rules.
- Error reporting format.
- Exact mapping from language constructs to bytecode.

## Bytecode

A minimal instruction set and file format are proposed in `spec/bytecode.md` (draft, pending approval by
the VM Core owner). Remaining open:

- Instruction set.
- Bytecode file format.
- Constant pool representation.
- Function representation.
- Metadata format.
- Versioning strategy.
- Encoding of Runtime calls.

## VM

- Exact execution model.
- Exact stack layout.
- Heap representation.
- Memory management strategy.
- Garbage collection requirements.
- Object representation.
- Runtime/native call ABI.
- Error handling during bytecode execution.

## Runtime

A minimal Runtime call ABI and one function (`runtime.log`) are proposed in `spec/runtime-api.md`
(draft, pending approval by the Runtime owner). The IPC mechanism and message format for the
Analyzer↔Map scenario are proposed in `spec/ipc.md` (draft, same owner). Remaining open:

- Exact Runtime API (beyond `runtime.log`).
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

A minimal transport, message type (`TARGET_DETECTED`), and payload format are proposed in
`spec/ipc.md` (draft, pending approval by the Runtime owner). Still unresolved:

- request/response requirements (the proposal is one-way notification only);
- behavior when the receiving process is unavailable (the proposal is: drop the message).

## Broker

A minimal transport, wire protocol, and command set are proposed in `spec/broker-protocol.md` (draft,
pending approval by the Broker and Target Service owner). Remaining open:

- Reconnection behavior.
- Whether message persistence is required.

Initial logical topics are expected to include:

- `observations`;
- `targets.updated`.

## Target Service

`Observation` and `Target` fields are proposed in `spec/data-model.md` (draft, pending approval by the
Broker and Target Service owner together with the Analyzer and Tactical Map owners). Remaining open:

- Aggregation algorithm.
- Coordinate aggregation strategy.
- Probability calculation formula.
- Time decay formula.
- Handling of duplicate observations.
- Handling of stale observations.

## DBMS

A minimal SQL subset is proposed in `spec/sql.md` (draft, pending approval by the SQL DBMS owner).
Remaining open:

- Storage format.
- Table representation.
- Whether indexes are required.
- Whether transactions are required.
- Client protocol between Target Service and DBMS.
- Whether DBMS runs as a separate process or is embedded as a library.

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

- Required unit-test framework per language.
- Integration test strategy.
- End-to-end environment.
- Mock strategy for camera, GPS, and network.
- Required CI checks before merge.

## Team process

- Final ownership assignment for all 9 directions.
- Required number of reviewers per Pull Request.
- CODEOWNERS rules.
- Branch protection rules.
- CI requirements for `main`.
