# First Map screen on .tc (#139)

`map.tc` draws the existing map image and one fixed Target using the real custom language,
VM, C++ Runtime and Go Window Manager. The application does not delegate projection or marker
selection to the Go Map prototype.

## Dependency and review status

Prepared against main `3877600e4e4a6fa06bc8081d849aa5f4913fcf96` and Window Runtime PR
[#156](https://github.com/itmo-mobile-app/tac-os/pull/156), revision
`9c57c95d75b105c88fee165a4c054c8358bc5194` (issue #136). That implementation registers the
existing proposed `window.*` signatures in Compiler and Runtime. It is not yet in main and
its contract explicitly awaits the Runtime owner's review and alignment with #128.
Successful local integration does not constitute owner approval. Merge the reviewed Window
Runtime dependency before merging #139; the required test fails configuration if it is absent.

No Compiler, VM, Runtime, WM production code or public protocol is changed by this task.
The fixture uses five scalar constants with the names/types from `spec/data-model.md`:
`targetId`, `latitude`, `longitude`, `probability`, `lastSeenAt`. It is one predefined valid Target,
not a JSON parser or a new record representation. Identity/time metadata do not affect pixels.
The new data APIs proposed in #127/#132/#133 are not simulated or reimplemented.

## Frame and coordinate rules

- The background is the existing 960x640 `test-map.png`, with WGS84 bounds
  latitude 59.9490..59.9510 and longitude 30.3080..30.3140.
- The default Target is `17` at `(59.9500, 30.3110)`, probability 0.9,
  lastSeenAt 1800000000100. Its marker center is `(480,320)`.
- Both Float formulas from `spec/map-rendering.md` execute inside `.tc`. North is up and east
  is right. All geographic edges are inside; outside points are omitted before rounding.
- `roundedPixel` counts Float half-pixel thresholds while maintaining an Int index. For these
  finite, nonnegative screen coordinates it implements nearest rounding with ties upward,
  then clamps to `size-1`, matching the Go reference. It uses ordinary language operations,
  adds no cast/Runtime API, and does at most 960 iterations for the fixed map size.
- The same helper rounds `255.0 * (0.25 + 0.75 * probability)` into `0..255`. Default alpha is
  236. The frame uses the Go reference's white radius-13 border and red radius-10 circle
  `(196,72,48,alpha)`. Probability is displayed, not aggregated by Map.
- Frame order is CREATE, BEGIN_FRAME, DRAW_IMAGE, two DRAW_CIRCLE calls for an inside Target,
  PRESENT, SHOW. Even when the Target is outside, the map window stays visible.
- The program polls `window.pollEvent(100)` until native close, then calls `window.close()`.
  Runtime knows that native close already destroyed the window; it avoids duplicate CLOSE.
  The private fixture constant `closeAfterPolls=0` keeps the normal application open.
  A positive value is used only by the explicit-close test.

The fixture assumes finite, valid coordinates/probability and the fixed valid map metadata.
There is no general file/JSON loader, multiple-target state or camera input in this increment.

## Linux build and headless E2E

Use a checkout with the reviewed Window Runtime dependency and these Map files. Requirements:
JDK 17, CMake, a C++17 compiler, Python 3 and Go 1.22+. From the repository root:

```bash
(cd compiler && bash ./gradlew --no-daemon test installDist)
cmake -S . -B build -DTACOS_SANITIZE=ON
cmake --build build --parallel
ctest --test-dir build -R '^map_tc_window_e2e$' --output-on-failure
ctest --test-dir build -R '^(map_tc_projection_e2e|map_unit|map_preview|map_wm_e2e|map_broker_wm_e2e|map_runtime_wm_e2e)$' --output-on-failure
```

`tacos_map_tc` compiles the actual `map.tc` to generated `build/tests/map-tc/map.bc`.
The E2E runs the unchanged `window_runtime_runner` from #136: a small host for the real
`tacos_vm::run_file` implementation that supplies private command/event socket paths.
The default case executes the exact application bytecode. Additional cases change only
the fixed input constants and recompile with the real `tacc`.

The production WM server and headless renderer process real JSONL commands. A test decorator
records operations and injects a native-close event; it does not implement the protocol or
draw instead of WM. The entire front buffer is compared with expected backgrounds/markers
at independently specified integer positions and alpha values.

The twelve cases cover center, probabilities 0.2/0.0, fractional projection to `(321,320)`,
north-west/south-east edges, an inside point at the last pixel, all four outside directions,
and explicit application closure. They reuse one WM across launches, require exactly one
window closure per launch, check Map's output, and require cleanup of WM sockets on WM shutdown.
The direct `go test` suite skips this integration only when none of its CTest-provided binaries
are configured; the required CTest always supplies them and cannot silently skip missing support.

## Native Linux SDL2 window

Install SDL2 development files (for example `libsdl2-dev` on Debian/Ubuntu). Build the production
WM server from the repository root:

```bash
(cd window-manager && go build -tags sdl -o ../build/wm-server ./cmd/wm-server)
```

In terminal 1, start WM from `app/map/tc` so the fixture's relative image path resolves correctly:

```bash
cd app/map/tc
../../../build/wm-server -backend sdl
```

In terminal 2, from the repository root, run the ordinary VM CLI:

```bash
./build/vm/tacvm ./build/tests/map-tc/map.bc
```

Close the window normally. Map exits with code 0 and WM stays running. A second VM launch creates
another map window without restarting WM. Stop WM with Ctrl+C when finished; WM owns and removes
`/tmp/tacos-wm.sock` and `/tmp/tacos-wm-events.sock`. Do not run another WM on those same paths.
The relative image path is resolved by WM, not by the VM or the directory containing bytecode.

## Actual verification on 2026-11-18

The dependency revision above and the Map files were tested in Debian 12 Linux/arm64:
OpenJDK 17.0.21, GCC 12.2.0, CMake 3.25.1, Go 1.22.12 and SDL2 2.26.5. VM/Runtime were
built natively with ASan/UBSan, without a macOS compatibility shim. The portable Kotlin compiler
distribution was built on macOS/JDK 17 and executed by Linux Java. Compiler `test installDist`
passed all 67 compiler tests. No external network or camera was used during the E2E runs.

- Required `map_tc_window_e2e`: passed, including all twelve fixtures and Go race detection.
- Existing Go Map and WM: `go vet ./...` and `go test -race -count=1 ./...` passed.
- Remaining CTest suite: 33/34 passed, including the original projection, Go Map/WM,
  Broker Map/WM, Runtime WM, IPC, Camera, Float and VM-context scenarios. `vm_instructions`
  exceeded its existing 60-second timeout. No fully green overall CTest run is claimed,
  and no acceptance test or timeout was disabled or relaxed in this change.
- The actual production `tacvm` CLI, not just the isolated test host, showed the Map in
  SDL2/X11 under Xvfb/Openbox. Two successive VM launches exited with status 0 after native
  Alt+F4. WM stayed alive after each; stopping WM removed both default sockets.

The [native screenshot](../../../docs/testing/map-tc-window/native-window.png) was captured from
that SDL window, not generated by Go Map's PNG-preview. The [result record](../../../docs/testing/map-tc-window/result.json)
contains revisions, native launch results and socket cleanup. Headless tests independently check
every pixel of the presented frame, including the default marker center `(480,320)` and alpha 236.

## Scope

This is the first fixed-data `.tc` screen, not the complete Tactical Map application.
Receiving Analyzer IPC and Broker Target updates on `.tc` is outside #139 (the next sprint).
The Go Broker integration was delivered separately in PR #114 / #99 and remains unchanged.
No WebView, new data syntax, Float conversion instruction, network API or server logic is added.
