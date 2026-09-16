# Specifications

This directory contains public contracts shared between `tac-os` components.

Specifications describe **what components exchange and what behavior they rely on**.
They should not contain unnecessary implementation details.

Create a specification only when the corresponding contract becomes necessary.

## Planned specifications

The project is expected to eventually contain files such as:

```text
language.md
bytecode.md
runtime-api.md
ipc.md
broker-protocol.md
data-model.md
sql.md
```

## Intended scope

### `language.md`

Defines the minimal custom language required by the application.

Expected topics:

- syntax;
- types;
- functions;
- control flow;
- data structures;
- Runtime API access.

### `bytecode.md`

Defines the contract between Compiler and VM.

Expected topics:

- `.bc` file structure;
- instruction encoding;
- instruction set;
- function representation;
- constants;
- Runtime/native call encoding.

### `runtime-api.md`

Defines the API visible to programs running inside the VM.

Expected areas:

- threads;
- IPC;
- camera;
- location;
- orientation;
- networking;
- files;
- time;
- Activity lifecycle;
- WebView;
- Window Manager access.

### `ipc.md`

Defines communication between local application processes.

Primary scenario:

```text
Analyzer process
   ↓
IPC
   ↓
Map process
```

Expected topics:

- message types;
- payload format;
- serialization;
- delivery behavior.

### `broker-protocol.md`

Defines communication with the message Broker.

Expected topics:

- transport;
- framing;
- publish/subscribe commands;
- topics;
- message format.

### `data-model.md`

Defines shared domain structures.

Expected entities include:

- `Detection`;
- `Observation`;
- `Target`.

### `sql.md`

Defines the SQL subset supported by the custom DBMS.

Only SQL required by the Target Service should be included.

## Rules

- Do not design APIs without a concrete project scenario that requires them.
- Keep contracts minimal.
- Avoid implementation-specific details unless they are part of interoperability.
- If a public contract changes, update the corresponding specification in the same Pull Request.
- Update both sides of the affected interface together whenever possible.
- Add or update an integration test for contract changes.
- If a decision is not fixed yet, keep it in `docs/ai-context/02-open-questions.md` instead of inventing a specification.
