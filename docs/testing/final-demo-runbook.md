# TAC OS final demonstration runbook

Status: **preparation draft for issue #150**, inspected against `main` at
`3877600e4e4a6fa06bc8081d849aa5f4913fcf96` on 2026-11-18.
This runbook **does not certify** the final Tactical App. Requirements and
claims are tracked in [the evidence matrix](requirements-evidence.md).
The intended final deadline is **30 November 2026**.

## Demonstration acceptance: what must happen

The full demonstration is a **single integrated scenario**, not merely a
compilation test or multiple independent prototypes:

1. Two Linux client executions run the custom Compiler-generated `.tc`
   Analyzer and Map in separate `tacvm` processes.
2. A permitted real camera frame or approved image is processed by a real
   vision model, and the ArUco marker provides a separate verified ID,
   relative bearing and distance. Do not claim that AI identifies an ArUco ID.
3. The Analyzer uses device pose/time to calculate an `Observation` and
   sends it through Runtime IPC to Map. Map renders the local sighting
   before a central-server response.
4. Map publishes the six-field `Observation` through the custom Broker.
   Target Service aggregates two independent observers, stores domain records
   through the custom DBMS, and publishes `targets.updated`.
5. Map subscribes and redraws the `Target`. Its probability changes after
   a second observer and decays in the absence of new observations.
6. A DBMS restart preserves the intended SQL state. A Broker outage does not
   crash the client; the implemented stale-data behavior is demonstrated.
7. The demonstration records actual component versions, available CI checks,
   environment, output and accepted limitations.

Do **not** substitute the existing Go Map prototype for the final `.tc`
Map or call an OpenRouter mock a real model inference.

## What can be executed today on current main

### A. Build and CTest (baseline platform checks)

Linux with CMake 3.16+, a C++17 toolchain, JDK 17, Go 1.22+ and the
project's Gradle dependencies must be prepared in advance.

From the repository root:

```sh
(cd compiler && ./gradlew installDist)
cmake -S . -B build
cmake --build build -j2
ctest --test-dir build --output-on-failure
```

Inspect the actual number of tests reported; do not write a fixed pass
count into the final report. The current baseline exercises Compiler/VM,
Float, IPC, Go Map/WM, SQL server and existing smoke tests, but is **not**
an end-to-end camera-to-map proof.

### B. Reproducible Linux system smoke test

From the repository root:

```sh
(cd tests/system-smoke && GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go run -mod=readonly .)
```

Expected success includes the independent legs:

```text
PASS build components (offline, headless)
PASS Broker -> Target Service (matching Observation receipt)
PASS Map -> headless Window Manager (frame, markers, CLOSE, reuse)
PASS unavailable Broker
PASS cleanup
```

Source of truth: [system-smoke README](../../tests/system-smoke/README.md).
This is **not** the full server chain. The service currently stores an
incoming observation in memory and does not automatically publish an
aggregated `Target`. The Go Map still draws fixture targets.

### C. Optional native Linux window demonstration (Go reference only)

The following requires a graphical Linux session with SDL2 development
packages (for example `pkg-config` and `libsdl2-dev`):

```sh
(cd window-manager && go build -tags sdl -o /tmp/tac-wm-sdl ./cmd/wm-server)
(cd app/map && go build -o /tmp/tac-map-demo ./cmd/map-demo)
```

Run the WM and Map in separate terminals:

```sh
/tmp/tac-wm-sdl -backend sdl
```

```sh
/tmp/tac-map-demo -config app/map/assets/test-map.json
```

Expected: a 960×640 map window with test targets. Close Map with the
native close control; WM should remain running. Record that this uses
**Go Map**, not the final `.tc` app. Use the detailed verification in
[Map–WM testing](map-wm.md) rather than relying on this runbook alone.

## Pending final-stage commands: do not present these as runnable yet

The following boundaries must be integrated and tested before replacing
the above baseline:

| Stage | Current barrier / proof needed |
| --- | --- |
| `.tc` values and serialization | #127/#132/#133: approved JSON/String helpers, runtime kinds, error and lifetime rules |
| Runtime sensor/network/WM | #128/#134/#135/#136: actual calls and a single event loop that does not block |
| Analyzer | #138/#142: compute and **send** actual Observation through IPC, not only fixture vectors |
| Map | #139/#143: subscribe, publish, show live Target and local observations on `.tc` |
| Central server | #130/#137: actual aggregation, SQL writes and automatic Target publication |
| Persistence | #131/#147: independently verified restart of the real SQL server |
| Final E2E | #140/#149: two observers, no injected Target, negative cases, cleanup |
| AI vision | #104/#129/#141/#142: permission, selected model, real inference and approved Runtime handoff |

Follow [Sprint 5 tracker](https://github.com/itmo-mobile-app/tac-os/issues/126).
Do not invent a final launcher command before the real entry points,
arguments, process ownership and startup order exist. The owner of #149
should provide one definitive Linux command and add it here when the
scenario runs.

## AI verification is a separate, explicit acceptance gate

OpenRouter access is prepared in **Draft PR #159**, not in `main`.
The CLI under `tools/openrouter-vision` sends a *consented* still image
to an external model and validates a restricted `status` / `bbox` answer.
Its offline `httptest` checks do not demonstrate actual recognition.

With an authorized key **held locally**, and only on that feature branch:

```sh
git fetch origin
git switch feat/issue-129-openrouter-poc
cd tools/openrouter-vision
export OPENROUTER_MODEL='provider/verified-image-capable-model'
read -rs OPENROUTER_API_KEY
export OPENROUTER_API_KEY
go run . -image /path/to/consented-test-image.jpg -send-image
```

Never paste the key, request body or private image into GitHub or chat.
Record a fixture identifier, chosen model slug, output status, latency,
invalid/negative results and provider error behavior — not secrets.
Return to the tested checkout before running baseline checks.

Cloud OpenRouter requires internet. Its relationship to local-network-only
NFR-1 and the own-server constraint C-8 must be explicitly approved by
the instructor; silence is not approval. Offline ArUco remains useful but
does not satisfy AI requirement C-13.

## Evidence capture checklist for demo day

- Write down the checked-out `main` commit SHA, Linux distribution,
  Go/JDK/CMake/C++ compiler versions and exact launch commands.
- Confirm all intended PRs are merged and all critical CI jobs are green.
  A PR-only E2E is **not** proof of deployed `main`.
- Record a real AI positive and negative example without leaking images.
- Show successful compilation of **both** `.tc` applications.
- Show separate Analyzer/Map VM processes, IPC, local rendering, and
  Broker round-trip using real Target Service and DBMS.
- Show first and second independent sightings, decay/expiry and DBMS
  restart recovery.
- Show recovery/failure behavior on Broker outage and window close.
- Stop/restart components and confirm that sockets, files and child
  processes are cleaned up; do not kill the shared WM from an app.

If a gate cannot be demonstrated, mark it **NOT VERIFIED** with a reason.
Do not silently replace the missing stage with a mock, Go app or canned
Target message.

## Release timeline

- **By 19 Nov:** first connected `.tc` E2E using controlled sensor
  fixtures where necessary (planned checkpoint, not yet verified).
- **By 22 Nov, 20:00 MSK:** functional freeze and actual integrated
  Linux acceptance, including a separate permitted real AI request.
- **23–29 Nov:** fixes, Linux environment rehearsal, evidence collection
  and correction of architectural/requirements documentation.
- **30 Nov:** final demonstration and submission.

Issue #150 owns this runbook and the evidence matrix. Contributors
update **observed results**, links to merged PRs and verified commands,
not optimistic projections.
