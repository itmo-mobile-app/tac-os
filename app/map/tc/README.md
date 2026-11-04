# Map coordinate calculation on .tc (#109)

Status: the real Compiler → VM Linux E2E passed on 2026-11-04 with the unmerged Float
implementations from Compiler PR #120 (`0324e967eb6caadba2dce95d4cd643f2eb66ad79`) and VM
PR #116 (`86b1d5102d0ee5ca3e8beeadcd4860ee8f0aa4aa`). Main at
`2bd0c61de2c8f5293ae9407bf15da0d483e733e2` still lacks both implementations. This change must
be integrated after those dependencies; their source code is not part of #109.

`projection.tc` is an actual custom-language source. It contains the two geographic-to-screen
formulas, an inclusive bounds check, Float parameters/locals/returns and self-checking fixtures.
It must be compiled by the real `tacc` and executed by the real `tacvm`. A host-language formula
check or a replacement interpreter is not acceptance evidence for this issue.

## Coordinate rules

The fixture matches `app/map/assets/test-map.json` and `spec/map-rendering.md`:

- WGS84 degrees on the small, unrotated test area: latitude 59.9490..59.9510,
  longitude 30.3080..30.3140; valid, finite bounds and dimensions are preconditions.
- Width 960 and height 640 are pixel counts, represented as `960.0` and `640.0` in the
  calculations. All arithmetic operands are Float; there is no implicit Int conversion.
- North is up: increasing latitude decreases y. East is right: increasing longitude increases x.
- All four geographic edges are inside. Outside points are rejected before projection.
- The formulas are `(longitude-lonMin)/(lonMax-lonMin)*width` and
  `(latMax-latitude)/(latMax-latMin)*height`.
- After those calculations, inside coordinates exceeding the last pixel position are clamped
  to `width-1.0` / `height-1.0`. In particular, the exact east/south edge maps to 959.0/639.0.
  Outside points must never be clamped into the map.
- Interior results remain fractional Float coordinates. Nearest-integer rounding and Float-to-Int
  conversion are deferred; no unapproved cast or Runtime function is introduced. Clamping before
  eventual rounding gives the same final pixel as the Go renderer's rounding-then-clamping for
  these nonnegative inside coordinates. This scenario does not produce drawable Int pixels yet.

Binary64 checks use an absolute tolerance of 0.000001 pixel, rather than comparing computed
fractional values for exact equality. It is a fixed-fixture test tolerance, not a coordinate rule.

| Case | Latitude | Longitude | Expected result |
| --- | --- | --- | --- |
| Center | 59.9500 | 30.3110 | inside, x=480.0, y=320.0 |
| North-west edge | 59.9510 | 30.3080 | inside, x=0.0, y=0.0 |
| South-east edge | 59.9490 | 30.3140 | inside, x=959.0, y=639.0 |
| Fractional interior | 59.950001 | 30.310001 | inside, x≈320.16, y≈319.68 |
| Inside last pixel | 59.949001 | 30.313999 | inside, x=959.0, y=639.0 after clamping |
| North outside | 59.9511 | 30.3110 | outside |
| South outside | 59.9489 | 30.3110 | outside |
| West outside | 59.9500 | 30.3079 | outside |
| East outside | 59.9500 | 30.3141 | outside |

## Linux E2E

Prerequisites: a checkout containing the implementations of Compiler #107 and VM #108, JDK 17,
CMake, a C++17 compiler and the existing project's Go/Python dependencies.
From the repository root:

```bash
(cd compiler && bash ./gradlew --no-daemon installDist)
cmake -S . -B build
cmake --build build --parallel
ctest --test-dir build -R '^map_tc_projection_e2e$' --output-on-failure
```

CTest uses the existing `tests/run_program.cmake`: it compiles the source, runs the bytecode, checks
both exit statuses and compares stdout with `tests/map-tc/expected.txt`. Every failed fixture prints
`FAIL` instead of its expected label and ends with `map-projection-failed`, so incorrect results
cannot pass just because the VM process exits normally. Compile errors and VM errors also fail.
No generated `.bc` is checked in. The test is required, without an automatic skip for absent Float.
Merge #107/#108 before this change. On the old Int-only toolchain this test fails explicitly.

### Verification on 2026-11-04

The real `tacc` launcher and `tacvm` executable ran in Debian 12 Linux/arm64, with OpenJDK
17.0.21, GCC 12.2.0, CMake 3.25.1 and Go 1.22.12. VM/Runtime were built natively in that Linux
container with `TACOS_SANITIZE=ON` (ASan/UBSan). The compiler's portable distribution was built
on macOS using JDK 17 and Gradle `installDist`; it was then executed by Linux Java. No Runtime
compatibility shim or substitute compiler/VM was used. The runs needed no camera, window or
external network.

`map_tc_projection_e2e` passed and direct execution printed exactly:

```text
center-ok
north-west-ok
south-east-ok
fractional-ok
last-pixel-ok
north-outside-ok
south-outside-ok
west-outside-ok
east-outside-ok
map-projection-ok
```

Three temporary source mutations were compiled and executed through the same test runner:
scaling x by `width-1.0`, reversing the y axis, and making the south boundary exclusive. Each
printed `FAIL` / `map-projection-failed` and made the runner exit with status 1. These mutated
fixtures are not application sources.

The broader Linux CTest run passed 26/27 tests, including the existing Map unit/preview/WM
checks, old Compiler → VM programs, IPC, and `float_coordinates_e2e`. The existing
`vm_instructions` suite exceeded its 60-second CTest timeout. Running that exact suite directly
with `python3 tests/vm/test_vm.py build/vm/tacvm -v` passed all 24 tests in 102.497 seconds.
A completely green full CTest run is not claimed; this task does not change the suite or its timeout.

The compiler's `installDist` succeeded, but its own `test` task failed before execution:
`compiler/src/test/kotlin/tacc/ParserTest.kt:18` has a non-exhaustive `when` after the addition
of `FloatLit` in PR #120. Compiler tests and that PR's red CI must be fixed in #107, not hidden
by the Map change. This is separate from the successful execution of the projection source.

A second Linux check overlaid the real Map implementation from PR #114
(`6e1728bc5ce547eb48b25035f03f924fb4af4137`) without changing it. All six selected CTest checks
passed: `map_unit`, `map_preview`, `map_wm_e2e`, `map_broker_wm_e2e`,
`float_coordinates_e2e` and `map_tc_projection_e2e`. The #109 and #99 file sets do not overlap;
Go Map, drawing and Broker code are not modified by this task.

To use real compiler and VM binaries from their owners' unmerged Float branches independently:

```bash
cmake -DTACC=/path/to/float-tacc -DTACVM=/path/to/float-tacvm \
  -DSOURCE="$PWD/app/map/tc/projection.tc" \
  -DBYTECODE=/tmp/map-projection.bc \
  -DEXPECTED="$PWD/tests/map-tc/expected.txt" \
  -P tests/run_program.cmake
```

## Scope

This scenario does not create or draw a WM window, load images, use Broker, receive Analyzer IPC,
or replace the Go Map. The Go Broker integration (#99) stays separate. The only Runtime call is the
existing `runtime.log(String)`. Window APIs, Float conversions, language features and bytecode
implementation are not changed by #109.
