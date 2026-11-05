# Linux system smoke-test

Black-box verification for issue #100. The runner builds and starts the existing
Broker, Target Service, Go Map demo, and production Window Manager with its
headless backend. It does not change their implementation or public contracts.

## Run

From the repository root, with Linux and Go 1.22+ already installed and `go` on
`PATH`:

```sh
(cd tests/system-smoke && GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go run -mod=readonly .)
```

The checkout includes the map PNG and metadata. No camera, OpenCV, SDL development
libraries, display server, Docker server, or internet connection is needed.
macOS is supported for development; release verification targets Linux.

The runner builds native executables in its own temporary directory. Child builds
disable module downloads, checksum-server access, automatic toolchain downloads,
cgo, workspace overrides, and inherited cross-compilation settings. The Go
toolchain must be installed before running the command. The launcher uses the
local Window Manager module through `go.mod`'s `replace` directive; SDL packages
are not imported into the headless executable.

An expected successful run prints:

```text
Resources: /tmp/tac-smoke-<unique suffix>
PASS build components (offline, headless)
PASS Broker -> Target Service (matching Observation receipt)
PASS Map -> headless Window Manager (frame, markers, CLOSE, reuse)
PASS unavailable Broker
PASS cleanup
```

Exit code `0` means all scenarios and cleanup passed. A failed scenario, deadline,
or cleanup failure produces `FAIL:` and a nonzero exit code. Invalid runner
arguments produce exit code `2`; `go run` itself may report that child exit status
through its own exit code.

## What is checked

Two independent chains exercise the functionality currently available:

```text
runner -> TCP Broker -> Target Service
Go Map demo -> Unix sockets -> production Window Manager/headless
```

### Observation delivery

The runner waits for its Broker's listening message and for Target Service's
subscription message, then publishes a six-field `Observation` from
`spec/data-model.md`. It matches `targetId`, the unique per-run `observerId`, and
the timestamp in Target Service's existing `observation stored` log message.
This is evidence that the actual service accepted and stored the observation in
memory, rather than merely evidence that the publisher wrote to a TCP socket.

Broker has no subscription acknowledgement. Commands on different connections
can race, so the runner repeats the same synthetic publication until matching
receipt or the deadline. This does not test exactly-once delivery, deduplication,
target aggregation, probability decay, or database persistence.

### Map and Window Manager

The actual Map executable loads `app/map/assets/test-map.json` and sends drawing
commands to the actual WM server and headless renderer. The runner checks the
Map's `Map shown.` message, three visible targets, one outside target, and
probabilities of 90%, 55%, and 20%. `Map shown.` is printed only after successful
responses to the frame commands and `SHOW`.

After `SIGINT`, Map must exit successfully. A subsequent `SHOW` request must fail
with the WM's no-window response, and a new `CREATE`/`CLOSE` sequence must succeed.
This checks window release and confirms that Map did not stop WM. Finally, `QUIT`
stops WM and its socket paths must disappear.

Map currently displays hardcoded demonstration targets. It does not subscribe to
Broker or display the observation published by this runner. This smoke-test does
not claim a complete Analyzer-to-map application flow, native GUI verification,
or pixel-level rendering verification. The existing Map/WM E2E covers detailed
rendering separately; see `docs/testing/map-wm.md`.

### Unavailable Broker

The runner reserves a local TCP port without listening on it and starts a real
Target Service against that address. The service must exit with a positive
failure code and a `connect to Broker` diagnostic. This expected component failure
counts as a successful negative scenario.

## Isolation and cleanup

Each invocation creates a private, short `/tmp/tac-smoke-*` directory for its
binaries and two Unix sockets. Broker receives an ephemeral port selected for
that run. Its CLI rejects port `0`, so there is a brief release-to-bind window;
the runner retries up to five confirmed address-in-use failures. Repeated and
concurrent invocations do not share fixed port or socket names.

The stock WM CLI only exposes its command socket path. The small launcher in
`cmd/wm-headless` instead passes both private paths to the existing
`server.Server` with `headless.New()`. It changes startup configuration only and
does not replace the server, protocol, or renderer.

The runner handles `SIGINT` and `SIGTERM`, and cleanup runs after success, failure,
or cancellation. Services are stopped in reverse startup order, so Map can
release its window while WM is still running and Target Service stops before
Broker. Child services run in separate process groups and are waited for. Cleanup
sends `SIGTERM`, waits up to three seconds, then uses `SIGKILL` if necessary and
waits up to three more seconds. Requiring forced termination is reported as a
failure. Cancelled builds kill their process group and bound output-pipe waiting.

WM socket removal is checked before the runner removes its temporary directory.
The directory is removed on errors as well. Standard Go build caches are outside
that directory and are retained. `SIGKILL` of the runner itself, a machine crash,
or power loss cannot execute cleanup handlers.

The default overall execution deadline is three minutes, including up to two
minutes for component builds. The positive scenario has a 15-second deadline;
the unavailable-Broker scenario has five seconds. Cleanup has separate bounded
waits and can extend beyond the overall deadline. To increase the overall limit:

```sh
(cd tests/system-smoke && GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go run -mod=readonly . -timeout 5m)
```

On failure, scenario diagnostics include each started service's recent combined
stdout/stderr, bounded to 64 KiB per service. The runner prints the temporary
directory path before starting work so it can be identified during a run.

## Verification commands

From `tests/system-smoke`, check formatting, static analysis, and the test suite:

```sh
gofmt -l .
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go vet -mod=readonly ./...
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -mod=readonly -count=1 -timeout 5m ./...
```

The suite includes simultaneous scenarios, a repeated run, cancellation after
real services start, cleanup with Map running, the unavailable-Broker case,
temporary-directory removal after a cancelled build, and real CLI interruption
with both `SIGINT` and `SIGTERM`. To enable Go's race detector where a C compiler
is available:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -mod=readonly -race -count=1 -timeout 5m ./...
```

To test offline dependency resolution without relying on an existing module
cache, use a disposable cache directory:

```sh
smoke_cache=$(mktemp -d)
GOMODCACHE="$smoke_cache" GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go run -mod=readonly .
rmdir "$smoke_cache"
```

For direct signal testing, build the runner and signal its PID rather than the
`go run` wrapper:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go build -mod=readonly -o /tmp/tac-system-smoke .
/tmp/tac-system-smoke
```

In another terminal, send `kill -INT <runner-pid>` or `kill -TERM <runner-pid>`
while services are running. Check that the printed resource directory and its
sockets disappear and that the child processes terminate. The binary must be
started from inside the checkout so it can locate the component sources.
