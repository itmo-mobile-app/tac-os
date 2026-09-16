# Open questions

Status: active

This document contains architectural and technical decisions that are intentionally not fixed yet.

AI agents must not silently resolve these questions. If implementation work requires one of these decisions, it should be discussed, documented, and then moved to the appropriate architecture or specification file.

## Language

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

- Exact Runtime API.
- Linux threading primitive.
- Thread synchronization primitives.
- Exact IPC mechanism.
- IPC message serialization format.
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

Still unresolved:

- exact message types;
- exact payload format;
- delivery guarantees;
- request/response requirements;
- behavior when the receiving process is unavailable.

## Broker

- Exact network transport.
- TCP vs WebSocket.
- Wire protocol.
- Message framing.
- Subscription format.
- Reconnection behavior.
- Delivery guarantees.
- Whether message persistence is required.

Initial logical topics are expected to include:

- `observations`;
- `targets.updated`.

## Target Service

- Exact `Observation` fields.
- Exact `Target` fields.
- Aggregation algorithm.
- Coordinate aggregation strategy.
- Probability calculation formula.
- Time decay formula.
- Handling of duplicate observations.
- Handling of stale observations.

## DBMS

- Exact supported SQL subset.
- SQL grammar.
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
