# Analyzer coordinate calculation on .tc (#138)

Status: the calculation, validation and local confidence of `spec/analyzer-observation.md` run on
the real Compiler and VM. Sending the `Observation` over IPC is **not** done; see "Not done yet".

`observation.tc` is an actual custom-language source. It must be compiled by the real `tacc` and
executed by the real `tacvm`; a host-language formula check is not acceptance evidence.

## What it contains

- Bearing, target latitude and longitude, and the local confidence, with the constants and formulas
  of `spec/analyzer-observation.md`. Angles are degrees, distances meters, all arithmetic is `Float`.
- `sinDeg` and `cosDeg`, because the language and the Runtime have no trigonometry. They reduce the
  angle to `[-90, 90]` with degree arithmetic and sum a 12-term Maclaurin series (about `1e-16`
  accuracy). No language or Runtime contract is added; a Runtime function can replace them later.
- The validation rules with their reason codes (`INVALID_POSITION` and the others). The conditions
  are written in the positive form, so a value that is not a number cannot pass them.
- One self-checking function per vector of `spec/analyzer-observation-vectors.json` (16 vectors:
  0, 1 and 2 detections, both hemispheres, the heading wrap, the maximum range, and every rejection
  reason). Values are compared within `1e-9`, as in the specification.

The vector functions were generated from the JSON file. When a vector is added or changed there,
update `observation.tc` and `tests/analyzer-tc/expected.txt` in the same change.

## Not done yet

The acceptance criterion "the `.tc` program sends the `Observation` through IPC" is blocked by
missing language and Runtime features, each of which needs its owner's decision:

| Missing | Needed for | Where it is tracked |
| --- | --- | --- |
| Owner approval of the String/JSON helpers (`text.concat`, `text.fromFloat`, `text.fromInt`, `json.quote`, `json.*` readers) | Building the one-line `OBSERVATION` JSON for `runtime.ipc_send` and reading `camera.detectMarkers()` fields | Proposal #127; implemented in Compiler and Runtime by #132 and covered by `data_tc_e2e`, using them here is #138 |
| Runtime location, orientation and time | Device position, heading and `timestamp` instead of fixtures | #134 |

Until then the inputs are literals in the vector functions, and no message is built or sent. Nothing
is sent as a prebuilt string, because that would not be a calculated `Observation`.

## Linux E2E

Prerequisites: JDK 17, CMake, a C++17 compiler. From the repository root:

```bash
(cd compiler && bash ./gradlew --no-daemon installDist)
cmake -S . -B build
cmake --build build --parallel
ctest --test-dir build -R '^analyzer_tc_observation_e2e$' --output-on-failure
```

CTest uses `tests/run_program.cmake`: it compiles the source, runs the bytecode, checks both exit
statuses and compares stdout with `tests/analyzer-tc/expected.txt`. A failed vector prints `FAIL`
and its label, and the program ends with `analyzer-observation-failed`.
