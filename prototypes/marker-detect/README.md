# marker-detect (Go)

Prototype of marker recognition for the Analyzer (issue #34). It is a separate program outside the VM,
written in Go. It reads frames from the laptop camera (or one image file), recognizes game markers and
prints, for each of them, the id, the team, the player, the angle relative to the center of the frame and
the distance. With `-socket` it is the private helper of the Runtime (issue #69): it answers `DETECT`
requests on a Unix-domain socket instead of printing, see "Runtime helper mode".

```text
marker/        marker format (ids, teams, players) and the Detection type; plain Go
pose/          position of a square marker from the pixels of its four corners; plain Go
detector/      recognition: the ArUco detector of OpenCV (through gocv) + pose/; renders markers
ipc/           private Unix-socket protocol of the Runtime helper (spec/camera-detector-protocol.md); plain Go
cmd/marker-detect/  command-line tool
```

`marker`, `pose` and `ipc` do not depend on OpenCV: they build and are tested anywhere. Only `detector` and
`cmd` need OpenCV.

## Requirements

- Go 1.21 or newer, a C++ compiler (gocv uses cgo) and `pkg-config`.
- OpenCV and [gocv](https://gocv.io). **The versions must match**: gocv v0.43.0 officially supports
  OpenCV 4.12.0 (see the gocv README). If you use another OpenCV, see "Versions" below.

macOS:

```bash
brew install opencv pkg-config
cd prototypes/marker-detect
go mod tidy
```

`go mod tidy` is needed once: it downloads gocv and creates `go.sum`.

If the build cannot find OpenCV (`Package opencv4 was not found`), point `pkg-config` at it:

```bash
export PKG_CONFIG_PATH="$(brew --prefix opencv)/lib/pkgconfig"
````

On older macOS versions Homebrew builds OpenCV from source, which can take hours. conda is faster. Note
that conda-forge now ships OpenCV 5 by default, which gocv does not support, so ask for version 4:

```bash
conda create -n tac-opencv -c conda-forge pkg-config "opencv>=4.12,<4.15"
conda activate tac-opencv
export PKG_CONFIG_PATH="$CONDA_PREFIX/lib/pkgconfig"
export CGO_LDFLAGS="-Wl,-rpath,$CONDA_PREFIX/lib"
```

The linker may print `duplicate -rpath ... ignored`; it is harmless.

## Versions

gocv v0.43.0 (the latest release) officially supports OpenCV 4.12.0. Homebrew currently installs
OpenCV 4.14.0. On Linux the project was built and tested with gocv v0.43.0 against both OpenCV 4.12.0 and
OpenCV 4.14.0: everything compiles and all tests pass. On macOS (Apple Silicon) it was built and all tests pass with
gocv v0.43.0 and OpenCV 4.13.0 from conda-forge (see above); the camera was tried on a MacBook.

If the build fails with compile errors inside gocv's C++ code, the OpenCV on your machine is too different
from the one gocv expects. Install the version named in the gocv README (the gocv repository has a
`make install` that builds it) or pick another gocv tag. After changing the gocv version run `go mod tidy`.

## Build and test

### Required CI checks (no native OpenCV)

The shared [`Go`](../../.github/workflows/go.yml) workflow runs on
**every pull request and push to `main`**. Its independent Marker Detector job
checks formatting and runs `go vet ./pose ./marker ./ipc` and
`go test -race -count=1 ./pose ./marker ./ipc`. These three packages use only Go's
standard library and can be tested **without** OpenCV, gocv or a camera.
The separate Window Manager job installs SDL2 only to build its graphical backend.

The `detector` and `cmd/marker-detect` packages **are not covered** by
this lightweight required check. Full OpenCV/gocv builds, detection tests and
physical-camera validation require native dependencies and should be run
separately on a configured machine. Do not interpret a green lightweight CI
run as proof that camera recognition works.

### Run locally

```bash
go vet ./pose ./marker ./ipc && go test ./pose ./marker ./ipc   # no OpenCV needed
go test ./...                                       # everything; needs OpenCV
go test -race ./detector                            # the detector can be shared by goroutines
go build -o marker-detect ./cmd/marker-detect
```

The tests of `detector` draw frames with markers of a known size at known angles and distances, by tracing
a ray through every pixel; the drawing does not use OpenCV or the code under test. They check the id, the
angle (within 1°) and the distance (within 5%), several markers in one frame, ignoring of ids outside
0..71, empty and invalid input, closing and concurrent use.

The tests of `ipc` run a real server on a Unix-domain socket with synthetic detections and need no camera:
two requests over one connection, an empty list, `CAMERA_UNAVAILABLE`, `DETECTION_FAILED`, malformed and
oversized requests, closing the socket, a stale or busy socket file. The tests of `cmd/marker-detect` check
the camera source with a fake camera.

## Run

```bash
./marker-detect -show
```

One line per detected marker per frame:

```text
id=17 team=1 player=5 angle=-12.3 deg distance=1.42 m
```

- `angle` is in degrees relative to the center of the frame; positive means to the right.
- `distance` is in meters from the camera to the marker center.

On the first run macOS asks for camera access for the terminal application. If it was denied, allow it in
System Settings → Privacy & Security → Camera.

| Option | Meaning |
| --- | --- |
| `-camera N` | camera index (default 0) |
| `-hfov DEG` | horizontal field of view of the camera in degrees (default 60) |
| `-marker-size M` | side of the black square of the printed marker in meters (default 0.10) |
| `-show` | preview window with the detected markers (`q` or Esc to quit) |
| `-image FILE` | recognize one image file and exit |
| `-gen-marker ID -out FILE` | write a printable marker (with a white margin) and exit |

`-image` is handy when the camera is not available:

```bash
./marker-detect -gen-marker 17 -out marker-17.png
./marker-detect -image marker-17.png -marker-size 0.10
```

## Runtime helper mode

The C++ Runtime starts the detector with `-socket`; the wire format, the timeouts and the lifecycle are
fixed in [`spec/camera-detector-protocol.md`](../../spec/camera-detector-protocol.md).

```bash
./marker-detect -socket /tmp/tac-os-detector-test.sock -marker-size 0.10
```

Each line `{"command":"DETECT"}` captures one camera frame and is answered by one line with the detections
(`id`, `relativeAngle`, `distance`; no frames) or with an error code. The camera is opened by the first
request. The program stops on `SIGTERM` or Ctrl+C and removes the socket file.

To try it by hand, in a second terminal (on macOS `nc` supports Unix sockets as well):

```bash
printf '{"command":"DETECT"}\n' | nc -U /tmp/tac-os-detector-test.sock
```

`-fake MODE` replaces the camera with fixed answers (`two`, `empty`, `camera-unavailable`,
`detection-failed`), which lets the Runtime tests run without a camera:

```bash
./marker-detect -socket /tmp/tac-os-detector-test.sock -fake two
```

## Markers

ArUco dictionary `DICT_4X4_100`. `id = team * 12 + player`, team 0..5, player 0..11, so valid ids are
0..71. Other ids are ignored.

Print the marker so that the black square is exactly 10 cm wide, or pass the real size as `-marker-size`.
Keep the white margin. If there is no printer, open the image on a phone or a second screen, measure the
black square with a ruler and pass that size (for example `-marker-size 0.06`).

## Calibration

The camera parameters are approximate: a pinhole camera without distortion and with a 60° horizontal field
of view. To tune the distance:

1. Put a marker exactly 1 m from the camera and run the prototype.
2. If the printed distance is too large, increase `-hfov`; if too small, decrease `-hfov`.
3. Use the value that gives the correct distance as the default.

A wider field of view means a smaller focal length in pixels, so the same marker size in the image
corresponds to a smaller distance. Angles are less sensitive to this and are usually fine without
calibration.

## Known limits

- Only the horizontal angle is reported.
- Lens distortion is not corrected.
- With a corner error of 0.3 px the distance is on average 1% off and at worst about 5% off (see
  `pose/pose_test.go`); real cameras may be worse.
- `confidence` is not produced: the ArUco detector does not give a score (open question).
- A single frame can contain a false detection of a marker that is not there (seen once in a camera run).
  The Analyzer should accept a marker only if it is seen in several frames in a row.
- The distance was not checked against a measured distance on a real camera (the camera is not
  calibrated); the angle was checked only by moving the marker to the left and to the right.
