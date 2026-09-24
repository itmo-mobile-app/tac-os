# Project overview

Status: draft

## Project

`tac-os` is an educational project that implements a simplified Android-like platform on top of Linux.

The concrete target scenario for the mobile application is a tool that simplifies opponent detection
during an airsoft game: the application recognizes an opponent's marker through the camera and shows
their position on a shared tactical map in real time, for the whole team to see.

## Main goal

The project is intended to demonstrate and study the core ideas behind a mobile software platform:

- a custom programming language;
- a compiler;
- custom bytecode;
- a virtual machine;
- a runtime layer;
- threads;
- IPC;
- window management;
- Activity-like lifecycle;
- a SQL DBMS;
- a message broker;
- backend logic for target aggregation.

## Application scenario

A client device should be able to:

1. receive a camera frame;
2. detect and identify a target (an opposing player wearing a marker) using a visible marker;
3. estimate target direction and distance;
4. calculate target coordinates using the device position and orientation;
5. create an observation;
6. send the observation to the central system;
7. receive aggregated target updates;
8. display targets on a simple 2D map.

Each target has an identifier and a probability/confidence value.

Fresh observations from multiple clients increase confidence in the target state.
Old observations lose weight over time.

## Application execution model

Source files use the `.tc` extension.

The compiler transforms `.tc` source code into `.bc` bytecode:

```text
source.tc
   ↓
compiler
   ↓
program.bc
```

The bytecode is executed by the custom VM:

```text
program.bc
   ↓
VM
   ↓
Runtime
   ↓
Linux
```

The VM executes the custom bytecode.

The Runtime provides high-level APIs over Linux facilities such as threads, IPC,
camera access, location, networking, windowing, and other system functionality.

## Client-side structure

The user sees one Tactical App.

Internally, the application uses at least two processes:

```text
Tactical App
├── Analyzer process
└── Map process
```

### Analyzer

Responsible for:

- camera input;
- target recognition;
- target identifier detection;
- confidence estimation;
- direction and distance estimation;
- target coordinate calculation;
- creating observations.

### Map

Responsible for:

- displaying the 2D tactical map;
- displaying targets;
- displaying target probability;
- receiving local observations through IPC;
- receiving aggregated updates from the central system.

Analyzer and Map communicate through the project's custom IPC mechanism.

Each process runs in its own VM instance.

## Central node

The central part of the system runs on a separate Linux device in the same local network.

It contains:

```text
Broker
   ↓
Target Service
   ↓
DBMS
   ↓
Database
```

### Broker

Responsible for publish/subscribe message delivery.

The Broker does not contain target-related business logic.

### Target Service

Responsible for:

- receiving observations;
- aggregating observations from multiple clients;
- updating target state;
- calculating probability and time decay;
- reading and writing data through the DBMS;
- publishing target updates.

### DBMS

A custom minimal SQL database management system.

Responsible for:

- parsing the supported SQL subset;
- executing queries;
- storing and retrieving data.

### Database

Persistent data managed by the DBMS.

The database itself is data, not a separate business service.

## UI model

The project uses a Window Manager as a separate system component.

Activity lifecycle belongs to the Runtime/framework layer.

Concrete application activities belong to application code.

A WebView may be used as the rendering layer for the application UI if this simplifies
development. The WebView is accessed through Runtime APIs and a bridge between the custom
language and JavaScript.

## Project constraints

- Linux is the base operating system.
- The project does not implement its own kernel, drivers, or TCP/IP stack.
- Required platform components should be implemented by the team instead of replaced by
  ready-made equivalents.
- The custom language should remain minimal and include only features required by the
  application.
- The VM should remain minimal and include only features required to execute the application.
- The SQL DBMS should implement only the SQL subset required by the Target Service.
- Architecture and public interfaces should be documented before dependent components diverge.

## Repository model

The project is developed as a monorepo.

Main architectural areas are expected to include:

```text
compiler/
vm/
runtime/
system/
server/
app/
spec/
docs/
tests/
```

Component boundaries should follow architecture, not team ownership.

A single developer may own multiple related components without merging them into one module.
