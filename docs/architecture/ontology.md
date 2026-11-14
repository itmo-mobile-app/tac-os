# TAC OS domain and system ontology

Status: descriptive model for architecture review (#123), **not** a new wire contract.
The current schemas in `spec/data-model.md`, `spec/ipc.md`, and the existing
component specifications remain authoritative. This model describes the system
at several different viewpoints without treating a class, an OS process,
an event and a physical object as equivalent kinds of thing.

## Scope, identity and interpretation

The subject is one outdoor airsoft game, its client devices, the team's local
network and the Linux platform. The model deliberately does not attempt to
represent weapon hits, authentication, every real-world person or every
physical signal inside a sensor. Names on diagrams are **model terms**;
a box does not become an autonomous running process merely because it is drawn.

| Term | Kind | Identity / lifetime | Meaning and ownership |
| --- | --- | --- | --- |
| Player | Physical person in a match | Physical continuity of a person; match membership may change | Wears a marker; is not the same thing as the Target data record |
| Team | Social role / grouping | Match-scoped membership, not derived from a process | Marker ID encodes team by `id / 12`; match roster is not implemented |
| Marker | Physical object attached to a player | Marker ID within configured ArUco dictionary; can be reassigned | Recognized by Marker Detector; **not** an immutable global person ID |
| Client device | Physical computing device | Device identity/observerId is not yet specified; exists independently of the app | Camera, position, heading and Linux OS; user-facing client |
| Camera frame | Captured measurement / image value | One exposure or supplied test image; transient | Visual evidence, not a target or an observation; may contain several people/markers |
| Detection | Local computed measurement | One detection in a frame; ephemeral; `id` relates to marker | `id`, `relativeAngle`, `distance` from Go Marker Detector; no confidence |
| AI recognition result | Model output, not verified fact | One model request/response; may be missing or invalid | Candidate presence/class/region from vision model; cannot invent marker ID or world position |
| Observation | Timestamped reported **event/fact** | Event is fixed once reported; `(targetId, observerId, timestamp)` may deduplicate but is not an independently allocated event ID | Six-field report from Analyzer, forwarded through Map; `confidence` is local estimate |
| Target | **Derived state** for one marker ID, not a player instance | Keyed by `targetId`; rebuilt as observations arrive/expire | Aggregated coordinates, `probability`, `lastSeenAt` computed by Target Service |
| Message | Transmitted serialized value | One attempted delivery; no durable message ID in v1 | `OBSERVATION` IPC or Broker payload; at-most-once, no implied acknowledgement |
| Coordinate / confidence / probability | Values, not entities | Equal values have no separate identity | WGS84 degrees / finite bounded numbers; local confidence differs from aggregate probability |
| Activity | Application lifecycle role / state model | Per running app/window, not the physical player | Starts, shows and closes a window via Runtime/WM; minimal contract only |
| OS process | Linux execution instance | PID/lifetime on a particular machine | Separate `tacvm` instances for Analyzer and Map; WM, helper, Broker, Target Service, DBMS |
| VM execution context | In-memory execution state | Lifetime of a `run_file` call, not an OS process by definition | Instruction pointer, stack, frames, locals; independent contexts can run in host threads |
| Program/source file | Formal description / artifact | Repository path and content revision | `.tc` denotes intended behavior, `.bc` encodes it; neither executes itself |
| Component / function | Designed unit / implementation construct | Identified by an interface/code definition | Compiler, VM, Runtime, Broker etc. are responsibilities; an instance may run inside a process |

A user can observe a physical player while the program reports a `Detection`
of a marker, synthesizes an `Observation` and derives a `Target`. These
objects are related, **not interchangeable**. A `Target` may become stale
without any change in the physical player's existence.

### Relationships and invariants

1. One physical player may wear one configured marker for a given demo;
   the design does not prove persistent identity beyond the marker.
2. A camera frame may produce zero or more ArUco detections and a separate
   AI recognition result. Only ArUco supplies the current `targetId`,
   bearing offset and distance. AI does not implicitly alter them.
3. Analyzer combines a valid detection with device pose to create one
   `Observation`; it owns global coordinate calculations and local confidence.
4. Map receives local `Observation` via IPC and forwards the unchanged
   six-field domain object to the Broker; Broker only routes messages.
5. Target Service deduplicates observations from one observer, groups compatible
   coordinates and computes the five-field `Target` including time decay.
6. DBMS stores SQL data; it does not interpret player identity or confidence.
7. A code component is a static responsibility, while an OS process is one
   deployment/runtime instance. In particular, Runtime is linked into each
   `tacvm` process and is **not** its own operating-system process.

The specified observer ID allocation, persistent human-player identity,
sensor calibration, AI model/output, and offline-service exception are still
open questions. See `docs/ai-context/02-open-questions.md`; do not silently
claim that `targetId` uniquely names a person across matches.

## Correspondence with the real world

| Phenomenon in the game / on hardware | Computational observation or abstraction | Information deliberately lost |
| --- | --- | --- |
| Light reflected by player/marker enters camera | Camera frame; ArUco detector yields ID, angle and distance | Lens distortion, lighting, marker occlusion and unmeasured uncertainty |
| A physical person is present in a scene | AI model's candidate classification or region | Stable human identity and ground-truth reliability are not guaranteed |
| Device is at a location and points in a direction | Latitude/longitude, true-north heading, timestamp from Runtime | GPS error, magnetic declination correction details, full 3D pose |
| Physical player moves between sightings | A time series of `Observation` reports | Continuous trajectory and exact movement are not modeled |
| Devices exchange radio/electrical signals | Linux TCP/Unix streams and message contracts | Retransmissions, electromagnetic propagation, dropped packets and transport timings |
| Participants form a team and report sightings | Broker pub/sub and Target Service evidence model | Actual statistical independence and every game rule |
| The screen displays targets | WM window with drawn markers | Physical display latency and subjective interpretation by a player |

The prediction/uncertainty represented by `Target.probability` is a
**heuristic trust in a reported location**, not a calibrated posterior
probability of a human being present. Spatial grouping and decay parameters
are documented in `spec/target-aggregation.md`.

## Three distinct notions of level

- **Level of abstraction:** the same subject described with fewer or more
  details (e.g. "Tactical App" then "Analyzer/Map" then VM calls).
- **Architecture layer:** a responsibility/dependency boundary, e.g.
  `.tc` application → VM → Runtime → Linux. This is not a taxonomy of
  physical entities, and does not put every server into a single stack.
- **Viewpoint / cut:** another question asked of the same system: business
  workflow, logical data exchange, process deployment, platform dependencies,
  build transformation, source/runtime internals. The six drawings are
  **different viewpoints**, not a universal linear ordering from 1 to 6.

Different models of computation apply **within** these views:
a VM interpreter has a sequential imperative transition model; independently
scheduled Linux processes communicate over streams; Broker distributes
messages asynchronously with at-most-once delivery; the Target Service also
has a periodic time-decay trigger. A static graph of boxes alone does not
define timing, determinism or delivery guarantees.

For per-diagram semantics, physical correspondence and local legend, see
`docs/architecture/README.md` and the legends **inside** each SVG. The
overview `docs/architecture/viewpoints.svg` is a map of viewpoints, not an
execution flow or deployment diagram.

## Model boundaries and known discrepancies

- `1-business.svg` shows a game narrative, not the real execution order.
  Its marker-recognition step does **not** depict the required new AI vision
  inference; this remains a separate planned path (#104/#129).
- `2-logical.svg` intentionally omits the Broker (transport mediator) and
  VM/Runtime. Its direct arrows denote **logical** information exchange,
  not physical direct connections.
- `3-processes.svg` is a **planned deployment topology**. Some executable
  components are already prototyped, while the final .tc Analyzer/Map and
  end-to-end integration are not yet complete.
- `4-platform.svg` uses VM/Runtime **inside** each client process; WM and
  the helper are separate Linux processes.
- `5-build.svg` depicts an example compiling and running `hello.tc`.
  A source artifact is not a running actor.
- `6-code.svg` shows a synchronous VM/Runtime call path within one process,
  not every VM feature or the complete application event loop.
- Several prose descriptions still say "six levels like OSI"; the corrected
  interpretation is six views, with a **layered platform view only**.
  See the root README updated under #123.

## Reading and validating the model

When editing a diagram, state: **what does each node mean, what does an edge
mean, what is its model of computation, which physical phenomena are abstracted,
and which details are excluded?** Verify claims against current `spec/`
contracts and repository code. Do not turn unexplained assumptions into
wire-protocol changes or new components merely to make a diagram look neat.
