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

- receiving camera frames;
- detecting a target marker;
- identifying the target;
- estimating detection confidence;
- estimating direction and distance;
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

The Window Manager is a separate system component running on the client system.

It is responsible for:

- creating windows;
- showing and hiding windows;
- closing windows;
- dispatching input events.

Applications do not access Linux windowing facilities directly.

They access the Window Manager through Runtime APIs.

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

## DBMS

The DBMS is a custom minimal SQL database management system.

It is responsible for:

- parsing the supported SQL subset;
- executing queries;
- managing tables;
- persistent storage.

The DBMS does not understand the target domain and does not interact with the Broker directly.

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
