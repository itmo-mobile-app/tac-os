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
It is provided to inspect this prototype while native drawing integration is pending.

With the WM drawing commands implemented, open the actual window:

```bash
go run ./cmd/map-demo
```

WM must implement the fields in `../../spec/map-rendering.md`. The supplied original WM does not
yet support these commands. Use `-socket` and `-events` to change the default Unix socket paths.
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
Native Linux/SDL operation must still be checked with the WM owner's implementation.
