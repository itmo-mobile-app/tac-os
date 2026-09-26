# Team ownership

Status: draft

This document defines the current ownership model for the 9-person `tac-os` team.

Ownership describes responsibility for a component or area.
It does not change architectural boundaries: components remain separate even when one person owns several related areas.

## Ownership model

| # | Area                            | Person    | GitHub              | Main responsibilities                                                                                               |
|---|---------------------------------|-----------|---------------------|---------------------------------------------------------------------------------------------------------------------|
| 1 | Architecture and integration    | Максим    | TheGeniusOfEternity | Requirements, architecture, shared contracts, integration, CI, end-to-end scenarios                                 |
| 2 | Language and compiler           | Маруся    | m4shustik-s         | Custom language, grammar, parsing, semantic analysis, `.tc` → `.bc` compilation                                     |
| 3 | VM Core                         | Никита    | pypynyaa            | Bytecode loading, interpreter, stack, heap, function calls, branching, Runtime call mechanism                       |
| 4 | Runtime                         | Андрей    | pateWY              | Threads, IPC, camera, location, orientation, network, files, time, Activity framework, WebView bridge               |
| 5 | Window Manager and UI framework | Настя     | khamitova-prog      | Window Manager, window lifecycle, input dispatch, Activity integration, WebView integration                         |
| 6 | SQL DBMS                        | Матвей К. | chatty-king         | SQL parser, executor, tables, persistent storage, supported SQL subset                                              |
| 7 | Broker and Target Service       | Ника      | nika877             | Publish/subscribe Broker, target aggregation, probability/time decay, DBMS interaction                              |
| 8 | Analyzer                        | Алиса     | slutswarming        | Camera processing, target identification, confidence, direction, distance, target coordinates, Observation creation |
| 9 | Tactical Map / application UI   | Матвей С. | mattthewww          | 2D map, markers, probability display, IPC input, remote target updates, application shell                           |

Not everyone has accepted the GitHub organization invite yet. Until a person joins, they cannot be set as an Issue/PR
assignee in this repository, even though their ownership area is already fixed above.

## Notes

### Architecture and integration

This role owns cross-component consistency, not all implementation work.

Responsibilities include:

- keeping architecture documentation current;
- coordinating public contracts;
- maintaining integration scenarios;
- preventing incompatible component changes;
- keeping the system buildable and testable as components evolve.

### Language and compiler

The language must remain intentionally minimal.

Only features required by the application should be added.

### VM Core

The VM should remain independent of Linux-specific implementation details where possible.

Linux-facing functionality belongs to Runtime.

### Runtime

Runtime owns the high-level APIs exposed to code running inside the VM.

This includes threading and IPC APIs, but not application business logic.

### Window Manager and UI framework

Window Manager remains a separate architectural component even if the same developer also owns Activity or WebView
integration.

### Broker and Target Service

These are separate components with one shared owner.

Broker responsibilities:

- connections;
- topics;
- subscriptions;
- message delivery.

Target Service responsibilities:

- target-domain logic;
- observation aggregation;
- probability and time decay;
- DBMS interaction;
- publishing target updates.

Their implementations should remain separated.

### Analyzer and Map

Analyzer and Map are separate processes of one Tactical App.

They communicate through the project IPC mechanism.

## Work distribution principle

Team ownership is based on workload.

Architecture should not be reshaped only to make the number of components match the number of developers.

A developer may own more than one related component while keeping those components logically and structurally separate.

## Updating ownership

Update this file when:

- a component changes owner;
- responsibilities are moved between team members;
- a task is split or merged for workload reasons.

Do not silently change ownership assumptions in implementation work.
