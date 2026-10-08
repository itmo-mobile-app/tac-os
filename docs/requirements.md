# Requirements

Status: draft

This document describes the problem domain, the stakeholders, and the functional and non-functional
requirements of `tac-os`. Architecture is described in `docs/ai-context/01-architecture.md`; public
contracts are in `spec/`. Decisions that are not made yet are listed in
`docs/ai-context/02-open-questions.md` and are referenced here instead of being invented.

## Problem domain

### Airsoft match

Airsoft is a team game in which players with replica guns try to hit players of the opposing team.
A match is played by two or more teams on a bounded outdoor area (forest, abandoned buildings), usually
without reliable mobile internet.

The hardest part of the game is situational awareness: a player sees an opponent for a moment, but the
rest of the team does not know where that opponent is. `tac-os` gives the whole team a shared picture
of where opponents were recently seen.

### How the system is used

1. Every player wears a visible marker that identifies them.
2. A player points the device camera at an opponent. The device recognizes the opponent's marker,
   estimates direction and distance, and combines them with its own position and orientation to
   calculate the opponent's coordinates.
3. The device shows the detection on its map immediately and sends it to the central node.
4. The central node combines detections of the same opponent from several devices into one target
   whose probability grows with fresh, independent observations and decays over time.
5. Every device of the team shows the aggregated targets on a shared 2D map.

### Glossary

| Term | Meaning |
| --- | --- |
| Player | A participant of the match who carries a client device and wears a marker |
| Team | A group of players playing together; opponents are players of other teams |
| Organizer | The person who runs the match and the central node on site |
| Marker | A visible tag worn by a player that identifies them to the camera |
| Detection | A local recognition result on one device: marker id, direction, distance, confidence |
| Observation | One device's report about a target with calculated coordinates (`spec/data-model.md`) |
| Target | The aggregated state of one opponent: position, probability, last seen time (`spec/data-model.md`) |
| Probability | The system's confidence that the target is at its current position; decays over time |
| Tag | An opponent confirmed as hit by the game rules (rule not decided yet) |
| Central node | The Linux device on site that runs the Broker, the Target Service, and the DBMS |
| Tactical App | The client application the player uses; internally the Analyzer and Map processes |

## Stakeholders

| Stakeholder | Interest | Weight |
| --- | --- | --- |
| Course instructor (Klyuchev A.O., ITMO) | The platform is built from scratch according to the assignment | Hard constraints, not negotiable |
| Players | Fast, simple, reliable picture of where opponents are | Drive functional requirements |
| Organizer | Easy deployment on site without internet, reuse across matches | Drive deployment requirements |
| Development team (9 people) | Small verifiable increments, parallel work, the deadline | Drive process and scope |

When the assignment and the airsoft scenario conflict, the assignment wins: the airsoft scenario gives
content to the requirements but cannot remove a required platform component.

### Course instructor: hard constraints

- C-1. A custom programming language is used for the mobile application; existing languages and
  interpreters are not allowed for it.
- C-2. A custom compiler translates the language into custom bytecode.
- C-3. A custom virtual machine executes the bytecode.
- C-4. The platform provides threads and IPC (an analogue of Android `Intent`).
- C-5. A window manager and an Activity-like lifecycle are part of the platform.
- C-6. A custom SQL DBMS (an analogue of SQLite) with its own parser, executor, and storage.
- C-7. A custom message broker for data exchange between several mobile processes or applications.
- C-8. Missing server components are written by the team; ready-made servers are not allowed.
- C-9. The result is a working mobile application built on this stack, not separate demos.
- C-10. Linux is the base operating system; running on a laptop with Linux is acceptable.

### Players

- P-1. I point the camera at an opponent and see them on the map without noticeable delay.
- P-2. I see my own position on the same map as the detected opponents.
- P-3. The system does not confuse my teammates with opponents.
- P-4. When the connection to the central node is lost, the app keeps working and shows the last known
  state.
- P-5. The interface is one screen with the map; nothing has to be configured during the game.

### Organizer

- O-1. I run the central node on one device on site, without internet.
- O-2. Everything works in the local network of the site (own Wi-Fi or LAN).
- O-3. I connect any number of client devices to a match without configuring each one by hand.
- O-4. I reuse the same setup for several matches in a row without reinstalling.

### Development team

- T-1. Every step is a small verifiable increment (see `AGENTS.md`).
- T-2. The nine areas are isolated by ownership and can be developed in parallel.
- T-3. The project is finished by early December, ideally within two months.
- T-4. Game rules that are not decided are kept as open questions, not decided implicitly in code.

