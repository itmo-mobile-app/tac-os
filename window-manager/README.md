# tac-os Window Manager prototype (Go)

This is a self-contained Go prototype for the Window Manager task (#33). It lives in `window-manager/` and builds as a standalone Go module alongside the C++ Runtime.

## Scope

- Standalone WM process.
- Unix-domain socket protocol.
- Commands: `CREATE`, `SHOW`, `CLOSE`, `QUIT`.
- A small test client.
- SDL2 graphical window backend (optional build, requires SDL2 development libraries).
- A headless protocol server mode for repeatable tests.

The Runtime command client is implemented in `runtime/src/window_manager_client.cpp`. Window-event subscription from Map remains a separate integration step.

## Requirements

- Go 1.22+
- Linux
- For graphical mode: SDL2 development package and `github.com/veandco/go-sdl2` binding.

## Build and test headless mode

```bash
go test ./...
go build -o wm-server ./cmd/wm-server
go build -o wm-client ./cmd/wm-client
```

Run server:

```bash
./wm-server -socket /tmp/tacos-wm.sock -backend headless
```

In another terminal:

```bash
./wm-client -socket /tmp/tacos-wm.sock CREATE
./wm-client -socket /tmp/tacos-wm.sock SHOW
./wm-client -socket /tmp/tacos-wm.sock CLOSE
./wm-client -socket /tmp/tacos-wm.sock QUIT
```

The headless backend validates the protocol and lifecycle but does not open a real GUI window.

## Build graphical backend

```bash
sudo apt-get install libsdl2-dev
go mod download
go build -tags sdl -o wm-server-sdl ./cmd/wm-server
```

Run with:

```bash
./wm-server-sdl -socket /tmp/tacos-wm.sock -backend sdl
```

The SDL implementation is behind a build tag so headless tests do not require system SDL packages.

The shared [`Go` CI workflow](../.github/workflows/go.yml) runs
`go vet` and race-enabled headless Go tests for this module on every PR.
Existing WM files are not yet subject to a required `gofmt` gate; the
CI consolidation does not introduce a new formatting requirement for WM.
The same Window Manager job also builds both command-line programs and
installs `libsdl2-dev` to verify the graphical backend with `-tags sdl`.
A successful build does not replace manual graphical testing on Linux.

## Wire protocol (newline-delimited JSON)

Request:

```json
{"command":"CREATE","window_id":"map","title":"Tactical Map","width":960,"height":640}
```

Response:

```json
{"ok":true,"message":"created"}
```

Commands:

- `CREATE`: create one window; defaults to `map`, 960×640.
- `SHOW`: show the created window.
- `CLOSE`: close the created window.
- `QUIT`: stop the WM process.

The socket path defaults to `/tmp/tacos-wm.sock`. The socket is local and restricted to its owning user (0600). The protocol is defined in `spec/window-manager.md`.
