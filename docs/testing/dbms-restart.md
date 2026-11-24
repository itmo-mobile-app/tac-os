# DBMS restart and storage verification (#147)

## Scope and command

Run on Linux with Go 1.22+ and a C compiler for the race detector:

```sh
cd tests/system-smoke
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -mod=readonly \
  -tags server_e2e -race -count=1 -timeout 5m -v -run '^TestDBMSRestartE2E$' .
```

The test reuses #140's process lifecycle and public-protocol helpers. It builds
real Broker, DBMS and Target Service executables from the current checkout,
without downloading dependencies. No camera, GUI, external network, or fabricated
Target publication is required. The tagged Server acceptance E2E workflow runs
this test separately from #140 on PRs and main.

## Assertions

- Start an isolated DBMS with `-data`, Broker and Target Service with `-dbms`.
  Publish a six-field Observation and require a real `targets.updated` event.
- Stop Target Service to freeze periodic writes. The test reads, but never inserts
  or updates, the domain `targets` and `observations` tables. Exactly one row per
  table must exist, with SQL type descriptors, matching coordinates and exact
  int64 timestamp. The stored Target must equal the published Target.
- Use a separate `bounds` probe table to check CREATE, INSERT, UPDATE, SELECT,
  both signed int64 extremes and fractional REAL values. These arbitrary
  boundary integers are not legal observation timestamps and are never
  presented as domain data.
- Invalid SQL, a type error after a valid SET assignment, unsupported DELETE and
  a multi-statement request must return SQL_ERROR without changing snapshot bytes.
- Stop DBMS with SIGTERM or SIGKILL and launch it again with the same snapshot.
  Before any new Observation or Target Service startup, compare complete rows
  and metadata for both domain tables and the independent numeric probe.
- Orderly stop must remove the Unix socket. Abrupt stop must allow a restart
  through stale-socket handling. Empty, truncated and invalid snapshot copies
  must fail startup explicitly, preserve their bytes and create no socket.
  A final launch from the original valid snapshot verifies it remains readable.
- Cleanup waits for child processes and removes isolated sockets/directories,
  on success and failure, using the existing fixture's diagnostics.

## Ambiguous writes

From the repository root, build the actual DBMS binary, then test its client:

```sh
(cd server/dbms && go build -buildvcs=false -o /tmp/tacdb-147 ./cmd/tacdb-server)
(cd target-service && TACOS_DBMS_SERVER=/tmp/tacdb-147 go test -race -count=1 -v \
  -run '^TestRealDBMS(Integration|LostReply)$' ./dbclient)
(cd target-service && go test -race -count=1 -v -run '^TestAmbiguousWriteNoReplay$' .)
```

`TestRealDBMSLostReply` forwards one INSERT to a real persistent DBMS and drops
its reply only after the real server acknowledges success. The existing client
must report an unknown outcome without reconnecting/replaying the write. A direct
read must find exactly one row with its unchanged int64 value. The tagged test is
not a replacement for this client-side check; the Go CI Target Service job runs
both real-client cases explicitly. Without `TACOS_DBMS_SERVER`, the real-client
tests skip, which is not evidence of a successful integration run.

`TestAmbiguousWriteNoReplay` additionally checks the Target Service boundary for
failed Observation INSERT, Target INSERT and Target UPDATE. Each write is attempted
once and no Target is published after the failure. No retry policy is changed.

## Boundaries and verification status

This task does not restart or reconstruct Target Service aggregation state (#148).
The current service unconditionally creates its schema on startup, so relaunching
it against populated tables is a known separate limitation. DBMS restart tests
stop the service before restart and read saved rows without silently resetting
that schema. No production SQL, storage format, wire contract or aggregation
formula is changed by #147.

Verification on Ubuntu under WSL passed: TestDBMSRestartE2E completed both
SIGTERM and SIGKILL scenarios; all Target Service and DBMS tests passed with
the race detector, including TestRealDBMSIntegration, TestRealDBMSLostReply
and TestAmbiguousWriteNoReplay. All 17 DBMS CTest checks and the existing
TestServerE2E regression suite passed.
