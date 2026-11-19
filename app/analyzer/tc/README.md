# Analyzer on the custom language: coordinates, confidence and IPC (#138)

Status: the real `observation.tc` is compiled by `tacc`, executed by `tacvm`
and sends **calculated** seven-field IPC Observations to an independent VM
process. It uses the existing `runtime.ipc_send` and the approved #127
String/JSON helpers, not a prebuilt Observation JSON fixture.

## What the source implements

- The 16 reference vectors of `spec/analyzer-observation-vectors.json` still
  exercise coordinate geometry, rejection rules and local confidence. The
  `sinDeg`/`cosDeg` functions remain implemented in `.tc` through the
  12-term Maclaurin series; no trigonometry Runtime API was introduced.
- `transmitDetections` decodes a JSON detection array, validates each
  candidate using the same `.tc` rules, calculates latitude/longitude and
  confidence, assembles exactly six Observation fields plus the IPC `type`,
  and sends the resulting String via `runtime.ipc_send`.
- The fixture delivery exercises 0, 1 and 2 valid detections and a mixed
  invalid+valid batch. An invalid detection does not suppress a valid one.
  `timestamp` stays an exact Int, even above 2^53; floats use
  `text.fromFloat` without fixed-precision truncation.
- If Map is unavailable, each send returns `-1` without hanging or crashing
  the Analyzer. Successful local sends do not imply Map acknowledgement;
  delivery remains at-most-once.

## Verified boundary

`tests/analyzer-tc/receiver.tc` is a minimal **test Map IPC listener**, not
the final graphical Map. `analyzer_tc_ipc_e2e` starts two separate Linux
`tacvm` processes, receives and checks the four generated Observations, tests
the empty and invalid cases, and verifies socket cleanup. It separately runs
the same Analyzer without Map and checks bounded failure. Both it and
`analyzer_tc_observation_e2e` share the IPC socket resource lock.

## Linux acceptance commands

From the repository root (JDK 17, CMake, C++17, Python 3 required):

```bash
(cd compiler && bash ./gradlew --no-daemon installDist)
cmake -S . -B build
cmake --build build --parallel
ctest --test-dir build -R '^analyzer_tc_(observation|ipc)_e2e$' --output-on-failure
```

The original vector test compares stdout to
`tests/analyzer-tc/expected.txt`, which now includes the expected
`analyzer-ipc-unavailable` when Map is absent. The IPC test checks actual
messages received over the Unix socket and `analyzer-ipc-sent` when Map exists.

## Sprint 5 work outside #138

The position, heading, time and detection arrays in this executable are
**test fixtures**. Real `camera.detectMarkers()` calls, device sensor
sampling and the AI vision integration belong to #142/#141. This increment
does not implement `runtime.ipc_poll`, a long-running Map event loop,
Broker forwarding or a native WM window (#143/#145). It does not change
the ArUco detector, Camera API, `DETECT` protocol or Target aggregation.
