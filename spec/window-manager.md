# Window Manager specification

Status: prototype contract for issue #33

## Scope

The first implementation supports one Tactical Map window. It does not implement Activity lifecycle, WebView rendering, multiple windows, persistence, or keyboard/mouse APIs for the Map application beyond the close-button interaction needed to produce `window_close`.

## Graphics backend

The Window Manager is a custom Wayland compositor process written in C++17. It uses wlroots 0.17.1 in the current development environment as a low-level compositor-building library for Linux backends, output rendering, Wayland globals, the scene graph, and pointer plumbing. It does not launch or delegate window management to Weston, Sway, KWin, or another ready-made compositor.

The initial development target is a nested compositor inside an existing Linux graphical session. wlroots chooses the nested Wayland or X11 backend based on the session. This permits testing without taking over the developer's physical display. Direct DRM/KMS session management is deferred until deployment requirements are defined.

The project owner has explicitly permitted wlroots as a low-level library for this prototype. The window lifecycle, IPC protocol, and Runtime API remain project-owned.

## Window contract

- The first window is `Tactical Map`, initially 960×640 logical pixels.
- Window position is selected by the WM; the prototype uses a fixed initial position.
- `create(width, height, title)` allocates a positive `window_id`.
- `show(window_id)` starts the Wayland client for that window and makes its scene node visible.
- `close(window_id)` closes the window programmatically and does not emit `window_close`.
- Clicking the close control in the prototype window emits `window_close` and closes the window.
- Unknown IDs and malformed commands return an `ERROR` response without crashing the WM.

## Runtime ↔ WM transport

- Transport: Unix-domain stream socket.
- Default socket: `/tmp/tac-os-wm.sock`; Runtime and clients may override it with `TACOS_WM_SOCKET` (the WM path is selected by `--socket`).
- One UTF-8 command per connection, terminated by `\n`; one response line is returned before the connection closes.
- The first version intentionally uses a small line protocol rather than JSON, so the component does not need an additional JSON library. `CREATE` title is the remainder of the line and may contain spaces, but not CR/LF.

Commands:

```text
CREATE <width> <height> <title>  -> OK <window_id>
SHOW <window_id>                 -> OK
CLOSE <window_id>                -> OK
POLL_EVENT                       -> EVENT window_close <window_id> | NONE
PING                             -> PONG
```

Errors are returned as `ERROR <message>`. `CLOSE` is programmatic; the close button uses an internal `USER_CLOSE <window_id>` command, which enqueues the public `window_close` event. `USER_CLOSE` is a prototype/testing command and is not exposed as a Runtime function.

Runtime functions:

```text
wm.create(width: Int, height: Int, title: String) -> Int
wm.show(window_id: Int) -> Int
wm.close(window_id: Int) -> Int
wm.poll_event() -> String
```

`wm.poll_event()` is non-blocking. It returns an empty string when no event is queued, or `window_close <window_id>` when the user closes the window. A successful function with no meaningful result returns `Int 0`, following `spec/runtime-api.md`.

## Input

The demo client draws its own close control in the top-right corner. Clicking that control triggers the prototype's `USER_CLOSE` command, queues a `window_close` event for Runtime, and closes the window. General pointer/keyboard forwarding to Map is out of scope.

## Build and verification

Dependencies: CMake 3.16+, C/C++ toolchain, `pkg-config`, a compatible wlroots development package (the current prototype is built against wlroots 0.17.1), Wayland client/server development packages, `wayland-protocols`, and `wayland-scanner`. Build the existing project with `cmake -S . -B build -DTACOS_BUILD_WINDOW_MANAGER=ON` to enable this optional target.

The nested compositor must be started from an existing graphical session. Run `./build/window-manager/tac_wm --window-client ./build/window-manager/tac_wm_window_client`, then in another terminal run `./build/window-manager/tac_wm_test_client demo`; a 960×640 window titled `Tactical Map` should appear. Click the red close control; the test client should print `EVENT window_close 1`. If the window is still open, press Enter in the test-client terminal to verify programmatic close.