## Functional requirements

Each requirement lists the components that implement it.

### Detection (client)

| ID | Requirement | Components |
| --- | --- | --- |
| FR-1 | The app reads frames from the device camera | Analyzer, Runtime |
| FR-2 | The app recognizes a player marker in a frame and reads its identifier | Analyzer, Runtime |
| FR-3 | The app estimates the direction (angle from the camera axis) and distance to a recognized marker | Analyzer, Runtime |
| FR-4 | The app reads the device position and orientation | Analyzer, Runtime |
| FR-5 | The app calculates the target coordinates and creates an `Observation` with a confidence value | Analyzer |
| FR-6 | The app ignores markers of the player's own team | Analyzer (encoding not decided, see open questions → "Application") |

### Map (client)

| ID | Requirement | Components |
| --- | --- | --- |
| FR-7 | The Analyzer passes every new observation to the Map process over IPC (`spec/ipc.md`) | Analyzer, Map, Runtime |
| FR-8 | The Map shows a local observation immediately, before any answer from the central node | Map |
| FR-9 | The Map publishes observations to the central node (`observations` topic) | Map, Broker |
| FR-10 | The Map subscribes to aggregated targets (`targets.updated` topic) and updates the map | Map, Broker |
| FR-11 | The Map shows every target with its probability and how long ago it was seen | Map, Window Manager |
| FR-12 | The Map shows the player's own position | Map, Runtime |
| FR-13 | When the central node is unavailable, the Map keeps the last known targets and shows that the data is stale | Map (behavior not decided, see open questions → "Application") |

### Central node

| ID | Requirement | Components |
| --- | --- | --- |
| FR-14 | The Broker delivers messages by topic between any number of publishers and subscribers (`spec/broker-protocol.md`) | Broker |
| FR-15 | The Target Service consumes observations and aggregates observations of the same `targetId` into one `Target` | Target Service |
| FR-16 | Fresh independent observations increase target probability; probability decays over time | Target Service (formulas not decided, see open questions → "Target Service") |
| FR-17 | The Target Service stores targets and observations in the DBMS and publishes every target update | Target Service, DBMS, Broker |
| FR-18 | The DBMS executes the SQL subset in `spec/sql.md` and keeps data between restarts | DBMS |

### Platform

| ID | Requirement | Components |
| --- | --- | --- |
| FR-19 | `.tc` programs are compiled to `.bc` and executed by the VM (`spec/language.md`, `spec/bytecode.md`) | Compiler, VM |
| FR-20 | Programs call platform functions through the Runtime API (`spec/runtime-api.md`) | VM, Runtime |
| FR-21 | Applications create and show windows and receive input through the Window Manager | Window Manager, Runtime |
| FR-22 | Applications have an Activity-like lifecycle | Runtime, Window Manager |

## Non-functional requirements

| ID | Requirement |
| --- | --- |
| NFR-1 | The whole system works in a local network without internet access |
| NFR-2 | A local detection appears on the player's own map within about one second |
| NFR-3 | An aggregated target update reaches all connected clients within a few seconds |
| NFR-4 | Every platform component is implemented by the team (constraints C-1 to C-8) |
| NFR-5 | Clients and the central node run on Linux; client devices may be laptops |
| NFR-6 | Losing the connection to the central node does not crash the client (P-4) |
| NFR-7 | The VM and the Runtime are written in C++17 and built with sanitizers in CI; the compiler is written in Kotlin |
| NFR-8 | Every pull request is checked by CI: build, integration tests, Markdown lint |

## Out of scope

- Own kernel, drivers, or TCP/IP stack.
- Real phones and mobile operating systems; client devices are Linux machines.
- Hit detection by the replica guns themselves; "tag" is a game rule on top of observations.
- Accounts, authentication, and security of the local network.
- Internet services, maps downloaded from the internet, and cloud storage.

## Not decided yet

These questions affect the requirements above and are tracked in
`docs/ai-context/02-open-questions.md`:

- how a marker encodes the team, so that FR-6 can be implemented ("Application");
- the tag rule and what happens to a tagged player ("Application");
- the client behavior when the central node is unavailable, FR-13 ("Application");
- probability and time decay formulas, FR-16 ("Target Service");
- whether the match history (who saw whom and when) must be kept after the match ("Application");
- the number of teams in a match and whether every team runs its own central node ("Application");
- the map coordinate system and the source of the map image ("Application").
