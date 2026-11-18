# Requirements and acceptance evidence

Status: **pre-release evidence inventory** for issue #150, as reviewed on
2026-11-18. Based on [requirements](../requirements.md),
[the runbook](final-demo-runbook.md) and the currently available `main`
baseline `3877600e4e4a6fa06bc8081d849aa5f4913fcf96`.

This document is **not a report of a successfully completed final demo**.
At this checkpoint, Sprint 4 implementation PRs are open and no complete
`.tc` Analyzer → server → `.tc` Map path was established in `main`.

## Rules for evidence labels

- **BASELINE** — a direct test or runnable current-main component proves
  this **limited** capability, not necessarily every detail of the final app.
- **PARTIAL** — there is a narrower implementation, fixture or unmerged PR,
  but the complete FR/C constraint is **not established**.
- **NOT VERIFIED** — no current-main demonstration establishes the required
  behavior. A design document or green unrelated CI is insufficient.

Evidence must cite an executable test, an approved specification and/or
a **merged** PR. Open PRs are useful work-in-progress links, not evidence
that those changes are part of `main`.

## Functional requirements (FR-1 through FR-22)

| Requirement | Status | Evidence and remaining acceptance |
| --- | --- | --- |
| FR-1 Runtime-managed camera detector | PARTIAL | Existing C++ Runtime ↔ Go helper, tested in fake mode (#94); live camera and full app are unverified |
| FR-2 AI recognition of visible players | NOT VERIFIED | [#104](https://github.com/itmo-mobile-app/tac-os/issues/104), [Draft #159](https://github.com/itmo-mobile-app/tac-os/pull/159); offline tests use mock HTTP, not a real model |
| FR-3 ArUco angle/distance via Runtime | PARTIAL | Existing detector pose tests and fake Camera API; live-calibrated end-to-end demo pending |
| FR-4 device position and orientation | NOT VERIFIED | [#134](https://github.com/itmo-mobile-app/tac-os/issues/134), proposed Runtime contract [#153](https://github.com/itmo-mobile-app/tac-os/pull/153) |
| FR-5 Analyzer creates world-coordinate Observation | PARTIAL | `spec/analyzer-observation.md` vectors and [PR #157](https://github.com/itmo-mobile-app/tac-os/pull/157) .tc calculations; no live IPC publication of computed JSON |
| FR-6 teammates/opponents distinct | NOT VERIFIED | Marker ID team rule (`id / 12`) specified; final .tc Analyzer/Map distinction not demonstrated |
| FR-7 Analyzer → Map actual Observation IPC | PARTIAL | `ipc_vm_e2e` passes between two .tc VM processes, but not the dynamically calculated Analyzer Observation |
| FR-8 Map immediately displays local observation | NOT VERIFIED | Go Map renders fixtures and receives server Target updates; local-before-server .tc behavior pending [#143](https://github.com/itmo-mobile-app/tac-os/issues/143) |
| FR-9 Map publishes Observation | PARTIAL | Go Map Broker mode #99 uses JSONL fixture; .tc Map network publication pending [#135](https://github.com/itmo-mobile-app/tac-os/issues/135)/[#143](https://github.com/itmo-mobile-app/tac-os/issues/143) |
| FR-10 Map subscribes to Target updates | PARTIAL | Go Map listens to manually published `targets.updated`; server-generated update to .tc Map not demonstrated |
| FR-11 Map shows probability and age | PARTIAL | Go Map fixture opacity/probability verified; time-since-last-seen and final .tc UI not proven |
| FR-12 player's own position displayed | NOT VERIFIED | Runtime location [#134](https://github.com/itmo-mobile-app/tac-os/issues/134) and Map [#143](https://github.com/itmo-mobile-app/tac-os/issues/143) pending |
| FR-13 stale data after disconnection | PARTIAL | Go Map preserves last frame on Broker loss, but stale indicator/final .tc behavior not established |
| FR-14 custom Broker pub/sub | BASELINE | Existing custom Go Broker and integration tests; at-most-once contract in `spec/broker-protocol.md` |
| FR-15 observations aggregated by targetId | NOT VERIFIED | Current main Target Service retains last Observation only; independent core in [PR #155](https://github.com/itmo-mobile-app/tac-os/pull/155), not connected to service |
| FR-16 independent observer support/time decay | NOT VERIFIED | Spec + [PR #155](https://github.com/itmo-mobile-app/tac-os/pull/155) tests; server loop/ticker integration [#137](https://github.com/itmo-mobile-app/tac-os/issues/137) pending |
| FR-17 Target Service stores and publishes targets | NOT VERIFIED | Real `dbclient` has a separate E2E (#97), but current main server does not use it and publish `targets.updated` |
| FR-18 separate DBMS with persistent file | PARTIAL | Separate SQL server and real SQL-client test are verified; persistence exists only in [PR #151](https://github.com/itmo-mobile-app/tac-os/pull/151) |
| FR-19 .tc → .bc → VM | BASELINE | `tacc` + `tacvm` and CTest examples (including Float and map projection); final app incomplete |
| FR-20 Runtime API invoked from .tc | PARTIAL | `runtime.log` and IPC proven; device/Broker/WM API proposal [#153](https://github.com/itmo-mobile-app/tac-os/pull/153) and WM [#156](https://github.com/itmo-mobile-app/tac-os/pull/156) not in main |
| FR-21 windows and input via WM | PARTIAL | Real Go Map ↔ WM drawing and close verified; .tc path in [PR #156](https://github.com/itmo-mobile-app/tac-os/pull/156) not merged |
| FR-22 Activity-like lifecycle | PARTIAL | Minimal lifecycle contract (#95) and WM process test; full final .tc Activity behavior not independently demonstrated |

## Instructor constraints (C-1 through C-13)

| Constraint | Status | Evidence and remaining acceptance |
| --- | --- | --- |
| C-1 custom language for mobile app | PARTIAL | `.tc` language exists; final Analyzer and Map need to run on it rather than Go |
| C-2 custom .tc → custom bytecode compiler | BASELINE | Kotlin/ANTLR4 Compiler and `.bc` CTest |
| C-3 custom bytecode VM | BASELINE | C++ `tacvm`, Float, control flow and IPC CTests |
| C-4 threads and IPC | PARTIAL | Independent VM contexts in C++ threads (#93) and Analyzer/Map IPC examples; final platform threading/lifecycle demonstration needs explicit evidence |
| C-5 WM and Activity-like lifecycle | PARTIAL | Custom Go WM and minimal lifecycle contract; .tc WM PR #156 not merged |
| C-6 custom SQL DBMS with own storage | PARTIAL | Custom Go SQL parser/executor/socket in main; file storage PR #151 not merged |
| C-7 custom pub/sub broker | BASELINE | Custom Go Broker, not a ready-made server |
| C-8 all missing server components written by team | PARTIAL | Local Broker/Target Service/DBMS are team components; external OpenRouter exception still unresolved |
| C-9 one working app on this stack | NOT VERIFIED | No complete real .tc Analyzer → IPC → Broker → Target Service → DBMS → .tc Map acceptance in main |
| C-10 Linux base acceptable | BASELINE | CI and Linux system smoke run on Linux; final Linux setup still needs rehearsal |
| C-11 required tech stack | PARTIAL | Existing C++ VM/Runtime, Kotlin Compiler and Go services match assignment; final application stack compliance needs E2E |
| C-12 both teams on map, confidence growth/decay | NOT VERIFIED | Domain and algorithms specified, Go map fixtures exist; real server-generated/decaying .tc map path not verified |
| C-13 mandatory AI recognition | NOT VERIFIED | OpenRouter PoC client tests pass with mock server, not real vision inference; consent, model selection and integration pending |

## Blocking acceptance dependencies

The **most urgent end-to-end chain** is:

```text
#127 / #128 approved contracts
      ↓
#132 + #133 Compiler/VM data support
      ↓
#134 + #135 Runtime device/network, #136 WM integration
      ↓
#138 / #139 first .tc Analyzer + Map
      ↓
#142 / #143 final .tc apps
      ↓
#149 final system acceptance
```

The server can progress independently: `#130 → #137 → #140 → #149`.
Persistent DBMS `#131` must be verified in the server chain and then
for restart under `#147`. Real vision recognition `#104/#129 → #141/#142`
is a **separate mandatory gate**.

Do not confuse **green CI for one open PR** with a confirmed E2E on
the integration branch or on `main`. PR #154's `server_e2e` tagged
acceptance scenarios are explicitly known to fail on the older baseline
because Target Service does not publish `targets.updated`; ordinary
Go CI success does not run that tagged acceptance set.

## Evidence record to fill during Sprint 5

| Date / commit | Requirement | Command and machine | Observed result | CI / artifact link | Reviewer |
| --- | --- | --- | --- | --- | --- |
| To be filled after real run | FR-2 / C-13 | Permitted model and test image | Not executed yet | #104 / #129 | Analyzer + architecture |
| To be filled after real run | FR-7–FR-13 | Two .tc client processes and WM | Not executed yet | #149 | Integration owner |
| To be filled after real run | FR-15–FR-18 | Real Broker/Target Service/DBMS | Not executed yet | #140 / #149 | Target Service + DBMS |
| To be filled after real run | C-9 / C-12 | Complete Linux scenario | Not executed yet | #149 | Architecture |

Before freezing release, replace placeholders with **actual observation**,
including counterexamples, negative checks and the last failing gate.
Keep all unresolved constraints visible rather than presenting planned
work as completed.
