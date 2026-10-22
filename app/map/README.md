# Map prototype for #35

Go 1.22+ prototype outside the VM. Only the standard library is used.
There is no Analyzer IPC, Broker subscription or `.tc` code.

From `app/map`, render the test map locally:

```bash
go run ./cmd/map-demo -preview /tmp/map-preview.png
```

The three visible targets have probability 90%, 55%, and 20%; their marker opacity decreases.
A fourth target is outside the image and is skipped. Edit `DemoTargets()` in `model.go` to change
the hardcoded list. PNG generation interprets the same drawing commands as the WM mode.
It is provided for local inspection. Native drawing integration is verified against WM #80;
see `../../docs/testing/map-wm.md` for the actual Linux SDL2 capture and E2E command.

With the WM drawing commands implemented, open the actual window:

```bash
go run ./cmd/map-demo
```

WM must include the drawing implementation from #80, with the fields in `../../spec/map-rendering.md`.
Use `-socket` and `-events` to change the default Unix socket paths.
Closing the window exits Map; Ctrl+C closes Map's window without stopping the WM service.

The map uses `assets/test-map.png` and its geographic bounds in `assets/test-map.json`.
Coordinates are north-up with east to the right. Inside boundary points use the last valid pixel;
outside points are omitted. The data model matches `spec/data-model.md`, whose approval is unchanged.

Verify coordinates, probability display and frame construction:

```bash
go vet ./...
go test ./...
```

The shared CMake build includes target `tacos_map` and tests `map_unit` and `map_preview`.
The cross-component test `map_wm_e2e` builds the real Map CLI and connects it to the production WM
server/headless renderer. Linux SDL2 operation and native close are recorded in
`../../docs/testing/map-wm.md`. Agreement with the WM owner on the final frame and command fields
was confirmed by the Map owner; the WM dependency still needs to be merged.
