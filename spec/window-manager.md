# Window Manager

**Status:** Prototype

**Scope:** Linux, one Tactical Map window

## 1. Purpose

The Window Manager (WM) is a standalone process responsible for creating, displaying, and closing application windows. Runtime communicates with WM through a local IPC protocol.

The initial use case is a single Tactical Map window. The prototype does not implement a general application lifecycle or multi-window management.

## 2. Process model and transport

WM runs as a separate Linux process. Runtime uses Unix-domain sockets to communicate with it.

The prototype uses two socket connections:

- **Command socket** — receives commands from Runtime or a test client.

- **Event socket** — delivers window events to subscribed clients.

The default socket paths are:

- `/tmp/tacos-wm.sock` — commands.

- `/tmp/tacos-wm-events.sock` — events.

Socket paths are configurable through the server configuration. The server restricts socket permissions to the owning user (`0600`).

Command messages and event messages use newline-delimited JSON (one JSON object per line). Each command connection handles one request and returns one response. Event clients subscribe through the event socket and receive an initial subscription response before events are delivered.

## 3. Command protocol

A command request contains the following fields:

| Field | Type | Description |
|---|---|---|
| `command` | string | Command to execute. |
| `window_id` | string | Window identifier. Defaults to `map` when omitted. |
| `title` | string | Window title. Defaults to `Tactical Map` when omitted. |
| `width` | integer | Window width for `CREATE` (default `960`); image width for `DRAW_IMAGE`. |
| `height` | integer | Window height for `CREATE` (default `640`); image height for `DRAW_IMAGE`. |
| `image_path` | string | Path to the image file used by `DRAW_IMAGE`. |
| `x` | integer | Horizontal coordinate for drawing operations. |
| `y` | integer | Vertical coordinate for drawing operations. |
| `radius` | integer | Circle radius in pixels for `DRAW_CIRCLE`. |
| `color` | object | RGBA color with integer `r`, `g`, `b`, and `a` components in the range 0–255. Used by `BEGIN_FRAME` and `DRAW_CIRCLE`. |

`title` is used by `CREATE`; `width` and `height` are used by both `CREATE` and `DRAW_IMAGE`.

### Commands

- **`CREATE`** — creates a window and associates it with a window ID. The initial Map window uses a drawing area of `960 × 640` pixels and the title `Tactical Map`. The window uses native window decorations and a close control. WM chooses its position.

- **`SHOW`** — displays an existing window.
- **`BEGIN_FRAME`** — starts a new drawing frame and fills the drawing buffer with the specified `color`.
- **`DRAW_IMAGE`** — draws the image at `image_path` at position (`x`, `y`), scaled to `width × height` pixels. Drawing is clipped to the window bounds.
- **`DRAW_CIRCLE`** — draws a filled circle centered at (`x`, `y`) with the specified `radius` and RGBA `color`. Drawing is clipped to the window bounds.
- **`PRESENT`** — presents the completed drawing frame in the window.

- **`CLOSE`** — closes the specified window and releases its resources.

- **`QUIT`** — closes the current window, if any, and requests WM shutdown.
Drawing commands require an existing window. A frame is prepared with
`BEGIN_FRAME`, followed by zero or more `DRAW_IMAGE` and `DRAW_CIRCLE`
commands, and completed with `PRESENT`. RGBA colors use 8-bit components
in the range 0–255. Drawing operations are clipped to the window bounds.

The prototype supports one window at a time. It does not define multi-window behavior.

### Command responses

A response contains:

| Field | Type | Description |
|---|---|---|
| `ok` | boolean | Indicates whether the operation succeeded. |
| `message` | string | Result information or an error description. |
| `event` | string | Event name when the response carries an event; otherwise omitted or empty. |
| `window_id` | string | Associated window identifier when applicable. |

Clients must check `ok` and handle unsuccessful responses.

## 4. Window events

Clients subscribe through the event socket to receive notifications independently of command responses.

The initial prototype supports the following event:

- **`window_close`** — emitted when the user requests that the window be closed using its native close control.

The event identifies the associated window through `window_id`. The Map process can use this notification to perform its own shutdown or terminate.

Mouse and keyboard events are not required for the initial Map integration. The initial window has a fixed size, so resize events are not part of this protocol.

## 5. Graphics backend

The graphics backend is an implementation detail of WM. It is responsible for native window creation, display, closure, and polling native window events.

WM separates the backend interface from the command protocol so that command handling does not depend directly on a specific graphics implementation.

The prototype includes a headless backend for non-graphical protocol development and testing, as well as an SDL backend for graphical operation where SDL support is enabled.

## 6. Limitations and out-of-scope functionality

The prototype intentionally excludes:

- multiple simultaneous windows;

- Activity lifecycle management;

- WebView integration;

- mouse and keyboard event forwarding;

- window resizing and resize events;

- remote or network-based IPC.

The protocol is an initial integration contract and may require alignment with the project's established Runtime IPC conventions before it is treated as a stable public interface.

## 7. Shutdown and resource management

`CLOSE` releases the resources associated with the requested window. `QUIT` requests process shutdown and closes any current window.

WM should clean up its socket resources when it exits. Clients should not assume that a command succeeded until they receive a successful response.

## 8. Map integration evidence

`docs/testing/map-wm.md` records the real Map CLI to WM headless E2E command and an actual Linux
SDL2/X11 run against #80. The Map sends a complete hidden frame before SHOW. Closing through the
native control destroys the window before `window_close` is delivered; Map must not send a duplicate
CLOSE for that event. The Map owner confirmed that the final frame and command fields had been
discussed with and approved by the WM owner; the source of that confirmation is recorded in the
integration report.
