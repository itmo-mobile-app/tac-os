# In-memory SQL DBMS

This component implements issues #31 and #67 in Go, matching the course technology stack.
It uses only the standard library and its own SQL lexer, parser, and executor.
It supports `CREATE TABLE`, `INSERT INTO ... VALUES`,
`SELECT ... FROM ... [WHERE column = value]`, and
`UPDATE ... SET column = value, ... WHERE column = value` from `spec/sql.md`.

Tables exist only in memory. Closing `tacdb` discards them. The CLI is a development
entry point, **not** the Target Service wire protocol. The planned socket client/server
contract is fixed in `spec/dbms-protocol.md`: a Unix-domain stream socket with
four-byte little-endian length-prefixed UTF-8 SQL requests and JSON responses.
The socket server is implemented separately as `tacdb-server` (#96). Persistence
is not implemented.
The local Go API is not a deployment decision.

## Build and test only this component

From the repository root on Linux, with Go 1.22+:

```bash
cd server/dbms
go test -count=1 ./...
go vet ./...
go build -buildvcs=false -o /tmp/tacdb ./cmd/tacdb
/tmp/tacdb
```

There are no third-party dependencies to download. Unit tests cover all commands,
literal boundaries, errors, independent databases, result copies, and failed writes.
CLI tests cover statement framing, error recovery, multiline and long TEXT, and I/O errors.
The fixtures in `tests/dbms/` also check the actual executable through CTest.

Run the race detector separately with `go test -race -count=1 ./...`.
It requires a C compiler on supported platforms. This does not make Database safe
for concurrent use; the current CLI executes statements sequentially.

## CMake integration

From the repository root, with CMake 3.16+ and Go 1.22+:

```bash
cmake -S server/dbms -B "$HOME/tac-os-build/dbms-go"
cmake --build "$HOME/tac-os-build/dbms-go" -j2
ctest --test-dir "$HOME/tac-os-build/dbms-go" --output-on-failure
"$HOME/tac-os-build/dbms-go/tacdb"
```

The isolated build registers fourteen DBMS checks. It does not require Kotlin or the VM.
For the complete monorepo build, follow the root README. The CLI then lives at
`build/server/dbms/tacdb`. DBMS checks carry the `dbms` label:

```bash
ctest --test-dir build -L dbms --output-on-failure
```

CMake stores the Go build cache in its binary directory. For a nonstandard Go
installation, configure with `-DTACOS_GO_EXECUTABLE=/absolute/path/to/go`.
ASan/UBSan apply to the C++ VM and Runtime, not to Go. CI checks Go formatting,
runs `go vet` and Go tests with the race detector, then builds and tests the monorepo.

## Try a session

Enter the following statements in the same running process:

```sql
CREATE TABLE targets (target_id TEXT, latitude REAL, last_seen_at INTEGER);
INSERT INTO targets (target_id, latitude, last_seen_at) VALUES ('t1', 59.95, 1732000000000);
UPDATE targets SET latitude = 60.0, last_seen_at = 1732000000001 WHERE target_id = 't1';
SELECT target_id, latitude, last_seen_at FROM targets WHERE target_id = 't1';
```

CREATE, INSERT and UPDATE print `OK` (no affected-row count). SELECT prints a tab-separated header, rows, and a
row count. TEXT uses single quotes and doubled quotes; control characters and
backslashes are escaped for display. This display format is not an SQL import or
client protocol. REAL uses the shortest decimal form that round-trips to its
float64 value: `30.0` can display as `30`, while its stored type remains REAL.

Statements can span lines or share a line. Only a semicolon outside TEXT ends a
statement. End input with Ctrl+D on Linux, or redirect an SQL file to stdin.
An incomplete final statement is an error. An unterminated TEXT consumes the
remaining input and is reported at EOF.

Errors go to stderr with a one-based byte position relative to the current statement,
including leading whitespace. After a complete failed statement, the CLI continues
with subsequent statements. Its exit code is 1 if any statement or I/O failed, else 0.

## Execution rules

Execution rules are documented in `spec/sql.md`:

- Unquoted identifiers use `[A-Za-z_][A-Za-z0-9_]*` and are case-sensitive.
- Table names and column names within one table must be unique.
- INSERT supplies every declared column exactly once, in any order; no NULL/defaults.
- INSERT, UPDATE assignments and WHERE require the exact declared type. Use `30.0` for REAL, not `30`.
- INTEGER uses signed int64; REAL uses finite IEEE-754 float64, with overflow rejected.
- Failed parsing or validation does not partially create, insert, or update data.
- UPDATE requires WHERE, updates all matching rows, and succeeds on zero matches.
- Duplicate SET columns are rejected. Every assignment and the predicate are
  validated before writing, even on empty tables or when nothing matches.
- A WHERE column can itself be updated; matching uses its previous value.
- Repeated SELECT columns are allowed. Row order is unspecified by the contract.

The table representation is an internal detail, not a persistent storage format.

## Code map

- `value.go`: typed values, copied results, query errors.
- `lexer.go`: lexical rules and TEXT decoding.
- `parser.go`: command grammar and numeric checks.
- `database.go`: schema validation, INSERT, atomic UPDATE, projection, equality filtering.
- `cmd/tacdb/main.go`: process entry point.
- `internal/cli/`: stdin framing, display, error reporting, CLI unit tests.
- `database_test.go`: executor behavior tests using Go's testing package.
- `update_test.go`: UPDATE behavior, error positions and unchanged data after errors.
- `tests/dbms/`: CTest registration and executable-level SQL fixtures.

The zero value of `Database` is ready to use. `Execute` accepts one semicolon-terminated
statement. `QueryError.Position` is a zero-based byte offset. Results own copies of rows.
A used Database must not be copied or used concurrently.

## Unix-socket server (Linux, issue #96)

```bash
cd server/dbms
go build -buildvcs=false -o /tmp/tacdb-server ./cmd/tacdb-server
/tmp/tacdb-server -socket /tmp/tac-os-dbms.sock
```

CMake builds `tacdb-server` next to `tacdb`. The existing CLI is unchanged.
The server implements `spec/dbms-protocol.md` without extending SQL or changing
its wire format: one raw UTF-8 SQL statement per four-byte uint32 LE frame,
one framed JSON response, and multiple exchanges on the same connection.
Requests/responses are limited to 1 MiB. SQL errors include a zero-based byte
position; malformed/truncated/oversized frames close only their connection.
Empty/invalid UTF-8 payloads in valid-size frames return BAD_REQUEST.
SELECT columns include types even for zero rows. INTEGER values remain int64.
Oversized SELECT responses return RESULT_TOO_LARGE.

Connections share one database; a mutex serializes executor access. Socket I/O
is outside that mutex. `-io-timeout 5s` controls the deadline for each frame read
or write; it is not a database execution timeout. The client should use its
own timeout and must not blindly retry writes after an ambiguous disconnect.
No persistence, authentication protocol, TCP endpoint or transactions are added.
Database contents are discarded when this process exits.

SIGINT (Ctrl+C) and SIGTERM stop accepting clients, interrupt blocked socket I/O,
wait for handlers and remove only the socket created by that server. Socket
permissions are 0600. Startup refuses regular files, symlinks, foreign-owned or
active sockets, and can remove an owned stale socket after ECONNREFUSED.
A same-owner 0600 `<socket>.lock` file uses flock to prevent simultaneous server
starts. It remains after exit intentionally; it contains no SQL data. Do not
remove the lock file while a server might be running or starting. Use a private
service-owned directory for deployment. The code uses Linux system APIs.

`go test -race -count=1 ./...` tests actual Unix sockets, partial/coalesced frames,
frame boundaries, UTF-8, malformed requests, int64 extremes, errors, empty
SELECT metadata, response limits, parallel clients, timeouts and cleanup.
It also builds and starts a separate server process and checks all four SQL
commands and SIGTERM cleanup. CTest `dbms.socket` runs the same socket tests
against its built server using `TACOS_DBMS_SERVER`; the general Go CI matrix
already includes this module. No camera or external network service is needed.

The local `Result.Types` slice follows `Result.Columns` in projection order;
like rows and names it is copied, including for an empty SELECT. This metadata
addition lets the wire adapter report exact column types without inferring them
from data. `Database` itself remains unsafe for concurrent direct callers.
