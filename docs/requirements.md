# Requirements

Status: draft

This is the agent-facing source of truth for requirements. A short human summary in Russian is kept in the root
`README.md` → "Требования"; update it in the same Pull Request when requirements change.

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
of where opponents and teammates were recently seen.

### How the system is used

1. Every player wears a visible marker that identifies them.
2. A player points the device camera at another player. The application must also demonstrate
   AI-based visual recognition of a player/target in a camera image. Existing ArUco marker recognition
   remains responsible for marker identity, relative direction and estimated distance; Analyzer
   combines measurements and device pose to calculate target coordinates.
3. Analyzer calculates an `Observation` from the local detection, sends it to Map
   via IPC, and Map shows it immediately and publishes it to the central node.
4. The central node combines observations of the same player from several devices into one target
   whose probability grows with fresh, independent observations and decays over time.
5. Every device of the team shows the aggregated targets on a shared 2D map.

### Glossary

| Term | Meaning |
| --- | --- |
| Player | A participant of the match who carries a client device and wears a marker |
| Team | A group of players playing together; opponents are players of other teams |
| Organizer | The person who runs the match and the central node on site |
| Marker | A visible tag worn by a player that identifies them to the camera |
| Detection | A private Go detector result: ArUco marker ID, camera-relative angle and distance; the detector does not output confidence (`spec/camera-detector-protocol.md`) |
| Observation confidence | Reliability assigned by Analyzer to a single local Observation, distinct from aggregated target probability |
| Observation | One device's report about a target with calculated coordinates (`spec/data-model.md`) |
| Target | The aggregated state of one detected player, teammate or opponent: position, probability, last seen time (`spec/data-model.md`) |
| Probability | The system's confidence that the target is at its current position; decays over time |
| Tag | An opponent confirmed as hit by the game rules (rule not decided yet) |
| Central node | The Linux device on site that runs the Broker, the Target Service, and the DBMS |
| Tactical App | The client app whose `.tc` logic runs in two separate VM processes, Analyzer and Map |
| Marker Detector | Private auxiliary Go + OpenCV Linux process behind Runtime Camera API, not a third Tactical App process |
| AI recognition | Vision-model analysis of camera imagery to recognize visible players/targets; output and interface are not yet approved |

## Stakeholders

| Stakeholder | Interest | Weight |
| --- | --- | --- |
| Course instructor (Klyuchev A.O., ITMO) | The platform is built from scratch according to the assignment | Hard constraints, not negotiable |
| Players | Fast, simple, reliable picture of where opponents are | Drive functional requirements |
| Organizer | Easy deployment on site without internet, reuse across matches | Drive deployment requirements |
| Development team (10 people, nine core ownership areas) | Small verifiable increments, parallel work, the deadline | Drive process and scope |

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
- C-11. Technology stack set by the course: the custom language for the mobile application; C/C++ for the
  virtual machine (a simplified analogue of ART); Kotlin (with ANTLR4) for the compiler; Go or Rust for all
  other components; Markdown or PDF for documentation; git for project management.
- C-12. The map shows both own and enemy targets. Target reliability grows with the number of players who see
  the target and decreases with the time since it was last seen.
- C-13. Updated instructor requirement (October 2026): the demonstration must include AI-based visual
  recognition of players/targets, not just ArUco marker recognition. Marker identification and geometry
  remain useful but alone no longer satisfy this requirement. OpenRouter was suggested as a possible
  vision-model API, not approved as the final architecture or model. The relationship to C-8, offline
  operation and the exact recognition output must be agreed before implementation.

### Players

- P-1. I point the camera at an opponent and see them on the map without noticeable delay.
- P-2. I see my own position on the same map as the detected opponents.
- P-3. I see teammates and opponents on the map and can tell them apart.
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
| FR-1 | A Runtime-managed detector reads camera frames for the app | Runtime, Marker Detector |
| FR-2 | The demonstration includes AI visual recognition of players/targets (C-13); the existing private ArUco detector continues returning marker identifiers, angles and distances via the approved Runtime Camera API. The AI integration and result contract remain to be defined. | Analyzer, Runtime, Marker Detector; AI provider integration pending |
| FR-3 | The detector estimates the camera-relative angle and distance to a marker and exposes them through Runtime | Marker Detector, Runtime, Analyzer |
| FR-4 | The app reads the device position and orientation | Analyzer, Runtime |
| FR-5 | The app calculates the target coordinates and creates an `Observation` with a confidence value | Analyzer |
| FR-6 | The app tells teammates from opponents by the marker, and the Map shows them differently | Analyzer, Map (marker ID encodes team as `id / 12`; which teams participate in a particular match is separate) |

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
| FR-18 | The DBMS runs as a separate process, executes the SQL subset in `spec/sql.md` received from the Target Service, and keeps data in a file between restarts | DBMS, Target Service |

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
| NFR-7 | Implementation languages follow C-11: the VM and the Runtime in C++17, the compiler in Kotlin, the Window Manager, Broker, Target Service, and DBMS in Go or Rust |
| NFR-8 | Every pull request is checked by CI: build, integration tests, Markdown lint |

**Open requirement conflict:** NFR-1 and the exclusion of internet services below describe the
existing offline platform. An OpenRouter vision API would require an internet connection and an
external model provider. It may be suitable for the mandatory AI demonstration, but it cannot be
presented as part of the offline path until the instructor explicitly approves the exception or
an offline AI alternative is chosen. The 1-second local feedback goal must not assume cloud-model latency.

## Out of scope

- Own kernel, drivers, or TCP/IP stack.
- Real phones and mobile operating systems; client devices are Linux machines.
- Hit detection by the replica guns themselves; "tag" is a game rule on top of observations.
- Accounts, authentication, and security of the local network.
- Internet services, maps downloaded from the internet, and cloud storage remain outside the
  offline platform. The instructor-proposed external AI recognition service is a candidate exception
  requiring an explicit decision; it is not currently an approved dependency.

## Not decided yet

These questions affect the requirements above and are tracked in
`docs/ai-context/02-open-questions.md`:

- which teams participate in a particular match; marker ID-to-team encoding is already fixed ("Application");
- the tag rule and what happens to a tagged player ("Application");
- the client behavior when the central node is unavailable, FR-13 ("Application");
- probability and time decay formulas, FR-16 ("Target Service");
- whether the match history (who saw whom and when) must be kept after the match ("Application");
- the number of teams in a match and whether every team runs its own central node ("Application");
- the map coordinate system and the source of the map image ("Application");
- whether OpenRouter or an offline vision model will satisfy C-13; what constitutes recognition,
  who sends camera images, model/output contract, privacy, cost, response time and failure behavior,
  and explicit instructor approval for the NFR-1/C-8 exception ("AI target recognition").
