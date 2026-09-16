# Server side

Status: draft

This document describes the current server-side architecture of `tac-os`.

The central node runs on a separate Linux device in the same local network as the clients.

## Components

The server side consists of three main architectural components:

```text
Broker
   ↓
Target Service
   ↓
DBMS
   ↓
Database
```

These components have different responsibilities and should remain logically separate even if one developer owns more than one of them.

## Broker

The Broker is responsible for message delivery using a publish/subscribe model.

It should know how to:

- accept client connections;
- register subscriptions;
- receive published messages;
- route messages to subscribers;
- maintain topic subscriptions.

The Broker must not contain target-domain business logic.

It should treat message payloads as opaque data whenever possible.

### Initial logical topics

The initial design expects at least:

```text
observations
targets.updated
```

### Intended flow

Clients publish observations:

```text
Client
   ↓ publish
observations
   ↓
Broker
   ↓
Target Service
```

The Target Service publishes aggregated target updates:

```text
Target Service
   ↓ publish
targets.updated
   ↓
Broker
   ↓
Clients
```

The exact transport, framing, serialization format, and delivery guarantees are not fixed yet.

## Target Service

The Target Service contains target-domain backend logic.

It is responsible for:

- consuming `Observation` messages;
- grouping observations by target identifier;
- reading existing target state;
- aggregating observations from multiple clients;
- calculating or updating target coordinates;
- calculating target probability;
- applying time-based decay;
- writing updated state through the DBMS;
- publishing target updates through the Broker.

The Target Service is the only server-side component that should understand the business meaning of `Observation` and `Target`.

## DBMS

The DBMS is the project's custom SQL database management system.

It is responsible for:

- parsing the supported SQL subset;
- executing SQL statements;
- managing tables;
- reading stored data;
- updating stored data;
- writing persistent data to storage.

The DBMS should not know:

- what a tactical target is;
- how probability is calculated;
- what Broker topics exist;
- which clients are connected.

It operates on generic tables, rows, and SQL statements.

## Database

The Database is the persistent data managed by the DBMS.

It is not a separate executable or business service.

Conceptually:

```text
Target Service
   ↓ SQL
DBMS
   ↓
persistent data
```

The exact on-disk storage format is not fixed yet.

## Broker and DBMS are independent

The Broker and DBMS should not communicate directly.

Incorrect model:

```text
DBMS
   ↓
Broker
```

Intended model:

```text
Broker
   ↓
Target Service
   ↓
DBMS
```

The Target Service acts as the boundary between message-driven communication and persistent storage.

## Example processing flow

A client detects a target and produces an observation.

```text
Client A
   ↓
publish Observation
   ↓
Broker
   ↓
Target Service
```

The Target Service then:

```text
1. receives Observation
2. loads relevant data through SQL
3. aggregates observations
4. recalculates Target state
5. stores the updated Target through SQL
6. publishes TargetUpdated
```

Then:

```text
Target Service
   ↓
publish TargetUpdated
   ↓
Broker
   ↓
Client A
Client B
Client C
```

## Relationship to existing technologies

Conceptually:

- Broker is similar in role to MQTT or RabbitMQ;
- Target Service is similar to an application backend service;
- DBMS is similar in role to SQLite/PostgreSQL, but intentionally much smaller.

The project must implement the required components itself instead of replacing them with these technologies.

## Process model

The current preferred model is to run server components as separate Linux processes:

```text
central Linux node
├── broker
├── target-service
└── dbms
    └── database storage
```

This process separation is a current architectural preference, not yet a finalized low-level deployment specification.

## Client interaction

Clients interact with the Broker for message exchange.

The intended minimal model is:

```text
Client
├── publish `Observation`
└── subscribe to `targets.updated`
```

The Target Service is also a Broker client:

```text
Target Service
├── subscribe to `observations`
└── publish `targets.updated`
```

The DBMS is not connected to the Broker.

## Design rules

- Broker handles delivery, not business meaning.
- Target Service handles target-domain logic.
- DBMS handles SQL and persistence.
- Database is stored data, not a service.
- Broker and DBMS do not communicate directly.
- Do not add server components unless required by a concrete project scenario.
- Keep protocols minimal.
- Keep server-side responsibilities separate even if they share one owner.
