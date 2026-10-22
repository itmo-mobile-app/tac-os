# Map rendering for #35

Status: wire compatibility and native Linux SDL2 integration verified against WM #80
(`425fd8f857af13b95b62a4a7cd006d9e9c3e5a16`). The Map owner reported approval of the final frame and
command fields by the WM owner following their discussion on 2026-10-21.
WM #80 has been merged.

## Scope

The Go prototype runs outside the VM. Map converts hardcoded `Target` records to screen coordinates
and sends drawing commands to WM. WM handles the native surface and knows nothing about targets.
The final `.tc` application will use Runtime; no language or VM changes are included here.

## Coordinates and assets

A 960x640 PNG is paired with a UTF-8 JSON file:

```json
{
  "version": 1,
  "image": "test-map.png",
  "width": 960,
  "height": 640,
  "bounds": {
    "latMin": 59.9490,
    "latMax": 59.9510,
    "lonMin": 30.3080,
    "lonMax": 30.3140
  }
}
```

The image path is relative to metadata, or absolute. Dimensions must match the PNG.
North is up, east right; latitude/longitude are assumed WGS84 degrees for this small test rectangle.

```text
x = (longitude - lonMin) / (lonMax - lonMin) * width
y = (latMax - latitude) / (latMax - latMin) * height
```

Outside targets are omitted. Inside coordinates are rounded to integer pixels; exact east/south
edges use width-1/height-1. Marker circles are clipped. Invalid bounds, coordinates or probability
are errors. Rotated imagery, large regions and antimeridian crossing are outside the prototype.

## WM drawing contract

Transport and response envelope follow `spec/window-manager.md`: one newline-terminated JSON
request and response per command connection. Every request specifies `window_id: "map"`;
clients check the boolean `ok` response. The commands below are the expected Map-side interface.

| Command | Additional fields | Effect |
| --- | --- | --- |
| `BEGIN_FRAME` | `color: {r,g,b,a}` | Clear a hidden back buffer |
| `DRAW_IMAGE` | `image_path`, `x`, `y`, `width`, `height` | Load local PNG and draw the full-size background at 0,0 |
| `DRAW_CIRCLE` | `x`, `y`, `radius`, `color: {r,g,b,a}` | Draw a filled circle, clipped to the surface |
| `PRESENT` | none | Display and retain the completed frame |

For example:

```json
{"command":"DRAW_CIRCLE","window_id":"map","x":240,"y":160,"radius":10,"color":{"r":196,"g":72,"b":48,"a":236}}
```

Colors are straight RGBA bytes, composited source-over. Drawing is supported before SHOW.
Map draws a white radius-13 circle behind a red radius-10 circle. The red alpha is
`round(255 * (0.25 + 0.75 * probability))`. Map displays the supplied probability, without calculating it.

Map subscribes to the event socket and reads `{"ok":true,"message":"subscribed"}`, then sends
CREATE, BEGIN_FRAME, DRAW_IMAGE, circles, PRESENT, SHOW. The default sockets are
`/tmp/tacos-wm.sock` and `/tmp/tacos-wm-events.sock`.
The existing WM destroys its window before sending `window_close`; Map exits on that event without
a second CLOSE. On cancellation or a drawing failure Map attempts CLOSE, never QUIT.

## Verification

The Map unit tests verify coordinate orientation, boundaries, invalid targets, frame construction
and opacity. `TestMapClientWindowManagerE2E` additionally runs the real Map executable against the
production WM server and headless renderer over Unix sockets, including successful CLOSE.
Native SDL2/X11 display, opacity and native window-close events were verified on Linux.
The reproducible commands, exact environment, screenshots and measured results are recorded in
`docs/testing/map-wm.md`. PNG-preview alone is not the native integration evidence.
